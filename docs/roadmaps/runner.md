# Roadmap: a runner that starts workers from each stage's queue

## Goal

Workers are started by hand today: one per issue per stage. A new process, `cmd/runner`, polls the tracker's three stage queues (status `todo`) and starts a headless `claude` worker with the matching prompt (`prompts/refine.md`, `prompts/implement.md`, `prompts/test.md`) for each issue it finds. The workers claim their own issues, as they do now, so the runner only starts them.

Tracker project: `PLAT`. Goal issue: PLAT-3.

## Assumptions

These were chosen without asking. Change them here before the first epic starts if they are wrong.

1. **The worker is a headless `claude` process.** The command comes from config, `runner.command`, and the runner appends the worker's message as its last argument. The dev config uses `["claude", "--agent", "haiku-worker", "-p"]`. `claude` has the flags `--agent` and `-p`, and the `haiku-worker` agent is in `.claude/agents/`, so a worker started from the repository root gets the same instructions as one started by hand.
2. **Permissions are not decided here.** A headless worker cannot answer a permission prompt, so it may stall on its first `bin/codeplat` or `go` command. `claude` has a `--permission-mode` flag and a `--dangerously-skip-permissions` flag. Choosing one is a decision for the owner; this roadmap leaves `runner.command` without a permission flag and says so in the issues that set it.
3. **The runner polls the tracker; it does not use the event log.** It calls `core.Service.ListIssues`, which exists today. It does not depend on the event log (PLAT-7) and does not touch `internal/core`.
4. **The runner is a process, not a use case.** Nothing new is added to `core.Service`, so nothing is exposed in REST, CLI or MCP. The repository rule about exposing use cases does not apply.
5. **Precondition: PLAT-7 has landed first.** Epic 1 compiles `internal/core`, and through `apptest` also `internal/app`, `internal/store/*` and `config`. Those files are being edited by the event-log worker right now. Do not start epic 1 until PLAT-7 is done and `go build ./...` passes on the tree.
6. **Stage order.** Each tick looks at the queues in the order testing, implementation, refinement, so finished work moves on before new work starts.
7. **Concurrency and timing defaults.** At most 2 workers run at once (`max_workers`). The runner looks every 5 seconds (`poll_interval`). After a worker exits, its issue and stage are left alone for 60 seconds (`cooldown`), so a worker that exits without claiming its issue is not restarted in a tight loop. A zero value in `runner.Options` means the default.
8. **Worker names are `<stage>-<ID>`,** for example `refinement-PLAT-3`. The name is the worker's `NAME` in its prompt, so it is the author of its comments and the claim's assignee.
9. **Dedupe is in memory.** The runner remembers which (issue, stage) pairs it has a live worker for. A runner restarted while a worker is running and has not yet claimed its issue could start a second worker for it. The window is small and the claim is atomic, so the second worker fails its claim and takes another issue. Accepted.
10. **Stopping the runner does not stop its workers.** A worker holding a claim must finish; killing it would leave the issue `in_progress` forever. Stopping the runner only stops new launches. The process package never kills a child.
11. **Workers run from the repository root.** The prompts call `bin/codeplat`, which is a relative path. The runner's working directory is used unless `runner.dir` is set, so the runner must be started from the repository root.
12. **Only the dev config gets a runner section.** `config/prod.json` and `config/test.json` are unchanged, so prod cannot run the runner until someone decides its command and permissions. A missing `runner` section is valid; `cmd/runner` refuses to start without `runner.command`.
13. **Worker output goes to the runner's standard error,** interleaved with its logs. There is no per-worker log file.
14. **A worker that crashes after claiming leaves its issue `in_progress`.** Nothing here recovers it. Recovery (for example, a stale-claim timeout) is not part of this goal and needs its own decision.
15. **The runner is in its own packages.** `internal/runner` holds the dispatch logic, `internal/process` starts OS processes, and `cmd/runner` is the entrypoint. The stage-to-prompt mapping lives in `internal/runner`, because the prompt files are part of the repository, not configuration.

## Design

```
cmd/runner                       load config, app.New, app.NewRunner, Run until interrupted
  app.NewRunner                  builds the launcher from config: process.Start with runner.dir
internal/runner                  Tick: list each stage's todo queue, launch a worker per issue
  core.Service.ListIssues        the only tracker call; claims are the workers' own
  Launcher (func)                injected; the runner never imports internal/process
internal/process                 os/exec: start argv, return a wait function
```

- `internal/runner` depends on `core` and on a launcher function it is given. It does not import `internal/process`, so the two can be written in parallel and tested separately.
- `internal/process` imports only the standard library.
- `app` is the only package that chooses the launcher, from config. The config section is `runner`, parsed by `config`.
- A launched worker is a child of the runner. When the runner exits, its workers keep running (assumption 10).

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Parts | The runner's dispatch logic and the process launcher each pass their own tests, and ARCHITECTURE.md describes the new packages. | 3, parallel |
| 2. Wiring | `cmd/runner` builds; `make build` produces `bin/codeplat-runner`; the runner starts with the dev config. | 2, parallel |

No issue has a dependency inside its epic.

Why two epics: `app.NewRunner` and `cmd/runner` import `internal/runner` and `internal/process`, so they cannot compile until both exist (epic 2 after epic 1).

Why epic 1 is parallel: the runner package imports `core` and its tests use `apptest`; the process package imports only the standard library; the ARCHITECTURE edit is documentation. No two of them share a file or a compiled package that another is editing.

Why the config section is in epic 2, not epic 1: the runner's tests use `apptest.New(t)`, which compiles `config` and `app`. If the config change were in epic 1, the runner's test would fail while the config worker was half-way through `config.go`. Config and app wiring are one issue (epic 2, issue 2.1).

Why `Launcher` is a plain function type: `runner` and `process` do not import each other, and `app` adapts one to the other with a three-line closure. The function type is identical across packages, so no shared interface file is needed.

Why the Makefile is separate from 2.1: it is a one-line edit that nothing else touches, and `make build` is only meaningful once `cmd/runner` exists, so it is checked at the end of epic 2 rather than in 2.1.

---

## Epic 1: Parts

Epic check when done: `go test -race ./internal/runner/ ./internal/process/` passes, and `ARCHITECTURE.md` contains the new lines.

Files changed by epic 1: `internal/runner/runner.go`, `internal/runner/runner_test.go`, `internal/process/process.go`, `internal/process/process_test.go`, `ARCHITECTURE.md`. No two issues touch the same file.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. The runner has no business rules; it only starts processes.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- The repository rule about exposing every use case in REST, CLI and MCP does not apply to this work: the runner is a process, not a use case.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when". Do not run `make build` or `make test`: a repository-wide build may fail because of someone else's half-finished file.
- Use the Go standard library only, plus the packages of this repository named in the issue. Do not edit `go.mod`.
```

### 1.1 Add the runner that starts a worker for each todo issue

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
A `Runner` that, on each `Tick`, lists each stage's todo queue and starts a worker process for each issue that does not already have one, within a limit on how many run at once. The worker process is started through a function the runner is given, so the runner itself never starts a process.

## Read first
- `internal/core/types.go`: `Stage`, `Status`, `Issue`, `IssueFilter`, and the constants `StageRefinement`, `StageImplementation`, `StageTesting`, `StatusTodo`.
- `internal/core/service.go`: `ListIssues`, which takes a `core.IssueFilter` and returns issues sorted by project and number.
- `internal/apptest/apptest.go`: `apptest.New(t)`, which returns an `*app.App` whose `Service` field is a `*core.Service`.
- `internal/transport/rest/rest_test.go`: an example of a test that uses `apptest.New(t)`, for style only.

## Change
- `internal/runner/runner.go`: package `runner`, with the types and functions under Contract.
- `internal/runner/runner_test.go` (tests): package `runner_test`, with the checks under "Done when". The testing worker writes this file.

## Contract
Package `runner`, file `internal/runner/runner.go`. Imports `codeplat/internal/core`, plus the standard library (`context`, `fmt`, `log/slog`, `sync`, `time`, `slices` if needed). It must not import `codeplat/internal/process`, `codeplat/internal/app` or `codeplat/config`.

```go
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

// Runner starts a worker for each todo issue in the stage queues.
type Runner struct { /* unexported fields */ }

// New returns a Runner that uses svc to list issues and launch to start workers.
func New(svc *core.Service, launch Launcher, opts Options, log *slog.Logger) *Runner

// Run calls Tick every PollInterval until ctx is done, then returns nil. A
// Tick error is logged with log.Error and Run carries on. Workers still
// running when Run returns are not stopped.
func (r *Runner) Run(ctx context.Context) error

// Tick starts the workers that are due. It returns an error only if a stage
// queue cannot be listed. Ticks are not called concurrently.
func (r *Runner) Tick(ctx context.Context) error
```

Behaviour of `Tick`, exactly:

- Stages are visited in this order: `core.StageTesting`, `core.StageImplementation`, `core.StageRefinement`.
- For each stage, call `svc.ListIssues(ctx, core.IssueFilter{Stage: stage, Status: core.StatusTodo})` and return the error if it fails. Take the issues in the order returned.
- Prompt file per stage:
  - `core.StageRefinement`: `prompts/refine.md`
  - `core.StageImplementation`: `prompts/implement.md`
  - `core.StageTesting`: `prompts/test.md`
- For each issue, in order:
  - If the number of running workers is `MaxWorkers`, stop visiting issues (and stages) for this tick.
  - The key is `issue.ID + "/" + string(stage)`. Skip the issue if the key has a live worker, or if its cooldown ends after the current time.
  - The worker name is `string(stage) + "-" + issue.ID`, for example `refinement-PLAT-3`.
  - The message is exactly, with `PROMPT`, `NAME` and `ID` filled in:
    `Read PROMPT and follow it exactly. Your worker name is NAME. The issue to work is ID.`
  - The argument vector is a new slice: `Options.Command` followed by the message as its last element. Do not modify `Options.Command`.
  - Call `launch(ctx, argv)`. If it fails, log with `log.Error` (keys `name` and `err`), set the key's cooldown to now plus the cooldown, and continue with the next issue.
  - If it succeeds, log `log.Info("started worker", "name", NAME, "issue", ID)`, mark the key live, count the worker as running, and start a goroutine that calls `wait()`. When `wait` returns, the goroutine logs any error with `log.Error` (keys `name`, `err`), then marks the key not live and sets its cooldown to now plus the cooldown.
- Running-worker count, live keys and cooldowns are guarded by one `sync.Mutex`, because the goroutines update them.
- Time comes from `time.Now`.

## Done when
- `go vet ./internal/runner/` succeeds and `gofmt -l internal/runner` prints nothing.
- `go test -race ./internal/runner/` passes.
- The tests use `apptest.New(t).Service` for the service and a fake `Launcher` defined in the test file. The fake records each argv and returns a wait function that blocks until the test closes a channel the test holds. Waits use a timeout of a second or two, never a fixed sleep. Tests cover:
  - an issue in each stage's todo queue is launched with the exact message from the Contract, with the right prompt file, and the argv starts with the configured command;
  - the order of launches is testing, implementation, refinement when the cap allows all three;
  - with `MaxWorkers: 1`, a second `Tick` launches nothing while the first worker is running, and launches the next issue after the first worker's wait returns and the cooldown has passed (use `Cooldown: time.Nanosecond` for that case);
  - a second `Tick` while a worker is still running for an issue launches nothing for that issue;
  - with `Cooldown: time.Hour`, an issue whose worker has exited is not launched again;
  - an issue whose status is `in_progress` (claimed) is not launched;
  - a launcher error is logged, does not make `Tick` return an error, and the issue is not launched again in the same tick;
  - `Run` returns nil after its context is cancelled.
- `Options.Command` is not modified by `Tick` (a test can check the slice length afterwards).

## Out of scope
`internal/process`, `internal/app`, `config`, `cmd/`, `Makefile`, `ARCHITECTURE.md`, `internal/core`. Do not add a use case to `core.Service`. Do not start processes in this package by any means other than the `Launcher`.
```

### 1.2 Add the process launcher

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
A function that starts an operating-system process and returns a function that waits for it. The runner will use it to start a worker. Child processes are never killed by this package, so a worker that holds a claim finishes its work.

## Read first
- `internal/runner/runner.go`: the `Launcher` type, which this function's signature must match once bound to a directory. It is written by another worker in parallel; read it only if it already exists, and do not edit it.

## Change
- `internal/process/process.go`: package `process`, with the function under Contract.
- `internal/process/process_test.go` (tests): package `process_test`, with the checks under "Done when". The testing worker writes this file.

## Contract
Package `process`, file `internal/process/process.go`. Standard library only.

```go
// Start runs argv[0] with the arguments argv[1:] in dir, or in the current
// directory if dir is empty. The process's standard output and standard
// error go to this process's standard error, and its standard input is
// empty. Start returns once the process has started; the returned function
// waits for the process to exit and returns the error from that wait.
// Nothing in this package kills the process.
func Start(dir string, argv []string) (wait func() error, err error)
```

Behaviour, exactly:
- If `len(argv) == 0`, return an error with the text `process: empty command`, and no wait function.
- Build the command with `exec.Command(argv[0], argv[1:]...)`. Set `cmd.Dir` to `dir`. Set `cmd.Stdout` and `cmd.Stderr` to `os.Stderr`. Leave `cmd.Stdin` nil.
- Call `cmd.Start()` and return its error wrapped as `fmt.Errorf("process: start %s: %w", argv[0], err)`.
- The returned function is `cmd.Wait`. It may be called once.
- Do not use `exec.CommandContext`, and do not take a context.

## Done when
- `go vet ./internal/process/` succeeds and `gofmt -l internal/process` prints nothing.
- `go test -race ./internal/process/` passes.
- Tests cover:
  - an empty `argv` returns the error with the text `process: empty command`;
  - `[]string{"true"}` starts and its wait returns nil;
  - `[]string{"sh", "-c", "exit 3"}` starts and its wait returns a non-nil error;
  - `dir` is honoured: create a file `marker` in `t.TempDir()`, start `[]string{"sh", "-c", "test -f marker"}` with that directory, and the wait returns nil;
  - a missing program (for example `[]string{"codeplat-no-such-program"}`) returns an error from `Start`, and its text starts with `process: start codeplat-no-such-program`.

## Out of scope
`internal/runner`, `internal/app`, `config`, `cmd/`, `Makefile`, `ARCHITECTURE.md`. Do not add a context parameter, a kill function or a log file.
```

### 1.3 Describe the runner in ARCHITECTURE.md

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)
- The file is also edited by the live-updates roadmap (its issue 3.2). Make only the edits listed here, and do not reformat or reword any other line.

## Goal
`ARCHITECTURE.md` lists the runner's packages and records why the runner is a separate process that starts headless workers, so the next person does not have to rediscover it.

## Read first
- `ARCHITECTURE.md`: the code block under "Shape" and the bullet list under "Decisions", including the "Stages" and "Claiming" bullets.

## Change
- `ARCHITECTURE.md`: two insertions. No other change.

## Contract
1. In the code block under "Shape", add these three lines directly above the line that starts with `config/`:

cmd/runner                     entrypoint: starts a worker for each todo issue in the stage queues
internal/runner                polls each stage's queue and launches workers, within a cap
internal/process               starts an operating-system process for a worker

2. In the bullet list under "Decisions", add this bullet directly after the "Claiming" bullet:

- **Worker runner:** `cmd/runner` polls each stage's `todo` queue through `core.Service.ListIssues` and starts a headless worker for each issue and stage that has none running, with the prompt file for that stage. The worker claims its own issue, so the runner does not. The command comes from `runner.command` in the config. At most `max_workers` run at once. Stopping the runner does not stop its workers, so a claim is never left behind by a kill.

## Done when
- `grep -c "^cmd/runner " ARCHITECTURE.md` prints 1.
- `grep -c "^internal/runner " ARCHITECTURE.md` prints 1.
- `grep -c "Worker runner:" ARCHITECTURE.md` prints 1.
- No Go file is changed.

## Out of scope
Any Go file, `CLAUDE.md`, `README.md`, `Makefile`. The "Web UI" and "Not built yet" bullets are owned by the live-updates roadmap.
```

---

## Epic 2: Wiring

Epic check when done: `make test` passes, and `make build` produces `bin/codeplat-runner`. These are run once by whoever integrates the epic, not by the parallel workers. Do not run them while epic 1 is still in progress elsewhere.

Files changed by epic 2: `config/config.go`, `config/config_test.go`, `config/dev.json`, `internal/app/app.go`, `internal/app/app_test.go`, `cmd/runner/main.go`, `Makefile`. No two issues touch the same file.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. The runner has no business rules.
  - Config chooses; `app.New` wires. Only `internal/app` may choose an implementation from config.
  - Schema changes are not part of this work.
  - A test that needs an app gets it from `apptest.New(t)` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand.
- The repository rule about exposing every use case in REST, CLI and MCP does not apply: the runner is a process, not a use case.
- Other workers are editing other files at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when". Do not run `make build` or `make test`: other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 2.1 Configure the runner, wire it in app and add cmd/runner

```
Epic: 2 Wiring
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
The config file can describe the worker runner, `app.NewRunner` builds it from config, and `cmd/runner` is a binary that runs it until interrupted.

## Read first
- `config/config.go`: the `Config` struct, `parse` and `validate`. The loader rejects unknown JSON fields, so the new section must be added to the struct.
- `config/config_test.go`: `TestParseRejectsBadConfig` shows the style of the parse tests.
- `config/dev.json`: the whole file.
- `internal/app/app.go`: `App`, `New`, and the `newStore` method for the switch style.
- `internal/runner/runner.go`: `Options`, `Launcher` and `New`. Written by another worker in epic 1; its signatures are in the Contract below.
- `internal/process/process.go`: `Start`. Written by another worker in epic 1; its signature is in the Contract below.
- `cmd/rest/main.go`: the whole file, as the model for `cmd/runner/main.go`.

## Change
- `config/config.go`: add the `Duration` type, the `Runner` type, the `Runner` field on `Config`, and the checks in `validate`, exactly as under Contract.
- `config/dev.json`: add the `runner` section under Contract.
- `config/config_test.go` (tests): add the cases under "Done when".
- `internal/app/app.go`: add the method `NewRunner` under Contract, and the imports it needs.
- `internal/app/app_test.go` (tests): new file, package `app_test`, with the checks under "Done when".
- `cmd/runner/main.go`: new file, package `main`, with the entrypoint under Contract.

## Contract
In `config/config.go`:

```go
// Duration is a time.Duration that is read from a JSON string such as "5s".
type Duration time.Duration

func (d *Duration) UnmarshalJSON(data []byte) error
// It accepts only a JSON string that time.ParseDuration accepts, and
// returns an error otherwise. It does not accept numbers.

// Runner configures the worker runner. The section is optional. A zero
// value means the runner's default (see runner.Options).
type Runner struct {
	Dir          string   `json:"dir,omitempty"`           // working directory of workers; empty means the runner's own
	Command      []string `json:"command,omitempty"`       // worker command; the runner appends the message
	PollInterval Duration `json:"poll_interval,omitempty"` // e.g. "5s"
	Cooldown     Duration `json:"cooldown,omitempty"`      // e.g. "60s"
	MaxWorkers   int      `json:"max_workers,omitempty"`
}
```

Add the field `Runner Runner \`json:"runner,omitempty"\`` to `Config`, after `Clock`.

In `validate`, add: `MaxWorkers` below 0 is an error with the text `runner.max_workers cannot be negative`; a `PollInterval` or `Cooldown` below 0 is an error with the text `runner durations cannot be negative`. Do not require the runner section: a config without it is valid.

In `config/dev.json`, add this member after `"clock": {}` (keep the existing members and their order):

  "runner": {
    "command": ["claude", "--agent", "haiku-worker", "-p"],
    "poll_interval": "5s",
    "cooldown": "60s",
    "max_workers": 2
  }

Do not add a `runner` section to `prod.json` or `test.json`.

In `internal/app/app.go`, add this method. Add the imports `context`, `codeplat/internal/process` and `codeplat/internal/runner`, and `time` is already imported.

// NewRunner builds the worker runner from the runner section of the config.
// It returns an error if runner.command is empty. The only launcher is
// process.Start, bound to runner.dir.
func (a *App) NewRunner() (*runner.Runner, error)

Behaviour, exactly:
- If `len(a.Config.Runner.Command) == 0`, return `nil` and the error `app: runner.command is required`.
- `launch := func(_ context.Context, argv []string) (func() error, error) { return process.Start(a.Config.Runner.Dir, argv) }`.
- `opts := runner.Options{Command: a.Config.Runner.Command, PollInterval: time.Duration(a.Config.Runner.PollInterval), Cooldown: time.Duration(a.Config.Runner.Cooldown), MaxWorkers: a.Config.Runner.MaxWorkers}`.
- Return `runner.New(a.Service, launch, opts, a.Log), nil`.

`cmd/runner/main.go`: package main, with a doc comment `// Command codeplat-runner starts a worker for each todo issue in the tracker's stage queues.` It has the same shape as `cmd/rest/main.go`:
- `config.FromEnvironment()`, then `app.New(cfg)`, then `defer a.Close()`;
- `r, err := a.NewRunner()`, and on error print it to stderr and return 1;
- `ctx` from `signal.NotifyContext` on `os.Interrupt` and `syscall.SIGTERM`;
- `if err := r.Run(ctx); err != nil` print the error with `a.Log.Error("runner stopped", "err", err)` and return 1; otherwise return 0.

## Done when
- `go vet ./config/ ./internal/app/ ./cmd/runner/` succeeds and `gofmt -l config internal/app cmd` prints nothing.
- `go build -o /dev/null ./cmd/runner` succeeds.
- `go test -race ./config/ ./internal/app/` passes.
- `config/config_test.go` covers: a valid config with a `runner` section parses; `"poll_interval": 5` (a number) is rejected; `"poll_interval": "soon"` is rejected; `"max_workers": -1` is rejected; `config.Load("dev")` gives `Runner.Command` equal to `claude --agent haiku-worker -p` and `PollInterval` equal to 5 seconds.
- `internal/app/app_test.go` covers: `apptest.New(t).NewRunner()` returns an error (the test config has no command) and a nil runner.

## Out of scope
`internal/runner`, `internal/process`, `internal/core`, `internal/store/*`, `Makefile`, `ARCHITECTURE.md`, `prod.json`, `test.json`, `CLAUDE.md`. Do not add a flag to `cmd/runner`. Do not change how `app.New` builds the store or clock.
```

### 2.2 Build the runner binary in the Makefile

```
Epic: 2 Wiring
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
`make build` also builds the runner binary, `bin/codeplat-runner`, so the runner is built the same way as the other entrypoints.

## Read first
- `Makefile`: the whole file, which has a `build` target with one `go build -o bin/... ./cmd/...` line per binary.

## Change
- `Makefile`: add one line to the `build` target.

## Contract
Add this line to the `build` target, directly after the line that builds `bin/codeplat-web`, indented with a tab like the others:

	go build -o bin/codeplat-runner ./cmd/runner

Change nothing else in the file.

## Done when
- `grep -c "bin/codeplat-runner ./cmd/runner" Makefile` prints 1.
- `grep -c "^build:" Makefile` prints 1 (the target is still there and not renamed).
- No Go file is changed.
- At the end of epic 2 the integrator runs `make build` and `make test`; this issue's worker does not.

## Out of scope
Every other file. `CLAUDE.md` says that `make build` builds "all four binaries"; after this change it builds five. That line is a repository rule, not this issue's to change.
```

---

## Check the plan

**Files changed per issue.** No file appears under two issues in the same epic.

| Issue | Changes |
|---|---|
| 1.1 | `internal/runner/runner.go`, `internal/runner/runner_test.go` |
| 1.2 | `internal/process/process.go`, `internal/process/process_test.go` |
| 1.3 | `ARCHITECTURE.md` |
| 2.1 | `config/config.go`, `config/config_test.go`, `config/dev.json`, `internal/app/app.go`, `internal/app/app_test.go`, `cmd/runner/main.go` |
| 2.2 | `Makefile` |

**Packages each "Done when" compiles.**
- 1.1 compiles `internal/runner`, `core`, `apptest`, `app`, `config`, both stores and `storetest`. None is edited in epic 1, provided PLAT-7 has landed (assumption 5).
- 1.2 compiles only the standard library.
- 1.3 compiles nothing.
- 2.1 compiles `config`, `app`, `runner`, `process`, `cmd/runner`. `runner` and `process` are epic 1 and finished before epic 2 starts. No other epic 2 issue edits any of them.
- 2.2 compiles nothing.

**Ports, databases, temp paths.** The runner tests use no port. The process tests use `t.TempDir()` and spawn `sh` and `true`. The `apptest` config is in-memory. Nothing is shared between concurrent checks.

**Could an issue start now if only earlier epics were done?** Epic 1 issues have no dependency and can start as soon as PLAT-7 is done. Epic 2 issues need epic 1.

**Dependencies.** None inside an epic. None on earlier epics (the order covers them).

**Could the epics merge?** No. Epic 2's `app.go` and `cmd/runner` import `runner` and `process`, so they cannot compile until epic 1's files exist.

**Collision with another roadmap.** `ARCHITECTURE.md` is also edited by `docs/roadmaps/live-updates.md`, issue 3.2, which changes the "Shape" code block and the "Web UI" and "Not built yet" bullets. Issue 1.3 here adds lines in the same code block and after the "Claiming" bullet. The two should not run at the same moment. If they do, merge them, or run 1.3 after live-updates 3.2. The "Shape" edits are adjacent lines, so a conflict is likely.

**Outside this roadmap.**
- `CLAUDE.md` says `make build` builds "all four binaries" and that rule becomes stale after 2.2. It is not edited here.
- `config/prod.json` has no runner section, so prod cannot run the runner yet.
- Permissions for headless workers are undecided (assumption 2). Without them, the first worker run is likely to stall.
- Recovering an issue left `in_progress` by a crashed worker is not planned (assumption 14).
