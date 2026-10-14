# Roadmap: epics and issue dependencies as tracker fields

## Goal

Epics and dependencies become fields of a tracker issue. An issue records its epic (a number within its project) and the IDs of the issues it depends on. The stage queue can hide an issue whose epic or dependencies are not finished, so a planner can create every issue of a goal up front, and workers take only the ones that are ready. Until now the planner held later epics back by not creating their issues.

Tracker project: `PLAT`. Tracker issue: `PLAT-2`.

## Assumptions

These are the readings chosen without asking. Change any of them before the first issue is created, because the issues are written to them.

1. **An epic is a number, not an entity.** `Issue.Epic` is an integer from 1 upward, scoped to the project. There is no epic table, no epic name and no epic API. Epic names stay in the roadmap file. An epic is "finished" when every issue in the project with that number has status `done`.
2. **Epics run in number order.** An issue with `Epic = n` (n > 0) is blocked by every issue of the same project with `0 < Epic < n` whose status is not `done`. An issue with `Epic = 0` is never blocked by epics and never blocks one.
3. **Dependencies are issue IDs.** `DependsOn` lists IDs such as `PLAT-4`. An ID may name an issue of any project. The issue is blocked by each listed issue whose status is not `done`. A listed ID that does not exist blocks too. On write, an unknown ID is rejected with `ErrInvalid`, as are a self-dependency and a dependency cycle.
4. **"Finished" means status `done`.** Stage does not matter. A `done` issue in the refinement stage counts as finished.
5. **Readiness is a filter, not a status.** Nothing is stored about readiness. `IssueFilter.Ready` keeps only issues with no blockers, computed from all stored issues when the list is read. The stage queue is `stage + status todo + ready`. Lists without `ready` return what they returned before.
6. **Claiming does not check readiness.** `ClaimIssue` is unchanged: a worker who claims a blocked issue by its ID succeeds. The queue is the gate. The dependency and cycle checks run before the write, not inside the store's transaction. This is a small race, accepted because planners write issues rarely.
7. **Existing data.** The migration gives every existing issue `epic = 0` and no dependencies. Nothing else changes for them.
8. **Event log.** The event log from PLAT-7 is complete when this roadmap starts. The new fields need no new events: the existing `issue.updated` and `issue.created` events cover them.
9. **Migrations.** The two new schema changes are appended after the `events` entry that PLAT-7 adds.
10. **`prompts/decompose.md` is not changed by this roadmap.** It still says to create only the first epic's issues. Whether planners should now create all issues up front is a decision for the user; it is not made here.
11. **The web UI shows the new fields** on the issue page only. The board is unchanged.
12. **Collision with the live-updates roadmap.** `internal/transport/web/web.go` and `page.html` are also edited by `docs/roadmaps/live-updates.md`, and `page.html` is where its three-second poll lives. Epic 5's web issue must not start until live-updates' Switch-over epic has landed.

## Design

```
core.Issue                      new fields: Epic int, DependsOn []string
  |
  +-- core.BlockedBy(issue, all) pure rule: the IDs that block an issue
  |
  +-- store (sqlite, memory)     persists both fields; never interprets them
  |
  +-- Service                    validates on write (epic >= 0, deps exist,
  |                              no self, no cycle); ListIssues with Ready
  |                              filters through BlockedBy
  |
  +-- REST, CLI, MCP             pass the fields through; expose ready
  +-- web issue page             shows epic, dependencies and blockers
```

- **Where the rule lives.** The readiness rule is a pure function in `core`, so the stores never decide readiness and the transports never repeat it.
- **Storage.** `epic` is an integer column. `depends_on` is one JSON-array text column, read and written whole, so an issue's row carries everything about it and no join is needed.
- **Queue.** A worker's queue is `issue list -stage S -status todo -ready`. The worker prompts switch to this in epic 5.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Fields and rule | `core.Issue`, `NewIssue`, `IssuePatch` and `IssueFilter` carry the new fields, and `core.BlockedBy` passes its tests. | 1, alone |
| 2. Persistence | Both stores round-trip the new fields, and the storetest suite covers them. | 1, alone |
| 3. Service | The Service validates epic and dependencies on write and filters by `Ready`. | 1, alone |
| 4. Transports | REST, CLI and MCP accept the fields and expose `ready`. | 3, parallel |
| 5. Queue and view | The web issue page shows epic and blockers, the worker prompts use `-ready`, and ARCHITECTURE.md describes the rule. | 3, parallel |

Dependencies between epics, with the reason each is needed:

- Epic 2 after epic 1: the stores must compile against the new field names.
- Epic 3 after epic 2: the Service's tests run through `apptest`, which builds the stores.
- Epic 4 after epic 3: the transports' tests check validation and `ready` behaviour that only the Service provides.
- Epic 5 after epic 4: the worker prompts pass the `-ready` flag that epic 4 adds. The web issue also needs the store and the rule from epics 1 and 2, and it must follow live-updates (assumption 12).

No issue has a dependency inside its epic. Each epic's issues edit disjoint files, and no two issues in an epic share a package that another issue in that epic is compiling.

## Epic 1: Fields and rule

Each issue in this epic is a single change that the other issues do not touch.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - A database schema change is a new entry appended to the `migrations` slice in `internal/store/sqlite/sqlite.go`. Never edit or reorder the existing entries.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Do not run `make test` or `go build ./...`: half-finished files elsewhere can fail them.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 1.1 Add epic and dependency fields and the readiness rule to core

```
Epic: 1 Fields and rule
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
An issue can carry an epic number and a list of issue IDs it depends on. A pure function in `core` says which unfinished issues block a given issue. Nothing reads or writes these fields yet.

## Read first
- `internal/core/types.go`: the `Issue`, `NewIssue`, `IssuePatch` and `IssueFilter` types, and the `Status` constants.

## Change
- `internal/core/types.go`: add the fields listed under Contract to `Issue`, `NewIssue`, `IssuePatch` and `IssueFilter`. Change nothing else in the file.
- `internal/core/ready.go` (new): package `core`, the function `BlockedBy`.
- `internal/core/ready_test.go` (new, tests): package `core` (an internal test), so it needs no other package.

## Contract
In `internal/core/types.go`, add to `Issue` after `Assignee`, before `CreatedAt`:

	// Epic is the number of the epic this issue belongs to within its
	// project, from 1 upward. Zero means no epic. Epic n waits for every
	// issue of the same project in epics 1 to n-1 to be done.
	Epic int `json:"epic"`
	// DependsOn lists the IDs of issues that must be done before this one
	// starts. Stores always return it non-nil.
	DependsOn []string `json:"depends_on"`

Add to `NewIssue`, after `Stage`:

	// Epic and DependsOn are optional; see Issue.
	Epic      int      `json:"epic,omitempty"`
	DependsOn []string `json:"depends_on,omitempty"`

Add to `IssuePatch`, after `Status`:

	Epic      *int      `json:"epic,omitempty"`
	DependsOn *[]string `json:"depends_on,omitempty"`

Add to `IssueFilter`, after `Assignee`:

	// Ready keeps only issues for which BlockedBy returns nothing. Stores
	// ignore it; the Service applies it.
	Ready bool `json:"ready,omitempty"`

In `internal/core/ready.go`:

	// BlockedBy returns the IDs that keep issue from being ready, in this
	// order: each ID in issue.DependsOn whose issue is not done, or is not
	// in all at all; then the ID of each issue in all with the same
	// ProjectKey, 0 < Epic < issue.Epic, and Status other than StatusDone,
	// in the order they appear in all. It returns nil when nothing blocks
	// the issue. all may contain issues of several projects. An issue with
	// Epic 0 is never blocked by epics, and never blocks one.
	func BlockedBy(issue Issue, all []Issue) []string

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test -race -run TestBlockedBy ./internal/core/` passes, and the tests cover:
  - an issue with Epic 0 and no DependsOn: BlockedBy returns nil;
  - a dependency that is `done` does not block; one that is `todo` or `in_progress` does;
  - a DependsOn ID that is not in all blocks, and is returned;
  - in one project, an issue in epic 2 is blocked by an unfinished issue in epic 1, and not by an unfinished issue in epic 2 or epic 3;
  - an issue in epic 2 is not blocked by an unfinished issue of epic 1 in a different project;
  - an issue with Epic 0 is not blocked by unfinished issues of any epic;
  - the result lists the dependency blockers first, in DependsOn order, then the epic blockers in the order of all;
  - an issue whose epic blockers are all done and whose dependencies are all done returns nil.
- `go test -race ./internal/core/` passes (the existing tests still pass; this issue changes no behaviour).

## Out of scope
Any change to `service.go`, the stores, the transports or the web UI. Do not add a Store method. Do not change `Status` or `Stage`.
```

## Epic 2: Persistence

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - A database schema change is a new entry appended to the `migrations` slice in `internal/store/sqlite/sqlite.go`. Never edit or reorder the existing entries. The slice already has an `events` entry at the end; append after it.
  - Business rules live in `internal/core`. The stores store and return fields; they do not decide readiness or validate epics or dependencies.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `go build ./...`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 2.1 Persist epic and dependencies in both stores

```
Epic: 2 Persistence
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
Both stores save and return `Issue.Epic` and `Issue.DependsOn`, and the shared store test suite checks that they round-trip. Existing rows read back as epic 0 with no dependencies.

## Read first
- `internal/core/types.go`: the `Issue` type, where `Epic` and `DependsOn` are the new fields.
- `internal/store/sqlite/sqlite.go`: the `migrations` slice, `issueColumns`, `CreateIssue`, `UpdateIssue` and `scanIssue`.
- `internal/store/memory/memory.go`: `CreateIssue` and `UpdateIssue`.
- `internal/store/storetest/storetest.go`: the `get round-trips every field` and `update` subtests, as the model for a new subtest.

## Change
- `internal/store/sqlite/sqlite.go`: append two entries to `migrations`, change `issueColumns`, `CreateIssue`, `UpdateIssue` and `scanIssue`, and add the two helpers under Contract.
- `internal/store/memory/memory.go`: normalize and copy `DependsOn` in `CreateIssue` and `UpdateIssue`.
- `internal/store/storetest/storetest.go`: add one subtest, `epic and dependencies`, inside `Run`.

## Contract
Append these two entries to the `migrations` slice in `sqlite.go`, after the existing last entry, one entry each:

	`ALTER TABLE issues ADD COLUMN epic INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE issues ADD COLUMN depends_on TEXT NOT NULL DEFAULT '[]';`,

Change `issueColumns` to end with `, epic, depends_on`. `depends_on` holds a JSON array of strings.

In `CreateIssue` and `UpdateIssue` the SQLite store writes `epic` as the integer and `depends_on` as the result of:

	// encodeDependsOn returns the JSON array text for ids; nil is "[]".
	func encodeDependsOn(ids []string) (string, error)

`scanIssue` reads `depends_on` into a string and decodes it with `json.Unmarshal` into `i.DependsOn`. If the decoded value is nil it sets `i.DependsOn = []string{}`. Add `"encoding/json"` to the imports.

In `CreateIssue`, before the insert and before returning, set `i.DependsOn` to `[]string{}` if it is nil, in both stores. `CreateIssue` returns the issue with `DependsOn` non-nil.

In `UpdateIssue` (both stores) the value `fn` leaves in `DependsOn` is saved as given, except nil is saved as an empty array. In the memory store, copy the slice (`slices.Clone`) before storing it so that the caller cannot change the stored value afterwards; `slices.Clone` of nil returns nil, so set it to `[]string{}` after cloning if it is nil.

The new storetest subtest, `epic and dependencies`, uses `seed(t)` and `newIssue` like the others and checks:
- an issue created with `Epic: 2` and `DependsOn: []string{"AAA-1"}` comes back from `GetIssue` with the same `Epic` and `DependsOn`;
- an issue created with no epic and no dependencies comes back with `Epic` 0 and `DependsOn` equal to an empty, non-nil slice;
- `UpdateIssue` with `fn` setting `Epic = 3` and `DependsOn = []string{"BBB-1", "AAA-1"}` is returned and then read back unchanged, in that order;
- `UpdateIssue` with `fn` setting `DependsOn = []string{}` clears the dependencies: `GetIssue` returns a non-nil empty slice;
- `ListIssues` with `IssueFilter{ProjectKey: "AAA"}` returns each issue with its fields.

## Done when
- `gofmt -l internal/store` prints nothing and `go vet ./internal/store/...` succeeds.
- `go test -race ./internal/store/...` passes. This runs the new subtest against both stores.
- Opening an existing database built with the older migrations is not tested by this issue; do not add a test for it.

## Out of scope
Validation (epic must be non-negative, dependencies must exist, cycles): that is the Service's job in Epic 3. Readiness: that is `core.BlockedBy`. Any file outside the three listed. Do not edit `types.go`, `service.go` or `sqlite_test.go`.
```

## Epic 3: Service

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`, and `core.Service` is the only place they are enforced. Transports validate nothing beyond decoding their own input.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service`. Do not construct a store or a service by hand in such a test.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when".
- Do not run `make build` or `go build ./...`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 3.1 Validate epic and dependencies in the Service and filter by ready

```
Epic: 3 Service
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
`Service` accepts an epic and dependencies on create and update, rejects bad ones, and `ListIssues` with `Ready` returns only the issues nothing blocks.

## Read first
- `internal/core/service.go`: `CreateIssue`, `UpdateIssue`, `ListIssues` and the `normalizeKey` and `invalid` helpers.
- `internal/core/ready.go`: `BlockedBy`.
- `internal/core/types.go`: the new fields on `NewIssue`, `IssuePatch` and `IssueFilter`.
- `internal/core/service_test.go`: its existing style. It is package `core_test`.

## Change
- `internal/core/service.go`: change `CreateIssue`, `UpdateIssue` and `ListIssues`, and add `checkDependencies` under Contract.
- `internal/core/service_test.go` (tests): add a test function `TestEpicsAndDependencies`, in the style of `TestServiceRules`.

## Contract
	// checkDependencies normalizes ids (trim, then normalizeKey) and drops
	// duplicates, keeping the first occurrence. It returns a non-nil slice.
	// self is the normalized ID of the issue being updated, or "" when the
	// issue is new. It returns an invalid error (wrapping ErrInvalid) when:
	//   - an ID is empty;
	//   - an ID equals self;
	//   - an ID is not a stored issue (GetIssue returns ErrNotFound);
	//   - self is not "" and following DependsOn from the IDs reaches self.
	// The message names the offending ID.
	func (s *Service) checkDependencies(ctx context.Context, self string, ids []string) ([]string, error)

Behaviour changes:
- `CreateIssue`: if `in.Epic < 0` return `invalid("epic must be 0 or a positive number")`. Otherwise `deps, err := s.checkDependencies(ctx, "", in.DependsOn)`, and set `Epic: in.Epic, DependsOn: deps` on the issue.
- `UpdateIssue`: if `patch.Epic != nil && *patch.Epic < 0` return the same invalid error. If `patch.DependsOn != nil`, compute `deps, err := s.checkDependencies(ctx, normalizeKey(id), *patch.DependsOn)` and replace `patch.DependsOn` with `&deps`. Inside the function passed to `store.UpdateIssue`: if `patch.Epic != nil` set `i.Epic = *patch.Epic`; if `patch.DependsOn != nil` set `i.DependsOn = *patch.DependsOn`. Both checks run before the store call.
- `ListIssues`: after the existing stage and status checks, if `f.Ready` is false, behave as now. If it is true: copy `f` with `Ready` set to false, call `s.store.ListIssues` with it to get `matching`; call `s.store.ListIssues(ctx, IssueFilter{})` to get `all`; keep each issue `i` of `matching` for which `len(BlockedBy(i, all)) == 0`. Return a non-nil slice.
- `ClaimIssue` is not changed (assumption 6).

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test -race ./internal/core/` passes. The new test covers:
  - a negative epic is rejected on create and on update, with `errors.Is(err, core.ErrInvalid)`;
  - a dependency on an issue that does not exist is rejected on create and on update;
  - a self-dependency on update is rejected;
  - a two-step cycle (A depends on B; then updating B to depend on A) is rejected on update, and B's dependencies are unchanged afterwards;
  - duplicate dependency IDs are stored once, in order, and lower-case IDs are normalized;
  - `ListIssues` with `Ready: true` and `Stage: refinement`, `Status: todo` leaves out an issue whose dependency is `todo`, leaves out an issue in epic 2 while an epic 1 issue of its project is not done, and keeps it once that issue is done;
  - `ListIssues` with `Ready: false` still returns the blocked issue.
- `make test` is NOT run by this issue. Run only the commands above.

## Out of scope
The stores, the transports and the web UI. `ClaimIssue`. Do not change `types.go`, `ready.go` or any store file.
```

## Epic 4: Transports

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Transports translate a protocol to `core.Service` calls and back. They validate nothing beyond decoding their own input. The Service rejects bad epics and dependencies with `core.ErrInvalid`, and the transport maps that error as it already does.
  - Tests get the Service from `apptest.New(t)`. Do not construct a store or a service by hand.
- Other workers are editing the other transport packages at the same time. Create or edit only the files this issue lists.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

The three issues below are parallel. They share no file, and each test uses an in-memory store from `config/test.json`, so no fixed port or path is shared.

### 4.1 Accept epic, dependencies and ready in the REST API

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
The REST API accepts `epic` and `depends_on` when creating an issue, and `GET /projects/{key}/issues` and `GET /issues` honour `ready=true`.

## Read first
- `internal/transport/rest/rest.go`: `createIssue`, `listIssues` and `updateIssue`. `updateIssue` already decodes `core.IssuePatch` directly, so it needs no change; check that.
- `internal/transport/rest/rest_test.go`: the `do` helper and the existing `TestREST` steps.

## Change
- `internal/transport/rest/rest.go`: `createIssue` and `listIssues` as under Contract.
- `internal/transport/rest/rest_test.go` (tests): add a test function `TestRESTEpicsAndReady`, using the `do` helper.

## Contract
`createIssue`'s body struct gains two fields:

	Epic      int      `json:"epic"`
	DependsOn []string `json:"depends_on"`

and passes them to `core.NewIssue` as `Epic` and `DependsOn`.

`listIssues` sets `Ready: r.URL.Query().Get("ready") == "true"` in the `core.IssueFilter` it builds.

## Done when
- `go vet ./internal/transport/rest/` succeeds and `gofmt -l internal/transport/rest` prints nothing.
- `go test -race ./internal/transport/rest/` passes. The new test covers:
  - a POST with `"epic":2,"depends_on":["PLAT-1"]` returns 201 and a body with those values;
  - a POST whose `depends_on` names a missing issue returns 400;
  - a PATCH setting `"depends_on":["PLAT-1"]` on an issue that would cycle back returns 400;
  - `GET .../issues?ready=true` leaves out an issue whose dependency is `todo`, and `GET` without `ready` includes it.

## Out of scope
`cmd/rest/main.go`, the Service, the other transports, the web UI. Any file other than `rest.go` and `rest_test.go`.
```

### 4.2 Accept epic, dependencies and ready in the CLI

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
The CLI can set an issue's epic and dependencies, show them, and list only ready issues with `-ready`.

## Read first
- `internal/transport/cli/cli.go`: `issueCreate`, `issueList`, `issueUpdate` and `printIssue`, and the `parse` and `flagSet` helpers.
- `internal/transport/cli/cli_test.go`: the table of `steps` in `TestCLI`, and how it checks stdout and stderr.

## Change
- `internal/transport/cli/cli.go`: change `issueCreate`, `issueList`, `issueUpdate` and `printIssue`, and add `splitIDs` under Contract.
- `internal/transport/cli/cli_test.go` (tests): add a test function `TestCLIEpicsAndReady` using the same `apptest.New(t).Service` and `Run` call style as `TestCLI`.

## Contract
	// splitIDs splits a comma-separated list of issue IDs. It trims each
	// item and drops empty ones. It returns a non-nil slice.
	func splitIDs(s string) []string

Flags:
- `issue create`: `-epic` (integer, sets `in.Epic`, usage "epic number, 0 for none") and `-depends-on` (string, sets `in.DependsOn = splitIDs(value)`, usage "comma-separated issue IDs this one waits for").
- `issue list`: `-ready` (boolean, sets `f.Ready`, usage "only issues that nothing blocks").
- `issue update`: `-epic` (integer) and `-depends-on` (string). In the `fs.Visit` switch, `epic` sets `patch.Epic = &epic`; `depends-on` sets `deps := splitIDs(dependsOn)` and `patch.DependsOn = &deps`, so passing `-depends-on ""` clears them.

`printIssue` adds two lines after the `Status` line, using the same `tabwriter` layout:
- `Epic` then the number (0 is printed as 0);
- `Depends on` then the IDs joined with ", ", or `-` when there are none.

`issue list` output keeps its columns.

## Done when
- `go vet ./internal/transport/cli/` succeeds and `gofmt -l internal/transport/cli` prints nothing.
- `go test -race ./internal/transport/cli/` passes. The new test covers:
  - `issue create -project plat -title T -epic 2 -depends-on plat-1` succeeds and `issue show` prints `Epic` 2 and `Depends on PLAT-1`;
  - `issue create ... -depends-on PLAT-99` fails with a message containing `does not exist`;
  - `issue list -ready` leaves out an issue whose dependency is `todo`;
  - `issue update PLAT-2 -depends-on ""` clears the dependencies.

## Out of scope
`cmd/cli/main.go`, the Service, the other transports. Any file other than `cli.go` and `cli_test.go`.
```

### 4.3 Accept epic, dependencies and ready in the MCP tools

```
Epic: 4 Transports
Depends on: none

## Environment
(the Epic 4 Environment section above, pasted in full)

## Goal
The MCP tools `issue_create`, `issue_update` and `issue_list` accept `epic`, `depends_on` and `ready`, and their input schemas say so.

## Read first
- `internal/transport/mcp/tools.go`: the `issue_create`, `issue_list` and `issue_update` entries. Their input schemas are hand-written JSON, so the new fields must be added to them by hand; the Go structs are decoded by the `typed` helper and already pick up the new fields.
- `internal/transport/mcp/mcp_test.go`: how the existing tests call tools and read the reply.

## Change
- `internal/transport/mcp/tools.go`: change the three entries as under Contract.
- `internal/transport/mcp/mcp_test.go` (tests): add a test function `TestMCPEpicsAndReady` in the style of the existing tests.

## Contract
In `issue_create`'s and `issue_update`'s `InputSchema`, add these properties:

	"epic":{"type":"integer","minimum":0,"description":"epic number within the project, 0 for none"},
	"depends_on":{"type":"array","items":{"type":"string"},"description":"IDs of issues that must be done first, e.g. PLAT-4"}

In `issue_list`'s `InputSchema`, add:

	"ready":{"type":"boolean","description":"true keeps only issues that nothing blocks"}

Update the `Description` of `issue_list` to: "List issues, optionally filtered by project, stage, status and assignee. A loop's queue is its stage with status todo and ready true." Update `issue_update`'s description to add: "Set epic to a number and depends_on to a list of issue IDs; pass an empty depends_on to clear them."

## Done when
- `go vet ./internal/transport/mcp/` succeeds and `gofmt -l internal/transport/mcp` prints nothing.
- `go test -race ./internal/transport/mcp/` passes. The new test covers:
  - `tools/list` shows `epic`, `depends_on` and `ready` in the schemas of the three tools;
  - an `issue_create` call with `epic` and `depends_on` returns them in the result;
  - an `issue_create` call with a `depends_on` ID that does not exist returns an error result;
  - an `issue_list` call with `ready: true` leaves out an issue whose dependency is `todo`.

## Out of scope
`mcp.go` (the JSON-RPC server), the Service, the other transports. Any file other than `tools.go` and `mcp_test.go`.
```

## Epic 5: Queue and view

Epic 5 starts only after epic 4 is merged and after live-updates' Switch-over epic (assumption 12). The three issues below are parallel and share no file.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports validate nothing beyond decoding their own input.
  - The web UI is read-only. It shows data and never changes it.
  - Tests get the Service from `apptest.New(t).Service`.
- Other workers are editing other files at the same time. Create or edit only the files this issue lists.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 5.1 Show epic and blockers on the web issue page

```
Epic: 5 Queue and view
Depends on: none

## Environment
(the Epic 5 Environment section above, pasted in full)

## Goal
The web issue page shows an issue's epic, its dependencies and the IDs that currently block it, so a person reading the tracker can see why an issue is not in a queue.

## Read first
- `internal/transport/web/web.go`: the `issue` handler, the `page` struct and how `render` is called.
- `internal/transport/web/page.html`: the `{{if .Issue}}` section that shows the issue, and the `.meta` and `.badge` classes in the `<style>` block.
- `internal/transport/web/web_test.go`: how it requests a page and checks its body.
- `internal/core/ready.go`: `BlockedBy`.

## Change
- `internal/transport/web/web.go`: in the `issue` handler, after `ListComments`, call `h.svc.ListIssues(r.Context(), core.IssueFilter{})` (on error, `h.fail`), compute `core.BlockedBy(issue, all)`, and set the three new fields below on the `page` value.
- `internal/transport/web/page.html`: in the issue section, show the epic, the dependencies and the blockers. Do not change the board or the project page.
- `internal/transport/web/web_test.go` (tests): add a test function `TestIssuePageShowsBlockers`.

## Contract
Add to the `page` struct:

	IssueEpic      int
	IssueDepends   []string
	IssueBlockedBy []string

In `page.html`, inside the issue section, add a line showing `Epic N` when `.IssueEpic` is not 0, a line `Depends on` with the IDs as links to `/issues/{ID}` when `.IssueDepends` is not empty, and a line `Blocked by` with the IDs as links when `.IssueBlockedBy` is not empty. Use the existing `.meta` class. Do not add any script.

## Done when
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `go test -race ./internal/transport/web/` passes. The new test creates a project, an issue `X-1` and an issue `X-2` with `Epic: 2` and `DependsOn: ["X-1"]` through the Service, then requests `/issues/X-2` and checks the body contains `Epic 2`, a link to `/issues/X-1` under `Blocked by`, and does not contain `Blocked by` after `X-1` is set to `done`.

## Out of scope
The board, the project page, the poll on `page.html` (the live-updates goal owns it), any change to a store or the Service. Do not edit `web.go` beyond the `issue` handler and the `page` struct.
```

### 5.2 Use the ready filter in the worker queues

```
Epic: 5 Queue and view
Depends on: none

## Environment
(the Epic 5 Environment section above, pasted in full)

## Goal
Each worker's queue command also filters by ready, so a worker never takes an issue whose epic or dependencies are unfinished.

## Read first
- `prompts/refine.md`, `prompts/implement.md`, `prompts/test.md`: the "See your queue" row of the table near the top of each file.

## Change
- `prompts/refine.md`: in the "See your queue" row, change `bin/codeplat issue list -stage refinement -status todo` to `bin/codeplat issue list -stage refinement -status todo -ready`.
- `prompts/implement.md`: the same change for `-stage implementation`.
- `prompts/test.md`: the same change for `-stage testing`.

## Contract
Only the command in that one table row changes. Nothing else in the three files changes.

## Done when
- `grep -l -- '-status todo -ready' prompts/refine.md prompts/implement.md prompts/test.md` lists all three files.
- `bin/codeplat` is not run, and no file other than the three listed is changed.

## Out of scope
`prompts/decompose.md` (assumption 10). Any other line of the three prompts.
```

### 5.3 Describe epics and dependencies in ARCHITECTURE.md

```
Epic: 5 Queue and view
Depends on: none

## Environment
(the Epic 5 Environment section above, pasted in full)

## Goal
ARCHITECTURE.md records the epic and dependency rules and no longer lists them as not built.

## Read first
- `ARCHITECTURE.md`: the "Decisions" list, especially the "Stages", "Claiming" and "Not built yet" bullets.

## Change
- `ARCHITECTURE.md`: in "Not built yet", remove "epics and issue dependencies as tracker fields (the decomposition prompt keeps them in `docs/roadmaps/` for now)". Add one bullet after "Claiming":

	- **Epics and dependencies:** an issue has `epic` (a number within its project, 0 for none) and `depends_on` (issue IDs). An issue is blocked while an issue of an earlier epic in its project, or any issue it depends on, is not `done`; `core.BlockedBy` computes this. A stage's queue is `stage` and `todo` and `ready`, where `ready` is the `IssueFilter.Ready` filter. The Service rejects unknown dependencies, self-dependencies and cycles.

## Contract
The bullet text is as above. Keep every other line unchanged.

## Done when
- `grep -n "Epics and dependencies" ARCHITECTURE.md` prints one line.
- `grep -n "epics and issue dependencies" ARCHITECTURE.md` prints nothing.

## Out of scope
Every other section of ARCHITECTURE.md, and `prompts/decompose.md`.
```

## Totals

- 5 epics, 9 issues: epic 1 has 1, epic 2 has 1, epic 3 has 1, epic 4 has 3 (parallel), epic 5 has 3 (parallel).
- No dependency is declared inside an epic. Epic order carries the sequence.

## Follow-ups not in this roadmap

- Whether `prompts/decompose.md` should now create all of a goal's issues up front, with `epic` and `depends_on` set (assumption 10).
- `ClaimIssue` does not refuse a blocked issue (assumption 6). If workers should not be able to claim by ID, that is a later issue, and it needs the claim check to move into the store's transaction.
- Dependency cycle checks are not atomic with the write (assumption 6).
