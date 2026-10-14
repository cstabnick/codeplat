// Package runner starts one worker process for each todo issue that is ready
// in the stage queues. It only decides when to start a worker; the process itself is
// started through the Launcher it is given.
package runner

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeplat/internal/core"
)

// Launcher starts one worker process. argv[0] is the program and argv[1:]
// its arguments. It returns once the process has started. The function it
// returns waits for the process to exit and returns the error from that wait.
type Launcher func(ctx context.Context, argv []string) (wait func() error, err error)

// Options configures a Runner. A zero PollInterval means 5 seconds, a zero
// Cooldown means 60 seconds, and a zero MaxWorkers means 2.
type Options struct {
	// Command is the worker command. The runner appends the worker's
	// message as one final argument.
	Command []string
	// PollInterval is how often Run calls Tick.
	PollInterval time.Duration
	// Cooldown is how long an (issue, stage) pair is left alone after its
	// worker exits or fails to start.
	Cooldown time.Duration
	// MaxWorkers is the most workers that may be running at once.
	MaxWorkers int
}

// stagePrompts names the prompt file each stage's worker reads.
var stagePrompts = map[core.Stage]string{
	core.StageRefinement:     "prompts/refine.md",
	core.StageImplementation: "prompts/implement.md",
	core.StageTesting:        "prompts/test.md",
}

// stageOrder is the order in which Tick visits the stage queues.
var stageOrder = []core.Stage{
	core.StageTesting,
	core.StageImplementation,
	core.StageRefinement,
}

// Runner starts a worker for each todo issue that is ready in the stage queues.
type Runner struct {
	svc      *core.Service
	launch   Launcher
	opts     Options
	log      *slog.Logger
	interval time.Duration
	cooldown time.Duration
	max      int

	// mu guards running, live and cooldowns, which the wait goroutines update.
	mu        sync.Mutex
	running   int
	live      map[string]bool
	cooldowns map[string]time.Time
}

// New returns a Runner that uses svc to list issues and launch to start workers.
func New(svc *core.Service, launch Launcher, opts Options, log *slog.Logger) *Runner {
	r := &Runner{
		svc:       svc,
		launch:    launch,
		opts:      opts,
		log:       log,
		interval:  opts.PollInterval,
		cooldown:  opts.Cooldown,
		max:       opts.MaxWorkers,
		live:      make(map[string]bool),
		cooldowns: make(map[string]time.Time),
	}
	if r.interval == 0 {
		r.interval = 5 * time.Second
	}
	if r.cooldown == 0 {
		r.cooldown = 60 * time.Second
	}
	if r.max == 0 {
		r.max = 2
	}
	return r
}

// Run calls Tick every PollInterval until ctx is done, then returns nil. A
// Tick error is logged with log.Error and Run carries on. Workers still
// running when Run returns are not stopped.
func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
		if err := r.Tick(ctx); err != nil {
			r.log.Error("tick failed", "err", err)
		}
	}
}

// Tick starts the workers that are due. It returns an error only if a stage
// queue cannot be listed. Ticks are not called concurrently.
func (r *Runner) Tick(ctx context.Context) error {
	for _, stage := range stageOrder {
		issues, err := r.svc.ListIssues(ctx, core.IssueFilter{Stage: stage, Status: core.StatusTodo, Ready: true})
		if err != nil {
			return fmt.Errorf("list %s queue: %w", stage, err)
		}
		for _, issue := range issues {
			if !r.start(ctx, stage, issue.ID) {
				// The cap is reached: stop visiting issues and stages.
				return nil
			}
		}
	}
	return nil
}

// start launches a worker for the issue in the stage if one is due. It
// returns false when the worker limit is reached, which ends the tick.
func (r *Runner) start(ctx context.Context, stage core.Stage, id string) bool {
	key := id + "/" + string(stage)

	r.mu.Lock()
	if r.running >= r.max {
		r.mu.Unlock()
		return false
	}
	if r.live[key] || r.cooldowns[key].After(time.Now()) {
		r.mu.Unlock()
		return true
	}
	r.mu.Unlock()

	name := string(stage) + "-" + id
	argv := make([]string, 0, len(r.opts.Command)+1)
	argv = append(argv, r.opts.Command...)
	argv = append(argv, fmt.Sprintf("Read %s and follow it exactly. Your worker name is %s. The issue to work is %s.", stagePrompts[stage], name, id))

	wait, err := r.launch(ctx, argv)
	if err != nil {
		r.log.Error("start worker failed", "name", name, "err", err)
		r.mu.Lock()
		r.cooldowns[key] = time.Now().Add(r.cooldown)
		r.mu.Unlock()
		return true
	}

	r.log.Info("started worker", "name", name, "issue", id)
	r.mu.Lock()
	r.live[key] = true
	r.running++
	r.mu.Unlock()

	go func() {
		if err := wait(); err != nil {
			r.log.Error("worker failed", "name", name, "err", err)
		}
		r.mu.Lock()
		r.live[key] = false
		r.running--
		r.cooldowns[key] = time.Now().Add(r.cooldown)
		r.mu.Unlock()
	}()
	return true
}
