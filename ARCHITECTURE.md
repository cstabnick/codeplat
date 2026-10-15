# Architecture

## Shape

```
cmd/rest, cmd/cli, cmd/mcp, cmd/web   entrypoints: load config, build the app, run one transport
internal/apptest               the test entrypoint: same wiring, test config
internal/transport/{rest,cli,mcp,web}   translate a protocol to core calls and back
internal/app                   composition root: the only place that reads config to choose implementations
internal/core                  domain types, the Service (all business rules), and the ports it needs
internal/store/{memory,sqlite} Store implementations
internal/bus                   in-process message bus with topics, used by the web UI
cmd/runner                     entrypoint: starts a worker for each ready todo issue in the stage queues
internal/runner                polls each stage's queue and launches workers, within a cap
internal/process               starts an operating-system process for a worker
internal/auth                  core.Authenticator implementations: open, and static tokens
internal/transport/httpauth    token check shared by the REST and web transports
config/                        one JSON file per environment, plus the loader
cmd/nav, internal/codenav      code navigation for agents: a map of a module's exported API, and symbol lookup through gopls
```

Dependencies point inward: transports and stores import `core`; `core` imports nothing of ours.

## Rules

1. **Config chooses, `app.New` wires.** The environment's config file says which store, clock and address to use. `app.New(cfg)` turns that into concrete objects by calling constructors. Nothing else switches on environment or config.
2. **Pure DI.** Dependencies are constructor arguments. No container, no reflection, no globals, no `init()` registration (the SQLite driver import is the one exception).
3. **Entrypoints are thin.** A `main` is: load config, `app.New`, hand `app.Service` to a transport. A new entrypoint is a new transport package plus a ten-line `main`.
4. **Business rules live in `core.Service` only.** Transports validate nothing beyond decoding their own input, so REST, CLI and MCP cannot disagree.
5. **Ports are defined by the consumer.** `core.Store` lives in `core`. A new backend implements it and passes `storetest.Run`.

## Testing

`apptest.New(t)` loads `config/test.json` (in-memory store, fixed clock) and builds the real app through `app.New`. Transport and service tests run against that, so they exercise the production wiring with test dependencies. Store implementations share one contract suite in `internal/store/storetest`.

## Decisions

- **Config format:** JSON, embedded with `go:embed`, so binaries are self-contained and need no parser dependency. Unknown fields are an error.
- **Data location:** `config.FromEnvironment` resolves a relative store path against the data directory, so entrypoints share a database regardless of working directory. A binary in a checkout's `bin/` uses that checkout's `data/` when it exists; this repository commits `data/codeplat.dev.db`, and `sqlite.Store.Close` checkpoints the write-ahead log so that file is current.
- **Storage:** SQLite through `modernc.org/sqlite` (pure Go, no cgo) for dev and prod; an in-memory map for test.
- **Issue IDs:** Jira-style `KEY-n`, numbered per project. The store allocates the number atomically.
- **Updates:** `Store.UpdateIssue` takes a function and runs it inside the store's lock or transaction, so read-modify-write is atomic.
- **MCP:** a small hand-written JSON-RPC server over stdio that supports tools only. Logs go to stderr because stdout is the protocol channel.
- **Web UI:** `internal/transport/web` is a read-only fourth transport: a header with at most six project tabs (the most recently changed, with the selected project replacing the sixth when it is not among them) and a switcher with a filter box that lists every project with its open count; `/projects` is a page that lists every project with its counts; the counts and the order come from reading every issue once per request (`summaries`), so any number of projects stays reachable; a board grouped by goal and epic with a column per stage; and a page per issue. It always renders whole pages; htmx boosts the links, and a live region re-fetches its own URL and selects itself from the response, so there is no fragment-rendering code. It loads htmx from unpkg, so the browser needs internet access.
- **Live updates:** every change a store makes also appends a row to an event log, in the same transaction. The web server reads new rows from that log every 500ms (`feed`), publishes each on an in-process bus under the topics `project.<KEY>` and `issue.<ID>`, and streams a topic to the browser as Server-Sent Events at `/events?topic=...`. A live region re-fetches when its topic fires, and once a minute as a safety net. The log exists because the CLI, the MCP server and the REST server are separate processes: it is how their changes reach a browser connected to the web server.
- **Comments:** numbered per issue, append-only. A comment does not change the issue's `updated_at`.
- **Stages:** an issue has a stage (`refinement`, `implementation`, `testing`) and a status within it (`todo`, `in_progress`, `done`). Each agent loop works one stage, and its queue is that stage with status `todo`, less the issues that are not ready (see Goals, epics and dependencies). A loop hands off by setting the next stage, status `todo` and no assignee. Transitions are not enforced; any stage and status can be set. A status of `blocked` marks an issue its assignee cannot finish; `Service.BlockIssue` sets it and records the reason as a comment starting `BLOCKED:`.
- **Claiming:** `Service.ClaimIssue` moves a `todo` issue to `in_progress` for one assignee inside `Store.UpdateIssue`, so parallel workers cannot take the same issue.
- **Goals, epics and dependencies:** an issue has `parent` (the ID of the goal it is part of, in the same project; goals do not nest), `epic` (a number within its parent, 0 for none) and `depends_on` (issue IDs). An `Epic` record gives an epic of a goal a name. An issue has blockers while an issue of an earlier epic of the same goal, or any issue it depends on, is not `done`, and a goal has its unfinished parts as blockers; `core.BlockedBy` lists them. A stage's queue is `stage`, `todo` and ready: `IssueFilter.Ready` keeps only issues whose `BlockedBy` is empty, and the worker prompts and the runner both list their queue with it. The Service rejects unknown dependencies, self-dependencies and cycles, and a parent that is missing, in another project or itself a part.
- **Auth:** `core.Authenticator` maps a token to a `core.Principal`. `httpauth.Require` runs it on every REST route except `GET /healthz`, and on every web route, and puts the principal in the request context. With a principal, `core.Service` accepts an assignee or comment author only if it is the principal's name or empty, and fills an empty one with the name. Tokens come from `CODEPLAT_TOKENS`, never from a config file. The CLI and MCP have no principal.
- **Paging:** issue and comment lists take `limit` and `offset` (`core.Page`). The default limit is 50 and the maximum is 200, and the service enforces both. Stores apply the window to `ListIssues` in SQL or memory, and treat limit 0 as no limit. Comments are sliced in the service. Lists stay bare JSON arrays with no total, so a short page means the end. The web UI reads every page.
- **Blocking:** a blocked issue keeps its assignee and stage. It is not claimable, because `ClaimIssue` takes only `todo`, and it is in no queue, because queues are status `todo`. A person resumes it with `issue update` and a status, or requeues it with status `todo` and an empty assignee.
- **Worker runner:** `cmd/runner` polls each stage's queue (`todo` and ready) through `core.Service.ListIssues` and starts a headless worker for each issue and stage that has none running, with the prompt file for that stage. The worker claims its own issue, so the runner does not. The command comes from `runner.command` in the config. At most `max_workers` run at once. Stopping the runner does not stop its workers, so a claim is never left behind by a kill.
- **Migrations:** the SQLite store applies an ordered list of statements and counts them in `PRAGMA user_version`. Append to the list; never edit an entry.
- **Code navigation:** `codeplat-nav` is separate from the tracker: it reads source, not the store, so it has no use case in `core.Service`, no config and no REST route. `codeplat-nav mcp` serves `code_map`, `symbol_find` and `symbol_refs` as the MCP server `codenav` through the same JSON-RPC code as `codeplat-mcp` (`mcp.NewToolServer`); its other subcommands print the same text for a shell. The map comes from `go/doc` on every request, so it is never stale and is not committed. Symbol lookups run the `gopls` CLI in the module that contains the given directory, so each sample is its own workspace; `gopls` is found on `PATH` or in Go's install directory, and the tools report how to install it when it is missing. The worker prompts tell agents to use these instead of grep.
- **Not built yet:** pruning the event log.

## Adding things

- **A use case:** add a method to `core.Service`, then expose it in each transport.
- **A dependency:** add its config, define the port in `core`, construct it in `app.New`, and set it in every `config/*.json`.
- **An environment:** add `config/<name>.json`.
