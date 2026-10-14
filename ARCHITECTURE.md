# Architecture

## Shape

```
cmd/rest, cmd/cli, cmd/mcp     entrypoints: load config, build the app, run one transport
internal/apptest               the test entrypoint: same wiring, test config
internal/transport/{rest,cli,mcp}   translate a protocol to core calls and back
internal/app                   composition root: the only place that reads config to choose implementations
internal/core                  domain types, the Service (all business rules), and the ports it needs
internal/store/{memory,sqlite} Store implementations
config/                        one JSON file per environment, plus the loader
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
- **Data location:** `config.FromEnvironment` resolves a relative store path against the data directory, so entrypoints share a database regardless of working directory.
- **Storage:** SQLite through `modernc.org/sqlite` (pure Go, no cgo) for dev and prod; an in-memory map for test.
- **Issue IDs:** Jira-style `KEY-n`, numbered per project. The store allocates the number atomically.
- **Updates:** `Store.UpdateIssue` takes a function and runs it inside the store's lock or transaction, so read-modify-write is atomic.
- **MCP:** a small hand-written JSON-RPC server over stdio that supports tools only. Logs go to stderr because stdout is the protocol channel.
- **Not built yet:** auth, comments, pagination, schema migrations.

## Adding things

- **A use case:** add a method to `core.Service`, then expose it in each transport.
- **A dependency:** add its config, define the port in `core`, construct it in `app.New`, and set it in every `config/*.json`.
- **An environment:** add `config/<name>.json`.
