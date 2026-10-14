# Roadmap: a blocked state for issues a worker cannot finish

## Assumptions

These were chosen without asking. Change them here before the issues are created if any is wrong.

1. **Blocked is a fourth status, `blocked`.** It is not a boolean and not a stage. A blocked issue keeps its stage, so the board and `issue list -stage` still say which loop it was in.
2. **The reason is a comment, not a field.** `core.Service.BlockIssue` adds a comment whose body starts `BLOCKED:`. That is the convention workers already use, so the reason stays where workers already read it. No `issues` column is added.
3. **No schema migration.** The `status` column is plain `TEXT NOT NULL` with no `CHECK`, so the SQLite store accepts `blocked` without a change. Nothing is appended to `migrations`.
4. **A blocked issue keeps its assignee.** It is therefore not claimable, because `ClaimIssue` takes only `todo`, and it is not in any loop's queue, because queues are `status todo`. This is the "nobody else takes it" property the convention had, now enforced by the state.
5. **Blocking needs an author and a reason.** Both are required (`ErrInvalid`). A `done` issue cannot be blocked (`ErrConflict`). Blocking an already blocked issue is allowed and adds another comment.
6. **Unblocking is not a new use case.** A person or worker uses `issue update` with a status: `-status in_progress` to resume with the same assignee, or `-status todo -assignee ""` to put it back in the queue. Transitions are not enforced, as today, so a direct `issue update -status blocked` also works, but it records no reason. The worker prompts say to use `issue block`.
7. **The board shows blocked issues in their stage column with a red badge.** No new column. Only the badge is added to the web UI.
8. **This roadmap starts after the event log (PLAT-7) is merged.** Epic 1 edits `core/types.go`, `core/service.go`, `core/service_test.go` and `store/storetest/storetest.go`, which that work is also editing. Each issue assumes the state it describes: `core.Store` has `EventsSince` and `LastEventID`, both stores record events, and the `events` migration exists. `BlockIssue` needs no event code of its own, because it goes through `UpdateIssue` and `AddComment`, which already record events.
9. **Other roadmaps touch some of the same files.** The live-updates roadmap edits `internal/transport/web/page.html`, `web.go` and `web_test.go`. Issue 2.4 edits `page.html` and `web_test.go` too. If both roadmaps run, the one that starts second must read the file as it is then and change only the lines its issue names.
10. **A person runs `make build` between epic 2 and epic 3.** Epic 3 changes the worker prompts, which tell workers to run `bin/codeplat issue block`. That command exists only in a rebuilt binary, so the prompts must not go live first. The workers in this plan do not run `make build`.
11. **Existing `BLOCKED:` comments are left alone.** Issues already in the tracker that carry the old convention, PLAT-4 among them, keep their comments and their status. Someone may re-block them with the new command if they want the state.

## Goal

A worker that cannot finish an issue sets it to a real state, `blocked`, with one command that records the reason. The issue keeps its assignee so nobody else takes it, it leaves every queue, and it can be listed with `issue list -status blocked` or `GET /issues?status=blocked`. The web board shows it with a red badge.

Tracker project: `PLAT`.

## Design

```
worker                          core.Service                         stores
------                          ------------                         ------
issue block PLAT-4              BlockIssue(id, author, reason)
  -author N -reason R   --->      UpdateIssue: status = blocked  -->  one row changed, event recorded
                                  AddComment "BLOCKED: R" as N  -->  one comment, event recorded
                                  returns the issue

queue for a loop = (stage, todo)   blocked issues are in neither a queue nor ClaimIssue's todo set
```

- **Status.** `blocked` joins `todo`, `in_progress` and `done` in `core.Status`. `Valid` accepts it, so every existing path that takes a status (`issue update`, `issue list -status`, REST `PATCH` and `GET`, MCP `issue_update` and `issue_list`) accepts it with no further change, except the MCP enum schema and CLI help text, which list the values.
- **Use case.** `Service.BlockIssue` is the one place that applies the rules. The transports only decode input and pass it on.
- **Reason.** Stored as a comment. The status change and the comment are two store calls. The status is changed first, so an unknown issue fails before any comment is written. If the comment write fails after the status change, the error is returned and the issue stays blocked; a retry adds the comment.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Core | `core` accepts the status and `BlockIssue` enforces its rules in both stores. | 1, alone |
| 2. Expose | REST, CLI, MCP and the web board each expose or show the blocked state, and their tests pass. | 4, parallel |
| 3. Workers and docs | The three worker prompts and `ARCHITECTURE.md` describe the blocked state. | 2, parallel |

No issue has a dependency inside its epic.

Why three epics: the transports call `Service.BlockIssue` and compare against `core.StatusBlocked`, so they do not compile until epic 1 has added both (epic 2). The worker prompts name a CLI command that exists only in a binary built from epic 2, so they change only after that (epic 3).

Why epic 1 is one issue: the status constant, the use case and the store tests all sit in `core` and the two store packages, and `storetest` imports `core`. Splitting them would make the store-test issue wait for the core issue, so they are one issue (technique 4).

Why epic 2 is parallel: each issue owns one package, and no two packages import each other. `rest`, `cli` and `mcp` import only `core`, and `web` imports only `core`. Each issue's "Done when" compiles only its own package and `core`, which is finished in epic 1. The MCP enum change and the CLI usage text are in the files those issues own, so nothing is shared.

Why epic 3 is parallel: the prompts and `ARCHITECTURE.md` are different files and neither compiles.

---

## Epic 1: Core

```
## Environment (epic 1 and 2, for every issue)
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - A database schema change is a new entry appended to `migrations` in `internal/store/sqlite/sqlite.go`. This work needs none.
- Other workers are editing other files at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when". A repository-wide `make test` or `go build ./...` may fail because of someone else's half-finished file.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 1.1 Add the blocked status and BlockIssue

```
Epic: 1 Core
Depends on: none

## Environment
(the Environment section above, pasted in full)

## Goal
An issue can have the status `blocked`. `Service.BlockIssue` sets it, keeps the assignee, and records the reason as a comment, so a worker that cannot finish leaves a state the tracker can list.

## Read first
- `internal/core/types.go`: the `Status` constants and `Valid`, and the `Store` interface.
- `internal/core/service.go`: `ClaimIssue` (the pattern to follow), `AddComment`, and `invalidStatus` at the bottom.
- `internal/store/storetest/storetest.go`: the `update` subtest and the `list filters and orders` subtest, as models for a new subtest.
- `internal/core/service_test.go`: the top of `TestServiceRules`, for the style and the fixed clock.

## Change
- `internal/core/types.go`: add the `StatusBlocked` constant and make `Valid` accept it, exactly as under Contract.
- `internal/core/service.go`: add `BlockIssue` exactly as under Contract, and change the text of `invalidStatus` to list four values.
- `internal/store/storetest/storetest.go` (tests): add a subtest `blocked status` inside `Run`, as described under Contract.
- `internal/core/service_test.go` (tests): add a test `TestBlockIssue`, as described under Contract.

Do not change either store implementation. The status column has no `CHECK` constraint, so `blocked` is stored as it is.

## Contract
In `internal/core/types.go`, the status constants become:

const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusDone       Status = "done"
	// StatusBlocked marks an issue its assignee cannot finish. It keeps the
	// assignee, so no other worker claims it, and it is in no loop's queue.
	StatusBlocked Status = "blocked"
)

and `Valid` returns true for all four:

func (s Status) Valid() bool {
	return s == StatusTodo || s == StatusInProgress || s == StatusDone || s == StatusBlocked
}

In `internal/core/service.go`, add this method after `ClaimIssue`:

// BlockIssue marks an issue the author cannot finish as blocked. The issue
// keeps its assignee and stage, so it is not claimable and leaves every
// queue. The reason is recorded as a comment whose body starts "BLOCKED: ".
// Author and reason are required. A done issue cannot be blocked.
func (s *Service) BlockIssue(ctx context.Context, id, author, reason string) (Issue, error) {
	author = strings.TrimSpace(author)
	reason = strings.TrimSpace(reason)
	if author == "" {
		return Issue{}, invalid("author is required to block an issue")
	}
	if reason == "" {
		return Issue{}, invalid("reason is required to block an issue")
	}
	issue, err := s.store.UpdateIssue(ctx, normalizeKey(id), func(i *Issue) error {
		if i.Status == StatusDone {
			return fmt.Errorf("issue %s is done and cannot be blocked: %w", i.ID, ErrConflict)
		}
		i.Status = StatusBlocked
		i.UpdatedAt = s.now().UTC()
		return nil
	})
	if err != nil {
		return Issue{}, err
	}
	if _, err := s.store.AddComment(ctx, Comment{
		IssueID:   issue.ID,
		Author:    author,
		Body:      "BLOCKED: " + reason,
		CreatedAt: s.now().UTC(),
	}); err != nil {
		return Issue{}, err
	}
	return issue, nil
}

Change `invalidStatus` so its message lists all four values, in this form:
	return invalid(fmt.Sprintf("status %q must be one of %s, %s, %s, %s", s, StatusTodo, StatusInProgress, StatusDone, StatusBlocked))

Signature of the new method, which the transports call: `func (s *Service) BlockIssue(ctx context.Context, id, author, reason string) (core.Issue, error)` (in package `core`, `(Issue, error)`).

## Done when
- `go vet ./internal/core/ ./internal/store/... ./internal/app/ ./internal/apptest/` succeeds.
- `gofmt -l internal/core internal/store` prints nothing.
- `go test -race ./internal/core/ ./internal/store/...` passes.
- `blocked status` in `storetest.go` covers, for both stores (it runs automatically through each store's test calling `storetest.Run`):
  - an issue with assignee `sam` and stage `testing` is set to `StatusBlocked` with `UpdateIssue`; `GetIssue` returns it with status `blocked`, assignee `sam` and stage `testing`;
  - `ListIssues` with `Status: core.StatusBlocked` returns that issue and no other.
- `TestBlockIssue` in `service_test.go`, using `apptest.New(t).Service`, covers:
  - creating project `PLAT` and an issue `PLAT-1` with assignee `sam`, then `BlockIssue(ctx, "plat-1", "sam", " needs a spec ")` returns status `blocked`, assignee `sam`, stage unchanged;
  - `ListComments("PLAT-1")` then has one comment with Author `sam` and Body `BLOCKED: needs a spec`;
  - `BlockIssue` with an empty author, or an empty or blank reason, returns `ErrInvalid` and adds no comment;
  - `ClaimIssue` on the blocked issue returns `ErrConflict`;
  - after `UpdateIssue` sets the issue to `done`, `BlockIssue` returns `ErrConflict` and adds no comment;
  - `BlockIssue` on `PLAT-99` returns `ErrNotFound`.
  Use the existing test's project setup, not a new one.

## Out of scope
The transports, the web UI, the worker prompts, `ARCHITECTURE.md`, both store implementations and `migrations`. Any file not listed under Change.
```

---

## Epic 2: Expose and show

```
## Environment
(the Environment section above, pasted in full)
Re-check against the code when epic 1 is done: `core.StatusBlocked`, `Service.BlockIssue` and the text of `invalidStatus`.
```

### 2.1 Expose BlockIssue over REST

```
Epic: 2 Expose
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
`POST /issues/{id}/block` marks an issue blocked, with the author and reason in the body, and returns the issue.

## Read first
- `internal/transport/rest/rest.go`: `NewHandler` (the route list), `claimIssue` (the pattern to copy) and `respond`.
- `internal/transport/rest/rest_test.go`: the `do` helper and the claim checks, as the style for new checks.

## Change
- `internal/transport/rest/rest.go`: add one route and one handler, as under Contract.
- `internal/transport/rest/rest_test.go` (tests): add checks, as under Done when.

## Contract
In `NewHandler`, add directly after the `POST /issues/{id}/claim` line:

	mux.HandleFunc("POST /issues/{id}/block", h.blockIssue)

Add this handler, after `claimIssue`:

func (h *handler) blockIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author string `json:"author"`
		Reason string `json:"reason"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	i, err := h.svc.BlockIssue(r.Context(), r.PathValue("id"), body.Author, body.Reason)
	h.respond(w, http.StatusOK, i, err)
}

The request body is `{"author":"NAME","reason":"TEXT"}`. The response is the issue, status 200. Errors map through `respond`: empty author or reason is 400, a missing issue is 404, a done issue is 409.

## Done when
- `go vet ./internal/transport/rest/` succeeds and `gofmt -l internal/transport/rest` prints nothing.
- `go test -race ./internal/transport/rest/` passes. The testing worker adds checks to `rest_test.go` that cover:
  - `POST /issues/PLAT-1/block` with `{"author":"sam","reason":"no spec"}` returns 200, status `blocked`, assignee `sam`;
  - the same call with `"reason":""` returns 400, and with a missing issue returns 404;
  - `GET /issues?status=blocked` then lists PLAT-1 only.

## Out of scope
The CLI, MCP and web packages, `core`, the stores. Any file outside `internal/transport/rest/`.
```

### 2.2 Expose BlockIssue as a CLI subcommand

```
Epic: 2 Expose
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
`codeplat issue block ID -author NAME -reason TEXT` marks an issue blocked and prints one line.

## Read first
- `internal/transport/cli/cli.go`: the `usage` constant, the `dispatch` switch, `issueClaim` (the pattern to copy) and `parse`.
- `internal/transport/cli/cli_test.go`: the table of cases and how it checks output, exit code and error text.

## Change
- `internal/transport/cli/cli.go`: add one usage line, change the STATUS line of the usage text, add one dispatch case and one method, as under Contract.
- `internal/transport/cli/cli_test.go` (tests): add table cases, as under Done when.

## Contract
In the `usage` constant, add this line after the `issue claim` line:

  codeplat issue block ID -author NAME -reason TEXT

and change the STATUS line to:

STATUS is one of: todo, in_progress, done, blocked

In `dispatch`, add a case directly after `"issue claim"`:

	case "issue block":
		return c.issueBlock(ctx, rest)

Add this method after `issueClaim`:

func (c *command) issueBlock(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errUsage
	}
	var author, reason string
	fs := c.flagSet("issue block")
	fs.StringVar(&author, "author", "", "who is blocked")
	fs.StringVar(&reason, "reason", "", "why the issue cannot be finished")
	if err := parse(fs, args[1:]); err != nil {
		return err
	}
	i, err := c.svc.BlockIssue(ctx, args[0], author, reason)
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "blocked %s\n", i.ID)
	return nil
}

Output on success is exactly `blocked PLAT-1` and a newline.

## Done when
- `go vet ./internal/transport/cli/` succeeds and `gofmt -l internal/transport/cli` prints nothing.
- `go test -race ./internal/transport/cli/` passes. The testing worker adds table cases to `cli_test.go` that cover:
  - `issue block PLAT-1 -author sam -reason "no spec"` exits 0 and prints `blocked PLAT-1`;
  - `issue show PLAT-1` then prints `Status    blocked` and `Assignee  sam`;
  - `issue block PLAT-1 -author sam` with no reason exits 1 with `invalid input`;
  - `issue block` with no ID exits 2 with `usage:`.

## Out of scope
The REST, MCP and web packages, `core`, the stores, and the wording of any other usage line. Any file outside `internal/transport/cli/`.
```

### 2.3 Expose BlockIssue as an MCP tool and list blocked as a status

```
Epic: 2 Expose
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
An MCP client can call `issue_block`, and the status enums that `issue_list` and `issue_update` advertise include `blocked`.

## Read first
- `internal/transport/mcp/tools.go`: the `statusSchema` constant, the `issue_update` and `issue_claim` entries, and `typed`.
- `internal/transport/mcp/mcp_test.go`: the tool count check near line 98 and the `tools/call` requests near line 43, as the style for new calls.

## Change
- `internal/transport/mcp/tools.go`: change `statusSchema`, add one tool entry after `issue_claim`, and change two descriptions, as under Contract.
- `internal/transport/mcp/mcp_test.go` (tests): update the tool count and add a call, as under Done when.

## Contract
Replace the `statusSchema` constant with:

	statusSchema = `{"type":"string","enum":["todo","in_progress","done","blocked"]}`

In the `issue_update` entry, replace its Description string with:
"Change an issue. Only the fields you pass are changed; pass an empty assignee to unassign. To hand an issue to the next loop, set the next stage, status todo and an empty assignee. To finish it, set status done. To mark an issue you cannot finish, use issue_block instead, which records why."

Add this entry directly after the `issue_claim` entry:

		{
			Name:        "issue_block",
			Description: "Mark an issue you cannot finish as blocked. It keeps its assignee, so no other worker takes it, and it leaves every queue. The reason is recorded as a comment starting BLOCKED:. Use it instead of stopping without a trace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"author":{"type":"string","description":"the worker that is blocked"},
				"reason":{"type":"string","description":"why the issue cannot be finished"}},
				"required":["id","author","reason"]}`),
			call: typed(func(ctx context.Context, in struct {
				ID     string `json:"id"`
				Author string `json:"author"`
				Reason string `json:"reason"`
			}) (any, error) {
				return svc.BlockIssue(ctx, in.ID, in.Author, in.Reason)
			}),
		},

The tool list has ten entries after this change, not nine.

## Done when
- `go vet ./internal/transport/mcp/` succeeds and `gofmt -l internal/transport/mcp` prints nothing.
- `go test -race ./internal/transport/mcp/` passes. The testing worker:
  - changes the tool count check in `mcp_test.go` from 9 to 10;
  - adds a `tools/call` for `issue_block` with `{"id":"PLAT-1","author":"sam","reason":"no spec"}` after the claim, and checks that its reply is an issue with status `blocked` and assignee `sam`;
  - checks that the `issue_update` tool's `inputSchema` enum contains `blocked`.
  If an existing check breaks because the reply count changed, change that count to match, and say so in your comment.

## Out of scope
The REST and CLI packages, `core`, the stores, and the transport's protocol code in `mcp.go`. Any file outside `internal/transport/mcp/`.
```

### 2.4 Show blocked issues on the web board

```
Epic: 2 Expose
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)
Another roadmap (live updates) also edits `page.html` and `web_test.go`. Read both as they are now and change only the lines named here.

## Goal
A blocked issue shows a red status badge on the board and on its page, distinct from todo, in progress and done.

## Read first
- `internal/transport/web/page.html`: the `:root` and dark-mode colour variables at the top of `<style>`, and the `.badge` rules.
- `internal/transport/web/web_test.go`: the `TestWeb` checks, as the style for a new check.

## Change
- `internal/transport/web/page.html`: add one colour variable to each colour block, and one badge rule, as under Contract.
- `internal/transport/web/web_test.go` (tests): add one check, as under Done when.

Do not change `web.go`. The board already puts a blocked issue in its stage column, because `board()` sorts only `done` issues elsewhere.

## Contract
In the first colour block, `:root`, change the line
    --accent: #2f6fde; --todo: #8a94a3; --progress: #c77700; --done: #2a8f5a;
to
    --accent: #2f6fde; --todo: #8a94a3; --progress: #c77700; --done: #2a8f5a; --blocked: #c23b3b;

In the dark-mode block, change the line
      --accent: #6ea0ff; --todo: #7d8794; --progress: #e3a13a; --done: #4fbf85;
to
      --accent: #6ea0ff; --todo: #7d8794; --progress: #e3a13a; --done: #4fbf85; --blocked: #ff7a7a;

Directly after the line `  .badge.done { color: var(--done); }` add:
  .badge.blocked { color: var(--blocked); }

The badge text comes from `{{label .Status}}`, which renders `blocked`, so no template change is needed.

## Done when
- `go vet ./internal/transport/web/` succeeds.
- `go test -race ./internal/transport/web/` passes. The testing worker adds to `TestWeb`: after creating an issue, blocking it with `a.Service.BlockIssue(ctx, id, "sam", "no spec")`, the board page for its project contains `badge blocked` and the issue's ID, and the issue page for it contains `badge blocked`.
- The page still contains `--blocked:` once in each colour block and `.badge.blocked` once (`grep -c "\-\-blocked:" internal/transport/web/page.html` prints 2).

## Out of scope
`web.go`, `feed.go`, the `sse` package, `core`, the stores. Any change to the layout or to other colours. `ARCHITECTURE.md`.
```

---

## Epic 3: Workers and docs

```
## Environment
(the Environment section above, pasted in full)
Epic 3 must not start until a person has run `make build` after epic 2 finished. The prompts it changes tell workers to run `bin/codeplat issue block`, which exists only in that rebuilt binary.
```

### 3.1 Point the worker prompts at the blocked state

```
Epic: 3 Workers and docs
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)
This issue edits prompt files, not Go. Its checks are text searches.

## Goal
The refinement, implementation and testing prompts tell a worker that cannot finish to run `issue block`, not to leave a comment starting `BLOCKED:` and stop.

## Read first
- `prompts/refine.md`: the tracker table and the section "If you cannot make the issue workable".
- `prompts/implement.md`: the tracker table and the section "If you cannot finish".
- `prompts/test.md`: the tracker table and the section "If you cannot get it to pass".

## Change
- `prompts/refine.md`, `prompts/implement.md`, `prompts/test.md`: in each, add one row to the tracker table and replace the "If you cannot" section, as under Contract.

Do not change the steps, the rest of the tables or the "Your final message" sections, except that the final message is named as shown under Contract.

## Contract
Add this row to the tracker table in each of the three files, after the "Leave a comment" row:

| Block it | `bin/codeplat issue block ID -author NAME -reason "TEXT"` |

Replace each "If you cannot ..." section (its heading and body) with this, using the heading "If you cannot finish":

## If you cannot finish

Run `bin/codeplat issue block ID -author NAME -reason "TEXT"`, where TEXT says what is missing or wrong, with the failing command and its output if there is one. The issue then shows as blocked and keeps your name as assignee, so no other worker takes it. Do not hand it on, and do not try a different design.

In each file's "Your final message" section, change the phrase "whether you handed it on or blocked it" (or "closed or blocked it" in `test.md`) so it still reads as a choice, and add: "If you blocked it, give the reason you recorded."

## Done when
- `grep -c "issue block" prompts/refine.md prompts/implement.md prompts/test.md` prints 2, 2 and 2 (one table row and one command in the section).
- `grep -c "BLOCKED:" prompts/refine.md prompts/implement.md prompts/test.md` prints 0, 0 and 0.

## Out of scope
`prompts/decompose.md`, `ARCHITECTURE.md`, every Go file, and `docs/`. Any file outside `prompts/refine.md`, `prompts/implement.md` and `prompts/test.md`.
```

### 3.2 Document the blocked state in ARCHITECTURE.md

```
Epic: 3 Workers and docs
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
`ARCHITECTURE.md` describes the blocked status, so the next person knows that it is a state, that it keeps the assignee, and where the reason goes.

## Read first
- `ARCHITECTURE.md`: the "Stages" bullet under "Decisions" and the "Claiming" bullet that follows it.

## Change
- `ARCHITECTURE.md`: add one sentence to the "Stages" bullet and one bullet after "Claiming", as under Contract.

## Contract
At the end of the "Stages" bullet, after "Transitions are not enforced; any stage and status can be set.", add this sentence:
A status of `blocked` marks an issue its assignee cannot finish; `Service.BlockIssue` sets it and records the reason as a comment starting `BLOCKED:`.

Directly after the "Claiming" bullet, add this bullet:
- **Blocking:** a blocked issue keeps its assignee and stage. It is not claimable, because `ClaimIssue` takes only `todo`, and it is in no queue, because queues are status `todo`. A person resumes it with `issue update` and a status, or requeues it with status `todo` and an empty assignee.

Change nothing else in the file.

## Done when
- `grep -c "BlockIssue" ARCHITECTURE.md` prints 1.
- `grep -c "\*\*Blocking:\*\*" ARCHITECTURE.md` prints 1.
- `grep -c "BLOCKED:" ARCHITECTURE.md` prints 1.

## Out of scope
Any Go file, the prompts, `README.md`, `CLAUDE.md`, and `docs/`.
```

---

## Shape check

| Epic | Issues | Changes (files) |
|---|---|---|
| 1. Core | 1.1 | `internal/core/types.go`, `internal/core/service.go`, `internal/core/service_test.go`, `internal/store/storetest/storetest.go` |
| 2. Expose | 2.1 REST | `internal/transport/rest/rest.go`, `internal/transport/rest/rest_test.go` |
| | 2.2 CLI | `internal/transport/cli/cli.go`, `internal/transport/cli/cli_test.go` |
| | 2.3 MCP | `internal/transport/mcp/tools.go`, `internal/transport/mcp/mcp_test.go` |
| | 2.4 Web | `internal/transport/web/page.html`, `internal/transport/web/web_test.go` |
| 3. Workers and docs | 3.1 Prompts | `prompts/refine.md`, `prompts/implement.md`, `prompts/test.md` |
| | 3.2 Docs | `ARCHITECTURE.md` |

No file appears under two issues in the same epic. The collision with live-updates on `page.html` and `web_test.go` is in Assumption 9.
Package check for epic 2: `rest`, `cli`, `mcp` and `web` import only `core` and `apptest`. `apptest` imports `app`, `config` and the stores, none of which epic 2 edits.
