# Roadmap: llmchat sample

## Goal

A sample project in `samples/llmchat/`: a chat application that sends conversations through the LLM router (`samples/llmrouter`) instead of a hosted model, built in two stages.

Stage 1 (epics 1 to 4): a web page with saved conversations (SQLite), a model picker (`auto`, or a route), streaming answers, and the route and model that answered. Stage 2 (epics 5 to 8): the model can use tools in a per-conversation workspace directory (read, list, search, write, edit, run a command), with the person approving every change, so it can do the kind of work Claude Code does.

It is its own Go module, `llmchat`. It uses the standard library, plus `modernc.org/sqlite` in one package. It calls the router only over HTTP. Tracker project: `CHAT`.

The point of it: pointed at this repository as its workspace, it should be able to contribute to the platform itself, the way the Claude Code conversation that built this does: read `CLAUDE.md` and the architecture notes, plan work with `prompts/decompose.md`, record it in the tracker with `bin/codeplat`, edit code, run `make test`, and look at `git`. Stages 1 and 2 build the app and the tools; stage 3 (below) is what makes it a contributor to this repository.

Facts checked on this machine:

- llmrouter at `http://127.0.0.1:8083` passes any `tools` array and `tool_calls` straight through (the gateway only rewrites `model`), and streams the backend's server-sent events unchanged. Through it, Bonsai (a llama.cpp `llama-server` started with `--jinja`) streams `delta.reasoning_content` pieces first, then `delta.tool_calls`: the first chunk of a call has `index`, `id`, `type`, `function.name` and a first piece of `function.arguments`; later chunks have only `index` and `function.arguments`; the last chunk has `finish_reason: "tool_calls"`; then `data: [DONE]`.
- `llama3:8b` on Ollama has no `tools` capability, so tool conversations need the Bonsai route (or another tool-capable model).
- The router classifies only the last user message and may pick a different model for each request, so the engine pins `backend/model` after a turn's first reply.
- `modernc.org/sqlite v1.60.1` is in the module cache; a new module gets it with `GOFLAGS=-mod=mod GOPROXY=off go get modernc.org/sqlite@v1.60.1` after copying the repository's `go.sum`. There is no Node here, so the page's JavaScript is checked only by Go tests on the served text and by hand in a browser.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists and the shared `chat` package compiles and passes its tests. | 1 |
| 2. Chat components | Store, router client, engine, API and page each pass their own tests. | 5, parallel |
| 3. Chat wiring | The server builds and its app package passes; the README exists. | 2, parallel |
| 4. Chat verify | An end-to-end test passes; an opt-in live test exists. | 2, parallel |
| 5. Sandbox | `sandbox.Resolve` confines paths to a workspace. | 1 |
| 6. Tools | The six tools each pass their own tests. | 4, parallel |
| 7. Agent wiring | The server offers the tools, the page shows calls and approvals, the README describes them. | 3, parallel |
| 8. Agent verify | An end-to-end agent test passes; an opt-in live agent test exists. | 2, parallel |

No issue has a dependency inside its epic.

Why eight epics: every component imports the `chat` package (epic 1). The app package imports all of epic 2 (epic 3), and the end-to-end tests import the app package (epic 4). Stage 2 repeats the pattern: the six tools all use one path-confinement function (epic 5) and are otherwise independent packages (epic 6); wiring them into the app and page edits files epics 3 owned, so it comes after (epic 7); the agent end-to-end tests need all of it (epic 8).

The `chat` package is what makes epic 2 parallel. It fixes `Store`, `Completer`, `Approver`, `Tool` and `Event`, and each component implements one or consumes them through a fake. The engine (epic 2) already contains the whole tool loop and the approval wait, so stage 2 adds packages and wiring but never reopens the engine.

## Stage 3: contributing to the platform (planned after stage 2)

Not recorded as issues yet: it depends on how stages 1 and 2 turn out, and `prompts/decompose.md` says later epics are re-checked against the code as it stands. What stages 1 and 2 already provide for it:

- A conversation's workspace can be this repository's root, so `bash` can run `bin/codeplat issue ...`, `make test`, `git` and `go`, and the file tools can edit any file here. The first things to try are `Read CLAUDE.md and ARCHITECTURE.md, then add a test for X` and `Plan feature Y with prompts/decompose.md`.
- The approval step is an interface (`chat.Approver`), so a rules-based approver can wrap it without touching the engine.
- The engine's system prompt is an option, so project instructions can be added to it.
- `app.Config.MaxSteps` (`CHAT_MAX_STEPS`) lets a long task run past the default.

What stage 3 will add, as its own epics:

1. **Project context.** Read `CLAUDE.md` (else `AGENTS.md`) from the workspace into the system prompt, so a conversation follows the repository's own rules.
2. **Approval rules.** A rules file (for example `CHAT_POLICY=policy.json`) that auto-approves commands that only read or check (`git status`, `git diff`, `go test`, `make test`, `bin/codeplat issue list`), auto-denies dangerous ones, and asks about the rest, so a person is not clicking Approve forty times. The engine will skip the approval event when a rule already decided.
3. **Tracker tools.** Typed `codeplat` tool calls for the tracker (list, show, claim, comment, block, create issues and epics), so an agent can plan and record work the way this conversation does, with only the tool's mutating calls needing approval.
4. **Headless worker.** `llmchat run -workspace . -prompt prompts/implement.md "LLM-3"`: run one task to completion with no browser, under the approval rules, printing a transcript. The platform's own runner (`cmd/runner`, `runner.command` in `config/*.json`) can then start these as workers, so local models (Bonsai through the router) pick up tracker issues. That is the self-hosting loop: the agent plans, the tracker holds the work, local-model workers do it.
5. **Long tasks.** Bonsai runs with a 32768-token context, so a long agent session needs the oldest tool results trimmed or summarised before the window fills.

Open questions for stage 3, to settle when it is planned: whether tracker tools go through the CLI or the REST API, how an unattended worker is kept inside its workspace (the sandbox covers files, not `bash`), and whether a local model is reliable enough at tool calling to be trusted with the tracker's refine, implement and test loops (the live agent tests in epic 8 are the first evidence).

---

## Epic 1: Foundation

Done when: The module exists and the shared `chat` package compiles and passes its tests.

### 1.1 Create the llmchat module and its shared chat package

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. Create it; it does not exist yet.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- `samples/llmrouter/` is a finished sample in the same style; you may read it for conventions (its `Makefile`, `internal/llm/llm.go`), but do not edit it.
- Go standard library only, plus `modernc.org/sqlite` (installed in the module cache; there is no network, so use `GOPROXY=off`).

## Goal
Create the Go module for llmchat, a chat application that talks to an LLM router, and the one package every other part imports: the shared types, errors and interfaces.

## Read first
- `../llmrouter/Makefile`: the Makefile style to follow.
- `../llmrouter/internal/llm/llm.go`: the style of a shared contract package.

## Change
- `go.mod` and `go.sum`: run these in `samples/llmchat/`:

      go mod init llmchat
      cp ../../go.sum go.sum
      GOFLAGS=-mod=mod GOPROXY=off go get modernc.org/sqlite@v1.60.1

  After this `go.mod` requires `modernc.org/sqlite v1.60.1` (marked `// indirect` until a package imports it; leave it) and its dependencies. Do not run `go mod tidy`.
- `internal/chat/chat.go`: create it with exactly the content under Contract.
- `internal/chat/chat_test.go`: package `chat`. Tests:
  - `Message` with only `Role`, `Content` and `CreatedAt` set marshals to JSON with keys `seq`, `role`, `content`, `created_at` and none of `reasoning`, `tool_calls`, `tool_call_id`, `route`, `backend`, `model`.
  - A `Message` with `ToolCalls` set marshals them as `"tool_calls":[{"id":..,"name":..,"arguments":..}]`, and unmarshalling that JSON gives back an equal `Message`.
  - `Event{Type: EventDone}` marshals to exactly `{"type":"done"}`; `Event{Type: EventContent, Text: "hi"}` marshals to exactly `{"type":"content","text":"hi"}`.
  - `ConversationPatch` with only `Title` set marshals to `{"title":"x"}`.
  - Compile-time checks that a small fake type in the test file satisfies `Store`, `Completer`, `Approver` and `Tool` (give each fake the right methods returning zero values).
- `Makefile`:

      .PHONY: test vet run

      test:
      	go test ./...

      vet:
      	go vet ./...

      run:
      	go run ./cmd/llmchat

  (Recipe lines start with a tab. `cmd/llmchat` does not exist yet; that is expected.)

## Contract
```go
// Package chat holds the types and interfaces shared by every part of
// llmchat. It imports only the standard library.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Errors returned by llmchat's parts. Callers test for them with errors.Is.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a request from the model to run a tool. Arguments is the JSON
// text the model produced.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is one message of a conversation. Seq numbers a conversation's
// messages from 1. An assistant message records how the router chose its
// model in Route, Backend and Model (Route is empty when no route was
// used). A tool message answers the call whose ID is ToolCallID.
type Message struct {
	Seq        int        `json:"seq"`
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Route      string     `json:"route,omitempty"`
	Backend    string     `json:"backend,omitempty"`
	Model      string     `json:"model,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Conversation is a saved chat. Model is "auto", a route name, or
// "backend/model". Workspace is the directory the tools may use; empty
// means tools are off for this conversation.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationPatch changes only the fields that are non-nil.
type ConversationPatch struct {
	Title     *string `json:"title,omitempty"`
	Model     *string `json:"model,omitempty"`
	Workspace *string `json:"workspace,omitempty"`
}

// Store is where conversations and their messages are kept.
type Store interface {
	// CreateConversation stores c with a new ID, CreatedAt and UpdatedAt set, and returns it.
	CreateConversation(ctx context.Context, c Conversation) (Conversation, error)
	// GetConversation returns ErrNotFound for an unknown id (also for the methods below).
	GetConversation(ctx context.Context, id string) (Conversation, error)
	// ListConversations returns every conversation, most recently updated first. Never nil.
	ListConversations(ctx context.Context) ([]Conversation, error)
	// UpdateConversation applies the non-nil fields of p and sets UpdatedAt.
	UpdateConversation(ctx context.Context, id string, p ConversationPatch) (Conversation, error)
	// DeleteConversation removes the conversation and its messages.
	DeleteConversation(ctx context.Context, id string) error
	// AppendMessage stores m as the conversation's next message (Seq is the
	// previous Seq plus 1; any Seq in m is ignored), sets CreatedAt, bumps
	// the conversation's UpdatedAt, and returns the stored message.
	AppendMessage(ctx context.Context, conversationID string, m Message) (Message, error)
	// ListMessages returns the conversation's messages oldest first. Never nil.
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
}

// ToolSpec describes a tool to the model. Parameters is a JSON Schema object.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Tool is something the model can ask to run.
type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the
	// person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// Event types.
const (
	EventInfo       = "info"        // routing for the assistant message about to stream
	EventContent    = "content"     // a piece of the answer
	EventReasoning  = "reasoning"   // a piece of the model's reasoning
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventApproval   = "approval"    // the person must approve that call
	EventToolResult = "tool_result" // the call finished (or was denied)
	EventError      = "error"
	EventDone       = "done" // always the last event of a turn
)

// Event is one thing that happens during a turn. Type says which fields are set.
type Event struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`      // content, reasoning (a delta); tool_result (the result)
	Route     string `json:"route,omitempty"`     // info
	Backend   string `json:"backend,omitempty"`   // info
	Model     string `json:"model,omitempty"`     // info
	CallID    string `json:"call_id,omitempty"`   // tool_call, approval, tool_result
	Name      string `json:"name,omitempty"`      // tool_call, approval
	Arguments string `json:"arguments,omitempty"` // tool_call, approval
	Error     string `json:"error,omitempty"`     // error
}

// CompletionRequest is one call to the router. Model is "auto", a route
// name, or "backend/model". No Tools means no tools are offered.
type CompletionRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
}

// Completer asks the router for one assistant message. It calls emit for
// each event as it streams (info, then content and reasoning), and returns
// the assembled assistant message: Content, Reasoning, ToolCalls, and
// Route, Backend and Model as the router reported them.
type Completer interface {
	Complete(ctx context.Context, req CompletionRequest, emit func(Event)) (Message, error)
}

// ApprovalRequest asks the person to approve one tool call.
type ApprovalRequest struct {
	ConversationID string
	CallID         string
	Name           string
	Arguments      string
}

// Approver asks the person whether a tool call may run. It blocks until the
// person answers or ctx ends.
type Approver interface {
	Approve(ctx context.Context, r ApprovalRequest) (bool, error)
}
```

## Done when
- `go vet ./...` succeeds in `samples/llmchat/`.
- `go test ./internal/chat/` passes.
- `gofmt -l .` prints nothing.
- `grep -c 'modernc.org/sqlite v1.60.1' go.mod` prints 1.

## Out of scope
- Every other package. Later issues create `internal/sqlitestore`, `internal/routerclient`, `internal/agent`, `internal/api`, `internal/web`, `internal/app`, `internal/tools/...` and `cmd/llmchat`.
- A README.
~~~~

## Epic 2: Chat components

Done when: The store, router client, engine, API and page each pass their own tests.

### 2.1 Add the SQLite conversation store

~~~~text
Epic: 2 Chat components
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. The one exception is `modernc.org/sqlite`, already in `go.mod`, which only `internal/sqlitestore` imports. Do not edit `go.mod` or `go.sum`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and import no other package under `internal/` except `llmchat/internal/chat`.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
A SQLite implementation of `chat.Store`, so conversations and their messages survive restarts.

## Read first
- `internal/chat/chat.go`: the `Store` interface you implement and the `Conversation`, `Message`, `ToolCall`, `ConversationPatch` types.
- `../../internal/store/sqlite/sqlite.go` (the repository root's SQLite store): the migrations-in-`PRAGMA user_version` pattern used elsewhere in this repository. Read it for the pattern only; do not import or edit it.

## Change
- `internal/sqlitestore/sqlitestore.go`: package `sqlitestore`, with

      // Store keeps conversations in a SQLite database file.
      type Store struct { /* *sql.DB, now func() time.Time */ }

      // Open opens (creating it if needed) the database at path and applies
      // the migrations. now returns the current time; nil means time.Now.
      func Open(path string, now func() time.Time) (*Store, error)

      func (s *Store) Close() error

  plus the seven `chat.Store` methods. Details:
  - `import _ "modernc.org/sqlite"`; `sql.Open("sqlite", dsn)` with dsn `"file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"`. Call `db.SetMaxOpenConns(1)`.
  - Migrations: a `[]string` applied in order, counted in `PRAGMA user_version`; each runs in a transaction with the version bump. Migration 1 creates:

        CREATE TABLE conversations (
          id TEXT PRIMARY KEY, title TEXT NOT NULL, model TEXT NOT NULL, workspace TEXT NOT NULL,
          created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
        CREATE TABLE messages (
          conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
          seq INTEGER NOT NULL, role TEXT NOT NULL, content TEXT NOT NULL, reasoning TEXT NOT NULL,
          tool_calls TEXT NOT NULL, tool_call_id TEXT NOT NULL, route TEXT NOT NULL,
          backend TEXT NOT NULL, model TEXT NOT NULL, created_at TEXT NOT NULL,
          PRIMARY KEY (conversation_id, seq));

  - Times are stored as `t.UTC().Format(time.RFC3339Nano)` and parsed with `time.Parse(time.RFC3339Nano, ...)`. `tool_calls` is the JSON of the `[]chat.ToolCall` (`"[]"` for none); on reading, an empty list comes back as nil.
  - `CreateConversation`: a new `ID` of 12 lowercase hex characters from `crypto/rand` (8 bytes is more than enough: use `hex.EncodeToString` of 6 bytes); an empty `Model` becomes `"auto"`; `CreatedAt` and `UpdatedAt` are `now()`.
  - `AppendMessage`: in one transaction, check the conversation exists (else `chat.ErrNotFound`), take `seq = COALESCE(MAX(seq),0)+1`, insert, and set the conversation's `updated_at` to `now()`.
  - `UpdateConversation`: applies non-nil fields, sets `updated_at`; unknown id is `chat.ErrNotFound`.
  - `DeleteConversation`: unknown id is `chat.ErrNotFound`; the messages go with it (the foreign key cascade).
  - `ListConversations`: `ORDER BY updated_at DESC, id`; never nil. `ListMessages`: `ORDER BY seq`; `chat.ErrNotFound` if the conversation does not exist; never nil.
  - Wrap other errors with `fmt.Errorf("sqlitestore: ...: %w", err)`.
- `internal/sqlitestore/sqlitestore_test.go`: package `sqlitestore`. Every test opens a store at `filepath.Join(t.TempDir(), "chat.db")` with a fake clock that advances one second per call. Cover: create then get; empty model becomes `auto`; list order after an append bumps an older conversation to the top; update with each field; update and get and delete and append and list-messages of an unknown id wrap `chat.ErrNotFound`; append assigns Seq 1, 2, 3 and ignores a Seq in the input; every `Message` field survives a round trip, including two `ToolCalls`; delete removes the messages (re-create is not needed: check `ListMessages` returns `chat.ErrNotFound`); data survives `Close` and `Open` of the same path; two stores' worth of IDs are distinct (create 50 conversations, all IDs unique and 12 characters); `ListConversations` and `ListMessages` on empty data return non-nil empty slices. Add `var _ chat.Store = (*Store)(nil)`.

## Contract
`sqlitestore.Open(path string, now func() time.Time) (*Store, error)`, `(*Store).Close() error`, and `*Store` implements `chat.Store` exactly as declared in `internal/chat/chat.go`:

```go
type Store interface {
	// CreateConversation stores c with a new ID, CreatedAt and UpdatedAt set, and returns it.
	CreateConversation(ctx context.Context, c Conversation) (Conversation, error)
	// GetConversation returns ErrNotFound for an unknown id (also for the methods below).
	GetConversation(ctx context.Context, id string) (Conversation, error)
	// ListConversations returns every conversation, most recently updated first. Never nil.
	ListConversations(ctx context.Context) ([]Conversation, error)
	// UpdateConversation applies the non-nil fields of p and sets UpdatedAt.
	UpdateConversation(ctx context.Context, id string, p ConversationPatch) (Conversation, error)
	// DeleteConversation removes the conversation and its messages.
	DeleteConversation(ctx context.Context, id string) error
	// AppendMessage stores m as the conversation's next message (Seq is the
	// previous Seq plus 1; any Seq in m is ignored), sets CreatedAt, bumps
	// the conversation's UpdatedAt, and returns the stored message.
	AppendMessage(ctx context.Context, conversationID string, m Message) (Message, error)
	// ListMessages returns the conversation's messages oldest first. Never nil.
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
}
```

## Done when
- `go test ./internal/sqlitestore/` passes.
- `go vet ./internal/sqlitestore/` succeeds and `gofmt -l internal/sqlitestore` prints nothing.

## Out of scope
- Every other package. No in-memory store.
~~~~

### 2.2 Add the router client

~~~~text
Epic: 2 Chat components
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. The one exception is `modernc.org/sqlite`, already in `go.mod`, which only `internal/sqlitestore` imports. Do not edit `go.mod` or `go.sum`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and import no other package under `internal/` except `llmchat/internal/chat`.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
A client for the LLM router's OpenAI-compatible endpoint that implements `chat.Completer`: it sends a conversation (and tool specs), streams the reply, and returns the assembled assistant message with how the router routed it.

## Read first
- `internal/chat/chat.go`: `Completer`, `CompletionRequest`, `Message`, `ToolCall`, `ToolSpec`, `Event` and the `Event*` constants.

## Change
- `internal/routerclient/routerclient.go`: package `routerclient`, with

      // Client talks to an llmrouter server.
      type Client struct { /* base URL, *http.Client */ }

      // New returns a client for the router at baseURL, such as
      // "http://127.0.0.1:8083". A trailing slash is ignored. hc nil means a
      // new http.Client with no timeout; callers bound calls with ctx.
      func New(baseURL string, hc *http.Client) *Client

      // Complete implements chat.Completer.
      func (c *Client) Complete(ctx context.Context, req chat.CompletionRequest, emit func(chat.Event)) (chat.Message, error)

      // Routes returns the router's route names: GET /api/routes answers
      // {"default":"general","routes":[{"name":"general",...},...]}; return the names in order.
      func (c *Client) Routes(ctx context.Context) ([]string, error)

- `Complete` sends `POST {base}/v1/chat/completions`, `Content-Type: application/json`, with this JSON body:

      {"model": req.Model ("auto" when empty), "stream": true,
       "messages": [...], "tools": [...]}

  `tools` is left out when `req.Tools` is empty; otherwise it is `[{"type":"function","function":{"name":N,"description":D,"parameters":P}}]`. Convert each `chat.Message`:
  - role `user` or `system`: `{"role": R, "content": C}`.
  - role `assistant` without tool calls: `{"role":"assistant","content": C}`. With tool calls: `{"role":"assistant","content": C (a string, "" when empty),"tool_calls":[{"id":ID,"type":"function","function":{"name":N,"arguments":A}}]}`.
  - role `tool`: `{"role":"tool","tool_call_id": ToolCallID,"content": C}`.
  Reasoning is never sent back.
- Response handling:
  - A status other than 200: read up to 512 bytes of the body and return `fmt.Errorf("router returned %s: %s", resp.Status, body)`. A failure of `hc.Do` returns `fmt.Errorf("router unreachable: %w", err)`.
  - On 200, first call `emit(chat.Event{Type: chat.EventInfo, Route: h.Get("X-Llmrouter-Route"), Backend: h.Get("X-Llmrouter-Backend"), Model: h.Get("X-Llmrouter-Model")})`.
  - Then read the body line by line (`bufio.Reader`, so long lines work). Ignore lines that do not start with `data: `. `data: [DONE]` ends the stream. Otherwise decode the JSON `{"choices":[{"delta":{"content":S,"reasoning_content":S,"tool_calls":[{"index":I,"id":ID,"function":{"name":N,"arguments":A}}]},"finish_reason":..}]}`. `content` and `reasoning_content` may be `null` or absent; a non-empty string is appended to the message and emitted as `chat.EventContent` or `chat.EventReasoning` with `Text` set to it. Tool calls are assembled by `index`: the first chunk of a call carries `id` and `function.name`; later chunks carry only `function.arguments` pieces, which are concatenated. Ignore `choices` that is empty.
  - Return `chat.Message{Role: chat.RoleAssistant, Content, Reasoning, ToolCalls (in index order), Route, Backend, Model}` where the last three come from the headers. A stream that ends without `[DONE]` is not an error. A read error mid-stream returns the error and the partial message.
  - Stop and return `ctx.Err()` when the context ends.
- `internal/routerclient/routerclient_test.go`: package `routerclient`. `httptest.Server` handlers that decode the request body and write SSE with `w.(http.Flusher)`. Cover: request body for a plain conversation (model, `stream: true`, messages, no `tools` key); empty model becomes `auto`; tools and an assistant tool-call message and a tool message are converted exactly as above; content and reasoning deltas emit events in order after the `info` event, and the returned message has them joined; `null` content deltas are ignored; a tool call streamed in four chunks (first with id and name and arguments `{`, then `"path":"`, `a.txt`, `"}`) comes back as one `ToolCall` with arguments `{"path":"a.txt"}`; two tool calls with indexes 0 and 1 in order; a line split across writes still parses; the three headers land in the info event and the message (and a missing `X-Llmrouter-Route` leaves `Route` empty); a 503 returns an error containing `503` and the body; a closed server returns an error containing `unreachable`; a cancelled context returns an error satisfying `errors.Is(err, context.Canceled)`; `Routes` returns names, and a 500 is an error. Add `var _ chat.Completer = (*Client)(nil)`.

## Contract
`routerclient.New(baseURL string, hc *http.Client) *Client`; `(*Client).Complete` implements `chat.Completer`; `(*Client).Routes(ctx context.Context) ([]string, error)`. Types it uses:

```go
// Package chat (internal/chat/chat.go), the types you use:
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a request from the model to run a tool. Arguments is the JSON
// text the model produced.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Seq        int        `json:"seq"` // numbers a conversation's messages from 1
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // set on role "tool"
	Route      string     `json:"route,omitempty"`        // how the router chose an assistant message's model
	Backend    string     `json:"backend,omitempty"`
	Model      string     `json:"model,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`     // "auto", a route name, or "backend/model"
	Workspace string    `json:"workspace"` // directory the tools may use; "" means tools are off
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConversationPatch struct {
	Title     *string `json:"title,omitempty"`
	Model     *string `json:"model,omitempty"`
	Workspace *string `json:"workspace,omitempty"`
}
```

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

// Event is one thing that happens during a turn. Type says which fields are set.
type Event struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`      // content, reasoning (a delta); tool_result (the result)
	Route     string `json:"route,omitempty"`     // info
	Backend   string `json:"backend,omitempty"`   // info
	Model     string `json:"model,omitempty"`     // info
	CallID    string `json:"call_id,omitempty"`   // tool_call, approval, tool_result
	Name      string `json:"name,omitempty"`      // tool_call, approval
	Arguments string `json:"arguments,omitempty"` // tool_call, approval
	Error     string `json:"error,omitempty"`     // error
}

const (
	EventInfo       = "info"        // routing for the assistant message about to stream
	EventContent    = "content"     // a piece of the answer
	EventReasoning  = "reasoning"   // a piece of the model's reasoning
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventApproval   = "approval"    // the person must approve that call
	EventToolResult = "tool_result" // the call finished (or was denied)
	EventError      = "error"
	EventDone       = "done" // always the last event of a turn
)
```

```go
type CompletionRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
}

type Completer interface {
	Complete(ctx context.Context, req CompletionRequest, emit func(Event)) (Message, error)
}
```

## Done when
- `go test ./internal/routerclient/` passes.
- `go vet ./internal/routerclient/` succeeds and `gofmt -l internal/routerclient` prints nothing.

## Out of scope
- Every other package. Do not import `llmrouter`; it is a separate module and this package talks to it only over HTTP.
~~~~

### 2.3 Add the agent engine

~~~~text
Epic: 2 Chat components
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. The one exception is `modernc.org/sqlite`, already in `go.mod`, which only `internal/sqlitestore` imports. Do not edit `go.mod` or `go.sum`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and import no other package under `internal/` except `llmchat/internal/chat`.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
The engine that runs one turn of a conversation: it saves the user's message, asks the router for a reply, and, when the model asks for tools, runs them (after the person's approval where needed) and asks again, until the model answers without tools. It streams everything that happens as events.

## Read first
- `internal/chat/chat.go`: `Store`, `Completer`, `Approver`, `Tool`, `Message`, `ToolCall`, `Conversation`, `Event`, `ToolSpec`, `CompletionRequest`, `ApprovalRequest` and the errors.

## Change
- `internal/agent/agent.go`: package `agent`, with

      type Options struct {
      	// MaxSteps bounds the model calls in one turn. Zero means 25.
      	MaxSteps int
      	// System, if not empty, is sent as the first message of every request
      	// (it is not stored). When the conversation has a Workspace, the
      	// text "\n\nWorkspace directory: " + Workspace is appended to it.
      	System string
      }

      type Engine struct { /* ... */ }

      func New(store chat.Store, completer chat.Completer, tools []chat.Tool, approver chat.Approver, opts Options) *Engine

      // Run performs one turn. It emits events as they happen, ends with
      // exactly one chat.EventDone when it returns nil, and returns an error
      // (without EventDone) when it fails.
      func (e *Engine) Run(ctx context.Context, conversationID, text string, emit func(chat.Event)) error

- `Run` does, in order:
  1. `text` with spaces trimmed empty: return an error wrapping `chat.ErrInvalid`. Get the conversation (`chat.ErrNotFound` passes through).
  2. Append `chat.Message{Role: chat.RoleUser, Content: text}`. If the conversation `Title` is empty, set it with `UpdateConversation` to the first line of `text` cut to 60 runes (append `…` if cut).
  3. Repeat, at most `MaxSteps` times:
     - `msgs := store.ListMessages`. The request is `chat.CompletionRequest{Model: model, Messages: msgs (with a first `system` message when `System` is not empty), Tools: specs}`. `model` is the conversation's `Model` (`"auto"` when empty), except that once a reply in this Run reported a `Backend` and `Model` and the conversation's model is `"auto"` or empty, later requests in this Run use `reply.Backend + "/" + reply.Model`, so one turn does not switch models. `specs` is every tool's `Spec()` when the conversation has a non-empty `Workspace`, otherwise nil.
     - `reply, err := completer.Complete(ctx, req, emit)`. On error return it wrapped. Append `reply` (role assistant) to the store.
     - No `reply.ToolCalls`: `emit(chat.Event{Type: chat.EventDone})` and return nil.
     - For each tool call in order: `emit(EventToolCall{CallID, Name, Arguments})`. Find the tool by `Spec().Name`; none: the result is `error: unknown tool "NAME"`. Otherwise, if `NeedsApproval(json.RawMessage(call.Arguments))`: `emit(EventApproval{CallID, Name, Arguments})` then `approver.Approve(ctx, chat.ApprovalRequest{ConversationID, CallID, Name, Arguments})`; an error from it is returned; a `false` makes the result `denied by the user`. Otherwise run it: `Run(ctx, workspace, json.RawMessage(call.Arguments))`; an error makes the result `error: ` + err text. A conversation with an empty `Workspace` makes every call's result `error: tools are off for this conversation (no workspace)` without running anything. Cut a result longer than 20000 bytes to its first 20000 bytes plus `\n[truncated]`. Append `chat.Message{Role: chat.RoleTool, ToolCallID: call.ID, Content: result}` and `emit(EventToolResult{CallID, Text: result})`.
  4. After `MaxSteps` rounds: `emit(EventError{Error: "stopped after N steps"})`, then `emit(EventDone)`, return nil.
  - Check `ctx.Err()` before each model call and each tool call; return it when set.
- `internal/agent/agent_test.go`: package `agent`. Write in-memory fakes in the test file: a `chat.Store` (a map and slices; implement all seven methods), a scripted `chat.Completer` (a list of replies; records each request; calls `emit` with an info and a content event for each), an `Approver` that returns a set answer and records requests, and a `chat.Tool` with a configurable name, `NeedsApproval` and result. Cover: a plain turn stores user and assistant messages, emits info, content, done, and sets the title; a second turn does not change the title; empty text is `chat.ErrInvalid`; unknown conversation is `chat.ErrNotFound`; a `System` prompt is the first message of the request and is not stored; tools are offered only with a workspace; a tool call without approval runs, stores assistant and tool messages in order, emits `tool_call` then `tool_result`, and the second request contains the tool message; a tool needing approval emits `approval` and waits, `true` runs it, `false` gives `denied by the user` and does not run it; an approver error is returned; an unknown tool name gives the unknown-tool result; a tool error gives `error: ...`; a long result is cut; an empty workspace gives the tools-off result; with conversation model `auto` the second request in a turn uses `backend/model` from the first reply, and with model `code` it stays `code`; `MaxSteps` 2 with a completer that always calls tools emits the step-limit error then done; a completer error is returned and no assistant message is stored; a cancelled context returns `context.Canceled`.

## Contract
`agent.New`, `Options`, `(*Engine).Run` as above. The interfaces and types it uses, from `internal/chat/chat.go`:

```go
// Package chat (internal/chat/chat.go), the types you use:
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a request from the model to run a tool. Arguments is the JSON
// text the model produced.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Seq        int        `json:"seq"` // numbers a conversation's messages from 1
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // set on role "tool"
	Route      string     `json:"route,omitempty"`        // how the router chose an assistant message's model
	Backend    string     `json:"backend,omitempty"`
	Model      string     `json:"model,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`     // "auto", a route name, or "backend/model"
	Workspace string    `json:"workspace"` // directory the tools may use; "" means tools are off
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConversationPatch struct {
	Title     *string `json:"title,omitempty"`
	Model     *string `json:"model,omitempty"`
	Workspace *string `json:"workspace,omitempty"`
}
```

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

// Event is one thing that happens during a turn. Type says which fields are set.
type Event struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`      // content, reasoning (a delta); tool_result (the result)
	Route     string `json:"route,omitempty"`     // info
	Backend   string `json:"backend,omitempty"`   // info
	Model     string `json:"model,omitempty"`     // info
	CallID    string `json:"call_id,omitempty"`   // tool_call, approval, tool_result
	Name      string `json:"name,omitempty"`      // tool_call, approval
	Arguments string `json:"arguments,omitempty"` // tool_call, approval
	Error     string `json:"error,omitempty"`     // error
}

const (
	EventInfo       = "info"        // routing for the assistant message about to stream
	EventContent    = "content"     // a piece of the answer
	EventReasoning  = "reasoning"   // a piece of the model's reasoning
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventApproval   = "approval"    // the person must approve that call
	EventToolResult = "tool_result" // the call finished (or was denied)
	EventError      = "error"
	EventDone       = "done" // always the last event of a turn
)
```

```go
type Store interface {
	// CreateConversation stores c with a new ID, CreatedAt and UpdatedAt set, and returns it.
	CreateConversation(ctx context.Context, c Conversation) (Conversation, error)
	// GetConversation returns ErrNotFound for an unknown id (also for the methods below).
	GetConversation(ctx context.Context, id string) (Conversation, error)
	// ListConversations returns every conversation, most recently updated first. Never nil.
	ListConversations(ctx context.Context) ([]Conversation, error)
	// UpdateConversation applies the non-nil fields of p and sets UpdatedAt.
	UpdateConversation(ctx context.Context, id string, p ConversationPatch) (Conversation, error)
	// DeleteConversation removes the conversation and its messages.
	DeleteConversation(ctx context.Context, id string) error
	// AppendMessage stores m as the conversation's next message (Seq is the
	// previous Seq plus 1; any Seq in m is ignored), sets CreatedAt, bumps
	// the conversation's UpdatedAt, and returns the stored message.
	AppendMessage(ctx context.Context, conversationID string, m Message) (Message, error)
	// ListMessages returns the conversation's messages oldest first. Never nil.
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
}

type CompletionRequest struct {
	Model    string // "auto", a route name, or "backend/model"
	Messages []Message
	Tools    []ToolSpec // none means no tools are offered
}

// Completer asks the router for one assistant message. It calls emit for
// each event as it streams (info, then content and reasoning), and returns
// the assembled assistant message: Content, Reasoning, ToolCalls, and
// Route, Backend and Model as the router reported them.
type Completer interface {
	Complete(ctx context.Context, req CompletionRequest, emit func(Event)) (Message, error)
}

type ApprovalRequest struct {
	ConversationID string
	CallID         string
	Name           string
	Arguments      string
}

// Approver asks the person whether a tool call may run. It blocks until the
// person answers or ctx ends.
type Approver interface {
	Approve(ctx context.Context, r ApprovalRequest) (bool, error)
}

type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}
```

## Done when
- `go test ./internal/agent/` passes.
- `go vet ./internal/agent/` succeeds and `gofmt -l internal/agent` prints nothing.

## Out of scope
- Every other package. The real store, router client and tools are other issues; use fakes.
~~~~

### 2.4 Add the chat HTTP API

~~~~text
Epic: 2 Chat components
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. The one exception is `modernc.org/sqlite`, already in `go.mod`, which only `internal/sqlitestore` imports. Do not edit `go.mod` or `go.sum`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and import no other package under `internal/` except `llmchat/internal/chat`.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
The JSON and server-sent-events API the web page uses: list, create, read, update and delete conversations, send a message and stream the turn, answer an approval request, and list the router's routes.

## Read first
- `internal/chat/chat.go`: `Store`, `Conversation`, `ConversationPatch`, `Message`, `Event`, `Approver`, `ApprovalRequest`, `ErrNotFound`, `ErrInvalid`.

## Change
- `internal/api/api.go`: package `api`, with

      // Runner runs one turn; internal/agent.Engine satisfies it.
      type Runner interface {
      	Run(ctx context.Context, conversationID, text string, emit func(chat.Event)) error
      }

      // Approvals holds the tool calls waiting for the person. It implements chat.Approver.
      type Approvals struct { /* mutex, map */ }
      func NewApprovals() *Approvals
      // Approve registers r and blocks until Resolve answers it (return the
      // answer) or ctx ends (return false and ctx.Err()). The key is r.ConversationID + "/" + r.CallID.
      func (a *Approvals) Approve(ctx context.Context, r chat.ApprovalRequest) (bool, error)
      // Resolve answers a waiting request. It returns an error wrapping chat.ErrNotFound if none is waiting.
      func (a *Approvals) Resolve(conversationID, callID string, approve bool) error

      // NewHandler serves the API. routes lists the router's route names.
      func NewHandler(store chat.Store, runner Runner, approvals *Approvals, routes func(ctx context.Context) ([]string, error)) http.Handler

- Routes (Go 1.22 `ServeMux` patterns); JSON bodies are decoded with `DisallowUnknownFields` and limited to 1 MiB; a bad body is 400:
  - `GET /api/conversations` → 200, `[]chat.Conversation`.
  - `POST /api/conversations`, body `{"title":"","model":"","workspace":""}` (every field optional) → 201, the conversation (`CreateConversation`).
  - `GET /api/conversations/{id}` → 200 `{"conversation": {...}, "messages": [...]}`.
  - `PATCH /api/conversations/{id}`, body a `chat.ConversationPatch` → 200, the conversation.
  - `DELETE /api/conversations/{id}` → 204.
  - `POST /api/conversations/{id}/messages`, body `{"text":"..."}` → a server-sent-events stream. Before streaming: unknown id 404; text empty after trimming 400; another turn already running for this conversation 409 (keep a `map[string]bool` under a mutex, cleared when the turn ends). Then set `Content-Type: text/event-stream`, `Cache-Control: no-cache`, status 200, and call `runner.Run(r.Context(), id, text, emit)` where `emit` writes `data: ` + the event as JSON + `\n\n` and flushes with `http.NewResponseController(w).Flush()`. Serialize writes with a mutex (events may come from more than one goroutine). If `Run` returns an error, emit `chat.Event{Type: chat.EventError, Error: err.Error()}` then `chat.Event{Type: chat.EventDone}`.
  - `POST /api/conversations/{id}/approvals/{call}`, body `{"approve":true}` → 204 after `approvals.Resolve`; nothing waiting is 404.
  - `GET /api/routes` → 200, a JSON array of strings (`[]` when none); an error from `routes` is 502 with `{"error": "..."}`.
  - `GET /healthz` → 200 `{"status":"ok"}`.
  - Errors are `{"error": "<text>"}` with `Content-Type: application/json`: `chat.ErrInvalid` 400, `chat.ErrNotFound` 404, anything else 500 (log it with `log.Printf`).
- `internal/api/api_test.go`: package `api`. An in-memory `chat.Store` fake (map-based; write it in the test file), a fake `Runner` that emits scripted events, and `httptest`. Cover: every route's success; every error status above; the stream: the response `Content-Type`, one `data:` line per event in order, and the final `done`; a runner error becomes an `error` event then `done`; a second concurrent `POST .../messages` while the first is blocked answers 409 and the first can still finish; events arrive before the runner returns (the runner blocks on a channel after its first emit; the client reads that first line, then releases the runner); `Approvals`: `Approve` blocks until `Resolve(.., true)` and returns true, `Resolve(.., false)` returns false, ctx cancel returns the error, `Resolve` with nothing waiting wraps `chat.ErrNotFound`, two conversations with the same call id do not interfere; the approvals route answers 204 then 404 the second time; an unknown JSON field is 400; `GET /api/routes` with a failing function is 502. Add `var _ chat.Approver = (*Approvals)(nil)`.

## Contract
`api.NewHandler`, `api.NewApprovals`, `api.Runner`, `*api.Approvals` as above.

```go
// Package chat (internal/chat/chat.go), the types you use:
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a request from the model to run a tool. Arguments is the JSON
// text the model produced.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Message struct {
	Seq        int        `json:"seq"` // numbers a conversation's messages from 1
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"` // set on role "tool"
	Route      string     `json:"route,omitempty"`        // how the router chose an assistant message's model
	Backend    string     `json:"backend,omitempty"`
	Model      string     `json:"model,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`     // "auto", a route name, or "backend/model"
	Workspace string    `json:"workspace"` // directory the tools may use; "" means tools are off
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ConversationPatch struct {
	Title     *string `json:"title,omitempty"`
	Model     *string `json:"model,omitempty"`
	Workspace *string `json:"workspace,omitempty"`
}
```

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

// Event is one thing that happens during a turn. Type says which fields are set.
type Event struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`      // content, reasoning (a delta); tool_result (the result)
	Route     string `json:"route,omitempty"`     // info
	Backend   string `json:"backend,omitempty"`   // info
	Model     string `json:"model,omitempty"`     // info
	CallID    string `json:"call_id,omitempty"`   // tool_call, approval, tool_result
	Name      string `json:"name,omitempty"`      // tool_call, approval
	Arguments string `json:"arguments,omitempty"` // tool_call, approval
	Error     string `json:"error,omitempty"`     // error
}

const (
	EventInfo       = "info"        // routing for the assistant message about to stream
	EventContent    = "content"     // a piece of the answer
	EventReasoning  = "reasoning"   // a piece of the model's reasoning
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventApproval   = "approval"    // the person must approve that call
	EventToolResult = "tool_result" // the call finished (or was denied)
	EventError      = "error"
	EventDone       = "done" // always the last event of a turn
)
```

```go
type Store interface { /* the seven methods in internal/chat/chat.go: CreateConversation, GetConversation,
	ListConversations, UpdateConversation, DeleteConversation, AppendMessage, ListMessages */ }

type ApprovalRequest struct {
	ConversationID string
	CallID         string
	Name           string
	Arguments      string
}

type Approver interface {
	Approve(ctx context.Context, r ApprovalRequest) (bool, error)
}
```

## Done when
- `go test ./internal/api/` passes.
- `go vet ./internal/api/` succeeds and `gofmt -l internal/api` prints nothing.

## Out of scope
- Every other package; the engine, store and web page are other issues. Do not import `internal/agent` or `internal/sqlitestore`.
~~~~

### 2.5 Add the chat web page

~~~~text
Epic: 2 Chat components
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. The one exception is `modernc.org/sqlite`, already in `go.mod`, which only `internal/sqlitestore` imports. Do not edit `go.mod` or `go.sum`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists, and import no other package under `internal/` except `llmchat/internal/chat`.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
The web page for chatting: a sidebar of saved conversations, a message log that streams the answer as it is written, a box to type in, and a model picker. It is plain HTML, CSS and JavaScript served from the Go binary, and it talks only to the JSON API below.

## Read first
- `../llmrouter/internal/ui/page.html`: the `#chat` section's inline script is a working example of reading a streamed `fetch` response and appending text with `textContent`. Read it for the technique; do not edit it.
- `../llmrouter/internal/ui/ui.go`: `//go:embed` and `html/template` usage. Do not edit it.

## Change
- `internal/web/web.go`: package `web`, with

      // NewHandler serves the page: GET /{$} returns page.html, GET /app.js
      // returns app.js with Content-Type "text/javascript; charset=utf-8".
      func NewHandler() http.Handler

  Embed both files with `//go:embed page.html app.js` into an `embed.FS`. Serve `page.html` as `text/html; charset=utf-8` without templating. Every other path is 404.
- `internal/web/page.html`: a complete HTML5 document with `<title>llmchat</title>`, a `<style>` block, and `<script src="/app.js" defer></script>`. Do not load anything from another origin. Layout (CSS grid, two columns, the sidebar 260px, stacks on narrow screens):
  - `<aside id="sidebar">`: a `<button id="new-chat">New chat</button>` and `<ul id="conversations"></ul>`.
  - `<main id="main">`: a `<header>` with `<span id="title">`, `<select id="model"></select>` and `<button id="delete" type="button">Delete</button>`; `<div id="log"></div>`; and `<form id="composer">` with `<textarea id="prompt" rows="3" placeholder="Ask something…" required></textarea>`, `<button id="send">Send</button>`.
  - CSS variables `--fg --bg --muted --line --accent --err` with a `prefers-color-scheme: dark` block. `#log` scrolls (`overflow-y: auto`, takes the remaining height). `.msg` has padding, a `1px solid var(--line)` border and a radius; `.msg.user` is right-aligned with a tinted background; `.msg.assistant` is left-aligned; `.msg .content` uses `white-space: pre-wrap`; `.meta` is small and `var(--muted)`; `pre` has a border, `overflow-x: auto` and a monospace font; `details.reasoning` is muted; `.msg.err` uses `var(--err)`; the active `#conversations li` is highlighted.
- `internal/web/app.js`: plain JavaScript, no libraries, no `innerHTML` with unescaped text. It talks to this API (all paths relative to the page's origin):

      GET    /api/conversations                         -> [{"id","title","model","workspace","created_at","updated_at"}]
      POST   /api/conversations   {"model":"auto"}      -> 201 conversation
      GET    /api/conversations/ID                      -> {"conversation":{...},"messages":[{"seq","role","content","reasoning","route","backend","model"}]}
      PATCH  /api/conversations/ID  {"model":"code"}    -> conversation
      DELETE /api/conversations/ID                      -> 204
      POST   /api/conversations/ID/messages {"text":"..."} -> text/event-stream of `data: {event}` lines
      GET    /api/routes                                -> ["general","code"]

  Event JSON: `{"type":"info","route":"code","backend":"ollama","model":"llama3:8b"}`, `{"type":"content","text":"..."}`, `{"type":"reasoning","text":"..."}`, `{"type":"error","error":"..."}`, `{"type":"done"}`. Ignore any other event type (a later issue adds tool events).
  Behaviour:
  1. On load: `GET /api/routes` and fill `#model` with `auto` first, then each route; if that fails only `auto` remains. `GET /api/conversations` and render the sidebar: one `<li>` per conversation showing its title (or `New chat` when empty); clicking opens it. Open the first conversation if there is one.
  2. Opening a conversation: `GET` it, set `#title`, set `#model` to its model (add an `<option>` for it if it is not in the list), and render its messages into `#log`. A `user` message is `<div class="msg user"><div class="content">…</div></div>`. An `assistant` message with non-empty content is `<div class="msg assistant">` with: `<details class="reasoning"><summary>Reasoning</summary>…</details>` when it has reasoning, `<div class="content">` with the content rendered by `renderMarkdown`, and `<div class="meta">route R · backend/model</div>` (omit `route R · ` when empty, omit the whole line when there is no backend). Messages of other roles, and assistant messages with empty content, are skipped.
  3. `renderMarkdown(text)` returns an HTML string: first escape `&`, `<`, `>` and `"`; then turn each fenced block (a line starting with three backticks, an optional language, the code, a closing three backticks) into `<pre><code>…</code></pre>` and each pair of single backticks in the remaining text into `<code>…</code>`. Everything else stays text (the CSS keeps newlines). Never produce any other tags.
  4. `#new-chat`: `POST /api/conversations` with the selected model, add it to the top of the sidebar, open it.
  5. Submitting `#composer`: ignore empty or whitespace-only text. If no conversation is open, create one first. Append the user message and an empty assistant bubble to `#log`, clear and disable the textarea and `#send`. `fetch` the messages endpoint, read `resp.body.getReader()` with a `TextDecoder`, split the stream into lines keeping an unfinished tail in a buffer, and handle each `data: ` line: `info` stores the route, backend and model for the meta line; `content` appends to a raw-text string and re-renders the bubble's `.content` with `renderMarkdown`; `reasoning` appends to the `<details class="reasoning">` text (create it on the first reasoning event, inside the bubble before the content, with `textContent`); `error` adds a `<div class="msg err">` with `textContent`; `done` ends. A non-200 response shows `Error STATUS: BODY` in an error bubble. When the stream ends: write the meta line, re-enable the form, focus the textarea, scroll `#log` to the bottom, and refresh the sidebar (the title is set by the server on the first message).
  6. Keep `#log` scrolled to the bottom while streaming unless the person has scrolled up.
  7. Enter sends; Shift+Enter inserts a newline.
  8. `#model` change: `PATCH` the open conversation's model (do nothing when none is open).
  9. `#delete`: `confirm("Delete this conversation?")` then `DELETE`; remove it from the sidebar and open the next one, or clear `#log` and `#title` when none is left.
- `internal/web/web_test.go`: package `web`, `httptest`. Cover: `GET /` is 200 with `text/html` and contains `id="sidebar"`, `id="conversations"`, `id="log"`, `id="composer"`, `id="prompt"`, `id="model"`, `id="new-chat"`, `id="delete"` and `src="/app.js"`; `GET /app.js` is 200 with a JavaScript content type and contains each of: `/api/conversations`, `/api/routes`, `/messages`, `renderMarkdown`, `getReader` and `textContent`, and that it does not contain `eval(` or `document.write`; `GET /nope` is 404; `POST /` is 405 or 404.

## Contract
`web.NewHandler() http.Handler`; element ids `sidebar conversations log composer prompt model new-chat delete title send`; the API and event shapes above.

## Done when
- `go test ./internal/web/` passes.
- `go vet ./internal/web/` succeeds and `gofmt -l internal/web` prints nothing.
- `grep -c 'renderMarkdown' internal/web/app.js` prints at least 2.

## Out of scope
- Every other package. The API is another issue; do not call it from Go.
- Tool-call rendering, approvals and the workspace field; a later issue adds them.
~~~~

## Epic 3: Chat wiring

Done when: The server builds and its app package passes its tests; the README exists.

### 3.1 Wire the llmchat server

~~~~text
Epic: 3 Chat wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `README.md` at the same time. Do not touch it.

## Goal
The server itself: a composition package that builds the store, router client, engine, API and page from configuration and serves them on one address, and the `main` that runs it.

## Read first
- `internal/sqlitestore/sqlitestore.go`: `Open`, `Close`.
- `internal/routerclient/routerclient.go`: `New`, `(*Client).Routes`.
- `internal/agent/agent.go`: `New`, `Options`.
- `internal/api/api.go`: `NewApprovals`, `NewHandler`.
- `internal/web/web.go`: `NewHandler`.
The constructors you call, verbatim:

    sqlitestore.Open(path string, now func() time.Time) (*sqlitestore.Store, error)   // implements chat.Store; has Close() error
    routerclient.New(baseURL string, hc *http.Client) *routerclient.Client             // implements chat.Completer; has Routes(ctx) ([]string, error)
    api.NewApprovals() *api.Approvals                                                  // implements chat.Approver
    agent.New(store chat.Store, completer chat.Completer, tools []chat.Tool, approver chat.Approver, opts agent.Options) *agent.Engine   // has Run(...)
    api.NewHandler(store chat.Store, runner api.Runner, approvals *api.Approvals, routes func(context.Context) ([]string, error)) http.Handler
    web.NewHandler() http.Handler

## Change
- `internal/app/app.go`: package `app`, with

      type Config struct {
      	Addr      string // listen address
      	DBPath    string // SQLite file
      	RouterURL string // the llmrouter base URL
      }

      // ConfigFromEnv reads CHAT_ADDR (default "127.0.0.1:8084"), CHAT_DB
      // (default: $XDG_DATA_HOME/llmchat/llmchat.db, else
      // ~/.local/share/llmchat/llmchat.db) and CHAT_ROUTER_URL (default
      // "http://127.0.0.1:8083").
      func ConfigFromEnv() Config

      type App struct {
      	Handler http.Handler
      	// unexported: the store
      }

      // New opens the database (creating its directory with os.MkdirAll,
      // mode 0o755, first) and builds everything. It starts nothing.
      func New(cfg Config) (*App, error)

      func (a *App) Close() error

  The handler is an `http.ServeMux`: `mux.Handle("/api/", apiHandler)`, `mux.Handle("/healthz", apiHandler)`, `mux.Handle("/", webHandler)`. The engine gets no tools (`nil`), `agent.Options{}`, and the `*api.Approvals` as its approver; the API gets the same `*api.Approvals`; `routes` is `routerClient.Routes`.
- `cmd/llmchat/main.go`: `config := app.ConfigFromEnv()`; `app.New`; an `http.Server{Addr, Handler, ReadHeaderTimeout: 10 * time.Second}` run in a goroutine; log `llmchat listening on http://ADDR (router CHAT_ROUTER_URL, db PATH)`; on `os.Interrupt` or `SIGTERM` (`signal.NotifyContext`) call `Shutdown` with a 10 second timeout, then `a.Close()`. A listen error other than `http.ErrServerClosed` or an `app.New` error is logged and exits 1.
- `internal/app/app_test.go`: package `app`. A fake router `httptest.Server` that answers `GET /api/routes` with `{"default":"general","routes":[{"name":"general"},{"name":"code"}]}` and `POST /v1/chat/completions` with an SSE stream: headers `X-Llmrouter-Route: general`, `X-Llmrouter-Backend: ollama`, `X-Llmrouter-Model: m`, body `data: {"choices":[{"delta":{"content":"hel"}}]}\n\ndata: {"choices":[{"delta":{"content":"lo"}}]}\n\ndata: [DONE]\n\n`. Build `app.New(Config{Addr: "127.0.0.1:0", DBPath: filepath.Join(t.TempDir(), "sub", "chat.db"), RouterURL: fake.URL})` and serve `a.Handler` with `httptest.NewServer`. Check: `GET /healthz` 200; `GET /` 200 containing `id="composer"`; `GET /api/routes` returns `["general","code"]`; create a conversation, `POST .../messages` with `{"text":"hi"}` streams `info`, two `content` events and `done`; `GET /api/conversations/ID` then has two messages (user `hi`, assistant `hello` with route `general`); `Close` returns nil; `ConfigFromEnv` with `t.Setenv` for the three variables returns them, and with all unset (set them to "" and set `XDG_DATA_HOME` to a temp dir) returns the defaults with the DB path under that temp dir.

## Contract
`app.Config`, `app.ConfigFromEnv() Config`, `app.New(cfg Config) (*App, error)`, `(*App).Close() error`, `App.Handler`. Environment variables `CHAT_ADDR`, `CHAT_DB`, `CHAT_ROUTER_URL`.

## Done when
- `go build -o /dev/null ./cmd/llmchat` succeeds.
- `go test ./internal/app/` passes.
- `go vet ./internal/app/ ./cmd/llmchat/` succeeds and `gofmt -l internal/app cmd` prints nothing.

## Out of scope
- `README.md`.
- Changes to the components; if one has a bug, block this issue with `bin/codeplat issue block` from the repository root and describe it.
~~~~

### 3.2 Document llmchat

~~~~text
Epic: 3 Chat wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `internal/app/` and `cmd/llmchat/` at the same time. Do not touch those.

## Goal
A README that tells a person how to run llmchat against the LLM router and how it is put together.

## Read first
- `../llmrouter/README.md`: the style to follow.
- `internal/chat/chat.go`: the types; `internal/api/api.go`: the routes (you need their exact paths).

## Change
- `README.md` with these sections:
  - What it is (two or three sentences): a chat application that sends conversations through the LLM router (`samples/llmrouter`) instead of a hosted model; conversations are saved in SQLite; the page streams the answer and shows which route and model answered.
  - Run: start the router first (`cd ../llmrouter && make run`, which listens on http://127.0.0.1:8083), then here `make run` and open http://127.0.0.1:8084.
  - Configuration: a table of `CHAT_ADDR` (default `127.0.0.1:8084`), `CHAT_DB` (default `$XDG_DATA_HOME/llmchat/llmchat.db`, else `~/.local/share/llmchat/llmchat.db`), `CHAT_ROUTER_URL` (default `http://127.0.0.1:8083`).
  - Models: the picker offers `auto` (the router classifies each message and picks a route), each route by name, and the conversation remembers its choice.
  - API: a table of `GET/POST /api/conversations`, `GET/PATCH/DELETE /api/conversations/{id}`, `POST /api/conversations/{id}/messages` (server-sent events; the event types `info`, `content`, `reasoning`, `tool_call`, `approval`, `tool_result`, `error`, `done`), `POST /api/conversations/{id}/approvals/{call}`, `GET /api/routes`, `GET /healthz`.
  - Layout: one line per package under `internal/` and `cmd/llmchat`: `chat`, `sqlitestore`, `routerclient`, `agent`, `api`, `web`, `app`, and `tools/...` (say "tools: added in a later stage" for the last).
  - Test: `make test`; `make live` is added by a later issue.
- Do not describe tools, workspaces or approvals as working; the later stage adds them.

## Contract
File path `README.md`.

## Done when
- `grep -c 'CHAT_ROUTER_URL' README.md` prints at least 1.
- `grep -c '/api/conversations' README.md` prints at least 1.
- `test -f README.md` succeeds.

## Out of scope
- Every Go file and the `Makefile`.
~~~~

## Epic 4: Chat verify

Done when: An end-to-end test passes against a fake router, and an opt-in live test exists.

### 4.1 Add an end-to-end chat test

~~~~text
Epic: 4 Chat verify
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `internal/live/` and editing the `Makefile` at the same time. Do not touch those.

## Goal
One test that runs the whole server, built by `app.New`, against a fake router, and proves chat, streaming, saving and restarting work together.

## Read first
- `internal/app/app.go`: `Config`, `New`, `Close`, `Handler`.
- `internal/chat/chat.go`: `Event`, `Conversation`, `Message`.

## Change
- `internal/e2e/e2e_test.go`: package `e2e_test`.
  - A fake router `httptest.Server`. `GET /api/routes` answers `{"default":"general","routes":[{"name":"general"},{"name":"code"}]}`. `POST /v1/chat/completions` records the decoded request body (guard the recording with a `sync.Mutex`), and answers with `Content-Type: text/event-stream` and headers `X-Llmrouter-Route: code`, `X-Llmrouter-Backend: ollama`, `X-Llmrouter-Model: qwen`. If the last message's content contains `think` the stream starts with a `reasoning_content` delta `hmm`. The answer is the text `answer to: ` plus the last user message's content, sent as one `content` delta per word, ending with `data: [DONE]`. If the last message's content contains `fail` it answers 503 with body `router down`.
  - A helper that posts to `/api/conversations/ID/messages` and returns the parsed events (read the stream to its end; each `data: ` line is a `chat.Event`).
  - A config with `Addr: "127.0.0.1:0"`, `DBPath` in `t.TempDir()` and `RouterURL` the fake's URL. Serve `a.Handler` with `httptest.NewServer`.
  - Check, in order:
    1. `GET /api/routes` is `["general","code"]`; `GET /` contains `id="composer"`.
    2. `POST /api/conversations` with `{"model":"auto"}` is 201 and the response has a 12-character `id`.
    3. Send `hello world`: events are `info` (route `code`, backend `ollama`, model `qwen`), then `content` events whose texts joined equal `answer to: hello world`, then `done` last; the fake received `model` `auto`, `stream` true, no `tools` key, and one `user` message `hello world`.
    4. `GET /api/conversations/ID` has two messages: user `hello world`, and assistant with that content and route `code`, backend `ollama`, model `qwen`, seqs 1 and 2; the title is `hello world`.
    5. Send `think about it`: there is a `reasoning` event with `hmm`; the fake's request has three messages (the earlier user and assistant messages, then the new user message); the stored assistant message has `reasoning` `hmm`.
    6. Send `fail please`: the events are an `error` event whose text contains `503` and `router down`, then `done`; the conversation still has the user message and no new assistant message.
    7. `PATCH` the conversation's model to `code`; the next send makes the fake receive model `code`.
    8. Two conversations: `GET /api/conversations` lists both, the one just used first.
    9. `DELETE` one: 204, then `GET` it is 404 and the list has one.
    10. Persistence: `Close` the app, build a new one on the same `DBPath`, and the remaining conversation and its messages are still there.
    11. A message to an unknown conversation id is 404; an empty `text` is 400.

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/e2e/` passes.
- `go vet ./internal/e2e/` succeeds and `gofmt -l internal/e2e` prints nothing.

## Out of scope
- Every non-test file. If the test finds a bug in a component, block this issue with `bin/codeplat issue block` from the repository root and describe the failing check; do not fix the component.
- `internal/live/` and the `Makefile`.
~~~~

### 4.2 Add an opt-in live chat test

~~~~text
Epic: 4 Chat verify
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `internal/e2e/` at the same time. Do not touch it.
- On this machine an llmrouter may be running at http://127.0.0.1:8083 with real models behind it. Do not start, stop or configure it.

## Goal
A test, skipped unless asked for, that runs llmchat against a real running llmrouter, so a person can check the whole stack with `make live`.

## Read first
- `internal/app/app.go`: `Config`, `New`, `Close`, `Handler`.
- `Makefile`: the existing targets.

## Change
- `internal/live/live_test.go`: package `live_test`. Every test starts with `if os.Getenv("LLMCHAT_LIVE") != "1" { t.Skip("set LLMCHAT_LIVE=1 to run against a real router") }`.
  - `TestChat`: router URL from `LLMCHAT_LIVE_ROUTER` (default `http://127.0.0.1:8083`). If `GET <router>/api/routes` fails, `t.Skip` saying the router is not reachable. Build the app with `app.New(app.Config{Addr: "127.0.0.1:0", DBPath: <t.TempDir()>/chat.db, RouterURL: <router>})`, serve its `Handler` with `httptest.NewServer`, `t.Cleanup(func() { a.Close() })`. Create a conversation with model `auto`, send `Reply with the single word: pong` (client timeout 5 minutes), parse the events, and expect: an `info` event, at least one `content` event, `done` last, no `error` event. Log the route, backend, model and the joined content. Then `GET` the conversation and expect two messages, the second non-empty.
  - `TestRoutePinned`: skip unless `LLMCHAT_LIVE_ROUTE` is set. Create a conversation with that model (a route name), send `Say hello.`, expect the `info` event's `route` to equal it.
- `Makefile`: add `live` to `.PHONY` and a target

      live:
      	LLMCHAT_LIVE=1 go test -count=1 -v -timeout 20m ./internal/live/

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/live/` passes (the tests skip).
- `LLMCHAT_LIVE=1 go test -count=1 -v -timeout 10m -run TestChat ./internal/live/` passes if a router is reachable at `http://127.0.0.1:8083`, and skips otherwise.
- `go vet ./internal/live/` succeeds and `gofmt -l internal/live` prints nothing.

## Out of scope
- `internal/e2e/`. Every non-test file except `Makefile`. If the live run finds a bug in a component, block this issue with `bin/codeplat issue block` from the repository root and describe what failed.
~~~~

## Epic 5: Sandbox

Done when: `sandbox.Resolve` confines every path to a workspace.

### 5.1 Add the workspace sandbox

~~~~text
Epic: 5 Sandbox
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.

## Goal
The one function every tool uses to turn a path the model gave into a real path inside the conversation's workspace, refusing anything that would escape it.

## Read first
- `internal/chat/chat.go`: `ErrInvalid`.

## Change
- `internal/tools/sandbox/sandbox.go`: package `sandbox`, with

      // Resolve returns the absolute, cleaned path that rel names inside
      // workspace. rel may be relative (to workspace) or absolute. It returns
      // an error wrapping chat.ErrInvalid when workspace is not an absolute
      // path to an existing directory, or when the path, after cleaning and
      // after resolving symbolic links, is not workspace itself or inside it.
      // The path does not have to exist: for the part that does not exist
      // yet, resolve symbolic links on the longest existing ancestor and
      // append the rest. An empty rel means the workspace itself.
      func Resolve(workspace, rel string) (string, error)

      // Rel returns path relative to workspace using forward slashes, "." for
      // the workspace itself. path is a result of Resolve.
      func Rel(workspace, path string) string

  Resolve the workspace itself with `filepath.EvalSymlinks` first, and compare resolved paths (so a workspace reached through a symlink works, e.g. macOS `/tmp`; compare against `ws + string(filepath.Separator)` so `/a/b` does not match `/a/bc`). Error messages name the offending path and say `outside the workspace`.
- `internal/tools/sandbox/sandbox_test.go`: package `sandbox`. Use `t.TempDir()` (resolve its symlinks first with `filepath.EvalSymlinks`). Cover: empty rel is the workspace; `a/b.txt` (not existing) is workspace-joined; `./a/../b` cleans to `b`; an absolute path inside works; an absolute path outside is `chat.ErrInvalid`; `..` and `../x` and `a/../../x` are `chat.ErrInvalid`; a sibling directory whose name starts with the workspace name (`ws` and `ws2`) is rejected; a symlink inside the workspace pointing to a directory outside is rejected for `link/file` (existing target) and for `link/new.txt` (not existing); a symlink inside pointing to another place inside is allowed; a workspace that is relative, missing, or a file is `chat.ErrInvalid`; `Rel` gives `.` for the workspace, `a/b.txt` for a nested path.

## Contract
`sandbox.Resolve(workspace, rel string) (string, error)` and `sandbox.Rel(workspace, path string) string`, as above.

## Done when
- `go test ./internal/tools/sandbox/` passes.
- `go vet ./internal/tools/sandbox/` succeeds and `gofmt -l internal/tools/sandbox` prints nothing.

## Out of scope
- The tools themselves; later issues add them. Every other package.
~~~~

## Epic 6: Tools

Done when: The six tools (`read_file`, `list_dir`, `grep`, `write_file`, `edit_file`, `bash`) each pass their own tests.

### 6.1 Add the file reading tools

~~~~text
Epic: 6 Tools
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are writing the other packages under `internal/tools/` at the same time. Import only `llmchat/internal/chat` and `llmchat/internal/tools/sandbox`.

## Goal
Two read-only tools the model can call: `read_file` and `list_dir`, confined to the conversation's workspace.

## Read first
- `internal/chat/chat.go`: the `Tool` interface and `ToolSpec`.
- `internal/tools/sandbox/sandbox.go`: `Resolve` and `Rel`.

## Change
- `internal/tools/fs/fs.go`: package `fs`, with `func ReadFile() chat.Tool` and `func ListDir() chat.Tool`. Neither needs approval (`NeedsApproval` returns false). Both decode their arguments with `json.Unmarshal` into a struct and return an error wrapping `chat.ErrInvalid` for bad JSON. Every path goes through `sandbox.Resolve(workspace, path)`; its error is returned as is.
  - `read_file`: description `Read a text file in the workspace. Returns the lines with line numbers.`; parameters schema `{"type":"object","properties":{"path":{"type":"string","description":"File path, relative to the workspace"},"offset":{"type":"integer","description":"First line to read, from 1 (default 1)"},"limit":{"type":"integer","description":"Number of lines to read (default 2000)"}},"required":["path"]}`. The output has one line per source line formatted `fmt.Sprintf("%6d\t%s\n", n, line)`. A directory is an error `path is a directory`. A file containing a NUL byte in its first 8000 bytes is an error `binary file`. A file over 2 MiB is an error `file too large (N bytes); use offset and limit`, only when `offset` and `limit` are both unset; with either set read it line by line with `bufio.Reader` (no 2 MiB check). When lines remain after `limit`, end the output with `[more lines follow; use offset N]` with N the next line number. An empty file gives `(empty file)`.
  - `list_dir`: description `List the entries of a directory in the workspace.`; parameters `{"type":"object","properties":{"path":{"type":"string","description":"Directory path, relative to the workspace (default: the workspace)"}}}`. Output: one name per line, sorted, directories with a trailing `/`, at most 500 entries (then a last line `[N more entries not shown]`). A file path is an error `not a directory`. An empty directory gives `(empty directory)`.
- `internal/tools/fs/fs_test.go`: package `fs`. A temp workspace with files. Cover: `Spec()` names, that `Parameters` is valid JSON, `NeedsApproval` false for both; read a small file (exact output with the line-number format); `offset` and `limit` and the `[more lines follow; use offset N]` footer; empty file; a directory; a binary file; a missing file (the error mentions the path); `../x` and an absolute outside path rejected; invalid JSON arguments wrap `chat.ErrInvalid`; a 2 MiB+ file without `limit` is an error and with `limit` works; `list_dir` of the workspace and of a subdirectory (sorted, `/` suffix), of a file, of an empty directory, of 600 files (500 shown plus the footer).

## Contract
`fs.ReadFile() chat.Tool` (name `read_file`) and `fs.ListDir() chat.Tool` (name `list_dir`).

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// from internal/tools/sandbox:
func Resolve(workspace, rel string) (string, error) // absolute path inside workspace, or an error wrapping chat.ErrInvalid
func Rel(workspace, path string) string             // path relative to workspace with forward slashes, "." for the workspace
```

## Done when
- `go test ./internal/tools/fs/` passes.
- `go vet ./internal/tools/fs/` succeeds and `gofmt -l internal/tools/fs` prints nothing.

## Out of scope
- Every other package, especially the other tool packages being written now.
~~~~

### 6.2 Add the grep tool

~~~~text
Epic: 6 Tools
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are writing the other packages under `internal/tools/` at the same time. Import only `llmchat/internal/chat` and `llmchat/internal/tools/sandbox`.

## Goal
A read-only `grep` tool: search the workspace's files for a regular expression.

## Read first
- `internal/chat/chat.go`: the `Tool` interface and `ToolSpec`.
- `internal/tools/sandbox/sandbox.go`: `Resolve` and `Rel`.

## Change
- `internal/tools/search/search.go`: package `search`, with `func Grep() chat.Tool`. `NeedsApproval` returns false. Name `grep`; description `Search files in the workspace for a regular expression (Go RE2 syntax). Returns matching lines as path:line:text.`; parameters `{"type":"object","properties":{"pattern":{"type":"string","description":"Regular expression"},"path":{"type":"string","description":"File or directory to search, relative to the workspace (default: the workspace)"},"glob":{"type":"string","description":"Only search files whose base name matches this glob, e.g. *.go"},"ignore_case":{"type":"boolean"}},"required":["pattern"]}`.
  - Decode arguments; bad JSON or an empty or invalid pattern is an error wrapping `chat.ErrInvalid` (a pattern error message includes the `regexp` error). `ignore_case` prepends `(?i)`.
  - `sandbox.Resolve(workspace, path)`. A file path searches just that file; a directory is walked with `filepath.WalkDir`, in lexical order, skipping directories named `.git`, `node_modules` and `vendor`, skipping symbolic links, skipping files whose base name does not match `glob` (`filepath.Match`; an invalid glob is `chat.ErrInvalid`), and skipping binary files (a NUL byte in the first 8000 bytes) and files over 2 MiB.
  - Each match is the line `PATH:LINE:TEXT` with PATH from `sandbox.Rel` of the file, TEXT the line cut to 300 characters. Stop at 200 matches and end with `[stopped after 200 matches]`. No matches gives `no matches`. Respect `ctx.Done()` between files and return `ctx.Err()`.
- `internal/tools/search/search_test.go`: package `search`. A temp workspace with nested directories, a `.git` directory containing a match, a binary file, and a file with a 1000-character line. Cover: `Spec()` name and valid JSON parameters, `NeedsApproval` false; a match across files in sorted order with exact `path:line:text` lines; `glob` filters; `ignore_case`; a single-file path; `.git` and binary skipped; a long line is cut to 300; 250 matching lines stop at 200 with the footer; no matches; invalid regexp and empty pattern and bad glob and bad JSON are `chat.ErrInvalid`; `../` rejected; a cancelled context returns `context.Canceled`.

## Contract
`search.Grep() chat.Tool` (name `grep`).

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// from internal/tools/sandbox:
func Resolve(workspace, rel string) (string, error) // absolute path inside workspace, or an error wrapping chat.ErrInvalid
func Rel(workspace, path string) string             // path relative to workspace with forward slashes, "." for the workspace
```

## Done when
- `go test ./internal/tools/search/` passes.
- `go vet ./internal/tools/search/` succeeds and `gofmt -l internal/tools/search` prints nothing.

## Out of scope
- Every other package, especially the other tool packages being written now.
~~~~

### 6.3 Add the file writing tools

~~~~text
Epic: 6 Tools
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are writing the other packages under `internal/tools/` at the same time. Import only `llmchat/internal/chat` and `llmchat/internal/tools/sandbox`.

## Goal
Two tools that change files, each needing the person's approval: `write_file` (create or replace a file) and `edit_file` (replace exact text in a file), confined to the workspace.

## Read first
- `internal/chat/chat.go`: the `Tool` interface and `ToolSpec`.
- `internal/tools/sandbox/sandbox.go`: `Resolve` and `Rel`.

## Change
- `internal/tools/edit/edit.go`: package `edit`, with `func WriteFile() chat.Tool` and `func EditFile() chat.Tool`. `NeedsApproval` returns true for both, always. Arguments are decoded into a struct; bad JSON is an error wrapping `chat.ErrInvalid`. Every path goes through `sandbox.Resolve`.
  - `write_file`: description `Create a file, or replace it completely, in the workspace. Missing directories are created.`; parameters `{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`. Create parent directories with `os.MkdirAll(dir, 0o755)`, write with mode `0o644` (keep an existing file's mode). A directory path is an error. Result: `wrote N bytes to PATH` with PATH from `sandbox.Rel`, or `created PATH (N bytes)` when it did not exist before. Content over 1 MiB is an error.
  - `edit_file`: description `Replace exact text in an existing file. old_string must match exactly once unless replace_all is true.`; parameters `{"type":"object","properties":{"path":{"type":"string"},"old_string":{"type":"string"},"new_string":{"type":"string"},"replace_all":{"type":"boolean"}},"required":["path","old_string","new_string"]}`. Errors: an empty `old_string`; `old_string` equal to `new_string`; the file missing; `old_string not found in PATH`; and, without `replace_all`, `old_string appears N times in PATH; add surrounding context so it matches once`. Success: `replaced N occurrence(s) in PATH`. Write the file back with its existing mode.
- `internal/tools/edit/edit_test.go`: package `edit`. A temp workspace. Cover: both `Spec()` names and valid JSON parameters and `NeedsApproval` true; write a new file in a new subdirectory (content on disk, the `created` result); overwrite (the `wrote` result, mode kept); a directory path; `../x` and an outside absolute path rejected and nothing written outside; oversize content; bad JSON; edit one occurrence; edit with `replace_all` on three; ambiguous match error includes the count and leaves the file unchanged; not found; empty `old_string`; identical strings; a missing file; edit keeps a `0o755` mode.

## Contract
`edit.WriteFile() chat.Tool` (name `write_file`) and `edit.EditFile() chat.Tool` (name `edit_file`).

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// from internal/tools/sandbox:
func Resolve(workspace, rel string) (string, error) // absolute path inside workspace, or an error wrapping chat.ErrInvalid
func Rel(workspace, path string) string             // path relative to workspace with forward slashes, "." for the workspace
```

## Done when
- `go test ./internal/tools/edit/` passes.
- `go vet ./internal/tools/edit/` succeeds and `gofmt -l internal/tools/edit` prints nothing.

## Out of scope
- Every other package, especially the other tool packages being written now.
~~~~

### 6.4 Add the shell tool

~~~~text
Epic: 6 Tools
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are writing the other packages under `internal/tools/` at the same time. Import only `llmchat/internal/chat` and `llmchat/internal/tools/sandbox`.

## Goal
A `bash` tool that runs a shell command in the workspace, always needing the person's approval, with a time limit and bounded output.

## Read first
- `internal/chat/chat.go`: the `Tool` interface and `ToolSpec`.
- `internal/tools/sandbox/sandbox.go`: `Resolve`.

## Change
- `internal/tools/shell/shell.go`: package `shell`, with `func Bash() chat.Tool`. Name `bash`; description `Run a shell command with bash in the workspace directory. Returns the output and the exit status.`; parameters `{"type":"object","properties":{"command":{"type":"string"},"timeout_seconds":{"type":"integer","description":"Time limit in seconds (default 60, at most 300)"}},"required":["command"]}`. `NeedsApproval` returns true, always.
  - Decode arguments (bad JSON or an empty command wraps `chat.ErrInvalid`). The timeout is 60 seconds when unset or not positive, and at most 300. Check the workspace with `sandbox.Resolve(workspace, "")` and use the result as the directory.
  - Run `exec.Command("bash", "-c", command)` with `Dir` the workspace, stdin empty (`nil`), `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`, and stdout and stderr both written into one capped buffer (keep at most 30000 bytes: the first 15000 and the last 15000, with `\n[... output truncated ...]\n` between, when more). Run it with a context that has the timeout; on timeout or cancellation kill the whole process group (`syscall.Kill(-pid, syscall.SIGKILL)`) and wait for it. Do not use `exec.CommandContext` alone: it would leave child processes behind.
  - The result is the output, then a final line: `exit status 0` on success, `exit status N` on a non-zero exit, `timed out after Ns` on timeout. A command that cannot start (`bash` missing) returns an error. Environment: `os.Environ()`.
- `internal/tools/shell/shell_test.go`: package `shell`. A temp workspace. Cover: `Spec()` name, valid JSON parameters, `NeedsApproval` true; `echo hi` gives `hi\nexit status 0`; the working directory is the workspace (`pwd` equals it after `EvalSymlinks`); stderr is included (`echo err 1>&2`); `exit 3` gives `exit status 3`; `sleep 5` with `timeout_seconds` 1 returns within 3 seconds with `timed out after 1s`; a command that starts a background child (`sleep 30 & wait`) is killed on timeout (after the call, `pgrep -f 'sleep 30'` style checks are fragile: instead check the call returned within the limit); output of 100000 bytes is truncated to under 31000 bytes and contains the truncation marker; empty command and bad JSON are `chat.ErrInvalid`; a missing workspace is an error; a cancelled context ends the command.

## Contract
`shell.Bash() chat.Tool` (name `bash`).

```go
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"` // a JSON Schema object
}

type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// from internal/tools/sandbox:
func Resolve(workspace, rel string) (string, error) // absolute path inside workspace, or an error wrapping chat.ErrInvalid
func Rel(workspace, path string) string             // path relative to workspace with forward slashes, "." for the workspace
```

## Done when
- `go test ./internal/tools/shell/` passes, and passes again with `-count=3`.
- `go vet ./internal/tools/shell/` succeeds and `gofmt -l internal/tools/shell` prints nothing.

## Out of scope
- Every other package, especially the other tool packages being written now.
- Sandboxing the command beyond its working directory; the person's approval is the safeguard.
~~~~

## Epic 7: Agent wiring

Done when: The server offers the tools, the page shows tool calls and approvals, and the README describes them.

### 7.1 Offer the tools in the llmchat server

~~~~text
Epic: 7 Agent wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are editing `internal/web/` and `README.md` at the same time. Do not touch those.

## Goal
Switch the tools on: the server gives the engine the six tools and a system prompt, so a conversation with a workspace can read, search, edit files and run commands, with approval where the tools ask for it.

## Read first
- `internal/app/app.go`: `New`, where `agent.New(...)` is called with `nil` tools and `agent.Options{}`.
- `internal/app/app_test.go`: the fake router and the helpers there.
- `internal/agent/agent.go`: `Options` (`System`), and that tools are offered only when the conversation has a `Workspace`.
The constructors to add, verbatim:

    fs.ReadFile() chat.Tool     // name read_file, package llmchat/internal/tools/fs
    fs.ListDir() chat.Tool      // name list_dir
    search.Grep() chat.Tool     // name grep, package llmchat/internal/tools/search
    edit.WriteFile() chat.Tool  // name write_file, needs approval, package llmchat/internal/tools/edit
    edit.EditFile() chat.Tool   // name edit_file, needs approval
    shell.Bash() chat.Tool      // name bash, needs approval, package llmchat/internal/tools/shell

## Change
- `internal/app/app.go`: build `tools := []chat.Tool{fs.ReadFile(), fs.ListDir(), search.Grep(), edit.WriteFile(), edit.EditFile(), shell.Bash()}` and pass it with `agent.Options{System: SystemPrompt}`. Add an exported constant

      const SystemPrompt = "You are a coding assistant. You can read, search and edit files and run shell commands in the workspace directory using the provided tools. Look at files before changing them, make small focused edits, and explain briefly what you did. Tool calls that change files or run commands are shown to the person for approval first."

  Add a `Config` field `MaxSteps int` read by `ConfigFromEnv` from `CHAT_MAX_STEPS` (a positive integer; default 40 when unset, empty or invalid) and pass it as `agent.Options.MaxSteps`. Add a `Config` field `Workspace string` read by `ConfigFromEnv` from `CHAT_WORKSPACE` (default empty), used when the API creates a conversation without a workspace: wrap the `chat.Store` handed to the API in a small unexported type whose `CreateConversation` fills an empty `Workspace` with `cfg.Workspace` and delegates everything else. (It is the only way to give new conversations a default without changing the API package.)
- `internal/app/app_test.go`: add tests (keep the existing ones passing). The fake router must record each request body. Create a conversation with `"workspace": <t.TempDir()>` and send a message: the recorded request has a `tools` array whose function names are exactly `read_file, list_dir, grep, write_file, edit_file, bash` and a first message with role `system` and content starting `You are a coding assistant` and ending with `Workspace directory: ` plus the directory. A conversation without a workspace: no `tools` key. With `Config.Workspace` set to a temp directory, `POST /api/conversations` with `{}` returns that workspace. `ConfigFromEnv` reads `CHAT_WORKSPACE`, and `CHAT_MAX_STEPS` (default 40; `abc` and `0` give 40; `7` gives 7).

## Contract
`app.SystemPrompt` (constant above); `app.Config.Workspace string` and `app.Config.MaxSteps int`; env vars `CHAT_WORKSPACE` and `CHAT_MAX_STEPS`.

## Done when
- `go test ./internal/app/` passes.
- `go build -o /dev/null ./cmd/llmchat` succeeds.
- `go vet ./internal/app/` succeeds and `gofmt -l internal/app` prints nothing.

## Out of scope
- `internal/web/` and `README.md`, which other workers own. The tool packages and the agent package.
~~~~

### 7.2 Show tool calls and approvals in the chat page

~~~~text
Epic: 7 Agent wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is editing `internal/app/` and `README.md` at the same time. Do not touch those.

## Goal
Make the page work for agent conversations: show each tool call and its result as it happens, ask the person to approve calls that need it, and let them set a conversation's workspace directory.

## Read first
- `internal/web/app.js`: the existing event handling (`info`, `content`, `reasoning`, `error`, `done`), `renderMarkdown`, how messages are rendered when a conversation is opened, and the open conversation state.
- `internal/web/page.html`: the header and the CSS variables.
- `internal/web/web_test.go`: the existing assertions.

## Change
- `internal/web/page.html`: in the `<header>` add `<input id="workspace" type="text" placeholder="Workspace directory (enables tools)" size="32">` and `<button id="set-workspace" type="button">Set</button>` after the model select. Add CSS: `.msg.tool` and `details.tool` have the `.msg` look, monospace, muted; `pre.args` and `pre.result` have `max-height: 14rem; overflow: auto; white-space: pre-wrap`; `.approval` has a `1px solid var(--accent)` border and padding, and its two buttons `.approve` and `.deny` sit side by side; `.approval.done` is muted.
- `internal/web/app.js`:
  1. New events (the JSON shapes: `{"type":"tool_call","call_id":"c1","name":"read_file","arguments":"{\"path\":\"a.txt\"}"}`, `{"type":"approval","call_id":"c1","name":"bash","arguments":"..."}`, `{"type":"tool_result","call_id":"c1","text":"..."}`):
     - `tool_call`: add `<details class="tool" data-call="ID" open><summary>🔧 NAME</summary><pre class="args"></pre><pre class="result"></pre></details>` to `#log` after the current assistant bubble, set `.args` to the arguments pretty-printed with `JSON.stringify(JSON.parse(a), null, 2)` (fall back to the raw text), via `textContent`. A tool call after a result means the model is continuing: the next `info` event starts a new assistant bubble (so treat each `info` as the start of a new bubble, not only the first).
     - `approval`: inside that call's `<details>` append `<div class="approval">Run NAME? <button class="approve" type="button">Approve</button> <button class="deny" type="button">Deny</button></div>`. A click does `fetch("/api/conversations/ID/approvals/CALLID", {method:"POST", headers:{"Content-Type":"application/json"}, body: JSON.stringify({approve: true|false})})` (`false` for Deny), then disables both buttons and replaces the text with `Approved` or `Denied` (class `done`). The approval for a `bash` call shows the `command` field of the arguments as the visible question: `Run command: ...`; for `write_file` and `edit_file` show `Change PATH?` from the `path` field.
     - `tool_result`: set that call's `.result` text with `textContent`, remove its `.approval` block if still present.
  2. When a conversation is opened, render stored messages fully: an assistant message that has `tool_calls` shows its content (if any) and then one `<details class="tool">` per call (name and pretty arguments); a `role: "tool"` message fills the matching `<details class="tool" data-call="ID">`'s `.result` (match on `tool_call_id`). The messages JSON has `tool_calls: [{"id","name","arguments"}]` and `tool_call_id`. This replaces the earlier rule that skipped tool messages and empty assistant messages: an assistant message with empty content and tool calls must now render (just the tool blocks).
  3. `#workspace`: show the open conversation's `workspace`; `#set-workspace` sends `PATCH /api/conversations/ID` with `{"workspace": value}` (trimmed), and shows `Error: ...` in an error bubble if the response is not 200 (the field is also sent when empty, which turns tools off). Do nothing when no conversation is open. When a conversation is created from `#new-chat` or by sending the first message, pass the workspace field's current value as `workspace` in the `POST /api/conversations` body.
  4. While a turn waits for approval keep the form disabled; the approval buttons stay usable.
- `internal/web/web_test.go`: add to the existing tests: the page contains `id="workspace"`, `id="set-workspace"`, `class="approval"` is not needed, but the CSS contains `.approval`; `app.js` contains `tool_call`, `approval`, `tool_result`, `/approvals/`, `tool_calls`, `tool_call_id`, `approve`, `deny`, `set-workspace` and `JSON.stringify`, and still does not contain `eval(`, `document.write` or `innerHTML = text`. Keep every existing assertion passing.

## Contract
Element ids `workspace`, `set-workspace`; event types `tool_call`, `approval`, `tool_result`; the approvals endpoint `POST /api/conversations/ID/approvals/CALLID` with `{"approve": bool}`.

## Done when
- `go test ./internal/web/` passes.
- `go vet ./internal/web/` succeeds and `gofmt -l internal/web` prints nothing.
- `grep -c 'tool_result' internal/web/app.js` prints at least 1.

## Out of scope
- Every Go file other than `internal/web/web_test.go`. `internal/app/` and `README.md`, which other workers own.
~~~~

### 7.3 Document the agent features in the llmchat README

~~~~text
Epic: 7 Agent wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Other workers are editing `internal/app/` and `internal/web/` at the same time. Do not touch those.

## Goal
Update the README so it describes the tools, workspaces and approvals that now exist.

## Read first
- `README.md`: the current sections.
- `internal/app/app.go`: `SystemPrompt`, `CHAT_WORKSPACE`.
- `internal/tools/fs/fs.go`, `internal/tools/search/search.go`, `internal/tools/edit/edit.go`, `internal/tools/shell/shell.go`: the tool names and what each does.
- `internal/tools/sandbox/sandbox.go`: the workspace rule.

## Change
- `README.md`:
  - Replace the sentence that says tools come in a later stage with a new section **Tools** that lists the six tools in a table (name, what it does, needs approval: `read_file` no, `list_dir` no, `grep` no, `write_file` yes, `edit_file` yes, `bash` yes).
  - A section **Workspaces**: tools are only offered to the model in a conversation that has a workspace directory, set in the page's header box or with `PATCH /api/conversations/{id}` `{"workspace": "/path"}`; `CHAT_WORKSPACE` gives new conversations a default; every file path is confined to the workspace (the sandbox rejects `..`, outside absolute paths and symbolic links that leave it); `bash` runs with the workspace as its working directory but is not otherwise sandboxed, so approving it is the safeguard.
  - A section **Approvals**: calls to tools that need approval pause the turn; the page shows Approve and Deny buttons; the API route is `POST /api/conversations/{id}/approvals/{call}` with `{"approve": true}`; closing the page while a call waits cancels the turn.
  - A section **Choosing a model for tools**: tool calling needs a model that supports it. Through the router that is the Bonsai backend (it supports native tool calls) or any Ollama model whose `ollama show` lists `tools`; `llama3:8b` does not. Say to pin the conversation's model to such a route in the picker (an `auto` conversation pins to whatever model answered first within a turn, so a turn never switches model).
  - Add `tools/sandbox, tools/fs, tools/search, tools/edit, tools/shell` lines to the Layout section.
  - Do not change the other sections.

## Contract
File path `README.md`.

## Done when
- `grep -c 'CHAT_WORKSPACE' README.md` prints at least 1.
- `grep -c 'write_file' README.md` prints at least 1.
- `grep -c 'approvals' README.md` prints at least 1.

## Out of scope
- Every Go and JavaScript file.
~~~~

## Epic 8: Agent verify

Done when: An end-to-end test passes against a fake router that calls tools, and an opt-in live test exists.

### 8.1 Add an end-to-end agent test

~~~~text
Epic: 8 Agent verify
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `internal/live/live_agent_test.go` at the same time. Do not touch it. `internal/e2e/e2e_test.go` already exists; leave it alone.

## Goal
One test that runs the whole server against a fake router that asks for tools, and proves the tool loop, the approvals, the sandbox and the saved history work together.

## Read first
- `internal/e2e/e2e_test.go`: the existing fake router and helpers; copy the ones you need into your own file (the two files are separate packages' worth of helpers in the same package `e2e_test`, so name yours differently: prefix with `agent`).
- `internal/app/app.go`: `Config`, `New`, `SystemPrompt`.
- `internal/chat/chat.go`: `Event`.

## Change
- `internal/e2e/agent_test.go`: package `e2e_test`.
  - A scripted fake router `httptest.Server`. `POST /v1/chat/completions` decodes the request body, records it (with a `sync.Mutex`), and picks the answer from the LAST message: if its role is `tool`, answer with the text `done: ` plus that message's content, as `content` deltas; otherwise the last message is the user's text and the answer depends on it: `read hello.txt` streams one tool call `{"id":"call_1","name":"read_file","arguments":"{\"path\":\"hello.txt\"}"}` split across three chunks the way a real server does (the first chunk has `index`, `id`, `type`, `function.name` and `arguments` `{`; the rest only `index` and `function.arguments` pieces; finish reason `tool_calls`); `write note.txt` streams a `write_file` call with path `note.txt` and content `hi`; `escape` streams a `read_file` call with path `../secret.txt`; `run echo` streams a `bash` call with command `echo ran`; anything else answers `plain`. Every stream sets headers `X-Llmrouter-Route: reason`, `X-Llmrouter-Backend: bonsai`, `X-Llmrouter-Model: b` and ends with `data: [DONE]`.
  - A temp workspace directory containing `hello.txt` with the content `first line\nsecond line\n`, and a file `secret.txt` next to (outside) the workspace.
  - Build the app (`DBPath` in a temp dir, `RouterURL` the fake) and serve it. Create a conversation with `"workspace": <the workspace>` and model `auto`. A helper streams `POST .../messages` and returns the events; a second helper does the same but, in a goroutine, answers every `approval` event by posting to `POST /api/conversations/ID/approvals/CALL` with a given `approve` value as soon as the event arrives.
  - Check, in order:
    1. `read hello.txt`: events in order are `info`, `tool_call` (name `read_file`, arguments containing `hello.txt`), `tool_result` (text containing `first line` and `second line`, with line numbers), `info`, `content` events whose text joins to `done: ` plus the tool result, `done`. No `approval` event. The fake's first request has a `tools` array with the six names and a first `system` message that starts with `You are a coding assistant`; its second request pinned the model to `bonsai/b` (not `auto`) and its last message has role `tool`.
    2. The stored history of the conversation: user, assistant (with one tool call), tool, assistant, in that order, the tool message's `tool_call_id` equal to `call_1`.
    3. `write note.txt` with approvals answered `true`: events include `approval` (name `write_file`) then `tool_result`; `note.txt` exists in the workspace with content `hi`.
    4. `write note.txt` again with the file removed first and approvals answered `false`: the `tool_result` text is `denied by the user` and `note.txt` does not exist.
    5. `run echo` approved: the `tool_result` contains `ran` and `exit status 0`. Denied: it is `denied by the user`.
    6. `escape`: no approval is needed (read-only) and the `tool_result` text contains `outside the workspace`; the secret file's content appears nowhere in the events.
    7. A conversation without a workspace: send `anything else` and expect the answer `plain`; the fake's request for it has no `tools` key, and its `messages` contain no `system` message mentioning a workspace.
    8. A `POST .../approvals/nope` with nothing waiting is 404.
    9. A turn the client abandons while waiting for approval (cancel the request context) ends the server's turn: afterwards a new `read hello.txt` on the same conversation is accepted (not 409) within 2 seconds.

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/e2e/` passes (both test files).
- `go vet ./internal/e2e/` succeeds and `gofmt -l internal/e2e` prints nothing.

## Out of scope
- Every non-test file. If the test finds a bug in a component, block this issue with `bin/codeplat issue block` from the repository root and describe the failing check; do not fix the component.
- `internal/live/`.
~~~~

### 8.2 Add an opt-in live agent test

~~~~text
Epic: 8 Agent verify
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`.
- Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`.
- Every package named in this issue already exists and its tests pass. Other workers may be editing the files this issue does not list; touch only the files it lists.
- Tests must never reach a real router, Ollama or model. Use `net/http/httptest` servers, `t.TempDir()` and hand-written fakes.
- Another worker is writing `internal/e2e/agent_test.go` at the same time. Do not touch `internal/e2e/`.
- On this machine an llmrouter may be running at http://127.0.0.1:8083 with real models, including a tool-capable Bonsai backend. Do not start, stop or configure it.

## Goal
A test, skipped unless asked for, that has a real tool-calling model read a file in a temporary workspace through llmchat, proving the agent loop works against real models.

## Read first
- `internal/live/live_test.go`: the existing live tests and their skip pattern and helpers; write yours in a new file and do not edit that one.
- `internal/app/app.go`: `Config`, `New`.

## Change
- `internal/live/live_agent_test.go`: package `live_test`. Start with `if os.Getenv("LLMCHAT_LIVE") != "1" { t.Skip(...) }`.
  - `TestAgentReadsFile`: router URL `LLMCHAT_LIVE_ROUTER` (default `http://127.0.0.1:8083`); skip if `GET <router>/api/routes` fails. The model to pin is `LLMCHAT_LIVE_AGENT_MODEL` (default `reason`, the route that points at the tool-capable model in `examples/bonsai-wsl.json`); skip if it is not one of the names `GET <router>/api/routes` lists with its `routes[].name` (that endpoint answers `{"default":..,"routes":[{"name":..}]}`) and does not contain `/`.
  - Make a temp workspace with `secret.txt` containing `The magic word is plugh-7421.` Build the app, serve it, create a conversation with that workspace and the pinned model, and send `Read the file secret.txt and tell me the magic word.` (client timeout 10 minutes). Parse the events; expect a `tool_call` event named `read_file`, a `tool_result` containing `plugh-7421`, no `approval` event, a final answer (the joined `content` events after the last `info`) containing `plugh-7421`, and `done` last. Log the route, model and answer.
  - `TestAgentWritesFile`: same setup; send `Create a file named out.txt containing exactly the text ok.`; answer every `approval` event with `true` (post to the approvals endpoint from a goroutine); expect an `approval` for `write_file` and, after the turn, `out.txt` in the workspace whose trimmed content is `ok`. Log the answer.
  - If the model does not call the tool (a weak model), the test fails with a message that says which events it saw.
- `Makefile`: the `live` target already runs `./internal/live/`; make no change.

## Contract
None new; this issue only tests.

## Done when
- `go test ./internal/live/` passes (the tests skip).
- `LLMCHAT_LIVE=1 go test -count=1 -v -timeout 15m -run TestAgentReadsFile ./internal/live/` passes if a router with a tool-capable route is reachable, and skips otherwise.
- `go vet ./internal/live/` succeeds and `gofmt -l internal/live` prints nothing.

## Out of scope
- `internal/e2e/`. Every non-test file.
~~~~
