# sandbox architecture

sandbox serves a small HTTP API for running commands in named boxes. The
design keeps the rules in one place, the service, and keeps the transports and
the container runtime thin.

## Rules

1. Config chooses; `app.New` wires. `internal/config` loads and validates
   settings and never constructs a driver. Only `internal/app` turns a config
   into an implementation.
2. Dependencies are passed explicitly. Constructors take their dependencies as
   arguments (`core.NewService(driver, opts)`, `docker.New(host, client)`).
   Nothing registers itself at init time, and no package-level value holds a
   live dependency.
3. Entrypoints are thin. `cmd/sandbox-server` reads config, calls `app.New`,
   and serves the handler. It contains no rules.
4. Business rules live in `core.Service`. Box names, workspace confinement,
   timeouts and output limits are checked there. Transports decode requests,
   call the service and map its errors to status codes. They do not validate
   the content of a request beyond decoding it.
5. Ports are declared in `core`. `core.Driver` is the interface a container
   runtime implements. Drivers implement it; they do not hold rules.
6. Tests build the app through `apptest.New(t)`, not by constructing stores,
   drivers or the service by hand. Transport tests follow the same rule.

## Layout

- `cmd/sandbox-server`: the HTTP server entrypoint.
- `internal/app`: the only place that chooses a driver from config, and builds the service.
- `internal/apptest`: `apptest.New(t)`, the test-only way to build the app.
- `internal/config`: loads and validates the embedded per-environment JSON and the environment overrides.
- `internal/core`: box types, the `Driver` port, and `Service`, which owns the rules.
- `internal/docker`: `core.Driver` on the Docker Engine HTTP API, one container per box.
- `internal/localdriver`: `core.Driver` on `os/exec` in memory, for tests; it isolates nothing.
- `internal/httpapi`: the HTTP handler; decodes requests, calls the service, maps errors.
- `internal/live`: an opt-in end-to-end test of the Docker driver, run by `make live`.
