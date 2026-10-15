# decide

decide is a decision engine. It loads decision models written in the JDM file
format (JSON decision graphs made of input, decision table, expression and
output nodes) and evaluates them against a JSON context. It serves the models
over HTTP and offers a command-line interface to the same models.

## Run

Run it from this directory:

    make run

This starts `go run ./cmd/decide-server`, which listens on 127.0.0.1:8085 and
loads the models from `models/`. The models directory is relative to the
working directory, so run `make run` from `samples/decide`.

Other targets:

    make test
    make vet

## Configuration

Configuration comes from an embedded JSON file per environment, then from
environment variables.

| Variable | Effect |
|---|---|
| `DECIDE_ENV` | Selects the embedded environment. Defaults to `dev`. The embedded environments are `dev` and `test`. |
| `DECIDE_ADDR` | Replaces the `addr` of the environment. `dev` listens on `127.0.0.1:8085`. |
| `DECIDE_MODELS` | Replaces the `models_dir` of the environment. `dev` reads `models`. `test` has an empty `models_dir`, so set `DECIDE_MODELS` when you use `test`. |

## HTTP API

| Method and path | Request | Response |
|---|---|---|
| `POST /api/evaluate/{key}` | `{"context": {...}}` | `200` with `{"result": {...}}` |
| `GET /api/models` | none | `200` with `{"models": [...]}` |
| `GET /healthz` | none | `200` with `{"status": "ok"}` |

Errors are JSON in the form `{"error": "..."}`:

- `400` when the model key is malformed, the body is not valid JSON, or the body is larger than 1 MiB.
- `404` when the model does not exist or the route is unknown.
- `500` with `internal error` for any other failure.

Evaluate `example-shipping`:

    curl -s -X POST http://127.0.0.1:8085/api/evaluate/example-shipping \
      -d '{"context": {"country": "US", "weight": 3}}'

    {"result":{"cost":5}}

## Command line

The CLI runs the same models without the server. It is `cmd/decide`.

    go run ./cmd/decide models
    go run ./cmd/decide eval example-shipping <<< '{"country":"US","weight":3}'

`models` prints one model key per line. `eval KEY` reads a JSON object from
standard input, which may be empty, and prints the result as indented JSON.
Errors go to standard error with exit code 1. Usage errors exit with code 2.

## Supported JDM subset

decide evaluates a subset of JDM:

- Node types: `inputNode`, `decisionTableNode`, `expressionNode` and `outputNode`. Other node types, such as function, switch or collect nodes, are rejected as invalid.
- Decision tables: only the `first` hit policy, or an empty hit policy. Other hit policies are rejected as invalid.
- Expressions: comparisons (`==`, `!=`, `<`, `<=`, `>`, `>=`), arithmetic (`+`, `-`, `*`, `/`, `%`), `and`, `or`, `not`, `in`, ternaries, member and index access, and the functions `len`, `abs`, `floor`, `ceil`, `round`, `min`, `max`, `sum`, `contains`, `lower` and `upper`.
- Table cells: a bare value such as `"US"`, a comparison such as `> 5`, a list such as `"a", "b"`, a range such as `[1..5]`, or an expression that uses `$` for the input column value. An empty cell matches anything.

The `chess-*` models in `models/` are used by `samples/chess`.

## Layout

- `cmd/decide-server`: the HTTP server.
- `cmd/decide`: the command-line interface.
- `models/`: the decision models, one `<key>.json` file per model.
- `internal/`: the packages, described in `ARCHITECTURE.md`.
