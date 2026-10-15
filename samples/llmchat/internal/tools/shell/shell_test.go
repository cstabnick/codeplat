package shell

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llmchat/internal/chat"
)

func newWorkspace(t *testing.T) string {
	t.Helper()
	ws, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func args(t *testing.T, command string, timeout int) json.RawMessage {
	t.Helper()
	m := map[string]any{"command": command}
	if timeout != 0 {
		m["timeout_seconds"] = timeout
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func run(t *testing.T, ws, command string, timeout int) (string, error) {
	t.Helper()
	return Bash().Run(context.Background(), ws, args(t, command, timeout))
}

func TestSpec(t *testing.T) {
	spec := Bash().Spec()
	if spec.Name != "bash" {
		t.Fatalf("name = %q, want bash", spec.Name)
	}
	if spec.Description == "" {
		t.Error("description is empty")
	}
	var schema struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(spec.Parameters, &schema); err != nil {
		t.Fatalf("parameters are not JSON: %v", err)
	}
	if schema.Type != "object" {
		t.Errorf("parameters type = %q, want object", schema.Type)
	}
	for _, p := range []string{"command", "timeout_seconds"} {
		if _, ok := schema.Properties[p]; !ok {
			t.Errorf("parameters lack property %q", p)
		}
	}
	if len(schema.Required) != 1 || schema.Required[0] != "command" {
		t.Errorf("required = %v, want [command]", schema.Required)
	}
}

func TestNeedsApprovalAlways(t *testing.T) {
	tool := Bash()
	for _, a := range []json.RawMessage{
		args(t, "echo hi", 0),
		args(t, "ls", 5),
		json.RawMessage(`{}`),
		nil,
	} {
		if !tool.NeedsApproval(a) {
			t.Errorf("NeedsApproval(%s) = false, want true", a)
		}
	}
}

func TestEchoOutput(t *testing.T) {
	got, err := run(t, newWorkspace(t), "echo hi", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "hi\nexit status 0" {
		t.Errorf("got %q, want %q", got, "hi\nexit status 0")
	}
}

func TestWorkingDirectoryIsWorkspace(t *testing.T) {
	ws := newWorkspace(t)
	got, err := run(t, ws, "pwd", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != ws+"\nexit status 0" {
		t.Errorf("got %q, want workspace %q", got, ws)
	}
}

func TestStderrIncluded(t *testing.T) {
	got, err := run(t, newWorkspace(t), "echo err 1>&2", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "err\nexit status 0" {
		t.Errorf("got %q, want %q", got, "err\nexit status 0")
	}
}

func TestNonZeroExit(t *testing.T) {
	got, err := run(t, newWorkspace(t), "exit 3", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "exit status 3" {
		t.Errorf("got %q, want %q", got, "exit status 3")
	}
}

func TestTimeout(t *testing.T) {
	start := time.Now()
	got, err := run(t, newWorkspace(t), "sleep 5", 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("timeout returned error %v, want a result", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("call took %v, want under 3s", elapsed)
	}
	if !strings.HasSuffix(got, "timed out after 1s") {
		t.Errorf("got %q, want it to end with %q", got, "timed out after 1s")
	}
}

func TestTimeoutKillsBackgroundChildren(t *testing.T) {
	start := time.Now()
	got, err := run(t, newWorkspace(t), "sleep 30 & wait", 1)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("timeout returned error %v, want a result", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("call took %v with a background child, want under 3s", elapsed)
	}
	if !strings.HasSuffix(got, "timed out after 1s") {
		t.Errorf("got %q, want it to end with %q", got, "timed out after 1s")
	}
}

func TestOutputTruncated(t *testing.T) {
	got, err := run(t, newWorkspace(t), "head -c 100000 /dev/zero | tr '\\0' x", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) >= 31000 {
		t.Errorf("output is %d bytes, want under 31000", len(got))
	}
	if !strings.Contains(got, "[... output truncated ...]") {
		t.Errorf("output lacks the truncation marker")
	}
	if !strings.HasPrefix(got, "xxxx") {
		t.Errorf("output does not start with the head of the command output")
	}
	if !strings.HasSuffix(got, "exit status 0") {
		t.Errorf("output does not end with the exit status")
	}
}

func TestOutputNotTruncatedWhenSmall(t *testing.T) {
	got, err := run(t, newWorkspace(t), "head -c 1000 /dev/zero | tr '\\0' y", 0)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "truncated") {
		t.Errorf("small output was truncated")
	}
	if strings.Count(got, "y") != 1000 {
		t.Errorf("got %d y bytes, want 1000", strings.Count(got, "y"))
	}
}

func TestEmptyCommandIsInvalid(t *testing.T) {
	for _, cmd := range []string{"", "   "} {
		_, err := run(t, newWorkspace(t), cmd, 0)
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("command %q: err = %v, want chat.ErrInvalid", cmd, err)
		}
	}
}

func TestBadJSONIsInvalid(t *testing.T) {
	_, err := Bash().Run(context.Background(), newWorkspace(t), json.RawMessage(`{"command":`))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("err = %v, want chat.ErrInvalid", err)
	}
}

func TestMissingWorkspaceIsError(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "missing")
	_, err := run(t, ws, "echo hi", 0)
	if err == nil {
		t.Fatal("missing workspace gave no error")
	}
	if !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("err = %v, want chat.ErrInvalid", err)
	}
}

func TestRelativeWorkspaceIsError(t *testing.T) {
	_, err := run(t, "relative/dir", "echo hi", 0)
	if err == nil {
		t.Fatal("relative workspace gave no error")
	}
}

func TestCancelledContextEndsCommand(t *testing.T) {
	ws := newWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)

	start := time.Now()
	got, err := Bash().Run(ctx, ws, args(t, "sleep 5", 0))
	elapsed := time.Since(start)
	if elapsed > 3*time.Second {
		t.Errorf("call took %v after cancel, want under 3s", elapsed)
	}
	if err == nil {
		t.Fatalf("cancelled call returned result %q, want an error", got)
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestWorkspaceFileVisible(t *testing.T) {
	ws := newWorkspace(t)
	if err := os.WriteFile(filepath.Join(ws, "f.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := run(t, ws, "cat f.txt", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "data\nexit status 0" {
		t.Errorf("got %q, want %q", got, "data\nexit status 0")
	}
}

// fakeRunner records what the bash tool passes to it and returns canned results.
type fakeRunner struct {
	gotWorkspace string
	gotCommand   string
	gotTimeout   time.Duration

	output   string
	exitCode int
	timedOut bool
	err      error
}

func (f *fakeRunner) Run(ctx context.Context, workspace, command string, timeout time.Duration) (string, int, bool, error) {
	f.gotWorkspace, f.gotCommand, f.gotTimeout = workspace, command, timeout
	return f.output, f.exitCode, f.timedOut, f.err
}

func TestBashWithPassesArgsToRunner(t *testing.T) {
	cases := []struct {
		name        string
		timeout     int
		wantTimeout time.Duration
	}{
		{"default timeout", 0, 60 * time.Second},
		{"given timeout", 7, 7 * time.Second},
		{"timeout capped", 999, 300 * time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeRunner{}
			if _, err := BashWith(r).Run(context.Background(), "/work", args(t, "make it", c.timeout)); err != nil {
				t.Fatal(err)
			}
			if r.gotWorkspace != "/work" {
				t.Errorf("workspace = %q, want /work", r.gotWorkspace)
			}
			if r.gotCommand != "make it" {
				t.Errorf("command = %q, want %q", r.gotCommand, "make it")
			}
			if r.gotTimeout != c.wantTimeout {
				t.Errorf("timeout = %v, want %v", r.gotTimeout, c.wantTimeout)
			}
		})
	}
}

func TestBashWithFormatsRunnerResult(t *testing.T) {
	cases := []struct {
		name     string
		output   string
		exitCode int
		timedOut bool
		want     string
	}{
		{"exit 0", "hi\n", 0, false, "hi\nexit status 0"},
		{"exit 2 without trailing newline", "bad", 2, false, "bad\nexit status 2"},
		{"terminated by signal", "", -1, false, "terminated by signal"},
		{"timed out", "partial\n", -1, true, "partial\ntimed out after 7s"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &fakeRunner{output: c.output, exitCode: c.exitCode, timedOut: c.timedOut}
			got, err := BashWith(r).Run(context.Background(), "/work", args(t, "make it", 7))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestBashWithReturnsRunnerError(t *testing.T) {
	sentinel := errors.New("boom")
	r := &fakeRunner{err: sentinel}
	got, err := BashWith(r).Run(context.Background(), "/work", args(t, "echo hi", 0))
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want the runner's error", err)
	}
	if got != "" {
		t.Errorf("got %q with a runner error, want empty", got)
	}
}
