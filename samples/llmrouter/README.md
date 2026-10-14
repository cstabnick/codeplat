# llmrouter

llmrouter is one server that speaks the OpenAI chat-completions API. For each
request it asks a small intent model which route fits the prompt, then sends
the request to that route's model on its backend. It starts and manages Ollama
for you, and serves a page that shows the backends, the routes and recent
routing decisions.

## Run

Run it from this directory:

    make run
    # or
    go run ./cmd/llmrouter [-config FILE]

Then open http://127.0.0.1:8083. The page has a Chat box that sends your questions through the router and shows which route and model answered.

With the default config, llmrouter starts `ollama serve` if Ollama is not
answering, and pulls `llama3.2:1b` and `qwen2.5-coder:1.5b` if they are
missing. The third default model, `llama3:8b`, is the `general` route and is
not pulled automatically.

To use another config, pass `-config FILE` or set `LLMROUTER_CONFIG` to the
file. The examples are in `examples/`.

## Call it

Send an OpenAI chat-completions request with `"model": "auto"` and let the
router choose:

    curl -i http://127.0.0.1:8083/v1/chat/completions \
      -H 'Content-Type: application/json' \
      -d '{"model": "auto", "messages": [{"role": "user", "content": "Write a Go function that reverses a string."}]}'

The response headers say what was chosen:

- `X-Llmrouter-Route` is the route name, such as `code`.
- `X-Llmrouter-Backend` is the backend name, such as `ollama`.
- `X-Llmrouter-Model` is the model name, such as `qwen2.5-coder:1.5b`.

The `model` field can name a model in four ways:

| Value | Meaning |
|-------|---------|
| `auto` | The intent model picks a route. |
| A route name, such as `code` | That route's backend and model. |
| `backend/model`, such as `ollama/llama3:8b` | That model on that backend, bypassing the routes. |
| A bare model name | That model on the default route's backend. |

OpenAI SDKs work unchanged once their base URL is `http://127.0.0.1:8083/v1`.
`GET /v1/models` lists `auto`, each route, and every model each backend serves.

## Admin API

The admin API is JSON under `/api/`. Errors are `{"error": "..."}`.

| Method and path | Body | Returns |
|-----------------|------|---------|
| `GET /api/backends` | | Each backend's name, kind, reachability, version, whether llmrouter started it, and loaded models. |
| `GET /api/backends/{backend}/models` | | The models the backend has. |
| `POST /api/backends/{backend}/models/pull` | `{"name": "llama3:8b"}` | `{"status": "success"}` once installed. Ollama only. |
| `DELETE /api/backends/{backend}/models/{name}` | | `204 No Content`. Ollama only. |
| `GET /api/routes` | | The default route name and the routes. |
| `GET /api/decisions?n=50` | | The most recent routing decisions, newest first. |
| `POST /api/route` | `{"prompt": "..."}` | The route the prompt would get, without sending it to a model. |

## Config

The config is a JSON file. Unknown fields are an error. Load it with
`-config FILE` or `LLMROUTER_CONFIG`. Without a file, llmrouter uses the
defaults shown in `examples/ollama.json`.

`LLMROUTER_ADDR`, when set, replaces `addr`, whether or not a file is given.

Top level (`config.Config`):

| Field | Meaning |
|-------|---------|
| `addr` | Listen address. Default `127.0.0.1:8083`. |
| `backends` | The model servers. At least one. |
| `intent` | The model that classifies prompts. |
| `default_route` | The route used when classification fails, and for bare model names. Must be a route name. |
| `routes` | The routes. At least one. Each has these fields: |

Route (`llm.Route`):

| Field | Meaning |
|-------|---------|
| `name` | Route name. Must not be `auto`, and must be unique. |
| `description` | What the route is for. The intent model reads it when it classifies a prompt. |
| `backend` | Name of a backend. |
| `model` | Model name on that backend. |

Backend (`config.Backend`, each entry of `backends`):

| Field | Meaning |
|-------|---------|
| `name` | Lowercase letters, digits and hyphens, starting with a letter or digit. Must be unique. |
| `kind` | `ollama` for an Ollama server, or `openai` for any OpenAI-compatible server such as llama-server. |
| `url` | `http` or `https` URL of the server. |
| `command` | Command that starts the server when it does not answer. Empty means llmrouter never starts it. |
| `dir` | Working directory for `command`. |
| `env` | Extra `KEY=value` entries added to the environment of `command` and `stop_command`. |
| `stop_command` | Command run after llmrouter stops the process it started, for example to stop a process that a Windows launcher left running. |
| `start_timeout_seconds` | How long to wait for a started server to answer. Default 120. Must not be negative. |
| `pull_missing` | Pull the intent and route models this backend lacks at startup. Only for `kind` `ollama`. |

Intent (`config.Intent`):

| Field | Meaning |
|-------|---------|
| `backend` | Name of the backend that serves the intent model. |
| `model` | The model that classifies prompts. |
| `timeout_seconds` | How long one classification may take. Default 15. Must not be negative. |

The default config is `examples/ollama.json`: one Ollama backend, intent model
`llama3.2:1b`, and routes `general` (`llama3:8b`, the default), `code`
(`qwen2.5-coder:1.5b`) and `quick` (`llama3.2:1b`).

## Bonsai

`examples/bonsai-wsl.json` adds a `bonsai` backend and a `reason` route for
hard problems. It assumes llmrouter runs in WSL and Bonsai runs on Windows.

Bonsai 2 needs the PrismML fork of `llama-server`. Stock llama.cpp and Ollama
cannot run it. The Bonsai demo at `C:\Users\cstab\Bonsai-demo` starts it with
`scripts\start_llama_server.ps1`.

- That script binds port 8080 and does not read a `PORT` variable. If 8080 is
  taken, edit `$Port` in the script, and change the `url` in the config to
  match.
- The script reads `BONSAI_HOST` (default `127.0.0.1`). From WSL in NAT
  networking mode, Windows' localhost is not WSL's localhost. Set
  `BONSAI_HOST=0.0.0.0` (the example does this) and use the Windows host's
  address as the `url`. That is the `default via` address in `ip route`, for
  example `http://192.168.176.1:8080`. Windows Firewall may need to allow the
  port.
- Loading the 7.2 GB model takes a while, so the example sets
  `start_timeout_seconds` to 600.

## Layout

- `internal/llm`: shared types and interfaces; imports only the standard library
- `internal/config`: loads and validates the config
- `internal/router`: chooses a route for each conversation with the intent model
- `internal/gateway`: the OpenAI-compatible API
- `internal/admin`: the admin API under `/api/`
- `internal/ui`: the web page
- `internal/ollama`: HTTP client for one Ollama server
- `internal/oaiclient`: HTTP client for one OpenAI-compatible server
- `internal/supervisor`: makes sure a backend is answering, starting it if needed
- `internal/app`: builds the backends, router and handlers from the config
- `cmd/llmrouter`: the server's entrypoint

## Test

    make test

`make live` runs the opt-in live test against a real Ollama.
