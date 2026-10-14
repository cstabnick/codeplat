package supervisor

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llmrouter/internal/llm"
)

// TestHelperProcess is not a real test. The supervisor starts the test
// binary with this test selected, and the mode is the last argument.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "serve":
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		<-ch
		os.Exit(0)
	case "exit":
		os.Exit(3)
	}
	os.Exit(2)
}

func helperCommand(mode string) []string {
	return []string{os.Args[0], "-test.run=TestHelperProcess", "--", mode}
}

var helperEnv = []string{"GO_WANT_HELPER_PROCESS=1"}

var errDown = errors.New("down")

// failFirst returns a Health func that fails its first n calls and then
// succeeds. Its calls counter is shared so tests can inspect it.
func failFirst(n int32, calls *atomic.Int32) func(context.Context) error {
	return func(context.Context) error {
		if calls.Add(1) <= n {
			return errDown
		}
		return nil
	}
}

func alwaysFail(context.Context) error { return errDown }

func quietLogger() *log.Logger { return log.New(io.Discard, "", 0) }

func TestStartWhenHealthy(t *testing.T) {
	s := New(Options{
		Name:         "test",
		Health:       func(context.Context) error { return nil },
		Command:      helperCommand("serve"),
		Env:          helperEnv,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.Managed() {
		t.Fatal("Managed = true, want false when Health already succeeded")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestStartFailsWithoutCommand(t *testing.T) {
	s := New(Options{
		Name:         "test",
		Health:       alwaysFail,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	err := s.Start(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Start error = %v, want one wrapping llm.ErrUnavailable", err)
	}
	if s.Managed() {
		t.Fatal("Managed = true, want false")
	}
}

func TestStartServesAfterRetries(t *testing.T) {
	var calls atomic.Int32
	s := New(Options{
		Name:         "test",
		Health:       failFirst(3, &calls),
		Command:      helperCommand("serve"),
		Env:          helperEnv,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Managed() {
		t.Fatal("Managed = false, want true while the started process runs")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if s.Managed() {
		t.Fatal("Managed = true after Stop, want false")
	}
	if err := s.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestStartFailsWhenProcessExits(t *testing.T) {
	s := New(Options{
		Name:         "test",
		Health:       alwaysFail,
		Command:      helperCommand("exit"),
		Env:          helperEnv,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	err := s.Start(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Start error = %v, want one wrapping llm.ErrUnavailable", err)
	}
	if s.Managed() {
		t.Fatal("Managed = true after the process exited, want false")
	}
}

func TestStartTimesOut(t *testing.T) {
	s := New(Options{
		Name:         "test",
		Health:       alwaysFail,
		Command:      helperCommand("serve"),
		Env:          helperEnv,
		StartTimeout: 200 * time.Millisecond,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	err := s.Start(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Start error = %v, want one wrapping llm.ErrUnavailable", err)
	}
	if s.Managed() {
		t.Fatal("Managed = true after timeout, want false")
	}
}

func TestStopCommandRuns(t *testing.T) {
	var calls atomic.Int32
	s := New(Options{
		Name:         "test",
		Health:       failFirst(1, &calls),
		Command:      helperCommand("serve"),
		StopCommand:  helperCommand("exit"),
		Env:          helperEnv,
		PollInterval: 10 * time.Millisecond,
		Log:          quietLogger(),
	})
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The StopCommand exits with status 3. Its error may be returned or
	// logged, so only require that Stop returns.
	done := make(chan struct{})
	go func() {
		_ = s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(45 * time.Second):
		t.Fatal("Stop did not return")
	}
}

func TestEnsureModels(t *testing.T) {
	m := &fakePuller{models: []llm.Model{{Name: "llama3:8b"}, {Name: "qwen:latest"}}}
	err := EnsureModels(context.Background(), m, []string{"llama3:8b", "qwen", "llama3.2:1b"})
	if err != nil {
		t.Fatalf("EnsureModels: %v", err)
	}
	if len(m.pulled) != 1 || m.pulled[0] != "llama3.2:1b" {
		t.Fatalf("pulled = %v, want [llama3.2:1b]", m.pulled)
	}
}

func TestEnsureModelsStopsAtPullError(t *testing.T) {
	errPull := errors.New("disk full")
	m := &fakePuller{pullErr: map[string]error{"b:1": errPull}}
	err := EnsureModels(context.Background(), m, []string{"a:1", "b:1", "c:1"})
	if !errors.Is(err, errPull) {
		t.Fatalf("EnsureModels error = %v, want it to wrap the pull error", err)
	}
	if !strings.Contains(err.Error(), "b:1") {
		t.Fatalf("error %q does not name the model b:1", err)
	}
	if len(m.pulled) != 2 || m.pulled[0] != "a:1" || m.pulled[1] != "b:1" {
		t.Fatalf("pulled = %v, want [a:1 b:1] with c:1 not attempted", m.pulled)
	}
}

type fakePuller struct {
	models  []llm.Model
	pulled  []string
	pullErr map[string]error
}

func (f *fakePuller) ListModels(context.Context) ([]llm.Model, error) {
	return f.models, nil
}

func (f *fakePuller) PullModel(_ context.Context, name string) error {
	f.pulled = append(f.pulled, name)
	return f.pullErr[name]
}
