# Architecture

## Rules

1. Config chooses, `app.New` wires. Only `internal/app` constructs objects from a `config.Config`. No other package picks an implementation from config.
2. Dependencies are passed in constructor arguments. There are no globals and no registration in `init`.
3. Entrypoints are thin: they read config, call `app.New` and start a transport.
4. Business rules live in `core.Service`. Transports only decode input and map errors to status codes or exit codes. They validate nothing that `core` does not.
5. Ports are declared by their consumer. `core` declares `core.ModelSource` and `core.Engine`. `engine` declares `engine.Expr`. Implementations live in other packages. Tests are written through `apptest.New(t)`, not by constructing the service by hand.

## Layout

- `cmd/decide-server`: entrypoint for the HTTP server. Reads config, calls `app.New`, serves `httpapi`.
- `cmd/decide`: entrypoint for the CLI. Reads config, calls `app.New`, runs `cli`.
- `internal/app`: the only package that turns a config into concrete objects.
- `internal/apptest`: builds the service for tests from the `test` config.
- `internal/cli`: command-line transport. Maps errors to exit codes.
- `internal/config`: per-environment configuration, embedded from `dev.json` and `test.json`, with environment overrides.
- `internal/core`: the decision service. Owns `ErrNotFound`, `ErrInvalid` and the model key pattern.
- `internal/dirsource`: `core.ModelSource` that reads `<key>.json` files from a directory.
- `internal/engine`: evaluates JDM graphs. Declares `engine.Expr`.
- `internal/golden`: test-only package that checks the chess models against fixed inputs.
- `internal/httpapi`: HTTP transport. Decodes requests, maps `core` errors to status codes.
- `internal/jdm`: parses and validates JDM models.
- `internal/zen`: expression language and table-cell evaluator. Standard library only.
