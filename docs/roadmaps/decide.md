# Roadmap: decide sample

## Goal

A sample project in `samples/decide/`: a small decision engine that evaluates GoRules JDM (JSON Decision Model) files. A model is a graph of an input node, decision tables and expression nodes, and an output node, written in JSON. A caller sends a JSON object and gets a JSON object back, over HTTP (`POST /api/evaluate/{key}`) or a CLI. It is the decision service the chess sample (`samples/chess`, project `CHESS`) calls, the same way apps call the LLM router: over HTTP, through an open format, chosen by config.

It is its own Go module, `decide`, standard library only. Tracker project: `DECIDE`. It follows the same dependency rules as the platform: config chooses, `app.New` wires, pure constructor injection, thin `main`s, business rules in `core.Service`, ports defined by their consumer, tests through `apptest.New`.

Supported JDM subset: node types `inputNode`, `outputNode`, `decisionTableNode` (hit policy `first`), `expressionNode`; a ZEN-style expression language (arithmetic, comparison, `and`/`or`/`not`, ternary, `in` with lists and ranges, member access, a few functions); decision-table cells as unary tests (`> 5`, `"a", "b"`, `[1..5]`, or any expression using `$`). Function nodes (JavaScript), switch nodes and the `collect` hit policy are out of scope.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists; `jdm` parses and validates a graph; `core.Service` and its ports compile and pass tests. | 1 |
| 2. Components | The expression language, the graph engine, the directory model source and the config each pass their own tests. | 4, parallel |
| 3. Wiring | `app.New` builds the service from config; `apptest.New` exists; an example model evaluates. | 1 |
| 4. Transports | HTTP server and CLI work through `apptest`; the chess models pass golden tests; README and ARCHITECTURE exist. | 4, parallel |

No issue has a dependency inside its epic.

Why four: every component imports `jdm` and `core` (epic 1). `app` imports all of epic 2 (epic 3). The transports test through `apptest`, which needs `app` (epic 4). The engine declares the small `Expr` interface it needs, so it is written in parallel with the expression language that satisfies it.

Contract shared with CHESS: the HTTP API in issue 4.1 and the three `chess-*` models in issue 4.3. The CHESS roadmap carries the same text.

---

## Epic 1: Foundation

Done when: the module exists; `jdm` parses and validates a graph; `core.Service` and its ports compile and pass tests.

### 1.1 Create the decide module with the jdm format and the core service

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Create `samples/decide/` under the repository root and run every command from it. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Its own Go module, standard library only. The root CLAUDE.md and ARCHITECTURE.md describe another project; do not edit anything outside `samples/decide/`.
- Rules for this module: dependencies are constructor arguments (no globals, no init registration); business rules live in `internal/core`; ports (interfaces) are declared in `core`, the package that uses them.

## Goal
The module, the JDM file format types with parsing and validation, and the core service with its two ports. Every later package builds on these.

## Read first
- samples/llmrouter/Makefile: copy its shape.

## Change
- samples/decide/go.mod: `module decide` and `go 1.27.2`. No requires.
- samples/decide/Makefile: targets `test` (`go test ./...`), `vet` (`go vet ./...`), `run` (`go run ./cmd/decide-server`), all .PHONY.
- samples/decide/internal/jdm/jdm.go and jdm_test.go: the contract below.
- samples/decide/internal/core/core.go and core_test.go: the contract below.

## Contract
```go
package jdm

const (
	TypeInput      = "inputNode"
	TypeOutput     = "outputNode"
	TypeTable      = "decisionTableNode"
	TypeExpression = "expressionNode"
)

// ErrInvalid is wrapped by every error Parse, Validate, Order, Table and Expressions return.
var ErrInvalid = errors.New("invalid model")

type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}
type Node struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Content json.RawMessage `json:"content,omitempty"`
}
type Edge struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
}
type Table struct {
	HitPolicy string              `json:"hitPolicy"`
	Inputs    []Column            `json:"inputs"`
	Outputs   []Column            `json:"outputs"`
	Rules     []map[string]string `json:"rules"` // column ID -> cell text; also "_id", "_description"
}
type Column struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Field string `json:"field"` // an expression for inputs, a dotted path for outputs
}
type ExpressionContent struct {
	Expressions []Expression `json:"expressions"`
}
type Expression struct {
	ID    string `json:"id"`
	Key   string `json:"key"`   // dotted output path
	Value string `json:"value"` // expression
}

func Parse(data []byte) (*Graph, error)                 // json.Unmarshal (unknown fields ignored: real files carry "position" etc.), then Validate
func (g *Graph) Validate() error
func (g *Graph) Order() ([]Node, error)                 // topological order (Kahn), ties in Nodes order; error on a cycle
func (n Node) Table() (Table, error)                    // decode Content; error if n.Type != TypeTable
func (n Node) Expressions() (ExpressionContent, error)  // decode Content; error if n.Type != TypeExpression
```
Validate rejects: no nodes; empty or duplicate node ID; a type other than the four; not exactly one input node; no output node; an edge whose sourceId or targetId is not a node; a cycle; a table whose hitPolicy is not "first" (empty counts as "first"), with an empty column ID, or an output column with empty Field; an expression with an empty Key. Messages name the node, e.g. `invalid model: node "policy": hitPolicy "collect" is not supported`.

```go
package core

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid")
)

// KeyPattern is what a model key must match.
var KeyPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type ModelSource interface {
	Get(ctx context.Context, key string) (*jdm.Graph, error) // ErrNotFound when missing
	List(ctx context.Context) ([]string, error)              // sorted keys
}
type Engine interface {
	Evaluate(ctx context.Context, g *jdm.Graph, input map[string]any) (map[string]any, error)
}

type Service struct{ /* models ModelSource; engine Engine */ }

func NewService(models ModelSource, engine Engine) *Service
// Evaluate: a key not matching KeyPattern -> error wrapping ErrInvalid; nil input -> empty map;
// errors from Get and Evaluate are returned unchanged; a nil result becomes an empty map.
func (s *Service) Evaluate(ctx context.Context, key string, input map[string]any) (map[string]any, error)
// Models returns List's result, never nil.
func (s *Service) Models(ctx context.Context) ([]string, error)
```
core imports jdm only.

## Done when
- `go vet ./... && go test ./...` passes in samples/decide.
- jdm tests cover: a valid three-node graph parses and Order returns input, table, output; each Validate rejection above; Table() on a table node.
- core tests use small fakes for ModelSource and Engine and cover: bad key, missing model, nil input, nil result.

## Out of scope
Evaluating expressions or tables, config, HTTP, any `cmd/`.
~~~~

---

## Epic 2: Components

Done when: the expression language, the graph engine, the directory model source and the config each pass their own tests.

### 2.1 Add the zen expression language

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are writing other packages under `samples/decide/internal/` right now. Change only the files below.

## Goal
A small expression language, compatible with the subset of GoRules ZEN that decision models use, to evaluate expressions and decision-table cells.

## Read first
Nothing; this package imports only the standard library.

## Change
- samples/decide/internal/zen/zen.go (split into lexer.go, parser.go, eval.go if you like) and zen_test.go.

## Contract
```go
package zen

type Lang struct{}

// Evaluate parses and evaluates expr against env.
func (Lang) Evaluate(expr string, env map[string]any) (any, error)
// Test evaluates a decision-table cell against the column value.
func (Lang) Test(cell string, value any, env map[string]any) (bool, error)
```
Values: nil, bool, float64, string, []any, map[string]any. Any Go integer type found in env is converted to float64.

Expression syntax, lowest precedence first:
1. `c ? a : b` (c must be bool; nil counts as false)
2. `or`, `||`
3. `and`, `&&` (operands bool; nil counts as false)
4. `not x`, `!x`
5. `== != < <= > >=`, `in`, `not in`
6. `+ -` (`+` concatenates when both sides are strings)
7. `* / %` (division or modulo by zero is an error)
8. unary `-`
9. postfix: `a.b`, `a[0]`, `a["k"]`, calls `f(x, y)`
Primaries: numbers (`12`, `1.5`), strings in `"..."` or `'...'` (escapes `\"`, `\'`, `\\`), `true`, `false`, `null`, list `[1, "a"]`, parentheses, identifiers `[A-Za-z_$][A-Za-z0-9_]*`. An unknown identifier or missing member is nil.
Ranges, only as the right side of `in`: `[a..b]` inclusive, `(a..b)` exclusive, `[a..b)` and `(a..b]` mixed; numbers only.
`==`/`!=` compare any values (numbers numerically, nil equals nil, lists element-wise). `< <= > >=` need two numbers or two strings, otherwise an error.
Functions: `len(s or list)`, `abs(n)`, `floor(n)`, `ceil(n)`, `round(n)`, `min(list)`, `max(list)`, `sum(list)`, `contains(s, sub)`, `lower(s)`, `upper(s)`. An unknown function is an error.

Test(cell, value, env), with env2 = env plus `"$": value`:
1. cell is empty or whitespace -> true.
2. cell contains `$` -> Evaluate(cell, env2) must be bool.
3. cell starts with `<`, `<=`, `>`, `>=`, `==`, `!=` -> Evaluate("$ " + cell, env2).
4. cell starts with `[` or `(` and contains `..` -> `$ in <cell>`.
5. otherwise split on commas outside quotes and brackets; true if any part (a value or a range) matches `$` by `==` or `in`.

## Done when
- `go test ./internal/zen/` passes, with table-driven cases for every operator, precedence (`1 + 2 * 3` is 7), each function, each range form, unknown identifier, the error cases, and each Test rule: `"US"`, `> 5`, `<= 5`, `[1..10]`, `"a", "b"`, `$ > 2 and $ < 4`, `true`, empty cell.

## Out of scope
Every other package. Date functions, closures and array methods like `map`/`filter`.
~~~~

### 2.2 Add the graph engine

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are writing other packages under `samples/decide/internal/` right now, including `internal/zen`. Do not import zen; declare the interface below and test with a fake. Change only the files below.

## Goal
Run a JDM graph: walk its nodes in order, evaluating decision tables and expression nodes, and return what reaches the output node.

## Read first
- samples/decide/internal/jdm/jdm.go: Graph, Node, Order, Table, Expressions.
- samples/decide/internal/core/core.go: the Engine port and ErrInvalid.

## Change
- samples/decide/internal/engine/engine.go and engine_test.go.

## Contract
```go
package engine

// Expr is the expression language the engine needs.
type Expr interface {
	Evaluate(expr string, env map[string]any) (any, error)
	Test(cell string, value any, env map[string]any) (bool, error)
}

type Engine struct{ /* x Expr */ }

func New(x Expr) *Engine
func (e *Engine) Evaluate(ctx context.Context, g *jdm.Graph, input map[string]any) (map[string]any, error)

var _ core.Engine = (*Engine)(nil)
```
Semantics:
- Visit nodes in `g.Order()`. Check ctx before each node.
- A node's input is the deep merge of its sources' outputs in edge order (maps merge recursively; otherwise later wins). The input node's output is `input`.
- decisionTableNode, hit policy first: a rule matches when, for every input column, the cell `rule[col.ID]` is empty or `Test(cell, v, nodeInput)` is true, where `v = Evaluate(col.Field, nodeInput)` (an empty Field gives v = nil). The first matching rule's output is a map: for each output column with a non-empty cell, `Evaluate(cell, nodeInput)` set at the dotted path `col.Field` (`"a.b"` makes `{"a": {"b": v}}`). No match gives `{}`.
- expressionNode: out starts as `{}`; for each expression in order, evaluate Value with env = nodeInput plus `"$": out`, and set the result at the dotted path Key in out. The output is out.
- outputNode: its output is its input. The result is the deep merge of all output nodes' outputs.
- Every error from Expr, Table() or Expressions() is returned as `fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)`.

## Done when
- `go test ./internal/engine/` passes, using a fake Expr written in the test (for example: Evaluate returns env[expr] if present, else json.Unmarshal of expr; Test compares json.Unmarshal(cell) with value). Cover: first-match, no match, empty cells, dotted output paths, expression node using `$`, two sources merged, an Expr error wrapped in core.ErrInvalid, a cancelled ctx.

## Out of scope
The expression language itself (another worker), the jdm and core packages.
~~~~

### 2.3 Add the directory model source

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are writing other packages under `samples/decide/internal/` right now. Change only the files below.

## Goal
Load models from a directory of `<key>.json` files, implementing core.ModelSource.

## Read first
- samples/decide/internal/core/core.go: ModelSource, ErrNotFound, ErrInvalid, KeyPattern.
- samples/decide/internal/jdm/jdm.go: Parse.

## Change
- samples/decide/internal/dirsource/dirsource.go and dirsource_test.go.

## Contract
```go
package dirsource

type Source struct{ /* dir string */ }

func New(dir string) *Source
// Get reads <dir>/<key>.json on every call, so edits apply without a restart.
// A key not matching core.KeyPattern or a missing file -> error wrapping core.ErrNotFound.
// A Parse error -> fmt.Errorf("%w: model %q: %v", core.ErrInvalid, key, err).
func (s *Source) Get(ctx context.Context, key string) (*jdm.Graph, error)
// List returns the keys of the *.json files directly in dir, sorted. Names not matching KeyPattern are skipped.
func (s *Source) List(ctx context.Context) ([]string, error)

var _ core.ModelSource = (*Source)(nil)
```

## Done when
- `go test ./internal/dirsource/` passes, writing models into `t.TempDir()`: Get of a valid model, a missing key, a `../x` key, an invalid file, List order and skipping `README.md` and `Bad Name.json`.

## Out of scope
Caching, watching the directory, every other package.
~~~~

### 2.4 Add the decide config

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are writing other packages under `samples/decide/internal/` right now. Change only the files below.
- Rule: config only describes; it never constructs anything. Only `internal/app` (a later issue) turns it into objects.

## Goal
Per-environment JSON config embedded in the binary, with environment-variable overrides.

## Read first
- config/config.go at the repository root: the platform does the same thing with `//go:embed *.json`; follow its shape.

## Change
- samples/decide/internal/config/config.go, config_test.go, dev.json, test.json.

## Contract
```go
package config

type Config struct {
	Addr      string `json:"addr"`
	ModelsDir string `json:"models_dir"`
}

// Load decodes the embedded <name>.json with DisallowUnknownFields. An unknown name or an empty Addr is an error.
func Load(name string) (Config, error)
// FromEnvironment loads DECIDE_ENV (default "dev"), then DECIDE_ADDR and DECIDE_MODELS replace Addr and ModelsDir when non-empty.
func FromEnvironment() (Config, error)
```
dev.json: `{"addr": "127.0.0.1:8085", "models_dir": "models"}`
test.json: `{"addr": "127.0.0.1:0", "models_dir": ""}`

## Done when
- `go test ./internal/config/` passes: both files load, an unknown name fails, an unknown field fails (test it by decoding through the same helper Load uses), and FromEnvironment honours each variable (use `t.Setenv`).

## Out of scope
Every other package.
~~~~

---

## Epic 3: Wiring

Done when: `app.New` builds the service from config; `apptest.New` exists; an example model evaluates.

### 3.1 Wire the decide app and its test harness

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Rules: `internal/app` is the only package that turns config into concrete objects. Dependencies are constructor arguments. No globals.

## Goal
One function that builds the real service from a config, a test harness that builds it with the test config, and one example model to prove it.

## Read first
- samples/decide/internal/config/config.go: Config, Load.
- samples/decide/internal/core/core.go: NewService.
- samples/decide/internal/engine/engine.go: New(x Expr).
- samples/decide/internal/zen/zen.go: Lang.
- samples/decide/internal/dirsource/dirsource.go: New(dir).

## Change
- samples/decide/internal/app/app.go and app_test.go.
- samples/decide/internal/apptest/apptest.go.
- samples/decide/models/example-shipping.json (below, verbatim).

## Contract
```go
package app
// New returns an error if cfg.ModelsDir is empty or is not a directory.
// It builds core.NewService(dirsource.New(cfg.ModelsDir), engine.New(zen.Lang{})).
func New(cfg config.Config) (*core.Service, error)

package apptest
// New builds the app from config.Load("test") with ModelsDir set to ModelsDir(t); t.Fatal on error.
func New(t testing.TB) *core.Service
// ModelsDir is <module root>/models, where the module root is the nearest ancestor of os.Getwd() containing go.mod.
func ModelsDir(t testing.TB) string
```
models/example-shipping.json:
```json
{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request"},
    {"id": "rate", "type": "decisionTableNode", "name": "Rate", "content": {
      "hitPolicy": "first",
      "inputs": [
        {"id": "country", "name": "Country", "field": "country"},
        {"id": "weight", "name": "Weight", "field": "weight"}
      ],
      "outputs": [{"id": "cost", "name": "Cost", "field": "cost"}],
      "rules": [
        {"_id": "r1", "country": "\"US\"", "weight": "<= 5", "cost": "5"},
        {"_id": "r2", "country": "\"US\"", "weight": "", "cost": "12"},
        {"_id": "r3", "country": "", "weight": "<= 5", "cost": "15"},
        {"_id": "r4", "country": "", "weight": "", "cost": "30"}
      ]
    }},
    {"id": "out", "type": "outputNode", "name": "Response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "rate"},
    {"id": "e2", "sourceId": "rate", "targetId": "out"}
  ]
}
```

## Done when
- `go vet ./... && go test ./...` passes.
- app_test, through apptest.New: example-shipping gives cost 5 for {"country":"US","weight":3}, 12 for US/10, 15 for FR/2, 30 for FR/9; Models includes "example-shipping"; an unknown key wraps core.ErrNotFound; app.New with an empty or missing ModelsDir fails.

## Out of scope
HTTP, CLI, `cmd/`.
~~~~

---

## Epic 4: Transports

Done when: HTTP server and CLI work through `apptest`; the chess models pass golden tests; README and ARCHITECTURE exist.

### 4.1 Add the decide HTTP server

~~~~text
Epic: 4 Transports
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are adding `internal/cli`, `cmd/decide`, `internal/golden`, README.md and ARCHITECTURE.md right now. Change only the files below.
- Rules: a transport decodes its own input and maps errors; it validates nothing else (business rules are in core.Service). A `main` is: load config, `app.New`, hand the service to the transport. Tests go through `apptest.New(t)`.

## Goal
Serve the service over HTTP. The chess sample calls this API.

## Read first
- samples/decide/internal/core/core.go: Service, ErrNotFound, ErrInvalid.
- samples/decide/internal/apptest/apptest.go: New.
- samples/decide/internal/config/config.go: FromEnvironment.

## Change
- samples/decide/internal/httpapi/httpapi.go and httpapi_test.go.
- samples/decide/cmd/decide-server/main.go.

## Contract
```go
package httpapi
func NewHandler(svc *core.Service) http.Handler
```
| Route | Request | Response |
|---|---|---|
| `POST /api/evaluate/{key}` | `{"context": {...}}` (missing context = `{}`; other fields ignored; body limit 1 MiB) | 200 `{"result": {...}}` |
| `GET /api/models` | | 200 `{"models": ["..."]}` |
| `GET /healthz` | | 200 `{"status": "ok"}` |
Errors are `{"error": "<message>"}` with 404 for core.ErrNotFound, 400 for core.ErrInvalid or an undecodable body or a non-object context, 500 otherwise. Every response is `Content-Type: application/json`.
main: `config.FromEnvironment()`, `app.New(cfg)`, `http.Server{Addr: cfg.Addr, Handler: httpapi.NewHandler(svc), ReadHeaderTimeout: 10 * time.Second}`, log `decide listening on http://<addr> (models <dir>)`, shut down gracefully on SIGINT/SIGTERM.

## Done when
- `go test ./internal/httpapi/` passes via apptest.New + httptest: evaluate example-shipping (US, 3 -> `{"result":{"cost":5}}`), unknown key 404, bad JSON 400, models list, healthz.
- `go build -o /dev/null ./cmd/decide-server` succeeds.

## Out of scope
CLI, models, README.
~~~~

### 4.2 Add the decide CLI

~~~~text
Epic: 4 Transports
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are adding `internal/httpapi`, `cmd/decide-server`, `internal/golden`, README.md and ARCHITECTURE.md right now. Change only the files below.
- Rules: a transport decodes its own input and maps errors; it validates nothing else. A `main` is: load config, `app.New`, hand the service to the transport. Tests go through `apptest.New(t)`.

## Goal
Evaluate a model from the command line: `decide eval KEY < input.json`.

## Read first
- samples/decide/internal/core/core.go: Service.
- samples/decide/internal/apptest/apptest.go: New.

## Change
- samples/decide/internal/cli/cli.go and cli_test.go.
- samples/decide/cmd/decide/main.go.

## Contract
```go
package cli
// Run returns the exit code.
func Run(ctx context.Context, svc *core.Service, args []string, stdin io.Reader, stdout, stderr io.Writer) int
```
- `models`: print each key on its own line; exit 0.
- `eval KEY`: read one JSON object from stdin (empty stdin = `{}`), print the result as indented JSON (two spaces) and a newline; exit 0.
- Any error: print `decide: <error>` to stderr, exit 1. No or unknown subcommand: usage to stderr, exit 2.
main: `config.FromEnvironment()`, `app.New(cfg)`, `os.Exit(cli.Run(ctx, svc, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))`.

## Done when
- `go test ./internal/cli/` passes via apptest.New: eval example-shipping with `{"country":"US","weight":3}` prints cost 5; models lists example-shipping; unknown key exits 1; no args exits 2.
- `go build -o /dev/null ./cmd/decide` succeeds.

## Out of scope
HTTP, models, README.
~~~~

### 4.3 Add the chess decision models with golden tests

~~~~text
Epic: 4 Transports
Depends on: none

## Environment
- Work in `samples/decide/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod and never run `go mod tidy`.
- Other workers are adding `internal/httpapi`, `internal/cli`, `cmd/`, README.md and ARCHITECTURE.md right now. Change only the files below.
- Tests go through `apptest.New(t)`.

## Goal
The three models the chess sample's opponent asks for, checked against fixed inputs. If a golden case fails because of an engine or expression bug, record the bug with `bin/codeplat issue comment` on this issue and fix the model only if the model is wrong.

## Read first
- samples/decide/internal/apptest/apptest.go: New.

## Change
- samples/decide/models/chess-opponent-policy.json, chess-illegal-move.json, chess-move-choice.json (verbatim below).
- samples/decide/internal/golden/chess_test.go (package golden; test-only).

## Contract
chess-opponent-policy.json:
```json
{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request"},
    {"id": "policy", "type": "decisionTableNode", "name": "Policy", "content": {
      "hitPolicy": "first",
      "inputs": [
        {"id": "d", "name": "Difficulty", "field": "difficulty"},
        {"id": "p", "name": "Phase", "field": "phase"},
        {"id": "c", "name": "In check", "field": "in_check"}
      ],
      "outputs": [
        {"id": "route", "name": "Route", "field": "route"},
        {"id": "temp", "name": "Temperature", "field": "temperature"},
        {"id": "persona", "name": "Persona", "field": "persona"},
        {"id": "n", "name": "Candidates", "field": "candidates"}
      ],
      "rules": [
        {"_id": "r1", "d": "\"easy\"", "p": "", "c": "", "route": "\"quick\"", "temp": "1.0", "persona": "\"You are a casual beginner. Answer fast.\"", "n": "1"},
        {"_id": "r2", "d": "", "p": "", "c": "true", "route": "\"general\"", "temp": "0.2", "persona": "\"You are in check. Find a move that gets your king out of check.\"", "n": "3"},
        {"_id": "r3", "d": "\"hard\"", "p": "", "c": "", "route": "\"general\"", "temp": "0.2", "persona": "\"You are a strong club player. Look for checks, captures and threats first.\"", "n": "5"},
        {"_id": "r4", "d": "", "p": "\"opening\"", "c": "", "route": "\"quick\"", "temp": "0.5", "persona": "\"You play sound openings: develop pieces, control the centre, castle early.\"", "n": "3"},
        {"_id": "r5", "d": "", "p": "", "c": "", "route": "\"general\"", "temp": "0.5", "persona": "\"You are a careful chess player.\"", "n": "3"}
      ]
    }},
    {"id": "out", "type": "outputNode", "name": "Response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "policy"},
    {"id": "e2", "sourceId": "policy", "targetId": "out"}
  ]
}
```
chess-illegal-move.json:
```json
{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request"},
    {"id": "recover", "type": "decisionTableNode", "name": "Recover", "content": {
      "hitPolicy": "first",
      "inputs": [
        {"id": "reason", "name": "Reason", "field": "reason"},
        {"id": "attempt", "name": "Attempt", "field": "attempt"},
        {"id": "d", "name": "Difficulty", "field": "difficulty"}
      ],
      "outputs": [{"id": "action", "name": "Action", "field": "action"}],
      "rules": [
        {"_id": "r1", "reason": "\"no_move\"", "attempt": ">= 2", "d": "", "action": "\"random\""},
        {"_id": "r2", "reason": "", "attempt": "< 3", "d": "", "action": "\"retry\""},
        {"_id": "r3", "reason": "", "attempt": "", "d": "\"easy\"", "action": "\"resign\""},
        {"_id": "r4", "reason": "", "attempt": "", "d": "", "action": "\"random\""}
      ]
    }},
    {"id": "out", "type": "outputNode", "name": "Response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "recover"},
    {"id": "e2", "sourceId": "recover", "targetId": "out"}
  ]
}
```
chess-move-choice.json:
```json
{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request"},
    {"id": "score", "type": "expressionNode", "name": "Score", "content": {
      "expressions": [
        {"id": "x1", "key": "score", "value": "captured_value * 10 + (check ? 3 : 0) + (mate ? 1000 : 0) + (promotion ? 8 : 0) - (attacked ? moved_value * 10 : 0) - rank"}
      ]
    }},
    {"id": "out", "type": "outputNode", "name": "Response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "score"},
    {"id": "e2", "sourceId": "score", "targetId": "out"}
  ]
}
```
Golden cases (input -> result):
- policy {difficulty normal, phase opening, in_check false} -> {route quick, temperature 0.5, candidates 3, persona "You play sound openings: ..."}
- policy {easy, middlegame, false} -> route quick, temperature 1, candidates 1
- policy {normal, middlegame, true} -> route general, temperature 0.2, candidates 3
- policy {hard, endgame, false} -> route general, temperature 0.2, candidates 5
- policy {normal, endgame, false} -> route general, temperature 0.5, candidates 3
- illegal {attempt 1, reason illegal, difficulty normal} -> action retry
- illegal {2, no_move, normal} -> random; {1, no_move, normal} -> retry; {3, illegal, easy} -> resign; {3, illegal, normal} -> random
- choice {capture true, captured_value 3, check false, mate false, promotion false, castle false, moved_value 1, attacked false, rank 0, difficulty normal} -> score 30
- choice {captured_value 0, check true, attacked true, moved_value 9, rank 1, rest false/0} -> score -88
- choice {mate true, rank 2, rest false/0} -> score 998
Numbers in results are float64.

## Done when
- `go test ./internal/golden/` passes with every case above, table-driven, through apptest.New(t).Evaluate.

## Out of scope
Engine and expression code; other models.
~~~~

### 4.4 Write the decide README and ARCHITECTURE

~~~~text
Epic: 4 Transports
Depends on: none

## Environment
- Work in `samples/decide/`. Other workers are adding `internal/httpapi`, `internal/cli`, `cmd/`, `internal/golden` and `models/chess-*.json` right now. Change only the two files below.

## Goal
Tell a reader how to run and call decide, and state its dependency rules.

## Read first
- samples/llmrouter/README.md: tone and layout.
- samples/decide/internal/config/config.go: variables.

## Change
- samples/decide/README.md: what it is (a JDM decision engine); Run (`make run`, listens on 127.0.0.1:8085, models from `models/`); Configuration table (DECIDE_ENV, DECIDE_ADDR, DECIDE_MODELS); HTTP API table (POST /api/evaluate/{key} with `{"context": {...}}` -> `{"result": {...}}`, GET /api/models, GET /healthz, errors `{"error"}` 404/400); a curl example evaluating `example-shipping`; CLI (`go run ./cmd/decide eval example-shipping <<< '{"country":"US","weight":3}'`, `models`); Supported JDM subset (node types, hit policy first, expression operators and functions, cell forms `> 5`, `"a", "b"`, `[1..5]`, `$` expressions; no function nodes, switch nodes or collect); the chess-* models are used by samples/chess.
- samples/decide/ARCHITECTURE.md: a Rules section: 1. config chooses, `app.New` wires (only internal/app constructs from config); 2. pure DI, constructor arguments, no globals or init registration; 3. entrypoints are thin (config, app.New, transport); 4. business rules live in core.Service, transports only decode and map errors; 5. ports are declared by their consumer (core.ModelSource, core.Engine, engine.Expr); tests go through apptest.New. Then a Layout list of packages, one line each.

## Done when
- Both files exist and every path, variable and route they mention exists in the code or in this issue.

## Out of scope
All Go code.
~~~~
