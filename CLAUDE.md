# Working in this repo

Read [ARCHITECTURE.md](ARCHITECTURE.md) first; its Rules section is binding.

- Go lives at `~/.local/go/bin` on this machine; add it to `PATH` if `go` is not found.
- `make test` must pass before a change is done. `make build` builds all three binaries.
- Business rules go in `internal/core`. Do not add validation to a transport.
- Only `internal/app` may choose an implementation from config.
- Write tests through `apptest.New(t)`; do not construct stores or the service by hand in transport tests.
- When you add a use case, expose it in REST, CLI and MCP together.
- Schema changes are a new entry appended to `migrations` in `internal/store/sqlite/sqlite.go`.
- To plan work for other agents, follow [prompts/decompose.md](prompts/decompose.md).
