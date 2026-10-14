package runner_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"codeplat/internal/apptest"
	"codeplat/internal/core"
	"codeplat/internal/runner"
)

const projectKey = "PLAT"

var prompts = map[core.Stage]string{
	core.StageRefinement:     "prompts/refine.md",
	core.StageImplementation: "prompts/implement.md",
	core.StageTesting:        "prompts/test.md",
}

// gate is the wait of one fake worker: the worker exits when it is opened.
type gate struct {
	ch   chan struct{}
	once sync.Once
}

func (g *gate) open() { g.once.Do(func() { close(g.ch) }) }

// fakeLauncher records each launch and returns a wait that blocks until the
// test opens that launch's gate.
type fakeLauncher struct {
	mu      sync.Mutex
	argvs   [][]string
	gates   []*gate
	err     error // returned by launch when set
	exitErr error // returned by wait
}

func newFake(t *testing.T) *fakeLauncher {
	t.Helper()
	f := &fakeLauncher{}
	t.Cleanup(func() {
		f.mu.Lock()
		gs := f.gates
		f.mu.Unlock()
		for _, g := range gs {
			g.open()
		}
	})
	return f
}

func (f *fakeLauncher) launch(_ context.Context, argv []string) (func() error, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.argvs = append(f.argvs, slices.Clone(argv))
	if f.err != nil {
		return nil, f.err
	}
	g := &gate{ch: make(chan struct{})}
	f.gates = append(f.gates, g)
	return func() error {
		<-g.ch
		return f.exitErr
	}, nil
}

func (f *fakeLauncher) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.argvs)
}

func (f *fakeLauncher) argv(i int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.argvs[i])
}

// release lets the worker of launch i exit.
func (f *fakeLauncher) release(t *testing.T, i int) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.gates) {
		t.Fatalf("release %d: only %d workers were started", i, len(f.gates))
	}
	f.gates[i].open()
}

func newService(t *testing.T) *core.Service {
	t.Helper()
	svc := apptest.New(t).Service
	if _, err := svc.CreateProject(context.Background(), core.NewProject{Key: projectKey, Name: "Platform"}); err != nil {
		t.Fatal(err)
	}
	return svc
}

func newIssue(t *testing.T, svc *core.Service, stage core.Stage) core.Issue {
	t.Helper()
	i, err := svc.CreateIssue(context.Background(), core.NewIssue{
		ProjectKey: projectKey,
		Title:      "issue in " + string(stage),
		Stage:      stage,
	})
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// expectedArgv is the argument vector the contract requires for an issue in
// a stage, given the worker command.
func expectedArgv(cmd []string, stage core.Stage, id string) []string {
	name := string(stage) + "-" + id
	msg := fmt.Sprintf("Read %s and follow it exactly. Your worker name is %s. The issue to work is %s.", prompts[stage], name, id)
	return append(slices.Clone(cmd), msg)
}

func discard() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// tickUntil calls Tick until n workers have been launched, failing the test
// if that takes more than two seconds.
func tickUntil(t *testing.T, r *runner.Runner, f *fakeLauncher, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for f.count() < n {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %d launches, have %d", n, f.count())
		}
		if err := r.Tick(context.Background()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTickLaunchesEachStageQueueWithContractMessage(t *testing.T) {
	svc := newService(t)
	ref := newIssue(t, svc, core.StageRefinement)
	impl := newIssue(t, svc, core.StageImplementation)
	test := newIssue(t, svc, core.StageTesting)
	fake := newFake(t)
	cmd := []string{"codeplat-worker", "-v"}
	r := runner.New(svc, fake.launch, runner.Options{Command: cmd, Cooldown: time.Hour, MaxWorkers: 3}, discard())

	if err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 3 {
		t.Fatalf("launches = %d, want 3", fake.count())
	}
	// Testing first, then implementation, then refinement.
	order := []struct {
		stage core.Stage
		id    string
	}{
		{core.StageTesting, test.ID},
		{core.StageImplementation, impl.ID},
		{core.StageRefinement, ref.ID},
	}
	for i, o := range order {
		want := expectedArgv(cmd, o.stage, o.id)
		if got := fake.argv(i); !slices.Equal(got, want) {
			t.Errorf("launch %d argv = %q, want %q", i, got, want)
		}
	}
}

func TestMaxWorkersOneRelaunchesNextIssueAfterWorkerExits(t *testing.T) {
	svc := newService(t)
	a := newIssue(t, svc, core.StageRefinement)
	b := newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Nanosecond, MaxWorkers: 1}, discard())
	ctx := context.Background()

	if err := r.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 1 {
		t.Fatalf("launches after first tick = %d, want 1", fake.count())
	}
	if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, a.ID); !slices.Equal(got, want) {
		t.Fatalf("first launch argv = %q, want %q", got, want)
	}

	// The cap is 1 and the first worker is still running.
	if err := r.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 1 {
		t.Fatalf("launches while worker runs = %d, want 1", fake.count())
	}

	// A worker claims its issue, so it leaves the todo queue.
	if _, err := svc.ClaimIssue(ctx, a.ID, "refinement-"+a.ID); err != nil {
		t.Fatal(err)
	}
	fake.release(t, 0)
	tickUntil(t, r, fake, 2)
	if fake.count() != 2 {
		t.Fatalf("launches = %d, want 2", fake.count())
	}
	if got, want := fake.argv(1), expectedArgv(nil, core.StageRefinement, b.ID); !slices.Equal(got, want) {
		t.Errorf("second launch argv = %q, want %q", got, want)
	}
}

func TestSecondTickWhileWorkerRunsLaunchesNothingForIssue(t *testing.T) {
	svc := newService(t)
	newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Hour, MaxWorkers: 2}, discard())
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		if err := r.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if fake.count() != 1 {
		t.Fatalf("launches = %d, want 1", fake.count())
	}
}

func TestCooldownAfterExitBlocksRelaunch(t *testing.T) {
	svc := newService(t)
	a := newIssue(t, svc, core.StageRefinement)
	b := newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Hour, MaxWorkers: 1}, discard())
	ctx := context.Background()

	if err := r.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	fake.release(t, 0)
	// A has exited but is in cooldown, so the free slot goes to B.
	tickUntil(t, r, fake, 2)
	if got, want := fake.argv(1), expectedArgv(nil, core.StageRefinement, b.ID); !slices.Equal(got, want) {
		t.Fatalf("second launch argv = %q, want %q", got, want)
	}

	fake.release(t, 1)
	for i := 0; i < 3; i++ {
		if err := r.Tick(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if fake.count() != 2 {
		t.Fatalf("launches = %d, want 2: an exited issue was launched again during its cooldown", fake.count())
	}
	if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, a.ID); !slices.Equal(got, want) {
		t.Errorf("first launch argv = %q, want %q", got, want)
	}
}

func TestClaimedIssueIsNotLaunched(t *testing.T) {
	svc := newService(t)
	claimed := newIssue(t, svc, core.StageRefinement)
	todo := newIssue(t, svc, core.StageRefinement)
	if _, err := svc.ClaimIssue(context.Background(), claimed.ID, "someone"); err != nil {
		t.Fatal(err)
	}
	fake := newFake(t)
	r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Hour, MaxWorkers: 2}, discard())

	if err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 1 {
		t.Fatalf("launches = %d, want 1", fake.count())
	}
	if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, todo.ID); !slices.Equal(got, want) {
		t.Errorf("launch argv = %q, want %q", got, want)
	}
}

func TestLauncherErrorIsLoggedAndNotRetriedInTick(t *testing.T) {
	svc := newService(t)
	a := newIssue(t, svc, core.StageRefinement)
	b := newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	fake.err = errors.New("no such program")
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, nil))
	r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Hour, MaxWorkers: 2}, log)
	ctx := context.Background()

	if err := r.Tick(ctx); err != nil {
		t.Fatalf("Tick returned %v, want nil", err)
	}
	if fake.count() != 2 {
		t.Fatalf("launch attempts = %d, want 2 (one per issue)", fake.count())
	}
	if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, a.ID); !slices.Equal(got, want) {
		t.Errorf("first attempt argv = %q, want %q", got, want)
	}
	if got, want := fake.argv(1), expectedArgv(nil, core.StageRefinement, b.ID); !slices.Equal(got, want) {
		t.Errorf("second attempt argv = %q, want %q", got, want)
	}
	if out := buf.String(); !bytes.Contains([]byte(out), []byte("no such program")) || !bytes.Contains([]byte(out), []byte("refinement-"+a.ID)) {
		t.Errorf("log does not record the launch error for %s: %q", a.ID, out)
	}

	// The failed issues are in cooldown, so the next tick does not retry them.
	if err := r.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.count() != 2 {
		t.Errorf("launch attempts after second tick = %d, want 2", fake.count())
	}
}

func TestTickDoesNotModifyCommand(t *testing.T) {
	svc := newService(t)
	issue := newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	cmd := []string{"codeplat-worker", "-v"}
	r := runner.New(svc, fake.launch, runner.Options{Command: cmd, Cooldown: time.Hour}, discard())

	if err := r.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(cmd) != 2 || cmd[0] != "codeplat-worker" || cmd[1] != "-v" {
		t.Errorf("Command changed to %q", cmd)
	}
	if got, want := fake.argv(0), expectedArgv(cmd, core.StageRefinement, issue.ID); !slices.Equal(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

func TestRunReturnsNilAfterContextCancelled(t *testing.T) {
	svc := newService(t)
	newIssue(t, svc, core.StageRefinement)
	fake := newFake(t)
	r := runner.New(svc, fake.launch, runner.Options{PollInterval: time.Millisecond, Cooldown: time.Hour}, discard())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for fake.count() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("Run did not launch a worker")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run returned %v, want nil", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after its context was cancelled")
	}
}

// TestTickSkipsIssuesThatAreNotReady covers the readiness rule the runner
// relies on: an issue waits for its dependencies and for earlier epics.
func TestTickSkipsIssuesThatAreNotReady(t *testing.T) {
	ctx := context.Background()

	t.Run("dependent issue is launched once its dependency is done", func(t *testing.T) {
		svc := newService(t)
		a := newIssue(t, svc, core.StageRefinement)
		b, err := svc.CreateIssue(ctx, core.NewIssue{
			ProjectKey: projectKey,
			Title:      "waits for a",
			Stage:      core.StageRefinement,
			DependsOn:  []string{a.ID},
		})
		if err != nil {
			t.Fatal(err)
		}
		fake := newFake(t)
		r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Nanosecond}, discard())

		if err := r.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if fake.count() != 1 {
			t.Fatalf("launches = %d, want 1", fake.count())
		}
		if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, a.ID); !slices.Equal(got, want) {
			t.Fatalf("first launch argv = %q, want %q", got, want)
		}

		status := core.StatusDone
		if _, err := svc.UpdateIssue(ctx, a.ID, core.IssuePatch{Status: &status}); err != nil {
			t.Fatal(err)
		}
		tickUntil(t, r, fake, 2)
		if fake.count() != 2 {
			t.Fatalf("launches = %d, want 2", fake.count())
		}
		if got, want := fake.argv(1), expectedArgv(nil, core.StageRefinement, b.ID); !slices.Equal(got, want) {
			t.Errorf("second launch argv = %q, want %q", got, want)
		}
	})

	t.Run("issue in a later epic is not launched before the earlier epic", func(t *testing.T) {
		svc := newService(t)
		c, err := svc.CreateIssue(ctx, core.NewIssue{
			ProjectKey: projectKey,
			Title:      "epic one",
			Stage:      core.StageRefinement,
			Epic:       1,
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.CreateIssue(ctx, core.NewIssue{
			ProjectKey: projectKey,
			Title:      "epic two",
			Stage:      core.StageRefinement,
			Epic:       2,
		}); err != nil {
			t.Fatal(err)
		}
		fake := newFake(t)
		r := runner.New(svc, fake.launch, runner.Options{Cooldown: time.Hour}, discard())

		if err := r.Tick(ctx); err != nil {
			t.Fatal(err)
		}
		if fake.count() != 1 {
			t.Fatalf("launches = %d, want 1", fake.count())
		}
		if got, want := fake.argv(0), expectedArgv(nil, core.StageRefinement, c.ID); !slices.Equal(got, want) {
			t.Errorf("launch argv = %q, want %q", got, want)
		}
	})
}
