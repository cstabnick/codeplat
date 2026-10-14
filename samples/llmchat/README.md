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
- `tools/...`: added in a later stage

## Test

    make test

`make live` is added by a later issue.
