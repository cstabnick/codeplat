# codeplat

A mini issue tracker (projects and issues) with three front doors: a REST API, a CLI, and an MCP server for coding agents.

## Run

```sh
make build                       # binaries in bin/
bin/codeplat project create PLAT "Platform"
bin/codeplat issue create -project PLAT -title "First issue"
bin/codeplat-rest                # REST on the configured address
bin/codeplat-mcp                 # MCP over stdio
make test
```

## Environments

`CODEPLAT_ENV` picks the config: `dev` (default), `test`, or `prod`. The files live in `config/` and are compiled into the binaries. `CODEPLAT_CONFIG=/path/to/file.json` uses an external file instead.

A relative SQLite path in the config is resolved against the data directory: `CODEPLAT_DATA_DIR` if set, otherwise `~/.local/share/codeplat` (or `$XDG_DATA_HOME/codeplat`). All three binaries therefore share one database per environment, whatever directory they run from.

## Use from Claude Code

`.mcp.json` registers `bin/codeplat-mcp` as a project MCP server. Run `make build`, start Claude Code in this directory, and approve the `codeplat` server.

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the code is put together.
