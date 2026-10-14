# Roadmap: goals, named epics and a grouped board

## Goal

The tracker shows how work is organised. An issue can be part of a goal (its parent issue). An epic is a named group of issues under one goal, not a number shared by the whole project. The web board groups a project's issues by goal and by epic, and the planner prompt records plans this way.

Tracker project: `PLAT`.

## Why

Epics and dependencies became tracker fields in `docs/roadmaps/epics-and-dependencies.md`, but an epic there is a bare number per project with no name and no owner. Six roadmaps each have an "epic 1", so their epics could not be recorded in one project, and nothing linked PLAT-14 to PLAT-2 except a comment. The board showed 49 issues in one flat list.

## Design

```
goal (an issue)            PLAT-2  Add epics and issue dependencies
  epic 1  "Fields and rule"    PLAT-14
  epic 2  "Persistence"        PLAT-25
  epic 3  "Service"            PLAT-34
  ...
```

- **Parent.** `Issue.Parent` is the ID of the goal an issue is part of, or empty. A goal and its parts are in the same project. Goals do not nest: a part cannot have parts, and an issue that has parts cannot be given a parent. One level is all a roadmap needs and it keeps the board to two levels.
- **Epic scope.** `Issue.Epic` keeps its meaning as an order, but it now counts within the issue's parent. Issues with no parent share one scope per project, which is what the field meant before, so existing data keeps its behaviour.
- **Epic names.** An `Epic` record names one epic: project, parent (may be empty), number, name. It is set with one upsert use case. An epic with no record still works; it just has no name.
- **Readiness.** `core.BlockedBy` gains two changes: the earlier-epic rule compares issues with the same project and the same parent, and a goal is blocked by each of its parts that is not done. A goal therefore leaves every queue while its parts are open and comes back when they are all done, in whatever stage the planner left it. A planner who leaves a goal in stage `testing` gets a final check of the whole goal for free.
- **Board.** One lane per goal, with the goal's progress, and inside it one row per epic with the four stage columns. Issues that are part of no goal get a last lane.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Model | Both stores keep a parent on every issue and keep epic names, and pass the contract tests. | 1 |
| 2. Rules | The Service validates parents and epic names, and readiness follows the new scope. | 2, parallel |
| 3. Transports | REST, the CLI, MCP and the web board each expose or show the hierarchy. | 4, parallel |
| 4. Adopt | The issue page shows a goal's parts, the planner prompt records plans as goals and epics, and ARCHITECTURE.md describes it. | 3, parallel |

No issue has a dependency inside its epic. After epic 4 the planner backfills the existing PLAT and BOOK issues by hand with the new commands; that is data, not code, so it is not an issue.

Why epic 1 is one issue: adding methods to `core.Store` breaks the build until both stores implement them (technique 4).

Why epic 2 is parallel: 2.1 owns `service.go` and its test, 2.2 owns `ready.go` and its test. Both are in package `core`, so each must keep its files compiling at every save.

Why epic 3 is parallel: each issue owns one transport package, and none imports another.

Why epic 4 follows epic 3: 4.1 edits the same web files as 3.4, and 4.2 tells planners to run CLI commands that 3.2 adds.

---

## Epic 1: Model

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Stores do not validate; they store what they are given.
  - A database schema change is a new entry appended to the `migrations` slice in `internal/store/sqlite/sqlite.go`. Never edit or reorder the existing entries.
  - Every `core.Store` implementation must pass `storetest.Run`.
- Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 1.1 Store a parent on issues and keep epic names in both stores

```
Epic: 1 Model
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
An issue can record its parent: the ID of the goal it is part of. A store can list the issues of one parent, and can keep a name for an epic. Nothing validates or uses these yet; later issues add the rules.

## Read first
- `internal/core/types.go`: `Issue`, `NewIssue`, `IssuePatch`, `IssueFilter`, the `EventType` constants and the `Store` interface.
- `internal/store/memory/memory.go`: all of it.
- `internal/store/sqlite/sqlite.go`: `migrations`, `issueColumns`, `CreateProject`, `CreateIssue`, `ListIssues`, `UpdateIssue`, `scanIssue` and `recordEvent`.
- `internal/store/storetest/storetest.go`: the top of `Run` (`seed`, `newIssue`) and the `epic and dependencies` and `events` subtests, as models.

## Change
- `internal/core/types.go`: add the fields, the `Epic` type, the event type and the two `Store` methods under Contract.
- `internal/store/memory/memory.go`: store `Parent`, filter on it, and implement `SetEpic` and `ListEpics`.
- `internal/store/sqlite/sqlite.go`: two new migration entries, the `parent` column in every issue statement, the `Parent` filter, and `SetEpic` and `ListEpics`.
- `internal/store/storetest/storetest.go` (tests): two new subtests, `parent` and `epics`.

## Contract
In `internal/core/types.go`:

Add to `Issue`, directly after `Assignee`:

	// Parent is the ID of the goal this issue is part of, or "" for none.
	// A goal and its parts are in the same project, and goals do not nest.
	Parent string `json:"parent"`

Replace the comment above `Issue.Epic` with:

	// Epic is the number of the epic this issue belongs to within its
	// parent, from 1 upward. Zero means no epic. Issues without a parent
	// count their epics within the project. Epic n waits for every issue
	// with the same project and parent in epics 1 to n-1 to be done.

Add to `NewIssue`, after `Stage`:

	// Parent is optional; see Issue.
	Parent string `json:"parent,omitempty"`

Add to `IssuePatch`, after `Status`:

	Parent *string `json:"parent,omitempty"`

Add to `IssueFilter`, after `Assignee`:

	// Parent keeps only the issues whose Parent is this ID.
	Parent string `json:"parent,omitempty"`

Add after the `Comment` types:

	// Epic names one epic. Parent is the goal the epic belongs to, or ""
	// for the epics of a project's issues that have no parent.
	type Epic struct {
		ProjectKey string `json:"project_key"`
		Parent     string `json:"parent"`
		Number     int    `json:"number"`
		Name       string `json:"name"`
	}

Add to the `EventType` constants:

	EventEpicSet EventType = "epic.set"

Add to the `Store` interface, after `ListComments`:

	// SetEpic stores e, replacing the name of an epic that already has the
	// same ProjectKey, Parent and Number. It returns ErrNotFound if the
	// project does not exist. It records one Event of type EventEpicSet
	// with e.ProjectKey, IssueID e.Parent and time at.
	SetEpic(ctx context.Context, e Epic, at time.Time) error
	// ListEpics returns the epics of a project ordered by Parent, then
	// Number. An empty projectKey lists every project's epics, ordered by
	// ProjectKey first. It never returns nil.
	ListEpics(ctx context.Context, projectKey string) ([]Epic, error)

Also change the sentence in the comment above `Store` so it reads: every successful CreateProject, CreateIssue, UpdateIssue, AddComment and SetEpic records exactly one Event.

Store behaviour, both stores:
- `CreateIssue` stores `Parent` as given. `GetIssue` and `ListIssues` return it. `UpdateIssue` saves a `Parent` changed by `fn`.
- `ListIssues` with a non-empty `IssueFilter.Parent` returns only issues whose `Parent` equals it. It combines with the other filter fields and with the page as they do today.
- A store does not check that the parent exists.

SQLite:
- Append these two entries to `migrations`, in this order:

	`ALTER TABLE issues ADD COLUMN parent TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE epics (
		project_key TEXT NOT NULL REFERENCES projects(key),
		parent      TEXT NOT NULL,
		number      INTEGER NOT NULL,
		name        TEXT NOT NULL,
		PRIMARY KEY (project_key, parent, number)
	);`,

- `issueColumns` ends with `, parent`. The INSERT in `CreateIssue` gains one placeholder, the UPDATE in `UpdateIssue` gains `parent = ?`, and `scanIssue` scans it last.
- `SetEpic` runs in one transaction: if no row of `projects` has the key, return an error wrapping `core.ErrNotFound` in the same form `CreateIssue` uses for a missing project; otherwise run `INSERT INTO epics (project_key, parent, number, name) VALUES (?, ?, ?, ?) ON CONFLICT (project_key, parent, number) DO UPDATE SET name = excluded.name`, then `recordEvent` with `core.EventEpicSet`, and commit.
- `ListEpics`: `SELECT project_key, parent, number, name FROM epics`, with `WHERE project_key = ?` when the key is not empty, then `ORDER BY project_key, parent, number`. Start from an empty non-nil slice.

Memory store: keep epics in a map or slice on the `Store` struct under the existing mutex; `SetEpic` returns an error wrapping `core.ErrNotFound` for an unknown project, and calls the existing `record` helper. `ListEpics` returns a new, sorted, non-nil slice.

## Done when
- `go vet ./internal/core/ ./internal/store/... ./internal/app/ ./internal/apptest/` succeeds.
- `gofmt -l internal/core internal/store` prints nothing.
- `go test -race ./internal/core/ ./internal/store/...` passes, including every existing subtest and `TestOpenUpgradesUnversionedDatabase` unchanged.
- The `parent` subtest covers, for both stores:
  - an issue created with `Parent: "AAA-1"` reads back with that parent through `GetIssue`; an issue created without one reads back `""`;
  - `UpdateIssue` setting `Parent` to `"AAA-2"`, then to `""`, reads back each value;
  - `ListIssues` with `Parent: "AAA-1"` returns exactly the issues with that parent, in order, and combining it with `Status` narrows it further.
- The `epics` subtest covers, for both stores:
  - a new store's `ListEpics("")` is empty and not nil;
  - `SetEpic` for a project that does not exist returns `core.ErrNotFound` and stores nothing;
  - after setting (AAA, "", 2, "Later"), (AAA, "AAA-1", 1, "First"), (AAA, "", 1, "Start") and (BBB, "", 1, "Other"), `ListEpics("AAA")` returns the three AAA epics ordered ("",1), ("",2), ("AAA-1",1), and `ListEpics("")` returns all four with AAA first;
  - setting (AAA, "", 1, "Renamed") leaves three AAA epics and changes that name;
  - each successful `SetEpic` adds exactly one event of type `epic.set` with the project key, the parent as `IssueID` and the given time; the failed one adds none.

## Out of scope
Validating a parent or an epic (existence, same project, nesting): the Service does that in a later issue. `internal/core/service.go`, `internal/core/ready.go`, the transports and the web UI. Any file not listed under Change.
```

---

## Epic 2: Rules

Re-check against the code when epic 1 is done: `Issue.Parent`, `IssuePatch.Parent`, `IssueFilter.Parent`, `core.Epic`, `Store.SetEpic`, `Store.ListEpics`.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- Another worker is editing a different file of package `internal/core` at the same time. Create or edit only the files this issue lists, and save each Go file only in a state that compiles. If a check fails because of an error in a file this issue does not list, wait a minute and run it again; do not fix that file.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 2.1 Validate parents and epic names in the Service

```
Epic: 2 Rules
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
The Service accepts a parent when an issue is created or updated and refuses one that would break the hierarchy. It also gains the two use cases for epic names.

## Read first
- `internal/core/service.go`: `CreateIssue`, `UpdateIssue`, `checkDependencies` (the model for a check helper), `normalizeKey` and `invalid`.
- `internal/core/types.go`: `Issue.Parent`, `NewIssue.Parent`, `IssuePatch.Parent`, `IssueFilter.Parent`, `Epic`, and `SetEpic` and `ListEpics` on `Store`.
- `internal/core/service_test.go`: `TestEpicsAndDependencies`, as the model for the new test.

## Change
- `internal/core/service.go`: a `checkParent` helper, its use in `CreateIssue` and `UpdateIssue`, and the `SetEpic` and `ListEpics` methods.
- `internal/core/service_test.go` (tests): add `TestParentsAndEpicNames`.

## Contract
	// checkParent normalizes parent (normalizeKey) and returns it. An empty
	// parent is valid and returns "". self is the normalized ID of the
	// issue being updated, or "" when the issue is new; projectKey is the
	// project of that issue. It returns an invalid error when the parent
	// is self, is not a stored issue, is in another project, or is itself
	// part of a goal, or when self already has parts.
	func (s *Service) checkParent(ctx context.Context, self, projectKey, parent string) (string, error)

Checks, in this order, each returning `invalid(...)` with the message shown (IDs filled in):
1. parent equals self: `issue PLAT-1 cannot be its own parent`
2. `s.store.GetIssue(ctx, parent)` returns `ErrNotFound`: `parent PLAT-9 does not exist`. Any other error is returned as it is.
3. the parent's `ProjectKey` differs from projectKey: `parent OTHER-1 is in another project`
4. the parent's own `Parent` is not empty: `parent PLAT-2 is part of PLAT-1; goals do not nest`
5. self is not "" and `s.store.ListIssues(ctx, IssueFilter{Parent: self})` returns at least one issue: `issue PLAT-1 has parts and cannot be given a parent`

`CreateIssue`: after the dependency check, `i.Parent, err = s.checkParent(ctx, "", i.ProjectKey, in.Parent)`.

`UpdateIssue`: when `patch.Parent` is not nil, read the issue with `s.store.GetIssue(ctx, normalizeKey(id))` (return its error as it is), call `checkParent` with the issue's ID and `ProjectKey`, and store the normalized value back in `patch.Parent`. Inside the store closure, `if patch.Parent != nil { i.Parent = *patch.Parent }`. An empty string clears the parent.

	// SetEpic names an epic, or renames it. The number must be 1 or more
	// and the name is required. A parent, if given, must be an issue of
	// the same project.
	func (s *Service) SetEpic(ctx context.Context, e Epic) (Epic, error)

`SetEpic` normalizes `ProjectKey` and `Parent` with `normalizeKey` and trims `Name`. Invalid errors: `project key is required`; `epic number must be 1 or more`; `epic name is required`; for a parent that is not stored `parent PLAT-9 does not exist`; for a parent in another project `parent OTHER-1 is in another project`. It then calls `s.store.SetEpic(ctx, e, s.now().UTC())` and returns the normalized epic. A missing project comes back from the store as `ErrNotFound` and is returned as it is.

	func (s *Service) ListEpics(ctx context.Context, projectKey string) ([]Epic, error)

`ListEpics` passes `normalizeKey(projectKey)` to the store.

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test -race ./internal/core/` passes, including the existing tests unchanged.
- `TestParentsAndEpicNames`, with a Service from `apptest.New(t)`, projects `PLAT` and `OTHER`, covers:
  - creating PLAT-1, then PLAT-2 with parent ` plat-1 `: PLAT-2 reads back with parent `PLAT-1`;
  - creating an issue with parent `PLAT-99` is `ErrInvalid`; with parent `OTHER-1` (an existing issue of project OTHER) is `ErrInvalid`;
  - creating an issue with parent `PLAT-2` (which is itself a part) is `ErrInvalid`;
  - updating PLAT-1 to have parent `PLAT-1` is `ErrInvalid`; updating PLAT-1, which has parts, to have a parent is `ErrInvalid`;
  - updating PLAT-2 with parent `""` clears it, and a patch without `Parent` leaves a parent unchanged;
  - `SetEpic` with number 0, with an empty name, with an unknown parent, and with an empty project key are each `ErrInvalid`; for project `NOPE` it is `ErrNotFound`;
  - `SetEpic{ProjectKey: "plat", Parent: "plat-1", Number: 1, Name: " Core "}` returns the epic with `PLAT`, `PLAT-1` and `Core`, and `ListEpics("plat")` returns it.

## Out of scope
`internal/core/ready.go` and its test (another issue owns them). `ClaimIssue`, `BlockIssue`, the stores, the transports. Any file not listed under Change.
```

### 2.2 Scope epics to a goal in the readiness rule and block a goal on its parts

```
Epic: 2 Rules
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
An issue waits only for the earlier epics of its own goal, and a goal waits for its parts. Before this, earlier epics were counted across the whole project, so two goals in one project blocked each other.

## Read first
- `internal/core/ready.go`: all of it.
- `internal/core/ready_test.go`: all of it.
- `internal/core/types.go`: `Issue.Parent` and `Issue.Epic`.

## Change
- `internal/core/ready.go`: change `BlockedBy` and its comment as under Contract.
- `internal/core/ready_test.go` (tests): add subtests to `TestBlockedBy` for the new rules; keep the existing ones.

## Contract
`BlockedBy(issue Issue, all []Issue) []string` keeps its signature. It returns, in this order:
1. each ID in `issue.DependsOn` whose issue is not done, or is not in `all` (unchanged);
2. the ID of each issue in `all` with the same `ProjectKey` and the same `Parent` as `issue`, with `0 < Epic < issue.Epic`, and a status other than `StatusDone`, in the order of `all`. This applies only when `issue.Epic > 0`. The only change is the added condition "the same `Parent`";
3. the ID of each issue in `all` whose `Parent` equals `issue.ID` and whose status is not `StatusDone`, in the order of `all`.

It returns nil when nothing blocks the issue. An ID already in the result is not added twice.

The comment above the function states these three rules.

Every existing subtest of `TestBlockedBy` uses issues with an empty `Parent`, which all share one scope, so their expectations do not change.

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test -race -run TestBlockedBy ./internal/core/` passes, with the existing subtests unchanged and new subtests covering:
  - two goals G1 and G2 in one project: an issue with parent G2 and epic 2 is not blocked by an unfinished issue with parent G1 and epic 1, and is blocked by an unfinished issue with parent G2 and epic 1;
  - an issue with a parent and epic 2 is not blocked by an unfinished issue of epic 1 that has no parent;
  - a goal with two parts, one done and one not, is blocked by exactly the unfinished part; with both done it is not blocked;
  - a goal that also lists one of its unfinished parts in `DependsOn` has that ID once in the result.
- `go test -race ./internal/core/` passes.

## Out of scope
`internal/core/service.go` and its test (another issue owns them). The stores, the transports. Any file not listed under Change.
```

---

## Epic 3: Transports

Re-check against the code when epic 2 is done: `Service.SetEpic`, `Service.ListEpics`, the parent messages, and the current text of each transport file.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. A transport decodes its input and calls the Service; it does not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- Other workers are editing other transport packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 3.1 Accept a parent and name epics in the REST API

```
Epic: 3 Transports
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
A REST client can set and filter by an issue's parent, and can name and list a project's epics.

## Read first
- `internal/transport/rest/rest.go`: `NewHandler`, `createIssue`, `listIssues`, `decode` and `respond`.
- `internal/transport/rest/rest_test.go`: the `do` helper and `TestRESTEpicsAndReady`, as the model for the new test.
- `internal/core/types.go`: `NewIssue.Parent`, `IssuePatch.Parent`, `IssueFilter.Parent` and `Epic`.

## Change
- `internal/transport/rest/rest.go`: the `parent` field on create, the `parent` query value on list, and two new routes.
- `internal/transport/rest/rest_test.go` (tests): add `TestRESTParentsAndEpics`.

## Contract
- `POST /projects/{key}/issues` accepts `"parent"` (a string) in its body and passes it to `core.NewIssue.Parent`.
- `PATCH /issues/{id}` already decodes `core.IssuePatch`, which now has `parent`; it needs no change. `"parent": ""` clears the parent.
- `GET /issues` accepts `parent=ID` and passes it to `core.IssueFilter.Parent`.
- New route `PUT /projects/{key}/epics`, registered next to the other project routes inside the authenticated mux. Body: `{"parent": "PLAT-2", "number": 1, "name": "Core"}`; `parent` may be absent. It calls `Service.SetEpic` with `ProjectKey` from the path and answers 200 with the epic as JSON through `respond`.
- New route `GET /projects/{key}/epics`: calls `Service.ListEpics` with the key from the path and answers 200 with a JSON array, `[]` when there are none.
- Errors map as `respond` already maps them: `ErrInvalid` 400, `ErrNotFound` 404.

## Done when
- `go vet ./internal/transport/rest/` succeeds and `gofmt -l internal/transport/rest` prints nothing.
- `go test -race ./internal/transport/rest/` passes, including the existing tests unchanged.
- `TestRESTParentsAndEpics`, on an app from `apptest.New(t)`, covers:
  - create project `plat` and PLAT-1; `POST /projects/plat/issues` with `{"title":"Part","parent":"plat-1"}` answers 201 with `"parent":"PLAT-1"`;
  - the same with `"parent":"PLAT-9"` answers 400;
  - `GET /issues?parent=PLAT-1` returns only PLAT-2;
  - `PATCH /issues/PLAT-2` with `{"parent":""}` answers 200 with an empty parent;
  - `PUT /projects/plat/epics` with `{"parent":"PLAT-1","number":1,"name":"Core"}` answers 200 with project key `PLAT`, parent `PLAT-1`, number 1 and name `Core`; with `"number":0` it answers 400; `PUT /projects/nope/epics` with a valid body answers 404;
  - `GET /projects/plat/epics` returns that one epic, and `GET /projects/other/epics` for a project with none returns `[]`.

## Out of scope
The CLI, MCP and web packages. `cmd/rest/main.go`. Any file not listed under Change.
```

### 3.2 Add -parent and the epic commands to the CLI

```
Epic: 3 Transports
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
From the command line a planner can make an issue part of a goal, list a goal's parts, and name the epics of a goal.

## Read first
- `internal/transport/cli/cli.go`: the `usage` constant, `dispatch`, `issueCreate`, `issueList`, `issueUpdate`, `printIssue`, `flagSet` and `parse`.
- `internal/transport/cli/cli_test.go`: `TestCLIEpicsAndReady`, as the model for the new test.
- `internal/core/types.go`: `NewIssue.Parent`, `IssuePatch.Parent`, `IssueFilter.Parent` and `Epic`.

## Change
- `internal/transport/cli/cli.go`: a `-parent` flag on `issue create`, `issue list` and `issue update`; a `Parent` line in `printIssue`; two new subcommands; the usage text.
- `internal/transport/cli/cli_test.go` (tests): add `TestCLIParentsAndEpics`.

## Contract
- `issue create ... [-parent ID]` sets `core.NewIssue.Parent`.
- `issue list ... [-parent ID]` sets `core.IssueFilter.Parent`.
- `issue update ID ... [-parent ID]` sets `core.IssuePatch.Parent` only when the flag was passed, the same way `-assignee` works, so `-parent ""` clears the parent.
- `printIssue` prints a line `Parent` directly above the `Epic` line, with the parent's ID, or `-` when there is none. The label column stays as wide as it is now, because `Depends on` is still the longest label.
- New subcommand `epic set -project KEY -number N -name NAME [-parent ID]`: calls `Service.SetEpic` and prints `set epic N of PARENT: NAME`, where PARENT is the parent's ID, or the project key when there is no parent. Example: `set epic 1 of PLAT-1: Core`.
- New subcommand `epic list -project KEY`: calls `Service.ListEpics` and prints a table with a tab writer like `issue list`, header `PARENT`, `EPIC`, `NAME`, with `-` for an empty parent.
- Both are dispatched on `epic set` and `epic list`. A positional argument after the flags is a usage error, as for the other commands.
- The `usage` constant gains `[-parent ID]` on the three issue lines and these two lines after the `issue comments` line:
    codeplat epic set -project KEY -number N -name NAME [-parent ID]
    codeplat epic list -project KEY

## Done when
- `go vet ./internal/transport/cli/` succeeds and `gofmt -l internal/transport/cli` prints nothing.
- `go test -race ./internal/transport/cli/` passes. The existing tests keep passing; if an existing expectation quotes `issue show` output that now has a `Parent` line in the middle of it, update that expectation and say so in your comment.
- `TestCLIParentsAndEpics`, with a Service from `apptest.New(t)` and commands run through `cli.Run`, covers:
  - `project create plat Platform`, `issue create -project plat -title Goal`, then `issue create -project plat -title Part -parent plat-1` exits 0; `issue show PLAT-2` contains `Parent      PLAT-1`;
  - `issue create -project plat -title Bad -parent PLAT-9` exits 1 and the error output contains `does not exist`;
  - `issue list -parent PLAT-1` lists PLAT-2 and not PLAT-1;
  - `issue update PLAT-2 -parent ""` exits 0 and its output contains `Parent      -`;
  - `epic set -project plat -parent plat-1 -number 1 -name Core` exits 0 and prints `set epic 1 of PLAT-1: Core`; with `-number 0` it exits 1; with no flags it exits 1 or 2 and changes nothing;
  - `epic list -project plat` prints a line containing `PLAT-1`, `1` and `Core`.

## Out of scope
The REST, MCP and web packages. `cmd/cli/main.go`. Any file not listed under Change.
```

### 3.3 Accept a parent and name epics in the MCP tools

```
Epic: 3 Transports
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
An agent using the MCP tools can make an issue part of a goal, list a goal's parts, and name and list epics.

## Read first
- `internal/transport/mcp/tools.go`: the `issue_create`, `issue_list` and `issue_update` entries and the `typed` helper.
- `internal/transport/mcp/mcp_test.go`: `TestMCPEpicsAndReady` and the helpers it uses, as the model for the new test, and the tool-count check in `TestMCP`.
- `internal/core/types.go`: `NewIssue.Parent`, `IssuePatch.Parent`, `IssueFilter.Parent` and `Epic`.

## Change
- `internal/transport/mcp/tools.go`: a `parent` property on three tools, and two new tools.
- `internal/transport/mcp/mcp_test.go` (tests): add `TestMCPParentsAndEpics`, and raise the tool count that `TestMCP` checks by two.

## Contract
- `issue_create`, `issue_update` and `issue_list` each gain this schema property: `"parent":{"type":"string","description":"ID of the goal this issue is part of, e.g. PLAT-2"}`. Their handlers already decode `core.NewIssue`, `core.IssuePatch` and `core.IssueFilter`, which now carry the field, so the handlers do not change.
- Append to the description of `issue_update`: ` Pass an empty parent to take the issue out of its goal.`
- Change the epic property description on `issue_create` and `issue_update` from `epic number within the project, 0 for none` to `epic number within the issue's parent, 0 for none`.
- New tool `epic_set`, placed after the last issue tool. Description: `Name an epic, or rename it. An epic is a numbered group of issues under one goal; pass the goal's ID as parent, or leave it out for a project's issues that have no goal.` Schema properties: `project_key` (string), `parent` (string), `number` (integer, minimum 1), `name` (string); required `project_key`, `number`, `name`. The handler decodes `core.Epic` and returns `svc.SetEpic(ctx, in)`.
- New tool `epic_list`. Description: `List the named epics of a project, ordered by parent and number.` Schema: `project_key` (string), required. The handler returns `svc.ListEpics(ctx, in.ProjectKey)`.

## Done when
- `go vet ./internal/transport/mcp/` succeeds and `gofmt -l internal/transport/mcp` prints nothing.
- `go test -race ./internal/transport/mcp/` passes, with the tool count in `TestMCP` raised by two and nothing else in the existing tests changed.
- `TestMCPParentsAndEpics`, in its own session with its own request ids, covers:
  - `tools/list` shows a `parent` property on `issue_create`, `issue_update` and `issue_list`, and lists `epic_set` and `epic_list`;
  - after creating project PLAT and PLAT-1, `issue_create` with `"parent":"plat-1"` returns an issue with parent `PLAT-1`; with `"parent":"PLAT-9"` it is a tool error whose text contains `does not exist`;
  - `issue_list` with `"parent":"PLAT-1"` returns only PLAT-2;
  - `epic_set` with project `PLAT`, parent `PLAT-1`, number 1, name `Core` returns that epic; with number 0 it is a tool error;
  - `epic_list` for `PLAT` returns that one epic.

## Out of scope
The REST, CLI and web packages. `internal/transport/mcp/mcp.go`. Any file not listed under Change.
```

### 3.4 Group the web board by goal and epic

```
Epic: 3 Transports
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
The project board shows how the work is organised: one lane per goal with its progress, and inside each lane one row per epic with the four stage columns. Before this the board was one flat set of columns for the whole project.

## Read first
- `internal/transport/web/web.go`: the `page` and `column` types, the `project` handler, `board` and the `all` helper.
- `internal/transport/web/page.html`: the `<style>` block and the `{{if .Board}}` section.
- `internal/transport/web/web_test.go`: `get`, `TestWeb` (its assertions about the project page) and `TestWebPaging`.
- `internal/core/types.go`: `Issue.Parent`, `Issue.Epic` and `Epic`.

## Change
- `internal/transport/web/web.go`: the `lane` and `row` types, a `lanes` function that replaces the direct use of `board` in the `project` handler, and the `page.Lanes` field.
- `internal/transport/web/page.html`: render lanes and rows inside the existing `<section id="board">`, plus a few lines of CSS.
- `internal/transport/web/web_test.go` (tests): add `TestBoardGroupsByGoalAndEpic`, and adjust existing assertions only where the new markup changes what they quote.

## Contract
In `web.go`:

	// lane is one goal on the board with its parts, or the issues that are
	// part of no goal.
	type lane struct {
		Goal  *core.Issue // nil for the lane of issues without a goal
		Title string      // "Other issues" for that lane, else ""
		Done  int         // parts with status done
		Total int         // parts
		Open  bool        // rendered expanded
		Rows  []row
	}

	// row is one epic of a lane.
	type row struct {
		Label   string // "Epic 2 · Persistence", "Epic 2", "No epic", or ""
		Columns []column
	}

	func lanes(issues []core.Issue, epics []core.Epic) []lane

`lanes` builds the board from one project's issues, in the order given:
- A goal is an issue that is the `Parent` of at least one issue in the list. There is one lane per goal, in the order the goals appear in the list. Its parts are the issues whose `Parent` is the goal's ID. The goal itself is not a card; it is the lane's heading.
- A last lane with `Goal` nil and `Title` `Other issues` holds the issues that have no parent and are not goals. It is left out when it would be empty. When the project has no goals at all, its `Title` is `""`, so a project without goals looks as it did before.
- An issue whose `Parent` is not in the list goes in the last lane.
- Inside a lane the parts are grouped into rows by `Epic`, ascending, with epic 0 last. Each row's `Columns` come from the existing `board` function applied to that row's issues.
- A row's `Label`: for epic n > 0, `Epic n · NAME` when `epics` has an entry with the lane's parent (the goal's ID, or `""` for the last lane), that number and the project key, else `Epic n`. For epic 0, `No epic` when the lane has other rows, else `""`.
- `Done` and `Total` count the lane's parts. `Open` is true when `Goal` is nil or `Done < Total`.

`page` gains `Lanes []lane`. The `project` handler reads the project's epics with `h.svc.ListEpics(r.Context(), key)` and sets `Lanes: lanes(issues, epics)`. It keeps setting `Board` (the whole project's columns) so the template's `{{if .Board}}` still decides whether to draw the board; the template no longer ranges over `.Board`.

In `page.html`, inside `<section id="board" ...>` (keep that element and all its attributes exactly as they are):
- For each lane: `<details class="lane"{{if .Open}} open{{end}}>` with a `<summary>` that holds, for a goal, a link `<a href="/issues/ID">ID · TITLE</a>`, the goal's status badge, and `<span class="count">DONE/TOTAL done</span>`; for the last lane, its `Title` when not empty and the same count. When a lane has no goal and an empty `Title`, render its rows without a `<details>` or `<summary>` around them.
- For each row: when `Label` is not empty, `<h3 class="epic">LABEL</h3>`; then `<div class="columns">` holding the four columns, each rendered exactly as a column is rendered today (the `<div class="column">` with its `<h2>`, cards and `Nothing here`).
- CSS: the grid that `.board` has today moves to `.columns`; `.board` becomes a plain block. Add small rules for `.lane`, its `summary` and `.epic` using the existing colour variables. Do not add any script.

## Done when
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `go test -race ./internal/transport/web/` passes. `TestWeb`, `TestWebPaging` and the other existing tests keep their meaning; change an existing assertion only if it quotes markup this issue changed, and list each such change in your comment.
- `TestBoardGroupsByGoalAndEpic`, on an app from `apptest.New(t)` with project `GRP`, covers:
  - GRP-1 `Goal one`; GRP-2 (parent GRP-1, epic 1), GRP-3 (parent GRP-1, epic 2), GRP-4 (no parent); epic (GRP, GRP-1, 1) named `Core`; GRP-2 set to done;
  - `GET /projects/GRP` contains a lane heading linking to `/issues/GRP-1` with `1/2 done`;
  - it contains `Epic 1 · Core` and `Epic 2`, in that order, and `Other issues`;
  - the card for GRP-4 comes after `Other issues`, and GRP-1 has no card (the text `href="/issues/GRP-1"` appears only in the lane heading);
  - the lane for GRP-1 is rendered with `open`; after GRP-3 is also set to done, the page shows `2/2 done` and that lane's `<details` has no `open`;
  - a project with no goals has no `<details` and no `Other issues` on its board.
- You cannot check this in a browser. Say so in your comment, so a person knows that step is still open.

## Out of scope
The issue page (the `{{with .Issue}}` section and the `issue` handler): a later issue changes it. `feed.go`, the `sse` package, `auth_test.go`, `feed_test.go`. Any file not listed under Change.
```

---

## Epic 4: Adopt

Re-check against the code when epic 3 is done: `lanes`, `row`, the CLI command spellings, and the current text of `prompts/decompose.md` and `ARCHITECTURE.md`.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. A transport decodes its input and calls the Service; it does not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- Other workers are editing other files at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 4.1 Show an issue's goal and a goal's parts on the web issue page

```
Epic: 4 Adopt
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
An issue page says which goal the issue is part of, and a goal's page lists its parts by epic with their status, so a reader can move up and down the hierarchy.

## Read first
- `internal/transport/web/web.go`: the `page` type, the `issue` handler, `lanes`, `lane` and `row`.
- `internal/transport/web/page.html`: the `{{with .Issue}}` section, in particular the second `<div class="meta">` inside `<div id="live">`.
- `internal/transport/web/web_test.go`: `TestIssuePageShowsBlockers`, as the model for the new test.

## Change
- `internal/transport/web/web.go`: two new `page` fields and their values in the `issue` handler.
- `internal/transport/web/page.html`: a `Part of` entry and a `Parts` list in the issue section.
- `internal/transport/web/web_test.go` (tests): add `TestIssuePageShowsGoalAndParts`.

## Contract
`page` gains:

	IssueParent string // ID of the goal this issue is part of, or ""
	IssueParts  []row  // a goal's parts grouped by epic; nil when it has none

In the `issue` handler, which already reads every issue into `others`:
- `IssueParent` is `issue.Parent`.
- `IssueParts`: collect the issues in `others` whose `Parent` is this issue's ID. If there are any, read the project's epics with `h.svc.ListEpics(r.Context(), issue.ProjectKey)`, call `lanes` with this issue followed by its parts, and take the `Rows` of the lane whose `Goal` has this issue's ID. Otherwise leave it nil.

In `page.html`:
- In the second `<div class="meta">` inside `<div id="live">`, before the epic entry: `{{if $.IssueParent}}<span>Part of <a href="/issues/{{$.IssueParent}}">{{$.IssueParent}}</a></span>{{end}}`.
- Directly after that `<div class="meta">`, still inside `<div id="live">` so it refreshes: when `$.IssueParts` is not empty, an `<h2>Parts</h2>` and, for each row, its `Label` in an `<h3 class="epic">` when not empty, then one line per issue of the row's columns, in column order: `<a href="/issues/ID">ID · TITLE</a>` followed by the issue's status badge as the board renders it.

## Done when
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `go test -race ./internal/transport/web/` passes, with the existing tests unchanged.
- `TestIssuePageShowsGoalAndParts`, on an app from `apptest.New(t)` with project `PRT`, covers:
  - PRT-1 `Goal`; PRT-2 (parent PRT-1, epic 1) and PRT-3 (parent PRT-1, epic 2); epic (PRT, PRT-1, 1) named `Core`;
  - `GET /issues/PRT-2` contains `Part of` followed by a link to `/issues/PRT-1`, and does not contain `>Parts<`;
  - `GET /issues/PRT-1` contains `>Parts<`, then `Epic 1 · Core`, then a link to `/issues/PRT-2`, then `Epic 2`, then a link to `/issues/PRT-3`, in that order, and does not contain `Part of`;
  - an issue with no parent and no parts contains neither `Part of` nor `>Parts<`.
- You cannot check this in a browser. Say so in your comment, so a person knows that step is still open.

## Out of scope
The board (`{{if .Board}}` section, `lanes` itself, the `project` handler). `feed.go`, the `sse` package, `auth_test.go`, `feed_test.go`. Any file not listed under Change.
```

### 4.2 Record plans as goals and epics in the planner prompt

```
Epic: 4 Adopt
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
`prompts/decompose.md` tells a planner to record a whole plan in the tracker: a goal issue, every issue as a part of it with its epic, and the epic names. Before this the planner created only the first epic's issues, because the tracker could not hold the rest back.

## Read first
- `prompts/decompose.md`: the "Dependencies" subsection, the "Issue format" section and the "Record the plan" section.

## Change
- `prompts/decompose.md`: replace the "Record the plan" section, and adjust the two sentences named under Contract.

## Contract
Replace the whole "Record the plan" section (from its heading up to, but not including, the "Illustration" heading) with:

## Record the plan

1. Write the roadmap to `docs/roadmaps/<short-name>.md`: the goal, then each epic in order with one line stating what is true when it is done, followed by its issues in full.
2. Create one issue for the goal itself, or use the issue you were given for it. Put it in stage `testing`: it leaves every queue while its parts are open, and comes back for a final check when they are all done. Ask which project key to use if you were not told.
3. Name each epic under the goal: `bin/codeplat epic set -project KEY -parent GOAL-ID -number N -name NAME`.
4. Create every issue of every epic, in stage `refinement`, as a part of the goal: `bin/codeplat issue create -project KEY -title TITLE -desc TEXT -parent GOAL-ID -epic N`, adding `-depends-on IDS` for a dependency inside its epic. The same fields exist on the `issue_create` and `epic_set` MCP tools.
5. Report the roadmap's shape: how many epics, how many issues in each, and every dependency with its reason.

The tracker holds the later epics back: an issue is in a queue only when every issue of the earlier epics of its goal is done, and every issue it depends on is done. So create them all now. When an epic finishes, re-check the next epic's issues against the code as it then stands and correct their descriptions, since earlier work often changes a detail.

Also:
- In the "Issue format" section, the first two lines of the description form (`Epic: <number and name>` and `Depends on: none` with its indented continuation) stay as they are: they tell the worker where the issue sits. Add one sentence directly after the closing of that code block: `Set the same epic number and dependencies as fields when you create the issue, so the tracker can order the work.`
- In the "Dependencies" subsection, the sentence `An issue never needs to declare a dependency on an earlier epic, because the roadmap order already covers that.` becomes `An issue never needs to declare a dependency on an earlier epic, because the tracker holds an epic back until the earlier epics of its goal are done.`

Change nothing else in the file.

## Done when
- `grep -c "epic set -project" prompts/decompose.md` prints 1.
- `grep -c "first epic only" prompts/decompose.md` prints 0.
- `grep -c "^## Record the plan" prompts/decompose.md` prints 1 and `grep -c "^## Illustration" prompts/decompose.md` prints 1.
- No tests are needed.

## Out of scope
The three worker prompts (`refine.md`, `implement.md`, `test.md`). Any Go file. `ARCHITECTURE.md` and `CLAUDE.md`.
```

### 4.3 Describe goals and named epics in ARCHITECTURE.md

```
Epic: 4 Adopt
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
`ARCHITECTURE.md` describes the hierarchy as it now is, so the next person does not read that an epic is a number shared by a whole project.

## Read first
- `ARCHITECTURE.md`: the "Epics and dependencies" bullet under "Decisions" and the "Web UI" bullet.

## Change
- `ARCHITECTURE.md`

## Contract
1. Replace the whole "Epics and dependencies" bullet with:

- **Goals, epics and dependencies:** an issue has `parent` (the ID of the goal it is part of, in the same project; goals do not nest), `epic` (a number within its parent, 0 for none) and `depends_on` (issue IDs). An `Epic` record gives an epic of a goal a name. An issue has blockers while an issue of an earlier epic of the same goal, or any issue it depends on, is not `done`, and a goal has its unfinished parts as blockers; `core.BlockedBy` lists them. A stage's queue is `stage`, `todo` and ready: `IssueFilter.Ready` keeps only issues whose `BlockedBy` is empty, and the worker prompts and the runner both list their queue with it. The Service rejects unknown dependencies, self-dependencies and cycles, and a parent that is missing, in another project or itself a part.

2. In the "Web UI" bullet, replace the words `a board by stage` with `a board grouped by goal and epic with a column per stage`.

3. In the "Stages" bullet, replace the words `(see Epics and dependencies)` with `(see Goals, epics and dependencies)`.

Change nothing else in the file.

## Done when
- `grep -c "Goals, epics and dependencies" ARCHITECTURE.md` prints 1.
- `grep -c "\*\*Epics and dependencies:\*\*" ARCHITECTURE.md` prints 0.
- `grep -c "grouped by goal and epic" ARCHITECTURE.md` prints 1.
- No tests are needed.

## Out of scope
Any Go file, `README.md`, `CLAUDE.md`, `prompts/`.
```
