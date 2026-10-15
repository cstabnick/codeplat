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

[prompts/bonsai/](prompts/bonsai/) holds the refine, implement and test prompts for a Bonsai model working one issue over MCP. They are tighter than the runner's prompts to keep the model from over-reasoning. Replace `ISSUE_ID` with the issue's ID before use, or give it `next.md`, which takes the first ready issue (testing, then implementation, then refinement) and follows the matching stage prompt.

## Environments

`CODEPLAT_ENV` picks the config: `dev` (default), `test`, or `prod`. The files live in `config/` and are compiled into the binaries. `CODEPLAT_CONFIG=/path/to/file.json` uses an external file instead.

A relative SQLite path in the config is resolved against the data directory: `CODEPLAT_DATA_DIR` if set; otherwise, for a binary in a checkout's `bin/` when that checkout has a `data/` directory, that `data/`; otherwise `~/.local/share/codeplat` (or `$XDG_DATA_HOME/codeplat`). The binaries therefore share one database per environment, whatever directory they run from. This repository has `data/`, and commits `data/codeplat.dev.db`, so the tracker's own issues travel with the code. Closing a store folds the write-ahead log into that file, so any CLI command (for example `bin/codeplat project list`) brings it up to date before a commit.

Requests need a token outside `dev` and `test`. Set `CODEPLAT_TOKENS` to a JSON object that maps a name to a token, for example `CODEPLAT_TOKENS='{"worker-1":"<long random string>"}'`. The name becomes the assignee or comment author of that token's writes. `bin/codeplat-rest` takes the token as `Authorization: Bearer <token>`. `bin/codeplat-web` asks for it as the password of a login; the username is ignored. The CLI and MCP do not use tokens. They open the database file directly. Without `CODEPLAT_TOKENS`, `prod` refuses every REST and web request.

## Use from Claude Code

`.mcp.json` registers `bin/codeplat-mcp` as a project MCP server. Run `make build`, start Claude Code in this directory, and approve the `codeplat` server.

See [ARCHITECTURE.md](ARCHITECTURE.md) for how the code is put together.
