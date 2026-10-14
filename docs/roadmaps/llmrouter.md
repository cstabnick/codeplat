# Roadmap: llmrouter sample

## Goal

A sample project in `samples/llmrouter/`: one Go server that apps call like the OpenAI API to reach local models. Behind it are backends: an Ollama server, and any OpenAI-compatible server such as the Bonsai demo's `llama-server`. llmrouter starts a backend that is not answering when its config gives a start command, pulls missing Ollama models, and manages Ollama's models. A client that sends `"model": "auto"` gets a model chosen by routing: a small, fast intent model classifies the last user message into one of the configured routes, and each route names a backend and a model. A small HTMX page shows the backends, routes, models and recent routing decisions, and lets a person try a prompt and pull or delete models.

It is its own Go module, `llmrouter`, standard library only. Tracker project: `LLM`.

Facts checked against the local machine (Ollama 0.32.1):

- `GET /api/version` returns `{"version":"0.32.1"}`. `GET /api/tags` returns `{"models":[{"name","size","modified_at",...}]}`. `GET /api/ps` returns `{"models":[{"name","size_vram","expires_at",...}]}`.
- `DELETE /api/delete` with `{"model":"nope:1"}` returns 404 and `{"error":"model 'nope:1' not found"}`.
- `POST /api/chat` with `"stream": false` and `"format"` set to a JSON schema with an `enum` returns `{"message":{"content":"{\"route\": \"general\"}"}}`.
- `POST /v1/chat/completions` works, and an unknown model returns 404 with `{"error":{"message":...,"type":"not_found_error"}}`.
- The Bonsai demo at `/mnt/c/Users/cstab/Bonsai-demo` is a Windows install: `bin/cuda/llama-server.exe`, started by `scripts/start_llama_server.ps1`, which reads `PORT` (default 8080) and `BONSAI_HOST` (default 127.0.0.1). The model is `models/bonsai2-gguf/27B/Ternary-Bonsai-2-27B-PQ2_0.gguf` (7.2 GB). It needs the fork's binaries; stock llama.cpp and Ollama cannot run it. WSL here uses NAT networking, so a Windows server must bind to `0.0.0.0` and be reached at the Windows host's address.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists and the shared `llm` package compiles and passes its tests. | 1 |
| 2. Components | Both backend clients, the supervisor, the router, the gateway, the admin API, the UI and the config each pass their own tests. | 8, parallel |
| 3. Wiring | The server builds and its app package passes its tests; the README and example configs exist. | 2, parallel |
| 4. Verify | An end-to-end test passes against fake backends, and an opt-in live test exists for the real ones. | 2, parallel |

No issue has a dependency inside its epic.

Why four epics: every component imports the `llm` package (epic 1). The app package imports all eight components, so it cannot compile until epic 2 is done (epic 3). The end-to-end and live tests import the app package (epic 4).

The `llm` package is what makes epic 2 parallel. It fixes the `Backend`, `ModelManager`, `Router` and `Fleet` interfaces, and each component either implements one or consumes them, tested against fakes. No epic 2 package imports another epic 2 package.

---

## Epic 1: Foundation

### 1.1 Create the llmrouter module and its shared llm package

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. Create it; it does not exist yet.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Add no dependencies.
- `samples/bookmarks/` is a finished sample in the same style; you may read it for conventions, but do not edit it.

## Goal
Create the Go module for llmrouter, a server that routes OpenAI-style chat requests to local model backends, and the one package every other part imports: the shared types, errors and interfaces.

## Read first
- `samples/bookmarks/Makefile`: the Makefile style to follow.

## Change
- `samples/llmrouter/go.mod`: create it by running `go mod init llmrouter` in `samples/llmrouter/`.
- `samples/llmrouter/internal/llm/llm.go`: create it with exactly the content under Contract.
- `samples/llmrouter/internal/llm/llm_test.go`: package `llm`. Tests, using a fake type that implements `Backend` (and a second fake that also implements `ModelManager`):
  - `CheckBackend` on a backend whose `Health` fails returns `Reachable` false, `Error` set to the error text, and a non-nil empty `Loaded`.
  - `CheckBackend` on a reachable plain `Backend` returns `Reachable` true, its `Version`, and empty `Loaded`.
  - `CheckBackend` on a reachable `ModelManager` returns the loaded models it reports.
  - `CheckBackend` copies `Name`, `Kind` and the `managed` argument.
  - `NewFleet`: `Backends` returns them in order; `Backend("x")` finds by name and reports false for an unknown name; `Managed` uses the function and is false when the function is nil.
- `samples/llmrouter/Makefile`:

      .PHONY: test vet run

      test:
      	go test ./...

      vet:
      	go vet ./...

      run:
      	go run ./cmd/llmrouter

  (Recipe lines start with a tab. `cmd/llmrouter` does not exist yet; that is expected.)

## Contract
```go
// Package llm holds the types and interfaces shared by every part of
// llmrouter. It imports only the standard library.
package llm

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Errors returned by llmrouter's parts. Callers test for them with errors.Is.
var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid input")
	ErrUnavailable = errors.New("backend unavailable")
	ErrUnsupported = errors.New("not supported by this backend")
)

// AutoModel is the model name a client sends to let the router choose.
const AutoModel = "auto"

// Backend kinds.
const (
	KindOllama = "ollama" // an Ollama server
	KindOpenAI = "openai" // any OpenAI-compatible server, such as llama-server
)

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Route sends prompts of one intent to one model on one backend. The
// intent model sees Name and Description when it classifies a prompt.
type Route struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Backend     string `json:"backend"`
	Model       string `json:"model"`
}

// Decision records how one conversation was routed. Fallback is true when
// the default route was used because classification failed, and Error then
// says why.
type Decision struct {
	At       time.Time     `json:"at"`
	Prompt   string        `json:"prompt"`
	Route    string        `json:"route"`
	Backend  string        `json:"backend"`
	Model    string        `json:"model"`
	Fallback bool          `json:"fallback"`
	Error    string        `json:"error,omitempty"`
	Took     time.Duration `json:"took"`
}

// Model is a model a backend can serve.
type Model struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

// LoadedModel is a model a backend holds in memory.
type LoadedModel struct {
	Name      string    `json:"name"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Backend is a server that runs models.
type Backend interface {
	// Name is the backend's name in the config, such as "ollama".
	Name() string
	// Kind is KindOllama or KindOpenAI.
	Kind() string
	// Health returns nil when the backend answers, with the server's
	// version if it reports one. An error wraps ErrUnavailable.
	Health(ctx context.Context) (version string, err error)
	// ListModels returns the models the backend can serve.
	ListModels(ctx context.Context) ([]Model, error)
	// Classify asks model to pick one of labels for prompt, with system as
	// the system message, and returns the label it answered.
	Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error)
	// ChatCompletions sends body, an OpenAI chat-completions request, to
	// the backend's /v1/chat/completions and returns the response unread,
	// whatever its status. The caller closes the response Body. A request
	// that cannot reach the backend returns an error wrapping ErrUnavailable.
	ChatCompletions(ctx context.Context, body []byte) (*http.Response, error)
}

// ModelManager is implemented by backends that can install and remove
// models, such as Ollama.
type ModelManager interface {
	LoadedModels(ctx context.Context) ([]LoadedModel, error)
	// PullModel downloads a model and returns once it is installed.
	PullModel(ctx context.Context, name string) error
	// DeleteModel removes a model. An unknown model is an error wrapping ErrNotFound.
	DeleteModel(ctx context.Context, name string) error
}

// BackendStatus describes one backend.
type BackendStatus struct {
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Reachable bool          `json:"reachable"`
	Version   string        `json:"version"`
	Managed   bool          `json:"managed"`
	Loaded    []LoadedModel `json:"loaded"`
	Error     string        `json:"error,omitempty"`
}

// CheckBackend reports on b. managed says whether llmrouter started b's
// process. Loaded is filled only for a reachable ModelManager, and is never nil.
func CheckBackend(ctx context.Context, b Backend, managed bool) BackendStatus {
	s := BackendStatus{Name: b.Name(), Kind: b.Kind(), Managed: managed, Loaded: []LoadedModel{}}
	v, err := b.Health(ctx)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.Reachable, s.Version = true, v
	if m, ok := b.(ModelManager); ok {
		loaded, err := m.LoadedModels(ctx)
		if err != nil {
			s.Error = err.Error()
			return s
		}
		if loaded != nil {
			s.Loaded = loaded
		}
	}
	return s
}

// Router chooses a route for each conversation.
type Router interface {
	// Route classifies the conversation and records the decision. It never
	// fails: when classification fails it returns the default route with
	// Fallback set.
	Route(ctx context.Context, messages []Message) Decision
	// Routes returns the configured routes in config order.
	Routes() []Route
	// DefaultRoute returns the name of the default route.
	DefaultRoute() string
	// Recent returns up to n of the latest decisions, newest first. n <= 0
	// returns every decision kept.
	Recent(n int) []Decision
}

// Fleet is the configured set of backends.
type Fleet interface {
	// Backends returns every backend in config order.
	Backends() []Backend
	// Backend returns the backend with this name.
	Backend(name string) (Backend, bool)
	// Managed reports whether llmrouter started this backend's process and
	// it is still running.
	Managed(name string) bool
}

// NewFleet returns a Fleet over backends, whose names must be distinct.
// managed may be nil, which means no backend is managed.
func NewFleet(backends []Backend, managed func(name string) bool) Fleet {
	return fleet{backends: append([]Backend(nil), backends...), managed: managed}
}

type fleet struct {
	backends []Backend
	managed  func(string) bool
}

func (f fleet) Backends() []Backend { return append([]Backend(nil), f.backends...) }

func (f fleet) Backend(name string) (Backend, bool) {
	for _, b := range f.backends {
		if b.Name() == name {
			return b, true
		}
	}
	return nil, false
}

func (f fleet) Managed(name string) bool { return f.managed != nil && f.managed(name) }
```

## Done when
- `go vet ./...` succeeds in `samples/llmrouter/`.
- `go test ./internal/llm/` passes.
- `gofmt -l .` prints nothing.

## Out of scope
- Every other package. Later issues create `internal/ollama`, `internal/oaiclient`, `internal/supervisor`, `internal/router`, `internal/gateway`, `internal/admin`, `internal/ui`, `internal/config`, `internal/app` and `cmd/llmrouter`.
- A README; a later issue writes it.
~~~~

---

## Epic 2: Components

All eight issues begin with the same Environment section:

```
## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.
```

### 2.1 Add the Ollama backend client

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
An HTTP client for an Ollama server that implements `llm.Backend` and `llm.ModelManager`, so llmrouter can check Ollama's health, list, pull and delete its models, classify a prompt with a small model, and forward chat requests to it.

## Read first
- `internal/llm/llm.go`: the `Backend` and `ModelManager` interfaces you implement, `Model`, `LoadedModel`, `KindOllama`, and the errors `ErrUnavailable`, `ErrNotFound`, `ErrInvalid`.

## Change
- `internal/ollama/ollama.go`: package `ollama`, with:

      // Client talks to one Ollama server.
      type Client struct { /* name, base URL, *http.Client */ }

      // New returns a client for the Ollama server at baseURL, such as
      // "http://127.0.0.1:11434". A trailing slash is ignored. hc nil means
      // a new http.Client with no timeout; callers bound calls with ctx.
      func New(name, baseURL string, hc *http.Client) *Client

  and these methods, each sending its request with `http.NewRequestWithContext`:
  - `Name() string` returns `name`; `Kind() string` returns `llm.KindOllama`.
  - `Health(ctx) (string, error)`: `GET /api/version`, response `{"version":"0.32.1"}`; returns the version.
  - `ListModels(ctx) ([]llm.Model, error)`: `GET /api/tags`, response `{"models":[{"name":"llama3:8b","size":4661224676,"modified_at":"2026-08-23T16:59:52.16993965-04:00"}]}` (other fields are present and ignored). Never returns nil on success.
  - `LoadedModels(ctx) ([]llm.LoadedModel, error)`: `GET /api/ps`, response `{"models":[{"name":"llama3:8b","size_vram":5000000000,"expires_at":"2026-10-14T23:00:00Z"}]}`. Never returns nil on success.
  - `PullModel(ctx, name) error`: an empty name is an error wrapping `llm.ErrInvalid`, sent nowhere. Otherwise `POST /api/pull` with body `{"model":NAME,"stream":false}`; success is status 200.
  - `DeleteModel(ctx, name) error`: `DELETE /api/delete` with body `{"model":NAME}`; 200 is success; 404 is an error wrapping `llm.ErrNotFound`.
  - `Classify(ctx, model, system, prompt string, labels []string) (string, error)`: `POST /api/chat` with this body:

        {"model": MODEL, "stream": false, "keep_alive": "30m",
         "options": {"temperature": 0},
         "format": {"type": "object",
                    "properties": {"route": {"type": "string", "enum": LABELS}},
                    "required": ["route"]},
         "messages": [{"role": "system", "content": SYSTEM},
                      {"role": "user", "content": PROMPT}]}

    The response is `{"message":{"role":"assistant","content":"{\"route\": \"code\"}"},"done":true}`. Decode `message.content` as JSON `{"route": string}` and return the route. Content that is not that JSON is an error that includes the content.
  - `ChatCompletions(ctx, body []byte) (*http.Response, error)`: `POST /v1/chat/completions` with `body` as is and `Content-Type: application/json`. Return the response unread for any status.
  - Error rules for every method: when `hc.Do` itself fails, return `fmt.Errorf("%w: %v", llm.ErrUnavailable, err)`. When a JSON method (all but `ChatCompletions`) gets a status other than those named above as success, read the body, take its `"error"` string if it is JSON `{"error": "..."}`, and return an error like `ollama GET /api/tags: 500 Internal Server Error: <message>`. Close every response body you read.
- `internal/ollama/ollama_test.go`: package `ollama`. One `httptest.Server` per test with a handler that checks the method, path and decoded request body and writes the response shown above. Cover every method's success; `DeleteModel` 404 wraps `llm.ErrNotFound`; a 500 on `ListModels` returns an error containing the server's message; `PullModel("")` returns `llm.ErrInvalid` without a request; `Classify` sends the `format` enum and messages above and returns the label; `Classify` with content `"not json"` errors; `ChatCompletions` passes the body through unchanged and returns a 404 response without error; a client whose base URL points at a closed server returns an error wrapping `llm.ErrUnavailable` from `Health`. Add `var _ llm.Backend = (*Client)(nil)` and `var _ llm.ModelManager = (*Client)(nil)` in the test file.

## Contract
`ollama.New(name, baseURL string, hc *http.Client) *Client`, and `*Client` implements `llm.Backend` and `llm.ModelManager` exactly as declared in `internal/llm/llm.go`.

## Done when
- `go test ./internal/ollama/` passes.
- `go vet ./internal/ollama/` succeeds and `gofmt -l internal/ollama` prints nothing.

## Out of scope
- Every other package, especially `internal/oaiclient`, which another worker is writing at the same time.
- Streaming pulls and progress reporting.
~~~~

### 2.2 Add the OpenAI-compatible backend client

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
An HTTP client for any OpenAI-compatible server, such as llama.cpp's `llama-server` running the Bonsai model, that implements `llm.Backend`. It cannot install models, so it does not implement `llm.ModelManager`.

## Read first
- `internal/llm/llm.go`: the `Backend` interface you implement, `Model`, `KindOpenAI`, and `ErrUnavailable`.

## Change
- `internal/oaiclient/oaiclient.go`: package `oaiclient`, with:

      // Client talks to one OpenAI-compatible server.
      type Client struct { /* name, base URL, *http.Client */ }

      // New returns a client for the server at baseURL, such as
      // "http://127.0.0.1:8085". A trailing slash is ignored. hc nil means a
      // new http.Client with no timeout; callers bound calls with ctx.
      func New(name, baseURL string, hc *http.Client) *Client

  and these methods, each sending its request with `http.NewRequestWithContext`:
  - `Name() string` returns `name`; `Kind() string` returns `llm.KindOpenAI`.
  - `Health(ctx) (string, error)`: `GET /v1/models`. Status 200 returns `"", nil`. Any other status returns `fmt.Errorf("%w: GET /v1/models: %s", llm.ErrUnavailable, resp.Status)` (llama-server answers 503 while it is still loading a model).
  - `ListModels(ctx) ([]llm.Model, error)`: `GET /v1/models`, response `{"object":"list","data":[{"id":"Ternary-Bonsai-2-27B-PQ2_0.gguf","object":"model","created":1792017950,"owned_by":"llamacpp"}]}`. Return one `llm.Model{Name: id}` per entry, never nil on success.
  - `Classify(ctx, model, system, prompt string, labels []string) (string, error)`: `POST /v1/chat/completions` with this body:

        {"model": MODEL, "stream": false, "temperature": 0, "max_tokens": 64,
         "response_format": {"type": "json_schema",
           "json_schema": {"name": "route", "strict": true,
             "schema": {"type": "object",
                        "properties": {"route": {"type": "string", "enum": LABELS}},
                        "required": ["route"]}}},
         "messages": [{"role": "system", "content": SYSTEM},
                      {"role": "user", "content": PROMPT}]}

    The response is `{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"route\":\"code\"}"},"finish_reason":"stop"}]}`. Decode `choices[0].message.content` as JSON `{"route": string}` and return the route. No choices, or content that is not that JSON, is an error that includes the content.
  - `ChatCompletions(ctx, body []byte) (*http.Response, error)`: `POST /v1/chat/completions` with `body` as is and `Content-Type: application/json`. Return the response unread for any status.
  - Error rules: when `hc.Do` itself fails, return `fmt.Errorf("%w: %v", llm.ErrUnavailable, err)`. When `ListModels` or `Classify` gets a status other than 200, read the body and return an error like `openai POST /v1/chat/completions: 400 Bad Request: <body, cut to 200 bytes>`. Close every response body you read.
- `internal/oaiclient/oaiclient_test.go`: package `oaiclient`. One `httptest.Server` per test that checks method, path and decoded request body. Cover: `Health` 200 and 503 (the 503 error wraps `llm.ErrUnavailable`); `Health` against a closed server wraps `llm.ErrUnavailable`; `ListModels` maps ids; `Classify` sends `response_format` with the enum and both messages and returns the label; `Classify` with no choices errors; `Classify` with a 400 errors with the body text; `ChatCompletions` passes the body through and returns a non-200 response without error. Add `var _ llm.Backend = (*Client)(nil)` in the test file.

## Contract
`oaiclient.New(name, baseURL string, hc *http.Client) *Client`, and `*Client` implements `llm.Backend` exactly as declared in `internal/llm/llm.go`. It does not implement `llm.ModelManager`.

## Done when
- `go test ./internal/oaiclient/` passes.
- `go vet ./internal/oaiclient/` succeeds and `gofmt -l internal/oaiclient` prints nothing.

## Out of scope
- Every other package, especially `internal/ollama`, which another worker is writing at the same time.
- Starting the server process; another issue does that.
~~~~

### 2.3 Add the backend process supervisor

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
A supervisor that makes sure one backend server is answering: if it is not, it starts the backend's command, such as `ollama serve`, waits until the backend is healthy, and stops it again on shutdown. Plus a helper that pulls the models a backend is missing.

## Read first
- `internal/llm/llm.go`: `ErrUnavailable` and the `Model` type.

## Change
- `internal/supervisor/supervisor.go`: package `supervisor`, with exactly these exported names:

      // Options configure a Supervisor.
      type Options struct {
      	// Name names the backend in log lines.
      	Name string
      	// Health returns nil when the backend answers.
      	Health func(ctx context.Context) error
      	// Command is the process to start when the backend does not answer,
      	// such as []string{"ollama", "serve"}. Empty means never start one.
      	Command []string
      	// Dir is the process's working directory; "" means the current one.
      	Dir string
      	// Env is added to the process's environment, as "KEY=value".
      	Env []string
      	// StopCommand, if set, runs after the process is stopped. It is for
      	// servers that outlive the process that started them, such as a
      	// Windows server started through powershell.exe from WSL.
      	StopCommand []string
      	// StartTimeout bounds the wait for a started backend to answer.
      	// Zero means 120 seconds.
      	StartTimeout time.Duration
      	// PollInterval is how often Health is called while waiting. Zero
      	// means 500 milliseconds.
      	PollInterval time.Duration
      	// Log receives progress lines. Nil means log.Default().
      	Log *log.Logger
      }

      type Supervisor struct { /* ... */ }

      func New(opts Options) *Supervisor

      // Start makes sure the backend answers. If Health succeeds it returns
      // nil and starts nothing. If it fails and Command is empty, it returns
      // an error wrapping llm.ErrUnavailable. Otherwise it starts Command and
      // calls Health every PollInterval until it succeeds (return nil), the
      // process exits (return an error wrapping llm.ErrUnavailable), or
      // StartTimeout passes or ctx ends (stop the process, then return an
      // error wrapping llm.ErrUnavailable).
      func (s *Supervisor) Start(ctx context.Context) error

      // Managed reports whether Start started the process and it is still
      // running.
      func (s *Supervisor) Managed() bool

      // Stop stops the process Start started: it sends os.Interrupt, waits up
      // to 5 seconds for it to exit, then kills it. It then runs StopCommand
      // if set, waiting up to 30 seconds. Stop does nothing if Start started
      // nothing, and is safe to call more than once.
      func (s *Supervisor) Stop() error

      // Puller is what EnsureModels needs from a backend.
      type Puller interface {
      	ListModels(ctx context.Context) ([]llm.Model, error)
      	PullModel(ctx context.Context, name string) error
      }

      // EnsureModels pulls each of names that m does not list, in order. A
      // name with no ":" also matches an installed name+":latest". It stops
      // at the first error and returns it wrapped as "pull NAME: ...".
      func EnsureModels(ctx context.Context, m Puller, names []string) error

  Notes: start the process with `exec.Command` (not `CommandContext`; the process must outlive the ctx passed to Start), set `cmd.Env = append(os.Environ(), opts.Env...)`, send its stdout and stderr to `os.Stderr`, and call `cmd.Wait` in one goroutine that closes a channel when the process exits. Guard shared state with a `sync.Mutex`. Log one line when starting, when healthy, and when stopping, prefixed with the name.
- `internal/supervisor/supervisor_test.go`: package `supervisor`. Use the standard helper-process pattern: a `TestHelperProcess` that returns at once unless `GO_WANT_HELPER_PROCESS=1`; when set, it reads the mode from its last argument: `"serve"` waits for `os.Interrupt` with `signal.Notify` and then `os.Exit(0)`; `"exit"` calls `os.Exit(3)`. Build commands as `[]string{os.Args[0], "-test.run=TestHelperProcess", "--", MODE}` with `Env: []string{"GO_WANT_HELPER_PROCESS=1"}`. Use a `PollInterval` of 10ms. Tests:
  - Health already succeeds: `Start` returns nil, `Managed` is false, `Stop` returns nil.
  - Health fails and `Command` is empty: the error wraps `llm.ErrUnavailable`.
  - Health fails for the first 3 calls, then succeeds, mode `"serve"`: `Start` returns nil, `Managed` is true; after `Stop`, `Managed` is false; a second `Stop` returns nil.
  - Mode `"exit"`, Health always fails: the error wraps `llm.ErrUnavailable`.
  - Mode `"serve"`, Health always fails, `StartTimeout` 200ms: the error wraps `llm.ErrUnavailable` and `Managed` is false afterwards.
  - `StopCommand` runs: give it a helper command in mode `"exit"` and check `Stop` still returns (its error may be reported or logged, but must not hang).
  - `EnsureModels` with a fake `Puller` listing `llama3:8b` and `qwen:latest`, names `["llama3:8b", "qwen", "llama3.2:1b"]`: pulls only `llama3.2:1b`. A pull error stops the loop and is returned with the model name in its text.

## Contract
The exported names above, verbatim.

## Done when
- `go test ./internal/supervisor/` passes, and passes again with `-count=3`.
- `go vet ./internal/supervisor/` succeeds and `gofmt -l internal/supervisor` prints nothing.

## Out of scope
- Every other package. Do not import `internal/ollama` or `internal/oaiclient`; `Health` and `Puller` keep this package independent of them.
~~~~

### 2.4 Add the intent router

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
A router that implements `llm.Router`: it asks a small intent model which configured route fits the last user message, falls back to the default route when that fails, and keeps a history of recent decisions.

## Read first
- `internal/llm/llm.go`: the `Router` interface you implement, and `Route`, `Decision`, `Message`, `ErrInvalid`.

## Change
- `internal/router/router.go`: package `router`, with exactly these exported names:

      // Classifier is what the router needs from a backend.
      type Classifier interface {
      	Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error)
      }

      type Options struct {
      	// IntentBackend and IntentModel name the model that classifies prompts.
      	IntentBackend string
      	IntentModel   string
      	Routes        []llm.Route
      	// Default is the name of the route used when classification fails.
      	Default string
      	// Timeout bounds one classification. Zero means 15 seconds.
      	Timeout time.Duration
      	// History is how many decisions Recent keeps. Zero means 100.
      	History int
      	// Now returns the current time. Nil means time.Now.
      	Now func() time.Time
      }

      type Router struct { /* ... */ }

      // New returns a router. classifiers maps each backend name to its
      // backend. It returns an error wrapping llm.ErrInvalid when
      // IntentModel is empty, IntentBackend is not in classifiers, Routes is
      // empty, a route's Name, Backend or Model is empty, two routes share a
      // Name, a route's Backend is not in classifiers, or Default is not a
      // route's Name.
      func New(classifiers map[string]Classifier, opts Options) (*Router, error)

      // SystemPrompt is the system message the intent model gets.
      func SystemPrompt(routes []llm.Route) string

  and the four `llm.Router` methods on `*Router`.
- `SystemPrompt` returns exactly, for routes `code` ("Writing or debugging code.") and `general` ("Anything else."):

      You route chat messages. Pick the one route that fits the user's message best.

      Routes:
      - code: Writing or debugging code.
      - general: Anything else.

      Answer with JSON: {"route": "<name>"}

  (One line per route in order, a blank line before "Routes:" and before "Answer", no trailing newline.)
- `Route(ctx, messages)`:
  1. The prompt is the `Content` of the last message whose `Role` is `"user"`. If there is none, return the default route with `Fallback` true and `Error` "no user message", without classifying.
  2. If there is only one route, return it with `Fallback` false, without classifying.
  3. Otherwise call `classifiers[IntentBackend].Classify` with a context bounded by `Timeout`, `IntentModel`, `SystemPrompt(routes)`, the prompt, and the route names in order. Match the answer to a route name after `strings.TrimSpace`, ignoring case. A classify error gives the default route, `Fallback` true and `Error` the error's text; an answer matching no route gives the default route, `Fallback` true and `Error` `unknown route "<answer>"`.
  4. The decision's `At` is `Now()` when Route began, `Took` is the time since then, `Prompt` is the prompt cut to its first 200 runes, and `Route`, `Backend` and `Model` come from the chosen route.
  5. Record the decision, keeping the latest `History`, then return it.
- `Routes()` returns a copy. `DefaultRoute()` returns `Default`. `Recent(n)` returns newest first, never nil. All methods are safe for concurrent use (use a `sync.Mutex` for the history).
- `internal/router/router_test.go`: package `router`, with a fake `Classifier` that records its arguments and returns a set answer or error, and a fake clock. Cover every `New` validation error; the `SystemPrompt` text above exactly; a classified route; case and space in the answer; an unknown answer; a classify error; no user message; a single route skipping classification; the classifier getting the right model, system prompt, the last user message and the labels; a prompt over 200 runes cut to 200; `Recent` order, `n` limits, `History` cap; a classifier that blocks until its ctx ends returning a fallback after `Timeout` (use 20ms); `var _ llm.Router = (*Router)(nil)`. Run the concurrency case under `go test -race` if your toolchain supports it.

## Contract
The exported names above, verbatim, and `*Router` implements `llm.Router` as declared in `internal/llm/llm.go`.

## Done when
- `go test ./internal/router/` passes.
- `go vet ./internal/router/` succeeds and `gofmt -l internal/router` prints nothing.

## Out of scope
- Every other package. Do not import `internal/ollama` or `internal/oaiclient`; `Classifier` keeps this package independent of them.
~~~~

### 2.5 Add the OpenAI-compatible gateway handler

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
The HTTP handler apps call: `POST /v1/chat/completions` and `GET /v1/models` in OpenAI's format. It picks a backend and model for each request, routing `"model": "auto"` through the router, and streams the backend's response back unchanged.

## Read first
- `internal/llm/llm.go`: `Fleet`, `Backend`, `Router`, `Message`, `AutoModel`, `ErrUnavailable`, and `NewFleet`, which your tests can use to build a fleet of fake backends.

## Change
- `internal/gateway/gateway.go`: package `gateway`, with

      // NewHandler serves the OpenAI-compatible API over the fleet's
      // backends, choosing models with r. logger nil means log.Default().
      func NewHandler(f llm.Fleet, r llm.Router, logger *log.Logger) http.Handler

  serving:
  - `POST /v1/chat/completions`:
    1. Read the body through `http.MaxBytesReader` with an 8 MiB limit, and decode it into `map[string]json.RawMessage`. Failure is a 400.
    2. Read `"model"` as a string ("" if absent) and `"messages"` as a list of `{role, content}`. `content` may be a JSON string, `null` (empty), or an array of parts like `[{"type":"text","text":"hi"},{"type":"image_url",...}]`: join the `text` of the `"text"` parts with "\n". Make an `[]llm.Message`.
    3. Choose route, backend and model:
       - model "" or `llm.AutoModel`: `d := r.Route(ctx, messages)`; use `d.Route`, `d.Backend`, `d.Model`.
       - model equal to the `Name` of a route in `r.Routes()`: that route's name, backend and model.
       - model `"<backend>/<rest>"` where `<backend>` is a name `f.Backend` knows: that backend, model `<rest>`, no route.
       - anything else: the backend of the default route (the route in `r.Routes()` named `r.DefaultRoute()`), the model unchanged, no route.
       If the backend name is not in the fleet, that is a 500.
    4. Set `"model"` in the map to the chosen model, marshal the map, and call the backend's `ChatCompletions`. An error wrapping `llm.ErrUnavailable` is a 503; any other error is a 502.
    5. Copy the response's `Content-Type` and `Cache-Control` headers, set `X-Llmrouter-Backend` and `X-Llmrouter-Model`, and `X-Llmrouter-Route` when there is a route, then write the backend's status code. Copy the body in a loop with a 32 KiB buffer, calling `http.NewResponseController(w).Flush()` after each write so streamed (`"stream": true`) responses reach the client as they arrive. Close the body.
  - `GET /v1/models`: `{"object":"list","data":[...]}` where each entry is `{"id":ID,"object":"model","created":0,"owned_by":OWNER}`: first `auto` (owner `llmrouter`), then each route name (owner `llmrouter`), then for each backend in `f.Backends()` each of its `ListModels` as `"<backend>/<model>"` (owner the backend name). A backend whose `ListModels` fails is logged and skipped.
  - Errors are written as OpenAI does: status code, `Content-Type: application/json`, body `{"error":{"message":MSG,"type":TYPE}}` with TYPE `invalid_request_error` for 400 and `server_error` otherwise.
- `internal/gateway/gateway_test.go`: package `gateway`. Use fake backends (implementing `llm.Backend`; their `ChatCompletions` records the body and returns a canned `*http.Response`) in `llm.NewFleet`, and a fake `llm.Router`. Serve with `httptest.NewServer`. Cover every model-choice rule (check the model the backend received and the three response headers); string, null and array `content`; invalid JSON 400; unavailable 503; other error 502; the backend's status and body passed through (e.g. 404); a streamed response: a backend whose body is an `io.Pipe` writing `data: {...}\n\n` lines one at a time, where the client reads the first line before the backend writes the second; `/v1/models` order and the skipped failing backend; that fields other than `model` (e.g. `"temperature": 0.2`, `"stream": true`) reach the backend unchanged.

## Contract
`gateway.NewHandler(f llm.Fleet, r llm.Router, logger *log.Logger) http.Handler`. Response headers `X-Llmrouter-Route`, `X-Llmrouter-Backend`, `X-Llmrouter-Model`.

## Done when
- `go test ./internal/gateway/` passes.
- `go vet ./internal/gateway/` succeeds and `gofmt -l internal/gateway` prints nothing.

## Out of scope
- Every other package. Use only fakes; do not import `internal/router`, `internal/ollama` or `internal/oaiclient`.
- The OpenAI `/v1/completions` and `/v1/embeddings` endpoints.
~~~~

### 2.6 Add the admin JSON API

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
A JSON API under `/api/` for managing llmrouter: backend status, listing, pulling and deleting models, the routes, recent routing decisions, and a dry-run of routing for a prompt.

## Read first
- `internal/llm/llm.go`: `Fleet`, `Backend`, `ModelManager`, `Router`, `CheckBackend`, `NewFleet`, and the four errors.

## Change
- `internal/admin/admin.go`: package `admin`, with

      // NewHandler serves the admin API over the fleet and router.
      func NewHandler(f llm.Fleet, r llm.Router) http.Handler

  serving (use Go 1.22+ `http.ServeMux` patterns):
  - `GET /api/backends`: `[]llm.BackendStatus`, one `llm.CheckBackend(ctx, b, f.Managed(b.Name()))` per backend in order.
  - `GET /api/backends/{backend}/models`: that backend's `ListModels`.
  - `POST /api/backends/{backend}/models/pull`, body `{"name":"llama3.2:1b"}`: the backend must be an `llm.ModelManager`, else an error wrapping `llm.ErrUnsupported`. An empty name wraps `llm.ErrInvalid`. On success, 200 `{"status":"success"}`.
  - `DELETE /api/backends/{backend}/models/{name...}` (model names contain ":" and may contain "/"): `ModelManager.DeleteModel`; 204 with no body on success.
  - `GET /api/routes`: `{"default": r.DefaultRoute(), "routes": r.Routes()}`.
  - `GET /api/decisions?n=20`: `r.Recent(n)`; `n` defaults to 50; a non-integer `n` is a 400.
  - `POST /api/route`, body `{"prompt":"..."}`: `r.Route(ctx, []llm.Message{{Role: "user", Content: prompt}})` as JSON. An empty prompt is a 400. (The router records this decision like any other.)
  - An unknown `{backend}` is a 404.
  - Decode bodies with `DisallowUnknownFields`; bad JSON is a 400.
  - Errors are `{"error": "<text>"}` with status: `llm.ErrInvalid` or `llm.ErrUnsupported` 400, `llm.ErrNotFound` 404, `llm.ErrUnavailable` 503, anything else 500. Every response sets `Content-Type: application/json` except the 204.
- `internal/admin/admin_test.go`: package `admin`. Fake backends (one plain `llm.Backend`, one that also implements `llm.ModelManager`) in `llm.NewFleet`, a fake `llm.Router`, and `httptest`. Cover each route's success and every error mapping listed above, including pull on the plain backend (400), delete of an unknown model (404), a name with a slash (`hf.co/user/model:Q4`) reaching `DeleteModel` intact, `n=abc` (400), and an unreachable backend in `/api/backends` (200, `reachable` false).

## Contract
`admin.NewHandler(f llm.Fleet, r llm.Router) http.Handler` and the routes above, verbatim.

## Done when
- `go test ./internal/admin/` passes.
- `go vet ./internal/admin/` succeeds and `gofmt -l internal/admin` prints nothing.

## Out of scope
- Every other package; use only fakes.
- Authentication; llmrouter listens on localhost.
~~~~

### 2.7 Add the HTMX web page

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
One web page for a person running llmrouter: it shows the backends, routes, models and recent routing decisions, lets them try routing a prompt, and lets them pull and delete models on backends that support it.

## Read first
- `internal/llm/llm.go`: `Fleet`, `Backend`, `ModelManager`, `Router`, `Decision`, `CheckBackend`, `NewFleet`.
- `../bookmarks/internal/ui/ui.go` and `../bookmarks/internal/ui/page.html`: the pattern to follow: `//go:embed page.html`, `template.Must(template.ParseFS(...))`, a `view` struct, render into a `bytes.Buffer` before writing. Do not edit them.

## Change
- `internal/ui/ui.go`: package `ui`, with

      // NewHandler serves the web page over the fleet and router.
      func NewHandler(f llm.Fleet, r llm.Router) http.Handler

  serving:
  - `GET /{$}`: the whole page.
  - `POST /try`, form field `prompt`: `r.Route(ctx, []llm.Message{{Role: "user", Content: prompt}})`, then the whole page with the result shown in the try section. An empty prompt shows the error "Enter a prompt." there instead.
  - `POST /models/pull`, form fields `backend` and `name`: pull through `llm.ModelManager`, then the whole page with "Pulled NAME." or the error shown in the models section.
  - `POST /models/delete`, form fields `backend` and `name`: delete through `llm.ModelManager`, then the whole page with "Deleted NAME." or the error.
  The POST handlers always answer 200 with the page, so the form works with and without JavaScript. An unknown backend or one that is not a `ModelManager` is an error message on the page, not a status code.
- `internal/ui/page.html`: one template. Load `<script src="https://unpkg.com/htmx.org@2.0.4"></script>`. Sections, each with this exact `id`:
  - `backends`: a table of `llm.CheckBackend` results: name, kind, reachable, version, managed, loaded model names, error. The section has `hx-get="/" hx-trigger="every 5s" hx-select="#backends" hx-swap="outerHTML"`.
  - `routes`: name, description, backend, model; the default route marked "(default)".
  - `models`: for each backend its `ListModels` (or its error text), with, for `ModelManager` backends only, a delete button per model and a pull form. Each form has `method="post"`, an `action`, and `hx-post` to the same URL with `hx-target="#models" hx-select="#models" hx-swap="outerHTML"`. The message from the last pull or delete is shown at the top of the section.
  - `try`: a form with a `prompt` textarea posting to `/try` with `hx-target="#try" hx-select="#try" hx-swap="outerHTML"`, and the result: route, backend, model, fallback, error, took.
  - `decisions`: the latest 20 decisions (`r.Recent(20)`): time (`15:04:05`), route, backend, model, fallback, took, prompt. The section has `hx-get="/" hx-trigger="every 5s" hx-select="#decisions" hx-swap="outerHTML"`.
  Keep the styling small and inline in a `<style>` block; it must work in light and dark mode (`@media (prefers-color-scheme: dark)`).
- `internal/ui/ui_test.go`: package `ui`. Fake backends (one plain, one `ModelManager`) and a fake `llm.Router`, with `httptest`. Cover: the page lists each backend, route (with "(default)"), model and decision; delete buttons and the pull form appear only for the `ModelManager` backend; `/try` shows the decision's route; empty prompt shows "Enter a prompt."; pull and delete call the fake and show their message; delete on the plain backend shows an error and answers 200; a backend whose `ListModels` fails shows its error text; HTML in a prompt is escaped.

## Contract
`ui.NewHandler(f llm.Fleet, r llm.Router) http.Handler`, routes `GET /`, `POST /try`, `POST /models/pull`, `POST /models/delete`, and section ids `backends`, `routes`, `models`, `try`, `decisions`.

## Done when
- `go test ./internal/ui/` passes.
- `go vet ./internal/ui/` succeeds and `gofmt -l internal/ui` prints nothing.

## Out of scope
- Every other package; use only fakes.
- The `/api/` and `/v1/` routes; other handlers serve them.
~~~~

### 2.8 Add the config loader

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`, and do not edit `internal/llm/`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and do not import any other package under `internal/` except `llmrouter/internal/llm`.
- Tests must never reach a real Ollama or llama-server. Use `net/http/httptest` servers and hand-written fakes.

## Goal
The config llmrouter runs from: its listen address, its backends and how to start them, the intent model, and the routes. It loads from a JSON file, has a working default for a machine with Ollama, and validates everything.

## Read first
- `internal/llm/llm.go`: `Route`, `KindOllama`, `KindOpenAI`, `AutoModel`, `ErrInvalid`.

## Change
- `internal/config/config.go`: package `config`, with exactly:

      // Backend is one model server.
      type Backend struct {
      	Name string `json:"name"`
      	// Kind is llm.KindOllama or llm.KindOpenAI.
      	Kind string `json:"kind"`
      	URL  string `json:"url"`
      	// Command starts the backend when it does not answer. Empty means
      	// llmrouter never starts it.
      	Command     []string `json:"command,omitempty"`
      	Dir         string   `json:"dir,omitempty"`
      	Env         []string `json:"env,omitempty"`
      	StopCommand []string `json:"stop_command,omitempty"`
      	// StartTimeoutSeconds bounds the wait after starting; 0 means the
      	// supervisor's default.
      	StartTimeoutSeconds int `json:"start_timeout_seconds,omitempty"`
      	// PullMissing pulls the intent and route models this backend lacks
      	// at startup. Only for kind ollama.
      	PullMissing bool `json:"pull_missing,omitempty"`
      }

      // Intent names the model that classifies prompts.
      type Intent struct {
      	Backend string `json:"backend"`
      	Model   string `json:"model"`
      	// TimeoutSeconds bounds one classification; 0 means the router's default.
      	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
      }

      type Config struct {
      	Addr         string      `json:"addr"`
      	Backends     []Backend   `json:"backends"`
      	Intent       Intent      `json:"intent"`
      	DefaultRoute string      `json:"default_route"`
      	Routes       []llm.Route `json:"routes"`
      }

      // Default is the config used when no file is given.
      func Default() Config

      // Load reads the JSON file at path, or uses Default() when path is "".
      // An empty Addr becomes "127.0.0.1:8083". The environment variable
      // LLMROUTER_ADDR, when set, replaces Addr. The result is validated.
      func Load(path string) (Config, error)

      // Validate returns an error wrapping llm.ErrInvalid for the first
      // problem it finds, naming the field, such as
      // `routes[1].backend: unknown backend "x"`.
      func (c Config) Validate() error

      // ModelsFor returns the models the given backend must have: the intent
      // model if it is on that backend, then each route's model on it, in
      // order and without repeats.
      func (c Config) ModelsFor(backend string) []string

- `Default()` returns:
  - `Addr` "127.0.0.1:8083".
  - One backend: `Name` "ollama", `Kind` `llm.KindOllama`, `URL` "http://127.0.0.1:11434", `Command` `["ollama", "serve"]`, `PullMissing` true.
  - `Intent` backend "ollama", model "llama3.2:1b".
  - `DefaultRoute` "general".
  - Routes, in order, all on backend "ollama":
    - `general`, "Conversation, questions, writing, and anything no other route fits.", model "llama3:8b"
    - `code`, "Writing, explaining, reviewing or debugging source code.", model "qwen2.5-coder:1.5b"
    - `quick`, "Greetings and short factual questions with a one-line answer.", model "llama3.2:1b"
- `Load`: decode the file into a zero `Config` with `DisallowUnknownFields` (unknown fields are an error). A missing file is an error.
- `Validate` checks, in this order: `Addr` not empty; at least one backend; each backend's `Name` matches `^[a-z0-9][a-z0-9-]*$` and is unique (names must not contain "/", because clients address models as "backend/model"); `Kind` is one of the two kinds; `URL` parses with `net/url` with scheme `http` or `https` and a host; `PullMissing` only on kind ollama; `StartTimeoutSeconds` and `Intent.TimeoutSeconds` not negative; `Intent.Backend` names a backend; `Intent.Model` not empty; at least one route; each route `Name` not empty, unique, and not `llm.AutoModel`; each route `Backend` names a backend; each route `Model` not empty; `DefaultRoute` names a route.
- `internal/config/config_test.go`: package `config`. Cover: `Default()` validates; `ModelsFor("ollama")` on the default is `["llama3.2:1b", "llama3:8b", "qwen2.5-coder:1.5b"]`; `Load("")` equals `Default()`; loading a file written to `t.TempDir()`; an unknown field fails; a missing file fails; empty `addr` becomes the default; `LLMROUTER_ADDR` overrides (use `t.Setenv`); one table-driven case per `Validate` rule, each checking `errors.Is(err, llm.ErrInvalid)` and the field name in the message.

## Contract
The exported names above, verbatim.

## Done when
- `go test ./internal/config/` passes.
- `go vet ./internal/config/` succeeds and `gofmt -l internal/config` prints nothing.

## Out of scope
- Every other package.
- Example config files and the README; a later issue writes them.
~~~~

---

## Epic 3: Wiring

### 3.1 Wire the llmrouter server

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Another worker is writing `README.md`, `examples/` and `internal/config/example_test.go` at the same time. Do not touch those, and do not edit any existing package under `internal/`.

## Goal
The server itself: a composition package that builds every part from the config and serves them on one address, and the `main` that runs it.

## Read first
- `internal/config/config.go`: `Config`, `Backend`, `Intent`, `Load`, `ModelsFor`.
- `internal/supervisor/supervisor.go`: `Options`, `New`, `Start`, `Managed`, `Stop`, `EnsureModels`.
- `internal/router/router.go`: `Classifier`, `Options`, `New`.
- `internal/llm/llm.go`: `Backend`, `NewFleet`, `KindOllama`, `KindOpenAI`.
The constructors you call are, verbatim:

    ollama.New(name, baseURL string, hc *http.Client) *ollama.Client          // implements llm.Backend, llm.ModelManager
    oaiclient.New(name, baseURL string, hc *http.Client) *oaiclient.Client    // implements llm.Backend
    gateway.NewHandler(f llm.Fleet, r llm.Router, logger *log.Logger) http.Handler   // serves /v1/...
    admin.NewHandler(f llm.Fleet, r llm.Router) http.Handler                         // serves /api/...
    ui.NewHandler(f llm.Fleet, r llm.Router) http.Handler                            // serves /, /try, /models/...

## Change
- `internal/app/app.go`: package `app`, with

      type App struct {
      	Config  config.Config
      	Fleet   llm.Fleet
      	Router  *router.Router
      	Handler http.Handler
      	// unexported: supervisors by backend name, ollama clients by name, logger
      }

      // New builds every part from cfg. It starts nothing. logger nil means log.Default().
      func New(cfg config.Config, logger *log.Logger) (*App, error)

      // Start starts each backend that does not answer and has a command,
      // logging (not returning) failures. For each kind-ollama backend with
      // PullMissing that is answering, it pulls cfg.ModelsFor(name) in the
      // background with supervisor.EnsureModels, logging the result. The
      // returned channel is closed when every background pull has finished.
      func (a *App) Start(ctx context.Context) <-chan struct{}

      // Close stops every backend process Start started, joining errors.
      func (a *App) Close() error

  In `New`, for each `cfg.Backends` entry: build the client by `Kind` (`ollama.New` or `oaiclient.New`, `hc` nil); build `supervisor.New(supervisor.Options{Name, Health: func(ctx) error { _, err := b.Health(ctx); return err }, Command, Dir, Env, StopCommand, StartTimeout: time.Duration(StartTimeoutSeconds) * time.Second, Log: logger})`. Then `llm.NewFleet(backends, func(name) bool { return sups[name].Managed() })`; `router.New` with a `map[string]router.Classifier` of every backend and `router.Options{IntentBackend, IntentModel, Routes, Default: cfg.DefaultRoute, Timeout: Intent.TimeoutSeconds seconds}`. The handler is an `http.ServeMux` with `mux.Handle("/v1/", gateway...)`, `mux.Handle("/api/", admin...)`, `mux.Handle("/", ui...)`. Return `router.New`'s error if any.
- `cmd/llmrouter/main.go`: flag `-config` (default `os.Getenv("LLMROUTER_CONFIG")`, usage "path to a JSON config; empty uses the built-in default"). `config.Load`; `app.New`; `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`; `a.Start(ctx)`; an `http.Server{Addr: cfg.Addr, Handler: a.Handler, ReadHeaderTimeout: 10 * time.Second}` running `ListenAndServe` in a goroutine; log "llmrouter listening on http://ADDR"; on `ctx.Done()` call `Shutdown` with a 10 second timeout, then `a.Close()`. A listen error other than `http.ErrServerClosed` logs, calls `a.Close()` and exits 1. A config or `app.New` error logs and exits 1.
- `internal/app/app_test.go`: package `app`. Start an `httptest.Server` that fakes Ollama: `GET /api/version` → `{"version":"test"}`, `GET /api/tags` → `{"models":[{"name":"llama3:8b"}]}`, `GET /api/ps` → `{"models":[]}`, `POST /api/pull` → 200 `{"status":"success"}` (record the model names pulled). Build a config with one backend `ollama` at that URL, `PullMissing` true, no command, intent model `small:1b`, routes `general` (model `llama3:8b`) and `code` (model `coder:1b`), default `general`. Tests: `New` succeeds; `Start` then waiting on its channel pulls `small:1b` and `coder:1b` but not `llama3:8b`; `GET /` answers 200; `GET /api/routes` answers 200 with both routes; `GET /v1/models` lists `auto`; `Close` returns nil; `New` with a config whose `DefaultRoute` is unknown returns an error.

## Contract
`app.New(cfg config.Config, logger *log.Logger) (*App, error)`, `(*App).Start(ctx context.Context) <-chan struct{}`, `(*App).Close() error`, and the exported `App` fields above.

## Done when
- `go build -o /dev/null ./cmd/llmrouter` succeeds.
- `go test ./internal/app/` passes.
- `go vet ./internal/app/ ./cmd/llmrouter/` succeeds and `gofmt -l internal/app cmd` prints nothing.

## Out of scope
- `README.md`, `examples/` and `internal/config/example_test.go`, which another worker owns.
- Changes to the components; if one has a bug, block this issue with `bin/codeplat issue block` from the repository root and describe it.
~~~~

### 3.2 Document llmrouter and add example configs

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Another worker is writing `internal/app/` and `cmd/llmrouter/` at the same time. Do not touch those, and do not edit any existing file under `internal/`.

## Goal
A README that tells a person how to run llmrouter, call it, configure it and manage models, and two example config files that the tests prove are valid: one for Ollama alone, and one that adds the Bonsai model served by the Bonsai demo's llama-server on Windows, reached from WSL.

## Read first
- `internal/config/config.go`: every config field and `Default()`; the README's config section must match them.
- `../bookmarks/README.md`: the README style to follow.

## Change
- `README.md` with these sections:
  - What it is (two or three sentences): one server, OpenAI-compatible, routes `"model": "auto"` with an intent model, starts and manages Ollama, serves a page.
  - Run: `make run` or `go run ./cmd/llmrouter [-config FILE]`, then open http://127.0.0.1:8083. With the default config llmrouter starts `ollama serve` if Ollama is not answering, and pulls `llama3.2:1b` and `qwen2.5-coder:1.5b` if they are missing (`llama3:8b` is the third default model).
  - Call it: a `curl` example to `http://127.0.0.1:8083/v1/chat/completions` with `"model": "auto"`, and that the response headers `X-Llmrouter-Route`, `X-Llmrouter-Backend` and `X-Llmrouter-Model` say what was chosen. Then the four ways to name a model: `auto`, a route name such as `code`, `backend/model` such as `ollama/llama3:8b`, or a bare model name, which goes to the default route's backend. Mention that OpenAI SDKs work by setting the base URL to `http://127.0.0.1:8083/v1`.
  - Admin API: a table of the routes `GET /api/backends`, `GET /api/backends/{backend}/models`, `POST /api/backends/{backend}/models/pull` (`{"name":...}`), `DELETE /api/backends/{backend}/models/{name}`, `GET /api/routes`, `GET /api/decisions?n=`, `POST /api/route` (`{"prompt":...}`).
  - Config: a table of every field in `config.Config`, `config.Backend` and `config.Intent` with its meaning, and that `LLMROUTER_CONFIG` names the file and `LLMROUTER_ADDR` overrides `addr`.
  - Bonsai: Bonsai 2 needs the PrismML fork's own `llama-server`; stock llama.cpp and Ollama cannot run it. The demo at `C:\Users\cstab\Bonsai-demo` starts it with `scripts\start_llama_server.ps1`, which reads `PORT` (default 8080, which clashes with other local servers, so use 8085) and `BONSAI_HOST`. From WSL in NAT networking mode, Windows' localhost is not WSL's: set `BONSAI_HOST=0.0.0.0` and use the Windows host's address, which is the `default via` address of `ip route` (for example `http://192.168.176.1:8085`); Windows Firewall may need to allow the port. Loading the 7.2 GB model takes a while, hence `start_timeout_seconds`. Point to `examples/bonsai-wsl.json`.
  - Layout: one line per package under `internal/` and `cmd/llmrouter`.
  - Test: `make test`, and `make live` for the opt-in live test against a real Ollama (another issue adds that target).
- `examples/ollama.json`: the same content as `config.Default()`, written out as JSON.
- `examples/bonsai-wsl.json`: the default config plus a second backend:

      {"name": "bonsai", "kind": "openai", "url": "http://192.168.176.1:8085",
       "command": ["powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
                   "$env:PORT='8085'; $env:BONSAI_HOST='0.0.0.0'; & 'C:\\Users\\cstab\\Bonsai-demo\\scripts\\start_llama_server.ps1'"],
       "stop_command": ["powershell.exe", "-NoProfile", "-Command",
                        "Stop-Process -Name llama-server -ErrorAction SilentlyContinue"],
       "start_timeout_seconds": 600}

  and a fourth route `reason`, "Hard problems that need careful step-by-step reasoning, planning, maths or long-form analysis.", backend `bonsai`, model `Ternary-Bonsai-2-27B-PQ2_0.gguf`.
- `internal/config/example_test.go`: package `config`. `TestExamples` loads every `../../examples/*.json` with `Load` and fails on any error; it also checks `examples/ollama.json` loads equal to `Default()` (compare with `reflect.DeepEqual`), and that there are at least two files.

## Contract
File paths `README.md`, `examples/ollama.json`, `examples/bonsai-wsl.json`, `internal/config/example_test.go`.

## Done when
- `go test ./internal/config/` passes.
- `grep -c X-Llmrouter-Route README.md` prints at least 1, and every field name in the `json:` tags of `internal/config/config.go` appears in `README.md`.

## Out of scope
- `internal/app/` and `cmd/llmrouter/`, which another worker owns.
- `Makefile`; a later issue adds the `live` target.
~~~~

---

## Epic 4: Verify

### 4.1 Add an end-to-end test against fake backends

~~~~text
Epic: 4 Verify
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Another worker is writing `internal/live/` and the `Makefile` at the same time. Do not touch those. Do not edit existing packages.

## Goal
One test that runs the whole server, built by `app.New`, against a fake Ollama and a fake OpenAI-compatible server, and proves routing, forwarding, streaming, model management and the page work together.

## Read first
- `internal/app/app.go`: `New`, `Start`, `Close`, and the `Handler` field.
- `internal/config/config.go`: `Config`, `Backend`, `Intent`.
- `internal/llm/llm.go`: `Route`, `Decision`, `BackendStatus`.

## Change
- `internal/e2e/e2e_test.go`: package `e2e_test`.
  - A fake Ollama `httptest.Server`: `GET /api/version` → `{"version":"fake"}`; `GET /api/tags` → `{"models":[{"name":"llama3:8b"}]}`; `GET /api/ps` → `{"models":[]}`; `POST /api/pull` records `model` and answers `{"status":"success"}`; `DELETE /api/delete` answers 200 for `llama3:8b` and 404 `{"error":"not found"}` otherwise; `POST /api/chat` reads `messages[1].content` and answers `{"message":{"role":"assistant","content":"{\"route\":\"R\"}"},"done":true}` where R is `code` if the prompt contains "func", `reason` if it contains "prove", else `general`; `POST /v1/chat/completions` records the request's `model` and answers `{"id":"x","object":"chat.completion","model":MODEL,"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`, or, when the request has `"stream": true`, `Content-Type: text/event-stream` and the body `data: {"choices":[{"delta":{"content":"o"}}]}\n\ndata: {"choices":[{"delta":{"content":"k"}}]}\n\ndata: [DONE]\n\n`.
  - A fake OpenAI-compatible `httptest.Server` (the "bonsai" backend): `GET /v1/models` → `{"object":"list","data":[{"id":"bonsai-27b"}]}`; `POST /v1/chat/completions` records `model` and answers like the fake Ollama's non-streaming reply.
  - The config: backend `ollama` (kind ollama, the fake's URL, `PullMissing` true, no command) and backend `bonsai` (kind openai, the fake's URL, no command); intent backend `ollama`, model `llama3.2:1b`; routes `general` (ollama, `llama3:8b`), `code` (ollama, `qwen2.5-coder:1.5b`), `reason` (bonsai, `bonsai-27b`); default `general`. Call `Validate` on it first.
  - `app.New`, then `<-a.Start(ctx)` (with a 10 second timeout), serve `a.Handler` with `httptest.NewServer`, and `t.Cleanup` `a.Close`.
  - Check, in order:
    1. The fake Ollama received pulls for exactly `llama3.2:1b` and `qwen2.5-coder:1.5b`.
    2. `POST /v1/chat/completions` with `"model":"auto"` and a user message "write a func that adds" → 200; headers `X-Llmrouter-Route: code`, `X-Llmrouter-Backend: ollama`, `X-Llmrouter-Model: qwen2.5-coder:1.5b`; the fake Ollama got model `qwen2.5-coder:1.5b`.
    3. `"model":"auto"` with "prove there are infinitely many primes" → the bonsai fake got model `bonsai-27b`.
    4. `"model":"bonsai/other"` → the bonsai fake got model `other`, and no route header.
    5. `"model":"auto"`, "hello", `"stream": true` → `Content-Type: text/event-stream` and the body is exactly the fake's three `data:` lines.
    6. `GET /v1/models` lists `auto`, `code`, `ollama/llama3:8b` and `bonsai/bonsai-27b`.
    7. `GET /api/decisions` has 3 decisions, newest first: `general`, `reason`, `code`, none with `fallback` true.
    8. `GET /api/backends` has two entries, both `reachable` and not `managed`.
    9. `GET /` answers 200 and contains `qwen2.5-coder:1.5b` and `reason`.
    10. `DELETE /api/backends/ollama/models/llama3:8b` → 204; `DELETE /api/backends/ollama/models/nope:1` → 404; `DELETE /api/backends/bonsai/models/bonsai-27b` → 400.
  - Guard the fakes' recorded values with a `sync.Mutex`.

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/e2e/` passes.
- `go vet ./internal/e2e/` succeeds and `gofmt -l internal/e2e` prints nothing.

## Out of scope
- Every non-test file. If the test finds a bug in a component, block this issue with `bin/codeplat issue block` from the repository root and describe the failing check; do not fix the component here.
- `internal/live/` and the `Makefile`, which another worker owns.
~~~~

### 4.2 Add an opt-in live test against real backends

~~~~text
Epic: 4 Verify
Depends on: none

## Environment
- Work in `samples/llmrouter/` under the repository root. It is a Go module named `llmrouter`.
- Run every command from `samples/llmrouter/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmrouter/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Another worker is writing `internal/e2e/` at the same time. Do not touch it. Do not edit existing packages.
- This machine runs a real Ollama at http://127.0.0.1:11434 with `llama3:8b` installed. Loading it takes about 20 seconds the first time. Do not pull or delete any model on it.

## Goal
A test, skipped unless asked for, that runs llmrouter against the real Ollama (and, when configured, a real OpenAI-compatible server such as Bonsai's llama-server), so a person can check the whole thing against real models with `make live`.

## Read first
- `internal/app/app.go`: `New`, `Start`, `Close`, `Handler`.
- `internal/config/config.go`: `Config`, `Backend`, `Intent`.
- `Makefile`: the existing targets.

## Change
- `internal/live/live_test.go`: package `live_test`. Every test starts with `if os.Getenv("LLMROUTER_LIVE") != "1" { t.Skip("set LLMROUTER_LIVE=1 to run against real backends") }`.
  - `TestOllama`: base URL `LLMROUTER_LIVE_OLLAMA` (default `http://127.0.0.1:11434`), model `LLMROUTER_LIVE_MODEL` (default `llama3:8b`). Build a config with one backend `ollama` (kind ollama, no command, `PullMissing` false); intent on that backend and model with `TimeoutSeconds` 120; routes `code` ("Writing or debugging source code.") and `general` ("Anything else.") both on that model; default `general`. `app.New`, `<-a.Start(ctx)`, serve `a.Handler` with `httptest.NewServer`, `t.Cleanup(a.Close)`. If `GET /api/backends/ollama/models` does not list the model, `t.Skip` saying so. Then `POST /v1/chat/completions` `{"model":"auto","max_tokens":32,"messages":[{"role":"user","content":"Write a Go function that reverses a string."}]}` with a 3 minute client timeout: expect 200, a non-empty `choices[0].message.content`, and an `X-Llmrouter-Route` header that is `code` or `general`; then `GET /api/decisions?n=1` has one decision; log the route, `fallback` and `took`.
  - `TestOpenAI`: skip unless `LLMROUTER_LIVE_OPENAI_URL` is set (for example `http://192.168.176.1:8085` for Bonsai). One backend `remote` (kind openai, that URL, no command), intent and one route `general` on model `LLMROUTER_LIVE_OPENAI_MODEL` (if empty, use the first id from that server's `GET /v1/models`). `POST /v1/chat/completions` with `"model":"remote/<model>"`, `"max_tokens": 32` and a user message "Say hello." with a 10 minute client timeout: expect 200 and a non-empty `choices[0].message.content`.
- `Makefile`: add `live` to `.PHONY` and a target

      live:
      	LLMROUTER_LIVE=1 go test -count=1 -v -timeout 20m ./internal/live/

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/live/` passes (both tests skip).
- `LLMROUTER_LIVE=1 go test -count=1 -v -timeout 10m -run TestOllama ./internal/live/` passes on this machine.
- `go vet ./internal/live/` succeeds and `gofmt -l internal/live` prints nothing.

## Out of scope
- `internal/e2e/`, which another worker owns.
- Every non-test file except `Makefile`. If the live run finds a bug in a component, block this issue with `bin/codeplat issue block` from the repository root and describe what failed.
~~~~
