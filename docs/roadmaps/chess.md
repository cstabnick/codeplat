# Roadmap: chess sample

## Goal

A sample project in `samples/chess/`: play chess in a web page against an opponent that is a language model behind the LLM router (`samples/llmrouter`, port 8083). How the opponent plays is decided by JDM decision models served by the decide sample (`samples/decide`, port 8085, project `DECIDE`):

- `chess-opponent-policy`: which router route, temperature, persona and how many candidate moves to ask for, from the game's phase, difficulty and whether the opponent is in check.
- `chess-illegal-move`: what to do when the model gives no legal move: retry with feedback, play a random legal move, or resign.
- `chess-move-choice`: a score for each legal candidate from simple features (capture value, check, mate, promotion, whether the piece lands on an attacked square); the highest wins.

Both outside services are ports chosen by config: the opponent is `router` or `random`, the decider is `http` (decide) or `fixed` (the same three decisions hard-coded in core). The test config uses `random` and `fixed`, so nothing outside the module is needed to test it. If decide is down, the service falls back to `fixed` for that decision and records the error on the move.

Its own Go module, `chess`, using `github.com/notnil/chess` for the rules and `modernc.org/sqlite` for storage. Tracker project: `CHESS`. It follows the platform's dependency rules: config chooses, `app.New` wires, pure constructor injection, thin `main`, business rules in `core.Service`, ports declared in `core`, tests through `apptest.New`.

Facts checked on this machine: `github.com/notnil/chess v1.10.0` and `modernc.org/sqlite v1.60.1` are in the module cache. A module with `go.sum` copied from `samples/llmchat` and `GOFLAGS=-mod=mod go get` of both builds packages that import either one without `go mod tidy`. notnil: `chess.NewGame()`, `g.Move(m)`, `g.ValidMoves()`, `g.Position()`, `g.Outcome()`, `g.Method()`, `g.FEN()`, `chess.UCINotation{}`, `chess.AlgebraicNotation{}` (Encode/Decode), `m.HasTag(chess.Capture|chess.Check|chess.KingSideCastle|chess.QueenSideCastle|chess.EnPassant)`, `m.Promo()`, `pos.Board().Piece(sq)`.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists with its dependencies; `core` (types, ports, Service, FixedDecider) passes its tests. | 1 |
| 2. Components | Rules, both stores, both opponents, the decide client, the config and the web page each pass their own tests. | 7, parallel |
| 3. Wiring | `app.New` builds the service from config; `apptest.New` exists; a game plays through it. | 1 |
| 4. Serve | The REST API and server work through `apptest`; README, ARCHITECTURE and an opt-in live test exist. | 2, parallel |

No issue has a dependency inside its epic.

Why four: everything implements or consumes a port in `core`, and the Service lives in `core`, so `core` is whole before anything else compiles against it (epic 1). `app` imports all of epic 2 (epic 3). The REST tests and the live test build through `app` (epic 4). The web page is static and only knows the REST contract, so it is written in epic 2.

Contract shared with DECIDE: the decide HTTP API (`POST /api/evaluate/{key}` with `{"context": {...}}` returning `{"result": {...}}`) and the three decision keys with their inputs and outputs, in issue 1.1.

---

## Epic 1: Foundation

Done when: the module exists with its dependencies; `core` passes its tests.

### 1.1 Create the chess module and its core package

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Create `samples/chess/` under the repository root and run every command from it. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Its own Go module. The root CLAUDE.md and ARCHITECTURE.md describe another project; do not edit anything outside `samples/chess/`.
- Rules: dependencies are constructor arguments (no globals, no init registration); business rules live in `internal/core`; ports are declared in core, which uses them.

## Goal
The module with its two dependencies recorded, and the core package: types, ports, the Service that runs a game, and FixedDecider.

## Read first
- samples/llmrouter/Makefile: copy its shape.

## Change
- samples/chess/go.mod, go.sum: write `module chess` / `go 1.27.2`, copy samples/llmchat/go.sum, then run `GOFLAGS=-mod=mod go get github.com/notnil/chess@v1.10.0 modernc.org/sqlite@v1.60.1`. Do not run `go mod tidy` (it would drop them; later packages import them).
- samples/chess/Makefile: .PHONY targets `test` (`go test ./...`), `vet` (`go vet ./...`), `run` (`go run ./cmd/chess`), `live` (`CHESS_LIVE=1 go test -count=1 -v -timeout 20m ./internal/live/`).
- samples/chess/internal/core/core.go: the types and ports below, verbatim.
- samples/chess/internal/core/service.go, fixed.go, service_test.go.

## Contract
```go
package core

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
	ErrIllegal  = errors.New("illegal move")
	ErrConflict = errors.New("conflict")
)

type Color string

const (
	White Color = "white"
	Black Color = "black"
)

func (c Color) Other() Color // White <-> Black

type Status string

const (
	StatusActive   Status = "active"
	StatusWhiteWon Status = "white_won"
	StatusBlackWon Status = "black_won"
	StatusDraw     Status = "draw"
)

type Game struct {
	ID         int64     `json:"id"`
	Human      Color     `json:"human"`
	Difficulty string    `json:"difficulty"` // easy, normal, hard
	Moves      []Move    `json:"moves"`
	Status     Status    `json:"status"`
	Method     string    `json:"method,omitempty"` // checkmate, stalemate, resignation, ...
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	// Derived; the Service fills them on every Game it returns.
	FEN   string   `json:"fen"`
	Turn  Color    `json:"turn"`
	Legal []string `json:"legal"` // UCI moves for the side to move; empty when the game is over
}

type Move struct {
	UCI  string `json:"uci"`
	SAN  string `json:"san"`
	By   Color  `json:"by"`
	Turn *Turn  `json:"turn,omitempty"` // set on opponent moves
}

// Turn records how the opponent chose a move.
type Turn struct {
	Route      string             `json:"route,omitempty"`
	Backend    string             `json:"backend,omitempty"`
	Model      string             `json:"model,omitempty"`
	Policy     map[string]any     `json:"policy,omitempty"`
	Candidates []string           `json:"candidates,omitempty"` // raw texts of the last proposal
	Scores     map[string]float64 `json:"scores,omitempty"`     // by UCI
	Attempts   int                `json:"attempts"`
	Fallback   string             `json:"fallback,omitempty"` // "", "random" or "resign"
	Errors     []string           `json:"errors,omitempty"`
}

type Position struct {
	FEN           string
	Turn          Color
	Legal         []string // UCI, sorted
	LegalSAN      []string // SAN, same order as Legal
	History       []string // SAN of every move played
	InCheck       bool     // the side to move is in check
	Status        Status
	Method        string
	WhiteMaterial int // pawn 1, knight 3, bishop 3, rook 5, queen 9
	BlackMaterial int
	Officers      int // knights, bishops, rooks and queens of both colours
}

type Features struct {
	Capture       bool `json:"capture"`
	CapturedValue int  `json:"captured_value"`
	Check         bool `json:"check"`
	Mate          bool `json:"mate"`
	Promotion     bool `json:"promotion"`
	Castle        bool `json:"castle"`
	MovedValue    int  `json:"moved_value"` // king 0
	Attacked      bool `json:"attacked"`    // after the move, the other side has a legal move to its square
}

type Store interface {
	Create(ctx context.Context, g Game) (Game, error) // assigns ID 1, 2, ...
	Get(ctx context.Context, id int64) (Game, error)   // ErrNotFound
	List(ctx context.Context) ([]Game, error)          // UpdatedAt newest first, then ID descending
	Save(ctx context.Context, g Game) error            // replaces by ID; ErrNotFound
}

type Rules interface {
	// Replay plays UCI moves from the start position. An illegal move wraps ErrIllegal.
	Replay(moves []string) (Position, error)
	// Parse reads text (UCI like "e2e4" or SAN like "Nf3") as a legal move after moves; returns UCI or wraps ErrIllegal.
	Parse(moves []string, text string) (string, error)
	Features(moves []string, uci string) (Features, error)
}

type MoveRequest struct {
	FEN         string
	Color       Color
	History     []string // SAN
	Legal       []string // SAN
	Count       int
	Route       string // "" means the opponent's configured model
	Temperature float64
	Persona     string
	Feedback    string // why the previous answer failed; "" on the first attempt
}

type MoveReply struct {
	Candidates []string // raw move texts, best first
	Route      string
	Backend    string
	Model      string
	Raw        string
}

type Mover interface {
	Propose(ctx context.Context, req MoveRequest) (MoveReply, error)
}

type Decider interface {
	Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error)
}

const (
	DecisionPolicy  = "chess-opponent-policy"
	DecisionIllegal = "chess-illegal-move"
	DecisionChoice  = "chess-move-choice"
)

type Options struct {
	Now  func() time.Time
	Intn func(n int) int // must be safe for concurrent use
}

func NewService(store Store, rules Rules, mover Mover, decider Decider, opts Options) *Service
func (s *Service) NewGame(ctx context.Context, human Color, difficulty string) (Game, error)
func (s *Service) Game(ctx context.Context, id int64) (Game, error)
func (s *Service) Games(ctx context.Context) ([]Game, error) // never nil
func (s *Service) Play(ctx context.Context, id int64, move string) (Game, error)
func (s *Service) Resign(ctx context.Context, id int64) (Game, error)

type FixedDecider struct{}
func (FixedDecider) Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error)
```

Decisions (inputs are what the Service sends; numbers may arrive as int or float64):
- chess-opponent-policy: in `difficulty`, `phase` (move_number <= 10: "opening"; else Officers <= 6: "endgame"; else "middlegame"), `move_number` (len(moves)/2 + 1), `material_balance` (opponent material minus human material), `in_check`, `legal_count`. Out `route` (string), `temperature`, `persona` (string), `candidates` (clamped to 1..5; missing = 3).
- chess-illegal-move: in `attempt` (1-based), `reason` ("no_move" when Propose failed, "illegal" when no candidate was legal), `difficulty`, `legal_count`. Out `action`: "retry", "random" or "resign" (anything else = "random").
- chess-move-choice: in every Features field plus `rank` (0-based index among the legal candidates) and `difficulty`. Out `score` (number).

FixedDecider: policy -> route "", persona "You are a careful chess player.", temperature easy 1.0 / normal 0.6 / hard 0.2, candidates easy 1 / normal 3 / hard 5. illegal -> attempt < 3 "retry", else "random". choice -> score = captured_value*10 + (check ? 3 : 0) + (mate ? 1000 : 0) + (promotion ? 8 : 0) - (attacked ? moved_value*10 : 0) - rank. Another key -> error wrapping ErrNotFound.

Service rules:
- NewGame: human must be White or Black, difficulty "" becomes "normal" and must be easy/normal/hard (else ErrInvalid). Create with StatusActive, CreatedAt = UpdatedAt = Now(). If human is Black the opponent moves first, then Save.
- Play holds a per-game mutex. Game not active -> ErrConflict; not the human's turn -> ErrConflict; Parse (ErrIllegal passes through). Append Move{UCI, SAN from the replayed History, By human}. If the game is still active, the opponent moves. Set Status/Method from the final Replay, UpdatedAt = Now(), Save, return.
- Resign: active only (else ErrConflict); the opponent's colour wins, Method "resignation".
- Every returned Game has FEN, Turn and Legal filled from Replay (Legal empty when not active).
- Opponent move: Decide(policy); then up to 5 attempts: Propose(MoveRequest from the position and policy, Feedback from the previous failure). Parse each candidate; keep the legal ones, unique, in order. If none, reason is "no_move" (Propose error) or "illegal", and Decide(illegal) chooses: retry (Feedback = `Your answer "<first 200 chars of Raw>" had no legal move. Pick one of the legal moves listed.`), random (a legal move at Intn(len(Legal)), Fallback "random"), resign (human wins, Method "resignation", Fallback "resign"). After the 5th failed attempt, random. With more than one legal candidate, Decide(choice) for each and take the highest score, ties to the lower rank. Append Move{..., By opponent, Turn}.
- Whenever Decide returns an error, use FixedDecider's answer for that call and append the error text to Turn.Errors. Propose errors also go to Turn.Errors.

## Done when
- `go vet ./... && go test ./...` passes in samples/chess.
- service_test uses fakes written in the test (a Rules fake with a fixed legal list, a scripted Mover, a Decider fake, a map Store) and covers: NewGame validation, black human gets an opponent move first, Play wrong turn and finished game, an illegal human move, retry then success, random fallback, resign fallback, decider error falling back to FixedDecider with Turn.Errors set, choice picking the highest score, Resign.
- FixedDecider tests cover each key and an unknown key.

## Out of scope
Any implementation of Store, Rules, Mover or Decider; config; HTTP.
~~~~

---

## Epic 2: Components

Done when: rules, both stores, both opponents, the decide client, the config and the web page each pass their own tests.

### 2.1 Implement the chess rules with notnil/chess

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- `github.com/notnil/chess v1.10.0` is already in go.mod. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
core.Rules backed by github.com/notnil/chess.

## Read first
- samples/chess/internal/core/core.go: Rules, Position, Features, Status, Color, ErrIllegal.
- `go doc github.com/notnil/chess` and `go doc github.com/notnil/chess.Game`.

## Change
- samples/chess/internal/board/board.go and board_test.go.

## Contract
```go
package board

type Rules struct{}

func New() Rules
func (Rules) Replay(moves []string) (core.Position, error)
func (Rules) Parse(moves []string, text string) (string, error)
func (Rules) Features(moves []string, uci string) (core.Features, error)

var _ core.Rules = Rules{}
```
- Replay: NewGame, decode each move with `chess.UCINotation{}.Decode(g.Position(), m)` and `g.Move`; failure -> `fmt.Errorf("%w: %s", core.ErrIllegal, m)`. History via `chess.AlgebraicNotation{}.Encode`. InCheck: the last move has tag `chess.Check`. Status from `g.Outcome()` ("1-0" white_won, "0-1" black_won, "1/2-1/2" draw, else active). Method: snake_case of `g.Method().String()` ("Checkmate" -> "checkmate", "FivefoldRepetition" -> "fivefold_repetition"), "" for NoMethod. Material and Officers from `g.Position().Board().SquareMap()`.
- Parse: trim spaces, accept "0-0"/"0-0-0" as "O-O"/"O-O-O", strip trailing "+", "#", "!", "?". Try UCI decode, then SAN decode; the result must be among the position's valid moves. Return its UCI encoding.
- Features: Capture includes en passant; CapturedValue is the captured piece's value (en passant 1); Mate when the game after the move is checkmate; Attacked when, after the move, any valid move of the other side ends on the move's destination square.

## Done when
- `go test ./internal/board/` passes: start position has 20 legal moves; Parse of "e4", "e2e4", "Nf3", "0-0" after a castling setup, "e5" from the start (illegal); fool's mate (f2f3 e7e5 g2g4 d8h4) gives black_won/checkmate; Features of a capture and of a move onto an attacked square.

## Out of scope
Every other package.
~~~~

### 2.2 Add the in-memory game store

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
core.Store in memory, for tests and for runs that need no file.

## Read first
- samples/chess/internal/core/core.go: Store, Game, ErrNotFound.

## Change
- samples/chess/internal/store/memory/memory.go and memory_test.go.

## Contract
```go
package memory

type Store struct{ /* mu sync.Mutex; next int64; games map[int64]core.Game */ }

func New() *Store
// Create, Get, List, Save as core.Store says. Copy games in and out with a JSON marshal/unmarshal round trip so callers never share slices or Turn pointers.

var _ core.Store = (*Store)(nil)
```

## Done when
- `go test ./internal/store/memory/` passes: IDs are 1, 2, 3; Get of a missing ID wraps core.ErrNotFound; Save of a missing ID wraps core.ErrNotFound; List order (UpdatedAt newest first, ties by ID descending); a Game returned by Get, changed by the caller, does not change the stored one; Moves with a Turn survive a round trip.

## Out of scope
Every other package.
~~~~

### 2.3 Add the SQLite game store

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- `modernc.org/sqlite v1.60.1` is already in go.mod. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
core.Store in a SQLite file, so games survive a restart.

## Read first
- samples/chess/internal/core/core.go: Store, Game, ErrNotFound.
- samples/llmchat/internal/sqlitestore/sqlitestore.go: how that sample opens modernc SQLite (driver name "sqlite", blank import).

## Change
- samples/chess/internal/store/sqlite/sqlite.go and sqlite_test.go.

## Contract
```go
package sqlite

type Store struct{ /* db *sql.DB */ }

// Open opens or creates the file and runs
// CREATE TABLE IF NOT EXISTS games (id INTEGER PRIMARY KEY AUTOINCREMENT, updated_ns INTEGER NOT NULL, data TEXT NOT NULL)
func Open(path string) (*Store, error)
func (s *Store) Close() error
// data is the Game as JSON; on read, ID comes from the id column. List orders by updated_ns DESC, id DESC.
// Save with no affected row wraps core.ErrNotFound.

var _ core.Store = (*Store)(nil)
```

## Done when
- `go test ./internal/store/sqlite/` passes with a file in `t.TempDir()`: IDs are 1, 2, 3; Get and Save of a missing ID wrap core.ErrNotFound; List order; Moves with a Turn survive a round trip; data is still there after Close and Open again.

## Out of scope
Every other package; migrations beyond the one CREATE TABLE.
~~~~

### 2.4 Add the router and random opponents

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
Two core.Mover implementations: one asks a model through the LLM router, one picks legal moves at random (tests and offline play).

## Read first
- samples/chess/internal/core/core.go: Mover, MoveRequest, MoveReply.

## Change
- samples/chess/internal/opponent/router/router.go and router_test.go.
- samples/chess/internal/opponent/random/random.go and random_test.go.

## Contract
```go
package router

type Client struct{ /* base, model string; hc *http.Client */ }

// New: baseURL like "http://127.0.0.1:8083" (trailing slash ignored); model "" means "auto"; hc nil means &http.Client{}.
func New(baseURL, model string, hc *http.Client) *Client
func (c *Client) Propose(ctx context.Context, req core.MoveRequest) (core.MoveReply, error)

var _ core.Mover = (*Client)(nil)
```
- POST `<base>/v1/chat/completions` with `{"model": req.Route or the configured model, "stream": false, "temperature": req.Temperature, "messages": [system, user]}`.
- system: `<Persona> You play <color>. Reply with only JSON: {"moves": ["<move>", ...]} listing up to <Count> moves in SAN, best first.`
- user: `Position (FEN): <FEN>` / `Moves so far: <History joined by spaces, or "none">` / `Legal moves: <Legal joined by spaces>` / then Feedback if non-empty; one per line.
- Reply: `choices[0].message.content` is Raw. Candidates: unmarshal the text from the first `{` to the last `}` as `{"moves": [...]}`; if that fails, every token matching `^(O-O(-O)?|[KQRBN]?[a-h]?[1-8]?x?[a-h][1-8](=[QRBN])?|[a-h][1-8][a-h][1-8][qrbn]?)[+#]?$` in order. Unique, at most Count.
- Route, Backend, Model from headers `X-Llmrouter-Route`, `X-Llmrouter-Backend`, `X-Llmrouter-Model`. Non-200: error with the status and the first 512 bytes of body. Connection failure: `router unreachable: <err>` (or ctx.Err() if the context ended).

```go
package random

type Mover struct{ /* mu sync.Mutex; r *rand.Rand */ }

func New(seed int64) *Mover
// Propose returns min(Count, len(Legal)) distinct entries of req.Legal in random order; Route, Backend, Model are "random".
func (m *Mover) Propose(ctx context.Context, req core.MoveRequest) (core.MoveReply, error)

var _ core.Mover = (*Mover)(nil)
```

## Done when
- `go test ./internal/opponent/...` passes. router tests use httptest: request body fields and model choice, JSON reply, prose reply with SAN tokens, headers copied, 500 error, unreachable. random tests: same seed gives the same moves, never more than Count, only from Legal.

## Out of scope
Deciding legality (core does it); every other package.
~~~~

### 2.5 Add the decide HTTP client

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
core.Decider that asks the decide server (a JDM decision engine) over HTTP.

## Read first
- samples/chess/internal/core/core.go: Decider.

## Change
- samples/chess/internal/decider/httpdecider/httpdecider.go and httpdecider_test.go.

## Contract
```go
package httpdecider

type Client struct{ /* base string; hc *http.Client */ }

// New: baseURL like "http://127.0.0.1:8085" (trailing slash ignored); hc nil means &http.Client{Timeout: 10 * time.Second}.
func New(baseURL string, hc *http.Client) *Client
func (c *Client) Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error)

var _ core.Decider = (*Client)(nil)
```
decide API: `POST <base>/api/evaluate/<key>` with body `{"context": <input>}`; 200 returns `{"result": {...}}`; errors are `{"error": "..."}` with 404 (unknown model) or 400.
- Non-200: `decide returned <status>: <first 512 bytes of body>`. Connection failure: `decide unreachable: <err>`. A null result becomes an empty map. Escape key with url.PathEscape.

## Done when
- `go test ./internal/decider/httpdecider/` passes with httptest: path and body sent, result returned, 404 error text, unreachable error.

## Out of scope
Fallback behaviour (core does it); every other package.
~~~~

### 2.6 Add the chess config

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.
- Rule: config only describes which implementation to use; it never constructs one. Only `internal/app` (a later issue) does.

## Goal
Per-environment JSON config embedded in the binary that picks the store, the opponent and the decider.

## Read first
- config/config.go at the repository root: the platform embeds its configs with `//go:embed *.json`; follow its shape.

## Change
- samples/chess/internal/config/config.go, config_test.go, dev.json, test.json.

## Contract
```go
package config

type Config struct {
	Addr     string   `json:"addr"`
	Seed     int64    `json:"seed"` // the service's random picks; 0 = time-based
	Store    Store    `json:"store"`
	Opponent Opponent `json:"opponent"`
	Decider  Decider  `json:"decider"`
}
type Store struct {
	Kind string `json:"kind"`           // "memory" | "sqlite"
	Path string `json:"path,omitempty"` // required for sqlite
}
type Opponent struct {
	Kind  string `json:"kind"`            // "router" | "random"
	URL   string `json:"url,omitempty"`   // required for router
	Model string `json:"model,omitempty"` // router model when the policy names no route; "" = "auto"
	Seed  int64  `json:"seed,omitempty"`  // random
}
type Decider struct {
	Kind string `json:"kind"`          // "http" | "fixed"
	URL  string `json:"url,omitempty"` // required for http
}

func Load(name string) (Config, error) // embedded <name>.json, DisallowUnknownFields, then Validate
// FromEnvironment loads CHESS_ENV (default "dev"); CHESS_ADDR, CHESS_DB (Store.Path), CHESS_ROUTER_URL (Opponent.URL), CHESS_DECIDE_URL (Decider.URL) replace their field when non-empty; then Validate.
func FromEnvironment() (Config, error)
func (c Config) Validate() error // Addr non-empty, known kinds, required fields present
```
dev.json: `{"addr": "127.0.0.1:8086", "seed": 0, "store": {"kind": "sqlite", "path": "chess.db"}, "opponent": {"kind": "router", "url": "http://127.0.0.1:8083", "model": "auto"}, "decider": {"kind": "http", "url": "http://127.0.0.1:8085"}}`
test.json: `{"addr": "127.0.0.1:0", "seed": 1, "store": {"kind": "memory"}, "opponent": {"kind": "random", "seed": 1}, "decider": {"kind": "fixed"}}`

## Done when
- `go test ./internal/config/` passes: both files load; unknown name; unknown field; each Validate error; each variable via t.Setenv.

## Out of scope
Every other package.
~~~~

### 2.7 Add the chess web page

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only, no JavaScript libraries, no build step (there is no Node here). Do not edit go.mod or go.sum.
- Other workers are writing other packages under `samples/chess/internal/` right now. Change only the files below.

## Goal
A page to play on: pick or start a game, click to move, see what the opponent decided.

## Read first
- samples/llmchat/internal/web/web.go: serving embedded files.

## Change
- samples/chess/internal/web/web.go, page.html, app.js, web_test.go.

## Contract
```go
package web
// NewHandler serves page.html at "/" and app.js at "/app.js" from go:embed.
func NewHandler() http.Handler
```
The page talks to this REST API (same origin):
- `GET /api/games` -> array of Game; `POST /api/games` `{"human": "white"|"black", "difficulty": "easy"|"normal"|"hard"}` -> 201 Game
- `GET /api/games/{id}` -> Game; `POST /api/games/{id}/moves` `{"move": "e2e4"}` -> Game (already including the opponent's reply; may take many seconds); `POST /api/games/{id}/resign` -> Game
- errors: `{"error": "..."}` with 400, 404 or 409
Game JSON: `{"id", "human", "difficulty", "status": "active"|"white_won"|"black_won"|"draw", "method", "fen", "turn": "white"|"black", "legal": ["e2e4", ...], "moves": [{"uci", "san", "by", "turn": {"route", "backend", "model", "policy", "candidates", "scores", "attempts", "fallback", "errors"}}], "created_at", "updated_at"}`
Page: a game list with a new-game form (colour, difficulty); an 8x8 board drawn from `fen` with Unicode pieces, the human's colour at the bottom; click a piece then a square, allowed only when `turn` is the human's and `from+to` is a prefix of an entry in `legal` (add "q" when the only matches are promotions); "Opponent is thinking…" while the POST is pending; the move list in SAN; for each opponent move, its route/model, fallback, attempts, candidates and errors; status and a Resign button.

## Done when
- `go test ./internal/web/` passes: `/` returns HTML containing `id="board"` and `app.js`; `/app.js` contains `/api/games`.

## Out of scope
The REST API itself (a later issue); every other package.
~~~~

---

## Epic 3: Wiring

Done when: `app.New` builds the service from config; `apptest.New` exists; a game plays through it.

### 3.1 Wire the chess app and its test harness

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Rules: `internal/app` is the only package that chooses an implementation from config. Dependencies are constructor arguments. No globals.

## Goal
One function that turns a config into the real service, and a test harness that builds it with the test config.

## Read first
- samples/chess/internal/config/config.go: Config and the kinds.
- samples/chess/internal/core/core.go: NewService, Options, FixedDecider.
- Constructors: internal/board (`board.New()`), internal/store/memory (`memory.New()`), internal/store/sqlite (`sqlite.Open(path)`, `Close`), internal/opponent/router (`router.New(url, model, nil)`), internal/opponent/random (`random.New(seed)`), internal/decider/httpdecider (`httpdecider.New(url, nil)`).

## Change
- samples/chess/internal/app/app.go and app_test.go.
- samples/chess/internal/apptest/apptest.go.

## Contract
```go
package app
// New switches on cfg.Store.Kind, cfg.Opponent.Kind and cfg.Decider.Kind ("fixed" -> core.FixedDecider{}),
// and builds core.NewService(store, board.New(), mover, decider, core.Options{Now: time.Now, Intn: <rand.New(rand.NewSource(seed)).Intn behind a sync.Mutex>}).
// seed = cfg.Seed, or time.Now().UnixNano() when 0. close closes the sqlite store, else does nothing. Unknown kind -> error.
func New(cfg config.Config) (svc *core.Service, close func() error, err error)

package apptest
// New builds the app from config.Load("test"); registers close with t.Cleanup; t.Fatal on error.
func New(t testing.TB) *core.Service
```

## Done when
- `go vet ./... && go test ./...` passes.
- app_test via apptest.New: NewGame white then Play "e4" gives two moves, the second By black with Turn.Model "random"; NewGame black already has one move; a sqlite config with a `t.TempDir()` path builds and closes; an unknown kind fails.

## Out of scope
HTTP, `cmd/`.
~~~~

---

## Epic 4: Serve

Done when: the REST API and server work through `apptest`; README, ARCHITECTURE and an opt-in live test exist.

### 4.1 Add the chess REST API and server

~~~~text
Epic: 4 Serve
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Another worker is adding README.md, ARCHITECTURE.md, .gitignore and internal/live right now. Change only the files below.
- Rules: a transport decodes its input and maps errors; it validates nothing else. A `main` is: load config, `app.New`, hand the service to transports. Tests go through `apptest.New(t)`.

## Goal
Serve the game over REST, and the server binary that also serves the page.

## Read first
- samples/chess/internal/core/core.go: Service methods and errors.
- samples/chess/internal/apptest/apptest.go, samples/chess/internal/config/config.go (FromEnvironment), samples/chess/internal/web/web.go (NewHandler).

## Change
- samples/chess/internal/api/api.go and api_test.go.
- samples/chess/cmd/chess/main.go.

## Contract
```go
package api
func NewHandler(svc *core.Service) http.Handler
```
| Route | Body | Returns |
|---|---|---|
| `GET /api/games` | | 200 array of Game (`[]`, never null) |
| `POST /api/games` | `{"human", "difficulty"}` | 201 Game |
| `GET /api/games/{id}` | | 200 Game |
| `POST /api/games/{id}/moves` | `{"move"}` | 200 Game |
| `POST /api/games/{id}/resign` | | 200 Game |
| `GET /healthz` | | 200 `{"status": "ok"}` |
Errors `{"error": "<message>"}`: 404 core.ErrNotFound; 400 core.ErrInvalid, core.ErrIllegal, a bad body or a non-integer id; 409 core.ErrConflict; 500 otherwise. JSON content type throughout.
main: `config.FromEnvironment()`, `app.New(cfg)`; mux "/api/" and "/healthz" -> api.NewHandler(svc), "/" -> web.NewHandler(); `http.Server{Addr, Handler, ReadHeaderTimeout: 10 * time.Second}` (no write timeout: opponent moves are slow); log `chess listening on http://<addr>`; graceful shutdown on SIGINT/SIGTERM, then close().

## Done when
- `go test ./internal/api/` passes via apptest.New + httptest: create, list, get, play "e4" (two moves back), illegal move 400, unknown game 404, move after resign 409, healthz.
- `go build -o /dev/null ./cmd/chess` succeeds.

## Out of scope
The page, README, live test.
~~~~

### 4.2 Write the chess README, ARCHITECTURE and live test

~~~~text
Epic: 4 Serve
Depends on: none

## Environment
- Work in `samples/chess/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Another worker is adding internal/api and cmd/chess right now. Change only the files below.

## Goal
Tell a reader how to run it against the router and decide, state its dependency rules, and add a test that plays against the real model when asked to.

## Read first
- samples/llmrouter/README.md: tone and layout.
- samples/chess/internal/config/config.go: variables and kinds.
- samples/chess/internal/app/app.go: New.

## Change
- samples/chess/README.md: what it is; Run (start `samples/llmrouter` with `make run` on 8083, `samples/decide` with `make run` on 8085, then `make run` here and open http://127.0.0.1:8086); Configuration (CHESS_ENV, CHESS_ADDR, CHESS_DB, CHESS_ROUTER_URL, CHESS_DECIDE_URL; the kinds); How the opponent decides (the three decision keys, their inputs and outputs, fallback to the fixed decider when decide is down, errors shown on the move); REST API table (GET/POST /api/games, GET /api/games/{id}, POST /api/games/{id}/moves, POST /api/games/{id}/resign, GET /healthz); Testing (`make test` uses random opponent and fixed decider; `make live`).
- samples/chess/ARCHITECTURE.md: Rules: 1. config chooses, `app.New` wires (only internal/app constructs from config); 2. pure DI, constructor arguments, no globals or init registration (the SQLite driver import excepted); 3. entrypoints are thin; 4. business rules live in core.Service, transports decode and map errors only; 5. ports are declared in core (Store, Rules, Mover, Decider); tests go through apptest.New. Then a Layout list of packages, one line each.
- samples/chess/.gitignore: `*.db`, `*.db-*`.
- samples/chess/internal/live/live_test.go: skip unless `CHESS_LIVE=1`. `config.Load("dev")` with Store set to `{"kind": "memory"}`, `app.New`, NewGame white normal, Play "e4"; assert two moves, `Moves[1].Turn.Model` is neither "" nor "random"; log the Turn as JSON.

## Done when
- `go vet ./internal/live/` succeeds and `go test ./internal/live/` passes (it skips without CHESS_LIVE).
- Every path, variable and route the docs mention exists in the code or in this issue.

## Out of scope
internal/api, cmd/chess.
~~~~
