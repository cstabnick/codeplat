# codeplat

A mini issue tracker (projects and issues) with a REST API, a CLI, an MCP server for coding agents, and a read-only web UI.

## Run

```sh
make build                       # binaries in bin/
bin/codeplat project create PLAT "Platform"
bin/codeplat issue create -project PLAT -title "First issue"
bin/codeplat issue comment PLAT-1 -body "Started on this" -author sam
bin/codeplat-rest                # REST on the configured address
bin/codeplat-mcp                 # MCP over stdio
bin/codeplat-web                 # web UI, http://127.0.0.1:8090 in dev
make test
```

## Agent workflow

An issue moves through three stages, each worked by its own agent loop: `refinement`, `implementation`, `testing`. Within a stage it is `todo`, `in_progress` or `done`.

```sh
bin/codeplat issue list -stage implementation -status todo      # a loop's queue
bin/codeplat issue claim PLAT-1 -assignee worker-1              # fails if already taken
bin/codeplat issue update PLAT-1 -stage testing -status todo -assignee ""   # hand off
```

[prompts/decompose.md](prompts/decompose.md) is the prompt for the planning model: it turns a goal into a roadmap of epics whose issues can be worked in parallel by small models.

## Environments

`CODEPLAT_ENV` picks the config: `dev` (default), `test`, or `prod`. The files live in `config/` and are compiled into the binaries. `CODEPLAT_CONFIG=/path/to/file.json` uses an external file instead.

A relative SQLite path in the config is resolved against the data directory: `CODEPLAT_DATA_DIR` if set, otherwise `~/.local/share/codeplat` (or `$XDG_DATA_HOME/codeplat`). All three binaries therefore share one database per environment, whatever directory they run from.

## Use from Claude Code

`.mcp.json` registers `bin/codeplat-mcp` as a project MCP server. Run `make build`, start Claude Code in this directory, and approve the `codeplat` server.

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the code is put together.
