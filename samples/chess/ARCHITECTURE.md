# chess architecture

These rules are binding for every change under `samples/chess/`.

## Rules

1. Config chooses, `app.New` wires. Only `internal/app` turns a `config.Config` into implementations. No other package constructs a store, a mover, a decider or the service from config.
2. Pure dependency injection. Components receive their dependencies as constructor arguments. No globals, no registration in `init`. The one exception is the blank import of the SQLite driver in `internal/store/sqlite`.
3. Entrypoints are thin. `cmd/chess` loads the config, calls `app.New`, mounts the handlers and serves. It holds no rules.
4. Business rules live in `core.Service`. Transports (`internal/api`, `internal/web`) decode requests, call the service and map its errors to responses. They validate nothing themselves.
5. Ports are declared in `core`: `Store`, `Rules`, `Mover` and `Decider`. Adapters live in their own packages and implement them.
6. Tests build the real service through `apptest.New`. Transport tests do not construct stores or the service by hand.

## Layout

- `cmd/chess`: the entrypoint. Loads the config, builds the service, serves the API and the page.
- `internal/api`: the REST handlers. Decodes bodies, calls the service and maps errors to status codes.
- `internal/app`: the only place that chooses implementations from config. `New` returns the service and a close function.
- `internal/apptest`: builds the service from the `test` configuration for tests.
- `internal/board`: `core.Rules` on top of `github.com/notnil/chess`.
- `internal/config`: the embedded per-environment JSON files, environment overrides and validation.
- `internal/core`: the game types, the ports and `Service`, which runs games.
- `internal/decider/httpdecider`: `core.Decider` that asks the decide server over HTTP.
- `internal/live`: tests that play against the real router and decide server. They skip unless `CHESS_LIVE=1`.
- `internal/opponent/random`: `core.Mover` that picks random legal moves.
- `internal/opponent/router`: `core.Mover` that asks a model through the LLM router.
- `internal/store/memory`: `core.Store` that keeps games in memory.
- `internal/store/sqlite`: `core.Store` backed by a SQLite file.
- `internal/web`: serves the chess page and its script.
