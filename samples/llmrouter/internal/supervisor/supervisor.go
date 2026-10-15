// Package supervisor makes sure one backend server is answering, starting
// its command when it is not, and stops the process it started.
package supervisor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"llmrouter/internal/llm"
)

// Options configure a Supervisor.
type Options struct {
	// Name names the backend in log lines.
	Name string
	// Health returns nil when the backend answers.
	Health func(ctx context.Context) error
	// Command is the process to start when the backend does not answer,
	// such as []string{"ollama", "serve"}. Empty means never start one.
	Command []string
	// Dir is the process's working directory; "" means the current one.
	Dir string
	// Env is added to the process's environment, as "KEY=value".
	Env []string
	// StopCommand, if set, runs after the process is stopped. It is for
	// servers that outlive the process that started them, such as a
	// Windows server started through powershell.exe from WSL.
	StopCommand []string
	// StartTimeout bounds the wait for a started backend to answer.
	// Zero means 120 seconds.
	StartTimeout time.Duration
	// PollInterval is how often Health is called while waiting. Zero
	// means 500 milliseconds.
	PollInterval time.Duration
	// Log receives progress lines. It must not be nil.
	Log *log.Logger
}

// Supervisor keeps one backend answering and stops the process it started.
type Supervisor struct {
	opts Options
	log  *log.Logger

	mu      sync.Mutex
	started bool          // Start started a process that Stop has not yet stopped
	cmd     *exec.Cmd     // the process Start started
	done    chan struct{} // closed when cmd has exited
	exitErr error         // cmd.Wait's result; written before done is closed
}

// New returns a Supervisor for opts.
func New(opts Options) *Supervisor {
	return &Supervisor{opts: opts, log: opts.Log}
}

// Start makes sure the backend answers. If Health succeeds it returns
// nil and starts nothing. If it fails and Command is empty, it returns
// an error wrapping llm.ErrUnavailable. Otherwise it starts Command and
// calls Health every PollInterval until it succeeds (return nil), the
// process exits (return an error wrapping llm.ErrUnavailable), or
// StartTimeout passes or ctx ends (stop the process, then return an
// error wrapping llm.ErrUnavailable).
func (s *Supervisor) Start(ctx context.Context) error {
	healthErr := s.opts.Health(ctx)
	if healthErr == nil {
		return nil
	}
	if len(s.opts.Command) == 0 {
		return fmt.Errorf("%s: %w: %v", s.opts.Name, llm.ErrUnavailable, healthErr)
	}

	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return fmt.Errorf("%s: %w: process already started", s.opts.Name, llm.ErrUnavailable)
	}
	cmd := exec.Command(s.opts.Command[0], s.opts.Command[1:]...)
	cmd.Dir = s.opts.Dir
	cmd.Env = append(os.Environ(), s.opts.Env...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("%s: %w: start %s: %v", s.opts.Name, llm.ErrUnavailable, strings.Join(s.opts.Command, " "), err)
	}
	done := make(chan struct{})
	s.cmd, s.done, s.started = cmd, done, true
	s.mu.Unlock()

	go func() {
		err := cmd.Wait()
		s.exitErr = err
		close(done)
	}()

	s.log.Printf("%s: starting %s", s.opts.Name, strings.Join(s.opts.Command, " "))

	timeout := s.opts.StartTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	poll := s.opts.PollInterval
	if poll <= 0 {
		poll = 500 * time.Millisecond
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		if err := s.opts.Health(waitCtx); err == nil {
			s.log.Printf("%s: healthy", s.opts.Name)
			return nil
		}
		select {
		case <-done:
			s.stopAfterFailure()
			return fmt.Errorf("%s: %w: process exited before answering: %v", s.opts.Name, llm.ErrUnavailable, s.exitErr)
		case <-waitCtx.Done():
			s.stopAfterFailure()
			return fmt.Errorf("%s: %w: not answering after %s: %v", s.opts.Name, llm.ErrUnavailable, timeout, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

// stopAfterFailure stops the process after Start has given up on it.
// Its error is logged, because Start is already returning another one.
func (s *Supervisor) stopAfterFailure() {
	if err := s.Stop(); err != nil {
		s.log.Printf("%s: stop after failed start: %v", s.opts.Name, err)
	}
}

// Managed reports whether Start started the process and it is still
// running.
func (s *Supervisor) Managed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.started {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

// Stop stops the process Start started: it sends os.Interrupt, waits up
// to 5 seconds for it to exit, then kills it. It then runs StopCommand
// if set, waiting up to 30 seconds. Stop does nothing if Start started
// nothing, and is safe to call more than once.
func (s *Supervisor) Stop() error {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return nil
	}
	s.started = false
	cmd, done := s.cmd, s.done
	s.cmd, s.done = nil, nil
	s.mu.Unlock()

	s.log.Printf("%s: stopping", s.opts.Name)

	select {
	case <-done:
		// The process already exited on its own.
	default:
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}

	if len(s.opts.StopCommand) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stop := exec.CommandContext(ctx, s.opts.StopCommand[0], s.opts.StopCommand[1:]...)
	stop.Dir = s.opts.Dir
	stop.Env = append(os.Environ(), s.opts.Env...)
	stop.Stdout = os.Stderr
	stop.Stderr = os.Stderr
	if err := stop.Run(); err != nil {
		return fmt.Errorf("%s: stop command %s: %w", s.opts.Name, strings.Join(s.opts.StopCommand, " "), err)
	}
	return nil
}

// Puller is what EnsureModels needs from a backend.
type Puller interface {
	ListModels(ctx context.Context) ([]llm.Model, error)
	PullModel(ctx context.Context, name string) error
}

// EnsureModels pulls each of names that m does not list, in order. A
// name with no ":" also matches an installed name+":latest". It stops
// at the first error and returns it wrapped as "pull NAME: ...".
func EnsureModels(ctx context.Context, m Puller, names []string) error {
	models, err := m.ListModels(ctx)
	if err != nil {
		return fmt.Errorf("list models: %w", err)
	}
	installed := make(map[string]bool, len(models))
	for _, mo := range models {
		installed[mo.Name] = true
	}
	for _, name := range names {
		if installed[name] || (!strings.Contains(name, ":") && installed[name+":latest"]) {
			continue
		}
		if err := m.PullModel(ctx, name); err != nil {
			return fmt.Errorf("pull %s: %w", name, err)
		}
		installed[name] = true
	}
	return nil
}
