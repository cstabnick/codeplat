# Roadmap: pagination for issue and comment lists

## Assumptions

These were chosen without asking. Change them here before any issue is created if one is wrong.

1. **Scope.** "Issue and comment lists" means the issue list (REST `GET /issues`, CLI `issue list`, MCP `issue_list`, the web board) and the comment list (REST `GET /issues/{id}/comments`, CLI `issue comments`, MCP `comment_list`, the web issue page). The project list is not paginated: the goal does not name it and projects are few.
2. **Offset and limit, not cursors.** Issues sort by project and number and comments by number, so offsets are stable as long as nothing is deleted. A page read while issues are being created can shift; that is accepted for now.
3. **Limits.** `limit` defaults to 50 when absent or 0, and must be between 1 and 200. `offset` must be 0 or more. Out-of-range values are `ErrInvalid`: REST answers 400, the CLI exits 1 with "invalid input", MCP returns an error result. Non-integer values are rejected by each transport while decoding.
4. **Response shape is unchanged.** Lists stay bare JSON arrays. There is no total count and no "has more" flag. A client has read the last page when it gets fewer items than it asked for. Wrapping the array would break every existing client.
5. **The default applies to everyone.** `issue list` and the MCP and REST list calls now return at most 50 unless told otherwise. That is the point of the change.
6. **The web UI keeps showing everything.** The board and the issue page are read-only views, so they read every page through a small helper rather than showing a page control. A later goal can add paging controls to the UI.
7. **Issues are paged in SQL, comments in core.** The store applies `LIMIT`/`OFFSET` to issue queries. Comments per issue are few, so the core slices the list that `Store.ListComments` already returns, and the `Store` port does not change. If comments ever grow large, move that slice into the store.
8. **Store semantics.** In the store, `Limit` 0 means no limit. The service is the only place that applies the default, so the store contract tests stay simple.
9. **No schema change.** No migration and no index. The event log (issue PLAT-7) is in flight. Every edit here is to `ListIssues` and to new code, so it sits beside that work and does not touch the event methods.

## Goal

List endpoints return a bounded page instead of everything. Callers ask for a page with `limit` and `offset` over REST, the CLI and MCP, and the web UI still shows every issue and comment.

Tracker project: `PLAT`. Tracker issue: PLAT-5.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Page contract | The `Page` type, the `IssueFilter` field and `Service` paging are in place, and the service rejects bad values. | 1 |
| 2. Store paging | Both stores return the requested window of `ListIssues`. | 1 |
| 3. Expose paging | REST, CLI, MCP and the web UI take or read pages, and the docs say so. | 5, parallel |

No issue has a dependency inside its epic.

Why three epics: the transports (epic 3) test paged results end to end, so they need the stores to page (epic 2). The stores need the `Page` field in `IssueFilter` (epic 1). Each arrow is a real compile or test dependency, so each step is its own epic.

Why epic 1 cannot merge with epic 2: it would be seven files across two layers. Splitting them by file would leave the store issue compiling against a field that does not exist yet.

Why epic 1 leaves the transports alone: `Service.ListIssues` keeps its signature (the page rides in `IssueFilter`) and `Service.ListComments` keeps its signature. Only a new method, `ListCommentsPage`, is added. So no transport stops compiling in epic 1, and the epic ends green.

Why epic 3 is parallel: each issue edits one transport package, and none of those packages imports another. Each only needs the contract from epic 1 and the stores from epic 2, both of which are already in place. The docs issue touches only `ARCHITECTURE.md`.

---

## Epic 1: Page contract

When this epic is done, `core` has a `Page` type with defaults and bounds. `IssueFilter` carries a page, and `Service` offers `ListCommentsPage`. The service rejects out-of-range values. The stores still ignore the page, so issue paging is not yet visible through the service.

### 1.1 Add the Page type and paging to the core service

```
Epic: 1 Page contract
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports validate nothing beyond decoding their own input.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- Other workers are editing other files at the same time, including the event log in `internal/core/types.go`, `internal/core/service.go` and the stores. Keep your edits to the functions and places named below and leave everything else as it is. Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
Give `core` a page type that says which window of a list to return, with a default limit and bounds, and make the service enforce the bounds. Add a paged comment list to the service. After this issue the service accepts paging, but the stores still ignore it; that is epic 2.

## Read first
- `internal/core/types.go`: the `IssueFilter` struct (it has four fields: `ProjectKey`, `Stage`, `Status`, `Assignee`, all with JSON tags) and the `ErrInvalid` var.
- `internal/core/service.go`: `invalid(msg string) error` (near the bottom, next to `invalidStage`), `ListIssues` (validates stage and status, then calls `s.store.ListIssues`), and `ListComments` (a one-line pass-through, directly after `AddComment`).
- `internal/core/service_test.go`: `TestServiceRules` for the style used. The file's package is `core_test` and it imports `apptest`.

## Change
- `internal/core/page.go` (new file, package `core`): the `Page` type, the two constants and the `resolve` method, exactly as under Contract.
- `internal/core/types.go`: add the `Page` embed to `IssueFilter` as the last field, exactly as under Contract. Change nothing else in the file.
- `internal/core/service.go`:
  - In `ListIssues`, after the existing `f.ProjectKey = normalizeKey(...)` line and before the stage check, add the page resolution from Contract. Keep the existing checks.
  - Directly after the `ListComments` method, add `ListCommentsPage`, exactly as under Contract.
  - Do not touch `EventsSince`, `LastEventID`, or any other method.
- `internal/core/service_test.go`: add one new test function `TestPaging` at the end of the file. Do not edit the existing tests.

## Contract
`internal/core/page.go`, new file:

```go
package core

const (
	// DefaultPageLimit is the limit used when a Page has Limit 0.
	DefaultPageLimit = 50
	// MaxPageLimit is the largest limit a list call accepts.
	MaxPageLimit = 200
)

// Page selects one window of a list. Limit is how many items to return;
// Offset is how many matching items to skip. The JSON names are "limit" and
// "offset", so a struct that embeds Page decodes them from request bodies.
type Page struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// resolve returns p with the default limit filled in. It fails with
// ErrInvalid if the limit is outside 1 to MaxPageLimit (after the default)
// or the offset is negative.
func (p Page) resolve() (Page, error) {
	if p.Limit == 0 {
		p.Limit = DefaultPageLimit
	}
	if p.Limit < 1 || p.Limit > MaxPageLimit {
		return Page{}, invalid(fmt.Sprintf("limit must be between 1 and %d", MaxPageLimit))
	}
	if p.Offset < 0 {
		return Page{}, invalid("offset cannot be negative")
	}
	return p, nil
}
```
The file needs `import "fmt"`.

`internal/core/types.go`, add as the last field of `IssueFilter`:

```go
	// Page selects one window of the matches. Service.ListIssues fills in
	// the default limit; a store treats Limit 0 as no limit.
	Page
```

`internal/core/service.go`:

```go
// ListIssues: after f.ProjectKey = normalizeKey(f.ProjectKey), add
	var err error
	if f.Page, err = f.Page.resolve(); err != nil {
		return nil, err
	}

// ListCommentsPage returns one page of an issue's comments, oldest first.
// It returns ErrNotFound if the issue does not exist, and an empty non-nil
// slice past the end.
func (s *Service) ListCommentsPage(ctx context.Context, issueID string, p Page) ([]Comment, error) {
	p, err := p.resolve()
	if err != nil {
		return nil, err
	}
	all, err := s.store.ListComments(ctx, normalizeKey(issueID))
	if err != nil {
		return nil, err
	}
	if p.Offset >= len(all) {
		return []Comment{}, nil
	}
	return all[p.Offset:min(p.Offset+p.Limit, len(all))], nil
}
```
`min` is a Go 1.21 builtin; the module already uses `slices` and `cmp`, so the toolchain has it.

## Done when
- `go vet ./internal/core/` succeeds and `gofmt -l internal/core` prints nothing.
- `go test ./internal/core/` passes.
- `TestPaging` covers:
  - with one issue `PAGE-1` and five comments made through `svc.AddComment` (bodies `comment 1` to `comment 5`), `ListCommentsPage(ctx, "PAGE-1", Page{Limit: 2})` returns comments numbered 1 and 2;
  - `Page{Limit: 2, Offset: 4}` returns only number 5;
  - `Page{Offset: 5}` returns an empty, non-nil slice;
  - `Page{}` returns all five (default limit);
  - `ListCommentsPage` with `Page{Limit: 201}`, `Page{Limit: -1}` or `Page{Offset: -1}` returns an error that matches `core.ErrInvalid` (use `errors.Is`);
  - `ListIssues(ctx, core.IssueFilter{Page: core.Page{Limit: 201}})` returns an error that matches `core.ErrInvalid`;
  - `ListCommentsPage` on `NOPE-1` returns an error that matches `core.ErrNotFound`.
  Use `errors.Is`, and check the numbers of returned comments, not their text.

## Out of scope
The stores (`internal/store/...`), the transports, `web`, `ARCHITECTURE.md`, and the event methods. Do not change `ListComments` or the `Store` interface.
```

---

## Epic 2: Store paging

When this epic is done, both stores return exactly the window that `IssueFilter.Limit` and `IssueFilter.Offset` describe, and the shared storetest suite checks it.

### 2.1 Apply the page in the memory and SQLite stores

```
Epic: 2 Store paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Stores hold no rules: they apply the window they are given.
  - A schema change is a new entry appended to `migrations`. This issue makes no schema change, so do not touch `migrations`.
- Other workers are editing the stores at the same time for the event log. Edit only the `ListIssues` function in each store and the new subtest in `storetest.go`. Leave the event methods (`EventsSince`, `LastEventID`, `recordEvent` and the `record` method in memory) as they are.
- Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
`Store.ListIssues` returns only the requested window of the matching issues, in the same order as before, in both stores. The shared contract suite proves it, so any future store has to do the same.

## Read first
- `internal/core/types.go`: `IssueFilter`. It now embeds `Page`, so `f.Limit` and `f.Offset` are the fields to read. `Page` is in `internal/core/page.go`.
- `internal/store/memory/memory.go`: `ListIssues`. It filters, sorts by project key and number, and returns `out`, which starts as `[]core.Issue{}`.
- `internal/store/sqlite/sqlite.go`: `ListIssues`. It builds `query` with `ORDER BY project_key, number` at the end and runs it with `args`.
- `internal/store/storetest/storetest.go`: the `list filters and orders` subtest, as the model for the new one. `seed` and `newIssue` are defined at the top of `Run`.

## Change
- `internal/store/memory/memory.go`: in `ListIssues`, after the sort, apply the window (see Contract).
- `internal/store/sqlite/sqlite.go`: in `ListIssues`, add `LIMIT` and `OFFSET` to the query (see Contract).
- `internal/store/storetest/storetest.go`: add a `t.Run("paging", ...)` subtest inside `Run`, after the `list filters and orders` subtest.

## Contract
Semantics both stores must follow: `Limit` 0 means no limit. `Offset` is how many matches to skip. The window is applied after filtering and after sorting. The result is never nil.

memory.go, after `slices.SortFunc(...)` in `ListIssues`:

```go
	if f.Offset >= len(out) {
		out = out[:0]
	} else {
		out = out[f.Offset:]
	}
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
```
Replace the existing `return out, nil` at the end of `ListIssues` with this block. `out[:0]` keeps the slice non-nil.

sqlite.go, in `ListIssues`, keep the existing `query += ` ORDER BY project_key, number`` line and add this directly after it:

```go
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	} else if f.Offset > 0 {
		// SQLite accepts OFFSET only after LIMIT, so "no limit" is LIMIT -1.
		query += ` LIMIT -1`
	}
	if f.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, f.Offset)
	}
```
Keep the `rows, err := s.db.QueryContext(ctx, query, args...)` line after it. The `out := []core.Issue{}` line already makes the result non-nil.

storetest.go, new subtest `paging`. Use `seed(t)` and `newIssue`. Create five issues in project `AAA` (`AAA-1` to `AAA-5`) with `CreateIssue`, then check with `ListIssues` that these results come back, in this order, comparing IDs:
- `{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 2}}` gives `AAA-3, AAA-4`
- `{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 4}}` gives `AAA-5`
- `{ProjectKey: "AAA", Page: core.Page{Offset: 4}}` gives `AAA-5` (offset without limit)
- `{ProjectKey: "AAA", Page: core.Page{Limit: 2}}` gives `AAA-1, AAA-2`
- `{ProjectKey: "AAA"}` gives all five
- `{ProjectKey: "AAA", Page: core.Page{Limit: 2, Offset: 9}}` gives an empty, non-nil slice
- `{ProjectKey: "AAA", Status: core.StatusDone, Page: core.Page{Limit: 1, Offset: 1}}` gives an empty slice, because no issue is done. Then set `AAA-2` to done with `UpdateIssue` and check that the same query with `Status: core.StatusDone` and `Page{Limit: 1, Offset: 0}` gives `AAA-2`. This proves the window is applied after the filter.

Use the same comparison style as the `list filters and orders` subtest. Name the helper's output in failure messages.

## Done when
- `go vet ./internal/store/...` succeeds and `gofmt -l internal/store` prints nothing.
- `go test ./internal/store/...` passes. Both `memory` and `sqlite` run the `paging` subtest through `storetest.Run`.

## Out of scope
The event methods and the `events` subtest, `migrations`, `core` (`types.go`, `service.go`, `page.go`), the transports, and `ARCHITECTURE.md`. Do not change `ListComments`.
```

---

## Epic 3: Expose paging

When this epic is done, every list endpoint takes paging, the web UI shows every issue and comment, and `ARCHITECTURE.md` describes the behaviour. The five issues are independent and can run at once.

### 3.1 Take limit and offset in the REST list endpoints

```
Epic: 3 Expose paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports validate nothing beyond decoding their own input. A non-integer query value is decoding; a value outside the bounds is validated by core, so pass it through.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - Every use case is exposed in REST, CLI and MCP together. This issue is the REST part.
- Other workers are editing the CLI, MCP and web packages at the same time. Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
`GET /issues` and `GET /issues/{id}/comments` accept `limit` and `offset` query parameters and return one page. Out-of-range values answer 400.

## Read first
- `internal/transport/rest/rest.go`: `listIssues` (it builds a `core.IssueFilter` from query values), `listComments` (it calls `h.svc.ListComments`), and `respond`, which maps `core.ErrInvalid` to 400.
- `internal/transport/rest/rest_test.go`: the `do` helper and `TestREST`, which creates `PLAT-1` and more issues. Read the `GET /issues` and comments steps to see the style.
- `internal/core/page.go`: `Page`, `DefaultPageLimit`, `MaxPageLimit`.
- `internal/core/service.go`: `ListCommentsPage(ctx, issueID, Page)`.

## Change
- `internal/transport/rest/rest.go`:
  - Add a helper `pageFrom(r *http.Request) (core.Page, error)` that reads `limit` and `offset` from `r.URL.Query()`. An empty value leaves the field 0. A value that `strconv.Atoi` cannot parse returns `fmt.Errorf("%w: %s must be an integer", core.ErrInvalid, name)`, with `name` being `limit` or `offset`. Import `fmt` and `strconv` as needed.
  - In `listIssues`, call `pageFrom` first. On error call `h.respond(w, http.StatusOK, nil, err)` and return. Otherwise add `Page: page` to the `core.IssueFilter` literal.
  - In `listComments`, call `pageFrom` the same way, then `h.svc.ListCommentsPage(r.Context(), r.PathValue("id"), page)`.
  - Register nothing new and change no other handler.
- `internal/transport/rest/rest_test.go`: add a new test function `TestRESTPaging`. Do not edit `TestREST`.

## Contract
Routes and query parameters:
- `GET /issues?project=&stage=&status=&assignee=&limit=&offset=`: the existing filters plus paging. Returns a JSON array.
- `GET /issues/{id}/comments?limit=&offset=`: returns a JSON array.
- Absent `limit` or `limit=0` means 50. `limit` above 200, `limit` below 0 and `offset` below 0 answer `400` with `{"error": "invalid input: ..."}`.
- `limit=abc` answers `400` with `{"error": "invalid input: limit must be an integer"}`.
- Responses are bare arrays, as today. No count or next-page field is added.

## Done when
- `go vet ./internal/transport/rest/` succeeds and `gofmt -l internal/transport/rest` prints nothing.
- `go test ./internal/transport/rest/` passes.
- `TestRESTPaging` creates three issues in `PLAT` and three comments on `PLAT-1`, through the `do` helper, and checks:
  - `GET /issues?limit=2&offset=1` returns `PLAT-2` and `PLAT-3`;
  - `GET /issues` with no paging returns all three;
  - `GET /issues/PLAT-1/comments?limit=1&offset=1` returns only comment number 2;
  - `GET /issues?limit=201` and `GET /issues?offset=-1` answer 400;
  - `GET /issues?limit=abc` answers 400;
  - `GET /issues/PLAT-1/comments?limit=500` answers 400.

## Out of scope
The CLI, MCP and web packages, `core`, the stores, `ARCHITECTURE.md`. Do not change `TestREST`.
```

### 3.2 Add -limit and -offset to the CLI list commands

```
Epic: 3 Expose paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports validate nothing beyond decoding their own input. A non-integer flag is decoding; a value outside the bounds is validated by core, so pass it through.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - Every use case is exposed in REST, CLI and MCP together. This issue is the CLI part.
- Other workers are editing the REST, MCP and web packages at the same time. Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
`codeplat issue list` and `codeplat issue comments` accept `-limit N` and `-offset N` and print one page.

## Read first
- `internal/transport/cli/cli.go`: the `usage` constant, `issueList` (it fills a `core.IssueFilter` with flags on `c.flagSet`), `issueComments` (it currently accepts exactly one argument and calls `c.svc.ListComments`), and the helpers `flagSet` and `parse`. `parse` returns `errUsage` on a bad flag or stray argument, and `Run` maps `errUsage` to exit code 2.
- `internal/transport/cli/cli_test.go`: the `steps` table. Each step has args, exit code, expected stdout and expected stderr.
- `internal/core/page.go`: `Page`.

## Change
- `internal/transport/cli/cli.go`:
  - `issueList`: register `fs.IntVar(&f.Limit, "limit", 0, "at most this many issues (default 50, max 200)")` and `fs.IntVar(&f.Offset, "offset", 0, "skip this many issues")`. `f` is a `core.IssueFilter`, which embeds `core.Page`, so `f.Limit` and `f.Offset` are promoted fields.
  - `issueComments`: change it to take `ID` first and then flags. If `len(args) < 1`, return `errUsage`. Create `var p core.Page`, a flag set named `issue comments` with `fs.IntVar(&p.Limit, ...)` and `fs.IntVar(&p.Offset, ...)`, and `parse(fs, args[1:])`. Then call `c.svc.ListCommentsPage(ctx, args[0], p)`. Keep the output format exactly as it is.
  - `usage`: update the two lines for `issue list` and `issue comments` to show `[-limit N] [-offset N]` and `issue comments ID [-limit N] [-offset N]`. Add one line under the existing STAGE and STATUS lines: `limit defaults to 50 and may be at most 200; offset skips that many`.
- `internal/transport/cli/cli_test.go`: add a new test function `TestCLIPaging` that calls `cli.Run` with its own `apptest.New(t).Service`. Do not edit `TestCLI`.

## Contract
- `codeplat issue list [-project KEY] [-stage STAGE] [-status STATUS] [-assignee NAME] [-limit N] [-offset N]`
- `codeplat issue comments ID [-limit N] [-offset N]`
- An out-of-range value (limit above 200, limit or offset below 0) makes `Run` return exit code 1 with `error: invalid input: ...` on stderr, the same as other validation errors.
- A non-integer value, or an unknown flag, makes `Run` return exit code 2 with the usage text, the same as other flag errors.
- Output format for both commands is unchanged.

## Done when
- `go vet ./internal/transport/cli/` succeeds and `gofmt -l internal/transport/cli` prints nothing.
- `go test ./internal/transport/cli/` passes.
- `TestCLIPaging` creates three issues in `PLAT` (through `cli.Run` with `project create` and `issue create`) and three comments on `PLAT-1`, then checks:
  - `issue list -limit 1 -offset 1` prints `PLAT-2` and not `PLAT-1` or `PLAT-3`, and exits 0;
  - `issue list -limit 500` exits 1 and stderr contains `invalid input`;
  - `issue comments PLAT-1 -limit 1 -offset 2` prints `#3` and not `#1`, and exits 0;
  - `issue comments` with no ID exits 2;
  - `issue comments PLAT-1 -limit abc` exits 2.

## Out of scope
The REST, MCP and web packages, `core`, the stores, `ARCHITECTURE.md`. Do not change `TestCLI` or the `printIssue` function.
```

### 3.3 Add limit and offset to the MCP list tools

```
Epic: 3 Expose paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports validate nothing beyond decoding their own input. Do not add a range check to the MCP schema or the handler; core checks the bounds.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - Every use case is exposed in REST, CLI and MCP together. This issue is the MCP part.
- Other workers are editing the REST, CLI and web packages at the same time. Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
The MCP tools `issue_list` and `comment_list` accept `limit` and `offset` arguments, advertise them in their schemas, and return one page.

## Read first
- `internal/transport/mcp/tools.go`: the `tools` function. Read the `issue_list` and `comment_list` entries, the `typed` helper at the bottom (it decodes arguments with `DisallowUnknownFields`), and the `stageSchema` and `statusSchema` constants.
- `internal/transport/mcp/mcp_test.go`: `TestMCP`, which runs a scripted JSON-RPC session over `mcp.NewServer(...)` and checks replies. Read how it reads `reply.Result.Content[0].Text` and `reply.Result.IsError`.
- `internal/core/page.go`: `Page`, and its JSON tags `limit` and `offset`.

## Change
- `internal/transport/mcp/tools.go`:
  - `issue_list`: its handler already decodes into `core.IssueFilter`. `IssueFilter` embeds `core.Page`, whose JSON fields are `limit` and `offset`, so the decoder accepts them with no change to the handler. Add the two properties to the `inputSchema`: `"limit":{"type":"integer","minimum":1,"maximum":200,"description":"at most this many issues; default 50"}` and `"offset":{"type":"integer","minimum":0,"description":"skip this many issues"}`. Update its `Description` to say that it returns 50 at a time by default and that `limit` (at most 200) and `offset` read further pages.
  - `comment_list`: change the argument struct in the handler to `struct { IssueID string `json:"issue_id"`; core.Page }`, and call `svc.ListCommentsPage(ctx, in.IssueID, in.Page)`. Add the same two properties to its `inputSchema`, and say in its `Description` that it returns 50 at a time by default.
- `internal/transport/mcp/mcp_test.go`: add a new test function `TestMCPPaging` that builds its own server with `apptest.New(t)` and drives `tools/call` requests. Do not edit `TestMCP`.

## Contract
- `issue_list` arguments: `project_key`, `stage`, `status`, `assignee`, `limit`, `offset`, all optional.
- `comment_list` arguments: `issue_id` (required), `limit`, `offset`.
- A value outside the bounds returns a tool result with `isError: true` and the text `invalid input: ...`. This comes from core, so the handler does not check it.
- A result is a JSON array in the single `text` content item, as today.

## Done when
- `go vet ./internal/transport/mcp/` succeeds and `gofmt -l internal/transport/mcp` prints nothing.
- `go test ./internal/transport/mcp/` passes.
- `TestMCPPaging` creates a project `PLAT` and three issues, and three comments on `PLAT-1`, then checks:
  - `issue_list` with `{"limit":1,"offset":1}` returns text containing `PLAT-2` and not `PLAT-1` or `PLAT-3`;
  - `comment_list` with `{"issue_id":"PLAT-1","limit":1,"offset":1}` returns only comment number 2;
  - `issue_list` with `{"limit":201}` returns `isError` true;
  - `tools/list` shows `limit` and `offset` in the schemas of `issue_list` and `comment_list`.

## Out of scope
The REST, CLI and web packages, `core`, the stores, `ARCHITECTURE.md`, and the other MCP tools. Do not change `typed`, `mcp.go` or `TestMCP`.
```

### 3.4 Make the web board and issue page read every page

```
Epic: 3 Expose paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. The web UI is read-only and validates nothing.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- Other workers are editing the REST, CLI and MCP packages at the same time. Create or edit only the files this issue lists.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.
- Use the Go standard library only. Do not edit `go.mod`.

## Goal
The web board and the issue page still show every issue and every comment, now that the service returns pages. They do this by reading each page in turn.

## Read first
- `internal/transport/web/web.go`: `project` (it calls `h.svc.ListIssues` with `core.IssueFilter{ProjectKey: key}`), and `issue` (it calls `h.svc.ListComments`). Also the `core` import and the `page` struct.
- `internal/transport/web/web_test.go`: `TestWeb`, the `get` helper, and how it creates issues through `a.Service`.
- `internal/core/page.go`: `MaxPageLimit`.
- `internal/core/service.go`: `ListCommentsPage`.

## Change
- `internal/transport/web/web.go`:
  - Add a generic helper `all` (signature below) to the bottom of the file.
  - In `project`, replace the `ListIssues` call with `issues, err := all(func(p core.Page) ([]core.Issue, error) { return h.svc.ListIssues(r.Context(), core.IssueFilter{ProjectKey: key, Page: p}) })`. Keep the error handling after it.
  - In `issue`, replace the `ListComments` call with `comments, err := all(func(p core.Page) ([]core.Comment, error) { return h.svc.ListCommentsPage(r.Context(), issue.ID, p) })`. Keep the error handling after it.
- `internal/transport/web/web_test.go`: add a new test function `TestWebPaging`. Do not edit `TestWeb`.

## Contract
```go
// all reads every page of a list. fetch is called with pages of
// core.MaxPageLimit items at increasing offsets. It stops at the first page
// that is shorter than the limit, and returns everything it read.
func all[T any](fetch func(core.Page) ([]T, error)) ([]T, error) {
	var out []T
	for offset := 0; ; offset += core.MaxPageLimit {
		items, err := fetch(core.Page{Limit: core.MaxPageLimit, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) < core.MaxPageLimit {
			return out, nil
		}
	}
}
```
The page template is not changed.

## Done when
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `go test ./internal/transport/web/` passes.
- `TestWebPaging` creates 201 issues in project `AAA` (through `a.Service.CreateIssue`, titles `issue 1` to `issue 201`) and one issue `AAA-1` with 201 comments (bodies `comment 1` to `comment 201`, author `test`). Then it checks:
  - `GET /projects/AAA` contains `AAA-201` and `AAA-1`;
  - `GET /issues/AAA-1` contains `comment 201` and `comment 1`;
  - `GET /issues/AAA-1` shows `Comments (201)`.

## Out of scope
The REST, CLI and MCP packages, `core`, the stores, `page.html`, `ARCHITECTURE.md`. Do not change `TestWeb`, the `page` struct or the template. If `web.go` already has a `NewHandler(ctx, ...)` signature from another change, keep it and change only the two calls named above.
```

### 3.5 Document paging in ARCHITECTURE.md

```
Epic: 3 Expose paging
Depends on: none

## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- These rules of this repository apply to you: `ARCHITECTURE.md` is the binding description of the design. Change only what this issue names.
- Other workers are editing code and other documents at the same time. Edit only `ARCHITECTURE.md`.
- Do not run `make build` (other workers use the binaries in `bin/`) and do not run `git commit`.

## Goal
`ARCHITECTURE.md` describes how list paging works, so the next person does not have to find `core.Page` and the store rule by reading code.

## Read first
- `ARCHITECTURE.md`: the "Decisions" list, and its "Not built yet" bullet.

## Change
- `ARCHITECTURE.md`: add one bullet to "Decisions", placed directly after the "Claiming" bullet. Remove the word `pagination,` from the "Not built yet" bullet and leave the rest of that bullet as it is.

## Contract
The new bullet, verbatim:

- **Paging:** issue and comment lists take `limit` and `offset` (`core.Page`). The default limit is 50 and the maximum is 200, and the service enforces both. Stores apply the window to `ListIssues` in SQL or memory, and treat limit 0 as no limit. Comments are sliced in the service. Lists stay bare JSON arrays with no total, so a short page means the end. The web UI reads every page.

Note: `docs/roadmaps/live-updates.md` (issue 3.2) also edits the "Not built yet" bullet, to add `pruning the event log`. If that edit is already in the file, keep it, and remove only `pagination,`.

## Done when
- `grep -c "core.Page" ARCHITECTURE.md` prints 1.
- `grep -c "pagination," ARCHITECTURE.md` prints 0.
- `grep -c "Not built yet" ARCHITECTURE.md` prints 1.
- No tests are needed.

## Out of scope
Any Go file, `README.md`, `CLAUDE.md`, and the other bullets of ARCHITECTURE.md.
```

---

## Coordination notes

- **Event log (PLAT-7).** Epic 1 edits `IssueFilter` in `types.go` and adds `ListCommentsPage` beside `ListComments` in `service.go`. Epic 2 edits only the `ListIssues` functions and the storetest suite. None of these touch `EventsSince`, `LastEventID`, `recordEvent`, `migrations`, or the `Store` interface. Each issue names the anchors it edits, so they can be applied to the event log's state as it is when they run.
- **Live updates roadmap** (`docs/roadmaps/live-updates.md`). Its issue 3.1 edits `web.go`, `web_test.go` and `page.html`. Issue 3.4 here edits `web.go` and `web_test.go` too. Run 3.4 after 3.1 lands, or re-read those files first. Its issue 3.2 and this roadmap's 3.5 both edit the "Not built yet" bullet of `ARCHITECTURE.md`. Whichever lands second keeps the first edit.
- **Other planned roadmaps** (epics and dependencies, a worker runner, a blocked state, authentication) will likely edit `rest.go`, `cli.go`, `tools.go` and `web.go` too. Re-check these files before recording epic 3.

## Check

- **Files each issue changes (no collisions inside any epic):**
  - 1.1: `internal/core/page.go` (new), `internal/core/types.go`, `internal/core/service.go`, `internal/core/service_test.go`.
  - 2.1: `internal/store/memory/memory.go`, `internal/store/sqlite/sqlite.go`, `internal/store/storetest/storetest.go`.
  - 3.1: `internal/transport/rest/rest.go`, `internal/transport/rest/rest_test.go`.
  - 3.2: `internal/transport/cli/cli.go`, `internal/transport/cli/cli_test.go`.
  - 3.3: `internal/transport/mcp/tools.go`, `internal/transport/mcp/mcp_test.go`.
  - 3.4: `internal/transport/web/web.go`, `internal/transport/web/web_test.go`.
  - 3.5: `ARCHITECTURE.md`.
- **Packages each "Done when" compiles.** Epic 1 tests import `apptest`, which imports `app` and the two stores. None of them is edited in epic 1, so they compile. Epic 3 packages each import only `core` and `apptest`, and no epic-3 package imports another, so the five can run together.
- **Ports and temp paths.** The tests use the memory store through `apptest` and `httptest`, so no fixed port or shared file is used.
- **Could each issue start now, if the earlier epics were done?** Yes. Each one needs only the contract from the epic before it, which the roadmap fixes in text.
- **Adjacent epics.** Epics 1 and 2 cannot merge without a seven-file issue (see the Shape notes). Epics 2 and 3 cannot run together because the transports' paging tests need the stores to page.
- **Dependencies.** None declared. Epics are ordered, so epic 2 follows epic 1 and epic 3 follows epic 2.

## Record the plan

1. Roadmap written to `docs/roadmaps/pagination.md`. The first epic's issue is 1.1; no tracker issues are created.
