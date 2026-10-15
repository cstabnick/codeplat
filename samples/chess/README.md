# chess

chess is a game of chess against the computer. You play White or Black in the
browser or over the REST API. The computer's moves come from a model behind the
LLM router, and its decisions (how to play, what to do after an illegal answer,
which candidate to pick) come from a decision model on the decide server.

## Run

Start the two services it depends on, each in its own terminal:

    # samples/llmrouter: the LLM router on 8083
    make run

    # samples/decide: the decide server on 8085
    make run

Then run chess from this directory:

    make run
    # or
    go run ./cmd/chess

Open http://127.0.0.1:8086. Choose your colour and difficulty, then play.

If the decide server is not running, chess still plays. Each decision falls back
to the fixed decider, and the error is recorded on the move (see below).

## Configuration

The configuration is one JSON file per environment, embedded in the binary from
`internal/config/`. `CHESS_ENV` chooses the file and defaults to `dev`.

| Variable | Replaces | Default in `dev` |
|----------|----------|------------------|
| `CHESS_ENV` | which environment file to load (`dev` or `test`) | `dev` |
| `CHESS_ADDR` | the listen address | `127.0.0.1:8086` |
| `CHESS_DB` | the SQLite file (`store.path`) | `chess.db` |
| `CHESS_ROUTER_URL` | the LLM router URL (`opponent.url`) | `http://127.0.0.1:8083` |
| `CHESS_DECIDE_URL` | the decide server URL (`decider.url`) | `http://127.0.0.1:8085` |

A variable replaces its field only when it is set and not empty. The result is
validated before chess starts.

The fields take these kinds:

- `store.kind`: `memory` (games live in process memory) or `sqlite` (a file at `store.path`).
- `opponent.kind`: `router` (moves from the LLM router at `opponent.url`) or `random` (random legal moves, with `opponent.seed`).
- `decider.kind`: `http` (decisions from the decide server at `decider.url`) or `fixed` (fixed rules, no server).

## How the opponent decides

Every computer move asks the policy model first. The other two are asked only
when needed: the illegal-move model when an answer has no legal move, and the
choice model when there are two or more legal candidates to rank. Each is a key
on the decide server, and the service names them in `internal/core/core.go`:

| Key | When | Input | Output |
|-----|------|-------|--------|
| `chess-opponent-policy` (`core.DecisionPolicy`) | before each computer move | `difficulty`, `phase`, `move_number`, `material_balance`, `in_check`, `legal_count` | `route`, `temperature`, `persona`, `candidates` (1 to 5, default 3) |
| `chess-illegal-move` (`core.DecisionIllegal`) | when the model's answer has no legal move | `attempt`, `reason` (`illegal` or `no_move`), `difficulty`, `legal_count` | `action`: `retry` asks the model again with feedback; `resign` ends the game; anything else plays a random legal move |
| `chess-move-choice` (`core.DecisionChoice`) | to rank each legal candidate | `capture`, `captured_value`, `check`, `mate`, `promotion`, `castle`, `moved_value`, `attacked`, `rank`, `difficulty` | `score`; the highest score wins and a tie goes to the better-ranked candidate |

The opponent makes at most 5 attempts to get a legal move from the model. After
that it plays a random legal move.

If a decision call fails, chess uses the fixed decider's answer for that key and
records the failure on the move as `turn.errors`, such as `decide chess-move-choice: ...`.
Failed model calls and unparseable candidates are recorded the same way. Each
computer move carries its `turn` record: the route, backend and model that
answered, the candidates, the number of attempts, any fallback
(`random` or `resign`) and the errors. When there are two or more legal candidates,
the `scores` field holds the choice score of each candidate with a numeric score,
keyed by UCI move; a single candidate leaves it empty.

## REST API

All bodies are JSON. Errors are `{"error": "..."}`.

| Method and path | Body | Returns |
|-----------------|------|---------|
| `GET /api/games` | | Every game, most recently updated first. |
| `POST /api/games` | `{"human": "white" or "black", "difficulty": "easy", "normal" or "hard"}` | `201` and the new game. Difficulty defaults to `normal`. If you play Black, the computer opens. |
| `GET /api/games/{id}` | | One game. |
| `POST /api/games/{id}/moves` | `{"move": "e2e4" or "Nf3"}` | The game after your move and the computer's reply. |
| `POST /api/games/{id}/resign` | | The game, with the computer as the winner. |
| `GET /healthz` | | `{"status": "ok"}` |

Status codes: `400` for a bad body, an invalid value or an illegal move; `404`
for an unknown game; `409` when the game is over or it is not your move; `500`
for anything else.

## Testing

    make test    # go test ./...
    make vet     # go vet ./...
    make live    # plays e4 against the real router and decide server

`make test` uses the `test` configuration: a memory store, random moves for the
opponent and the fixed decider. It never reaches a model.

`make live` sets `CHESS_LIVE=1` and runs `internal/live`. It uses the `dev`
configuration, so the router on 8083 must be running. The decide server on 8085
may be down; the test still runs, and the fallback is recorded in the move's
`turn.errors`.
