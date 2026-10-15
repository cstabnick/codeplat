# llmchat

llmchat is a chat application that sends each conversation through the LLM
router in `../llmrouter` instead of a hosted model. Conversations are saved in
SQLite. The page streams the answer as it arrives and shows which route and
model answered.

## Run

Start the router first. From `../llmrouter`:

    make run

It listens on http://127.0.0.1:8083. Then, from this directory:

    make run

Open http://127.0.0.1:8084.

## Configuration

llmchat reads these environment variables:

| Variable | Default | Meaning |
|----------|---------|---------|
| `CHAT_ADDR` | `127.0.0.1:8084` | Listen address. |
| `CHAT_DB` | `$XDG_DATA_HOME/llmchat/llmchat.db`, else `~/.local/share/llmchat/llmchat.db` | SQLite database file. |
| `CHAT_ROUTER_URL` | `http://127.0.0.1:8083` | Base URL of the LLM router. |

An empty variable counts as unset. The database file's directory is created
at startup if it does not exist.

## Models

The model picker offers:

- `auto`: the router classifies each message and picks a route.
- Each route by name, as listed by `GET /api/routes`.

A conversation remembers its choice. The stored value is `auto`, a route
name, or `backend/model`.

## Tools

A conversation with a workspace (see below) offers the model six tools. Each
one runs inside the workspace.

| Name | What it does | Needs approval |
|------|--------------|----------------|
| `read_file` | Reads a text file, with line numbers. Takes an optional `offset` and `limit`. | No |
| `list_dir` | Lists the entries of a directory. | No |
| `grep` | Searches files for a regular expression, optionally limited by a file-name glob. | No |
| `write_file` | Creates or overwrites a file with the given content. | Yes |
| `edit_file` | Replaces one exact occurrence of `old_string` in a file (or every occurrence with `replace_all`). | Yes |
| `bash` | Runs a command with `bash -c` in the workspace, with a timeout of up to 300 seconds. | Yes, every call |

A turn makes at most `CHAT_MAX_STEPS` model calls (default 40). The limit
counts each call to the router, so one turn can run several tools.

Whether the model uses these tools depends on the model. The router passes the
tool list to the route's backend. A model that does not make tool calls
answers in text and never runs a tool. For a conversation that uses tools,
choose a route whose model you have checked makes tool calls.

## Workspaces

Tools are offered only in a conversation that has a workspace: an absolute
path to an existing directory. Set it in the header box of the page, or with
`PATCH /api/conversations/{id}` and `{"workspace": "/path"}`. The workspace
can also be set when a conversation is created. A new conversation with no
workspace in its request gets the directory in `CHAT_WORKSPACE`, if that is
set. The workspace is stored with the conversation.

Every path that a file tool receives is cleaned and has its symbolic links
resolved before use. A path that ends up outside the workspace is refused.
That covers `..` that climbs out, absolute paths elsewhere, and symbolic links
that point out of the workspace. A path that does not exist yet is checked by
its nearest existing parent. A dangling symbolic link is refused.

`bash` starts in the workspace, but it is not sandboxed. A command can read or
write anywhere the server's user can. Approving a `bash` call is the only
check on it, so read each command before you approve it.

## Approvals

When the model calls a tool that needs approval, the turn pauses and the
page shows the call with Approve and Deny buttons. The answer goes to
`POST /api/conversations/{id}/approvals/{call}` with `{"approve": true}` or
`{"approve": false}`. A denied call is not run, and the model is told so. If
the page disconnects while a call is waiting, the turn is cancelled.

## API

All paths are under the server address. Errors are `{"error": "..."}`.

| Method and path | Body | Returns |
|-----------------|------|---------|
| `GET /api/conversations` | | Every conversation, most recently updated first. |
| `POST /api/conversations` | `{"title", "model", "workspace"}`, all optional | The new conversation, `201 Created`. |
| `GET /api/conversations/{id}` | | `{"conversation", "messages"}`. |
| `PATCH /api/conversations/{id}` | Any of `title`, `model`, `workspace` | The updated conversation. |
| `DELETE /api/conversations/{id}` | | `204 No Content`. |
| `POST /api/conversations/{id}/messages` | `{"text": "..."}` | Server-sent events, one `data:` JSON event each. Event types: `info`, `content`, `reasoning`, `tool_call`, `approval`, `tool_result`, `error`, `done`. `done` is always last. |
| `POST /api/conversations/{id}/approvals/{call}` | `{"approve": true}` | `204 No Content`. |
| `GET /api/routes` | | The route names. |
| `GET /healthz` | | `{"status": "ok"}`. |

A second message sent while a turn is running in the same conversation gets
`409 Conflict`.

## Layout

- `internal/chat`: the types and interfaces shared by every part; imports only the standard library
- `internal/sqlitestore`: stores conversations and messages in SQLite
- `internal/routerclient`: HTTP client for the router: streams a chat completion and lists the route names
- `internal/agent`: runs one turn: saves the message, asks the router for a reply, runs any tool calls (after approval where needed), repeats until the model answers without tools, and streams the events
- `internal/api`: the JSON and server-sent-events API
- `internal/web`: serves the embedded chat page and its script
- `internal/app`: reads the configuration from the environment and wires the store, router client, engine, API and page into one handler
- `cmd/llmchat`: the server's entrypoint
- `internal/tools/sandbox`: turns the paths the model gives into paths inside the workspace, refusing any that leave it
- `internal/tools/fs`: the `read_file` and `list_dir` tools
- `internal/tools/search`: the `grep` tool
- `internal/tools/edit`: the `write_file` and `edit_file` tools
- `internal/tools/shell`: the `bash` tool

## Test

    make test

`make live` runs the tests in `internal/live` with `LLMCHAT_LIVE=1`. They
call a real router, so start the router first. Without that variable they skip.
