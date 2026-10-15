// Package shell provides the bash tool, which runs a shell command in the
// conversation's workspace. Every call needs the person's approval.
package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"llmchat/internal/chat"
	"llmchat/internal/tools/sandbox"
)

const (
	defaultTimeout = 60 * time.Second
	maxTimeout     = 300 * time.Second
	// keepEach is how many bytes of output are kept from the start and
	// from the end of a command's output.
	keepEach = 15000
	marker   = "\n[... output truncated ...]\n"
)

// Runner runs one command for the bash tool. exitCode is -1 when the process was killed by a signal.
type Runner interface {
	Run(ctx context.Context, workspace, command string, timeout time.Duration) (output string, exitCode int, timedOut bool, err error)
}

// Local runs commands with bash on this machine, in the workspace (today's behaviour:
// sandbox.Resolve, process group, kill on timeout, head/tail capture).
type Local struct{}

var _ Runner = Local{}

// Run implements Runner for the local machine.
func (Local) Run(ctx context.Context, workspace, command string, timeout time.Duration) (string, int, bool, error) {
	dir, err := sandbox.Resolve(workspace, "")
	if err != nil {
		return "", 0, false, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out := &capture{}
	cmd := exec.Command("bash", "-c", command)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.Env = os.Environ()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return "", 0, false, fmt.Errorf("start bash: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var waitErr error
	timedOut := false
	select {
	case waitErr = <-done:
	case <-runCtx.Done():
		// Kill the whole process group so that background children
		// do not outlive the command or keep its output open.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		waitErr = <-done
		timedOut = true
	}

	if timedOut && ctx.Err() != nil {
		return "", 0, false, fmt.Errorf("command cancelled: %w", ctx.Err())
	}
	if timedOut {
		return out.String(), -1, true, nil
	}
	if waitErr == nil {
		return out.String(), 0, false, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(waitErr, &exitErr) {
		return "", 0, false, fmt.Errorf("run bash: %w", waitErr)
	}
	return out.String(), exitErr.ExitCode(), false, nil
}

// Bash returns the bash tool, running commands on this machine.
func Bash() chat.Tool {
	return BashWith(Local{})
}

// BashWith returns the bash tool, running each command through r.
func BashWith(r Runner) chat.Tool {
	return bashTool{runner: r}
}

type bashTool struct {
	runner Runner
}

type bashArgs struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

var parameters = json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer","description":"Time limit in seconds (default 60, at most 300)"}},"required":["command"]}`)

func (bashTool) Spec() chat.ToolSpec {
	return chat.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command with bash in the workspace directory. Returns the output and the exit status.",
		Parameters:  parameters,
	}
}

// NeedsApproval is always true: a shell command can do anything the
// person's account can do.
func (bashTool) NeedsApproval(json.RawMessage) bool {
	return true
}

func (t bashTool) Run(ctx context.Context, workspace string, args json.RawMessage) (string, error) {
	var a bashArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return "", fmt.Errorf("%w: arguments: %v", chat.ErrInvalid, err)
	}
	if strings.TrimSpace(a.Command) == "" {
		return "", fmt.Errorf("%w: command is empty", chat.ErrInvalid)
	}
	timeout := defaultTimeout
	if a.TimeoutSeconds > 0 {
		timeout = time.Duration(a.TimeoutSeconds) * time.Second
		if timeout > maxTimeout {
			timeout = maxTimeout
		}
	}

	output, exitCode, timedOut, err := t.runner.Run(ctx, workspace, a.Command, timeout)
	if err != nil {
		return "", err
	}

	text := output
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	switch {
	case timedOut:
		text += fmt.Sprintf("timed out after %ds", int(timeout/time.Second))
	case exitCode == -1:
		text += "terminated by signal"
	default:
		text += fmt.Sprintf("exit status %d", exitCode)
	}
	return text, nil
}

// capture keeps the first and the last keepEach bytes written to it and
// counts nothing else, so a command's output uses bounded memory.
type capture struct {
	mu    sync.Mutex
	total int
	head  []byte
	tail  []byte
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := len(p)
	c.total += written
	if room := keepEach - len(c.head); room > 0 {
		n := min(room, len(p))
		c.head = append(c.head, p[:n]...)
		p = p[n:]
	}
	if len(p) > 0 {
		c.tail = append(c.tail, p...)
		if len(c.tail) > 2*keepEach {
			c.tail = append([]byte(nil), c.tail[len(c.tail)-keepEach:]...)
		}
	}
	return written, nil
}

// String returns the captured output, with the middle removed when the
// output was longer than the two kept parts.
func (c *capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total <= 2*keepEach {
		return string(c.head) + string(c.tail)
	}
	tail := c.tail
	if len(tail) > keepEach {
		tail = tail[len(tail)-keepEach:]
	}
	return string(c.head) + marker + string(tail)
}
