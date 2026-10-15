# Roadmap: remote MCP servers in llmchat

## Goal

llmchat's agent can call tools from remote MCP servers (Streamable HTTP), and the person manages those servers from the llmchat page: add, edit, enable/disable, test and delete. The first server is Exa, `https://mcp.exa.ai/mcp` (tools `web_search_exa`, `web_fetch_exa`; works without a key, answers in SSE, issues an `Mcp-Session-Id` — probed 2026-10-15).

Tracker goal: CHAT-33.

Design decisions:
- Servers live in SQLite (`mcp_servers`), not env, so the page can manage them. Validation is `chat.MCPServer.Validate`, called by the store; the API never validates.
- The engine keeps its fixed tools and gains `Options.MoreTools`, called once per turn, so UI changes apply on the next message without a restart. MCP tools follow the existing rule that tools are offered only in conversations with a workspace.
- Tool names are `<server>__<tool>`; a fixed tool wins a clash.
- Approval is per server: `always` (default) or `never`.
- The API never returns header values (only `header_names`), so a stored API key cannot leak through the page.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Parts | MCP servers can be stored; the MCP client passes its tests. | 2, parallel |
| 2. Pieces | The engine takes per-turn tools; MCP servers become tools; REST endpoints, page dialog and README exist, each tested alone. | 5, parallel |
| 3. Wiring | The app connects store, source, engine and API; e2e tests prove a stored server's tool is offered and called. | 1 |

No issue has a dependency inside its epic. Contracts (the `chat.MCPServer` types, `mcpclient` API, `agent.Options.MoreTools`, `api.ServerTools`/`NewMCPHandler`, `mcptools.Source`, the REST shapes) are pasted into every issue that touches them. The engine change sits in epic 2, not 1, so it never compiles against `internal/chat` while 1.1 is editing that package. `api.go`, `NewHandler` and `agent.New` keep their signatures, so `internal/app/app.go` is edited only in epic 3.

Note: CHAT-31 (Stop button) and CHAT-32 (context size) are open and edit `internal/web/page.html`/`app.js`; issue 2.4 is told to re-read and stay additive.

---
## Epic 1: Parts

### 1.1 Store MCP servers in llmchat

~~~~text
Epic: 1 Parts
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only, plus `modernc.org/sqlite`, already in `go.mod`. Do not edit `go.mod` or `go.sum`; never run `go mod tidy`.
- Tests must never reach the network. Use `t.TempDir()` for databases.
- Another worker is writing the new package `internal/mcpclient/` at the same time. Change only the files listed below.
- Rule: input checks are business rules and belong in `internal/chat` and the store, never in `internal/api`.

## Goal
llmchat can store the remote MCP servers whose tools the model may use: a `chat.MCPServer` type with its validation, a store port, and the SQLite implementation.

## Read first
- `internal/chat/chat.go`: `Conversation`, `ConversationPatch`, `Store`, `ErrInvalid`, `ErrNotFound` — copy their style.
- `internal/sqlitestore/sqlitestore.go`: `migrations`, `newID`, `formatTime`/`parseTime`, `nullable`, `CreateConversation`, `UpdateConversation` (transaction + COALESCE pattern), `DeleteConversation`.
- `internal/sqlitestore/sqlitestore_test.go`: how tests open a store (with a fixed `now`).

## Change
- `internal/chat/mcp.go` (new): the types, interface and `Validate` below.
- `internal/chat/mcp_test.go` (new): table test of `Validate`.
- `internal/sqlitestore/sqlitestore.go`: append migration 2 (below) to `migrations` — never edit the first entry — and add the five methods. Add `var _ chat.MCPServerStore = (*Store)(nil)` at the end.
- `internal/sqlitestore/sqlitestore_test.go`: tests for the five methods.

## Contract
```go
// internal/chat/mcp.go
package chat

// Approval policies of an MCP server.
const (
	ApprovalAlways = "always" // every call of the server's tools needs the person's approval
	ApprovalNever  = "never"  // calls run without asking
)

// MCPServer is a remote MCP server (Streamable HTTP) whose tools the model
// may use. Name prefixes its tools' names ("<name>__<tool>"), so it is
// unique. Headers are sent with every request, e.g. an API key.
type MCPServer struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Enabled   bool              `json:"enabled"`
	Approval  string            `json:"approval"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// MCPServerPatch changes only the fields that are non-nil. Headers, when
// non-nil, replaces the whole map.
type MCPServerPatch struct {
	Name     *string            `json:"name,omitempty"`
	URL      *string            `json:"url,omitempty"`
	Headers  *map[string]string `json:"headers,omitempty"`
	Enabled  *bool              `json:"enabled,omitempty"`
	Approval *string            `json:"approval,omitempty"`
}

// Validate reports an error wrapping ErrInvalid when:
//   - Name does not match ^[a-z0-9][a-z0-9_-]{0,31}$
//   - URL does not parse with net/url, or its scheme is not "http" or "https", or its Host is empty
//   - Approval is not ApprovalAlways or ApprovalNever
//   - a Headers key is empty or contains a space, ':' , '\r' or '\n', or a value contains '\r' or '\n'
// The message names the field, e.g. "name must match ^[a-z0-9][a-z0-9_-]{0,31}$".
func (s MCPServer) Validate() error

// MCPServerStore keeps the MCP servers.
type MCPServerStore interface {
	// CreateMCPServer sets a new ID, CreatedAt and UpdatedAt, defaults an
	// empty Approval to ApprovalAlways and nil Headers to an empty map,
	// validates, and stores s. A Name already used is ErrInvalid.
	CreateMCPServer(ctx context.Context, s MCPServer) (MCPServer, error)
	// GetMCPServer returns ErrNotFound for an unknown id (also Update and Delete).
	GetMCPServer(ctx context.Context, id string) (MCPServer, error)
	// ListMCPServers returns every server ordered by Name. Never nil.
	ListMCPServers(ctx context.Context) ([]MCPServer, error)
	// UpdateMCPServer applies the non-nil fields of p, validates the result,
	// sets UpdatedAt, and returns it. A Name another server uses is ErrInvalid.
	UpdateMCPServer(ctx context.Context, id string, p MCPServerPatch) (MCPServer, error)
	DeleteMCPServer(ctx context.Context, id string) error
}
```
Returned servers always have a non-nil `Headers` map.

Migration 2 (append as a new string to `migrations`):
```sql
CREATE TABLE mcp_servers (
	id         TEXT PRIMARY KEY,
	name       TEXT NOT NULL UNIQUE,
	url        TEXT NOT NULL,
	headers    TEXT NOT NULL,
	enabled    INTEGER NOT NULL,
	approval   TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);
```
`headers` holds the map as JSON text (`{}` when empty). Check for a duplicate name with a `SELECT` (on update: `name = ? AND id <> ?`) inside the same transaction and return `fmt.Errorf("sqlitestore: mcp server name %q is taken: %w", name, chat.ErrInvalid)`. Error messages start with `sqlitestore: ` like the existing ones.

## Done when
- `go vet ./internal/chat/ ./internal/sqlitestore/ && go test ./internal/chat/ ./internal/sqlitestore/` passes.
- Tests cover: create then get round-trips every field (including headers); list is ordered by name and is `[]` (not nil) when empty; a duplicate name on create and on rename is `ErrInvalid`; an invalid URL or approval is `ErrInvalid` and nothing is stored; update changes only the given fields and bumps UpdatedAt; delete then get is `ErrNotFound`; an existing database at schema version 1 (open it once before your change's code path, or create the version-1 schema by hand) migrates to version 2 with its conversations intact.

## Out of scope
- `internal/chat/chat.go` stays as it is (other packages compile against it).
- No API, page or agent changes.
~~~~

### 1.2 Add an MCP client for Streamable HTTP servers

~~~~text
Epic: 1 Parts
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`; never run `go mod tidy`.
- Tests must never reach the network: use a `net/http/httptest` server that plays the MCP server.
- Another worker is editing `internal/chat/` and `internal/sqlitestore/` at the same time. This package must import neither; it imports only the standard library.

## Goal
A small client for remote MCP servers over the Streamable HTTP transport, so llmchat can list a server's tools and call them.

## Read first
- `internal/sandboxclient/sandboxclient.go`: an existing HTTP client in this module — copy its style (constructor taking `*http.Client`, error wrapping, body limits).
- `internal/routerclient/routerclient.go`: how it reads a `text/event-stream` body line by line.

## Change
- `internal/mcpclient/mcpclient.go` (new): package `mcpclient` with the contract below.
- `internal/mcpclient/mcpclient_test.go` (new).

## Contract
```go
package mcpclient

// Tool is one tool a server offers.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// Result is what a tool call returned. Text joins the text content items
// with "\n"; a non-text item becomes "[<type> content]". IsError is the
// server's isError flag.
type Result struct {
	Text    string
	IsError bool
}

// Client talks to one MCP server. It is safe for concurrent use and
// initializes the session on first use.
type Client struct { /* unexported */ }

// New returns a client for the server at url that sends headers with every
// request. hc nil means http.DefaultClient.
func New(url string, headers map[string]string, hc *http.Client) *Client

// ListTools returns every tool, following nextCursor (at most 20 pages).
func (c *Client) ListTools(ctx context.Context) ([]Tool, error)

// CallTool calls the tool name with args (a JSON object; empty or "null" sends {}).
func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (Result, error)
```

Protocol (JSON-RPC 2.0 over HTTP POST to `url`):
- Every request: `Content-Type: application/json`, `Accept: application/json, text/event-stream`, the configured headers, and after initialization `Mcp-Session-Id: <id>` (only if the server sent one) and `MCP-Protocol-Version: <version from the initialize result>`.
- Request IDs come from an atomic counter starting at 1.
- Initialize (lazily, once, guarded by a mutex): `{"jsonrpc":"2.0","id":N,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"llmchat","version":"0.1"}}}`. Save the `Mcp-Session-Id` response header and `result.protocolVersion`. Then POST the notification `{"jsonrpc":"2.0","method":"notifications/initialized"}` (no id); any 2xx is fine and its body is ignored.
- `tools/list`: params `{}` or `{"cursor":"<c>"}`; result `{"tools":[...],"nextCursor":"..."}`.
- `tools/call`: params `{"name":"<name>","arguments":<args>}`; result `{"content":[{"type":"text","text":"..."}],"isError":false}`.
- Response by Content-Type: `application/json` → one JSON-RPC message. `text/event-stream` → SSE events: accumulate `data:` lines (strip one leading space, join with "\n") until a blank line; parse each event as JSON; skip messages whose `id` does not equal the request's id (server notifications and requests); return the first matching one. Stream ends without a match → error. Read at most 10 MiB.
- Errors: non-2xx → `mcp: <method>: HTTP <status>: <first 200 bytes of body>`; a JSON-RPC `error` member → `mcp: <method>: <message> (code <code>)`.
- Session expiry: when a request that carried a session ID gets HTTP 404, forget the session, initialize again, and retry that request once.

The real server this targets (`https://mcp.exa.ai/mcp`) answers initialize with `Content-Type: text/event-stream`, a body like `event: message\ndata: {"result":{"protocolVersion":"2025-06-18",...},"jsonrpc":"2.0","id":1}\n\n`, and an `mcp-session-id` header. Its `tools/list` returns tools `web_search_exa` (required args `query`, `objective`) and `web_fetch_exa` (required `urls`).

## Done when
- `go vet ./internal/mcpclient/ && go test ./internal/mcpclient/` passes.
- Tests with an httptest fake server cover: initialize + initialized notification happen once before the first call; session ID and protocol version headers are sent afterwards; configured headers are sent; tools/list follows a cursor across 2 pages; tools/call returns joined text and `IsError`; both JSON and SSE responses (SSE with an unrelated notification before the answer); a JSON-RPC error; a non-2xx status; a 404 on a session triggers one re-initialize and a successful retry.

## Out of scope
- stdio transport, OAuth, GET/SSE listening streams, session DELETE, resources and prompts.
- No other package uses this one yet; do not wire it anywhere.
~~~~

## Epic 2: Pieces

### 2.1 Let the agent offer extra tools looked up each turn

~~~~text
Epic: 2 Pieces
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`.
- Tests use the hand-written fakes already in `internal/agent/agent_test.go`; nothing reaches the network.
- Other workers are editing `internal/api/`, `internal/web/`, `internal/mcptools/`, `README.md` and `Makefile` at the same time. Change only the two files below. `agent.New` keeps its signature, so `internal/app/app.go` keeps compiling unchanged.

## Goal
The engine can offer extra tools that are looked up fresh at the start of every turn, so tools from MCP servers the person adds or removes take effect on the next turn without a restart.

## Read first
- `internal/agent/agent.go`: `Options`, `Engine`, `Run` (where `specs` is built only when `conv.Workspace != ""`), `runCall`, `findTool`.
- `internal/agent/agent_test.go`: `fakeStore`, `fakeCompleter`, `fakeApprover`, `fakeTool`, `newFixture`.

## Change
- `internal/agent/agent.go`:
  - Add to `Options`:
    ```go
    // MoreTools, if not nil, is called once per turn, before the first
    // model call, for a conversation with a workspace. Its tools are
    // offered after the fixed tools. When a name is already taken by a
    // fixed tool, the fixed tool wins and the extra one is dropped.
    MoreTools func(ctx context.Context) []chat.Tool
    ```
  - In `Run`, when `conv.Workspace != ""`, build the turn's tool list once: the fixed `e.tools`, then the result of `MoreTools` (if set) minus name clashes. Build `specs` from that list, and have `runCall`/`findTool` look calls up in that same list (pass it as a parameter; do not store it on the Engine, since turns of different conversations run concurrently).
  - With no workspace, `MoreTools` is not called and behaviour is unchanged.
- `internal/agent/agent_test.go`: tests below.

## Contract
`agent.New(store chat.Store, completer chat.Completer, tools []chat.Tool, approver chat.Approver, opts Options) *Engine` is unchanged. Only the `MoreTools` field is new.

## Done when
- `go vet ./internal/agent/ && go test ./internal/agent/` passes.
- Tests: extra tools appear in the request's specs after the fixed ones; a model call to an extra tool runs it (and asks for approval when its `NeedsApproval` says so); an extra tool whose name clashes with a fixed one is dropped and the fixed one runs; `MoreTools` is called once per turn even when the turn makes several model calls, and its result can differ between two turns; a conversation without a workspace never calls `MoreTools`.

## Out of scope
- `internal/app/app.go` (a later issue sets `MoreTools`), `internal/chat/`.
~~~~

### 2.2 Turn stored MCP servers into agent tools

~~~~text
Epic: 2 Pieces
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`.
- Tests must never reach the network: use `net/http/httptest` servers that play MCP servers, and a hand-written fake `Lister`.
- Other workers are editing `internal/agent/`, `internal/api/`, `internal/web/`, `README.md` and `Makefile` at the same time. Import only `llmchat/internal/chat` and `llmchat/internal/mcpclient`; create only the new package below.

## Goal
A tool source that turns the enabled MCP servers in the store into `chat.Tool`s the agent can offer, plus a way to list one server's tools for the page's "test" button.

## Read first
- `internal/chat/mcp.go`: `MCPServer`, `ApprovalAlways`, `ApprovalNever`.
- `internal/chat/chat.go`: `Tool`, `ToolSpec`.
- `internal/mcpclient/mcpclient.go`: `New`, `Client.ListTools`, `Client.CallTool`, `Tool`, `Result`.
- `internal/mcpclient/mcpclient_test.go`: its fake MCP server, which you can copy for your tests.

## Change
- `internal/mcptools/mcptools.go` (new) and `internal/mcptools/mcptools_test.go` (new).

## Contract
```go
package mcptools

// Lister is the part of the store Source reads; chat.MCPServerStore satisfies it.
type Lister interface {
	ListMCPServers(ctx context.Context) ([]chat.MCPServer, error)
}

// NewSource returns a Source. hc nil means &http.Client{Timeout: 2 * time.Minute}.
// logger receives the errors Tools skips over.
func NewSource(servers Lister, hc *http.Client, logger *log.Logger) *Source

// Tools returns the tools of every enabled server. A server that fails
// (listing or connecting) is logged and skipped; a store error is logged
// and Tools returns nil. Its signature matches agent.Options.MoreTools.
func (s *Source) Tools(ctx context.Context) []chat.Tool

// ServerTools lists srv's tools as the model would see them (prefixed
// names), whether or not srv is enabled. It returns the error instead of
// logging it. Its signature matches api.ServerTools.
func (s *Source) ServerTools(ctx context.Context, srv chat.MCPServer) ([]chat.ToolSpec, error)
```
Behaviour:
- Clients: keep one `*mcpclient.Client` per server ID so the MCP session is reused across turns; make a new one when the server's URL or headers changed. Drop cached clients of servers no longer listed. Guard the cache with a mutex.
- Each server's `ListTools` gets its own `context.WithTimeout(ctx, 15*time.Second)`.
- Each remote tool becomes a `chat.Tool`:
  - `Spec().Name` = `srv.Name + "__" + tool.Name`. Skip (and log) a tool whose full name does not match `^[A-Za-z0-9_-]{1,64}$`.
  - `Spec().Description` = the tool's description; `Spec().Parameters` = its `InputSchema`, or `{"type":"object","properties":{}}` when empty or `null`.
  - `NeedsApproval` returns `srv.Approval != chat.ApprovalNever`.
  - `Run(ctx, workspace, args)` ignores `workspace`, calls `CallTool(ctx, tool.Name, args)` with the original (unprefixed) name; a transport error is returned as the error; `IsError` returns `errors.New(result.Text)`; otherwise it returns `result.Text`.
- Log lines start with `mcptools: server <name>: `.

## Done when
- `go vet ./internal/mcptools/ && go test ./internal/mcptools/` passes.
- Tests: two enabled servers give prefixed tools in server order; a disabled server is left out; an unreachable server (closed httptest server) is skipped and the others still come back; approval follows the server's policy; `Run` forwards the unprefixed name and args and returns the text; `isError` becomes an error; `ServerTools` returns an error for an unreachable server; the same client (one initialize) is reused across two `Tools` calls, and a changed URL makes a new one.

## Out of scope
- Wiring into `internal/app` (a later issue) and any API or page code.
~~~~

### 2.3 Add REST endpoints to manage MCP servers

~~~~text
Epic: 2 Pieces
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`.
- Rule: no validation in the API. The store validates (`chat.MCPServer.Validate`) and returns errors wrapping `chat.ErrInvalid`/`chat.ErrNotFound`; the API only maps them to status codes with the existing `writeError`.
- Tests use a hand-written fake store and a fake `ServerTools` func, as `internal/api/api_test.go` does with its `fakeStore`; nothing reaches the network.
- Other workers are editing `internal/agent/`, `internal/web/`, `internal/mcptools/`, `README.md` and `Makefile` at the same time. Import only `llmchat/internal/chat`. Do not edit `api.go` or `api_test.go`: put everything in the two new files below, so `NewHandler`'s signature and `internal/app` stay unchanged.

## Goal
REST endpoints to list, add, change and remove MCP servers and to list one server's tools, for the page to manage them.

## Read first
- `internal/api/api.go`: `NewHandler`, `handler`, `decode`, `badBody`, `writeJSON`, `writeError`, `maxBody`, and `createConversation`/`updateConversation`/`deleteConversation` as the pattern.
- `internal/chat/mcp.go`: `MCPServer`, `MCPServerPatch`, `MCPServerStore`.
- `internal/api/api_test.go`: how the handler is served with httptest and called.

## Change
- `internal/api/mcp.go` (new): `ServerTools`, `NewMCPHandler` and the handlers. Reuse the helpers in `api.go`, e.g. by giving your handler type a `*handler` with only `log` set, or by embedding it.
- `internal/api/mcp_test.go` (new).

## Contract
```go
// ServerTools lists one MCP server's tools as the model sees them;
// mcptools.Source.ServerTools satisfies it.
type ServerTools func(ctx context.Context, s chat.MCPServer) ([]chat.ToolSpec, error)

// NewMCPHandler serves the MCP server endpoints under /api/mcp/servers.
func NewMCPHandler(servers chat.MCPServerStore, tools ServerTools, logger *log.Logger) http.Handler
```
Routes:
| Method and path | Body | Returns |
|---|---|---|
| `GET /api/mcp/servers` | | `200`, array of views (`[]` when none) |
| `POST /api/mcp/servers` | `{"name","url","headers","enabled","approval"}`; `headers`, `enabled`, `approval` optional; missing `enabled` means `true` | `201`, the view |
| `GET /api/mcp/servers/{id}` | | `200`, the view |
| `PATCH /api/mcp/servers/{id}` | any of `name`, `url`, `headers` (replaces all), `enabled`, `approval` — decode into `chat.MCPServerPatch` | `200`, the view |
| `DELETE /api/mcp/servers/{id}` | | `204` |
| `GET /api/mcp/servers/{id}/tools` | | `200`, `[]chat.ToolSpec` (`[]` when none); unknown id `404`; when `tools` fails, `502` with `{"error": "<err>"}` |

The view never contains header values, so a page cannot leak a stored API key:
```go
type mcpServerView struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	HeaderNames []string  `json:"header_names"` // sorted; [] when none
	Enabled     bool      `json:"enabled"`
	Approval    string    `json:"approval"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
```
Bodies are decoded with `decode` (unknown fields rejected); a bad body goes through `badBody` (400).

## Done when
- `go vet ./internal/api/ && go test ./internal/api/` passes.
- Tests: create returns 201 and a view with `header_names` but no header values anywhere in the JSON; missing `enabled` stores `true`; list, get, patch (only given fields change) and delete work; a store `ErrInvalid` is 400 and `ErrNotFound` is 404; an unknown body field is 400; the tools endpoint returns the specs, 404 for an unknown id, and 502 with the error text when `ServerTools` fails.

## Out of scope
- `api.go`, `api_test.go`, and mounting the handler in `internal/app` (a later issue).
~~~~

### 2.4 Add an MCP servers dialog to the llmchat page

~~~~text
Epic: 2 Pieces
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Plain HTML, CSS and JavaScript only, no libraries or build step; the files are embedded by `internal/web/web.go`.
- Other workers are editing `internal/agent/`, `internal/api/`, `internal/mcptools/`, `README.md` and `Makefile` at the same time. Change only the three files below.
- Issues outside this plan (a Stop button, a context-size display) may also have touched `page.html` and `app.js` recently. Re-read both files before editing, keep your changes additive and self-contained (a new block of markup, a new section of script), and do not reformat existing code.

## Goal
The page has an "MCP servers" dialog where the person can see, add, edit, enable/disable, test and delete the remote MCP servers whose tools the model may use.

## Read first
- `internal/web/page.html`: the sidebar (`#sidebar`, `#new-chat`), the CSS variables and button styles.
- `internal/web/app.js`: `$`, `request(method, url, body)` (throws `Error("Error STATUS: BODY")`), and how lists are built with `document.createElement` and `textContent`.
- `internal/web/web_test.go`: `TestIndexPage` and `TestAppScript` (strings that must appear, and banned ones: `eval(`, `document.write`, `innerHTML = text`).

## Change
- `internal/web/page.html`: a button `<button id="mcp-servers" type="button">MCP servers</button>` in the sidebar under `#new-chat`, and a `<dialog id="mcp-dialog">` containing:
  - `<ul id="mcp-list">` — one row per server;
  - `<form id="mcp-form">` with `<input id="mcp-name">`, `<input id="mcp-url" type="url">`, `<textarea id="mcp-headers">` (one `Name: value` per line), `<select id="mcp-approval">` with options `always` ("Ask before each call") and `never` ("Run without asking"), `<input id="mcp-enabled" type="checkbox">`, and Save / Cancel buttons;
  - `<pre id="mcp-tools">` for the result of a tools test, and `<p id="mcp-error">` for errors;
  - a Close button. Style it with the existing CSS variables.
- `internal/web/app.js`: a new section that drives the dialog against the API below. Build all DOM with `createElement` and `textContent` (no `innerHTML` with data).
  - Opening the dialog loads `GET /api/mcp/servers` and renders rows: name, URL, header names (e.g. `headers: x-api-key`), an enabled checkbox (change → `PATCH {"enabled": bool}`), and buttons Edit, Tools, Delete.
  - Tools → `GET /api/mcp/servers/{id}/tools`; show each tool's `name` and first line of `description` in `#mcp-tools`, or the error.
  - Delete → `confirm()`, then `DELETE`, then reload the list.
  - Add (empty form) → `POST` with name, url, headers parsed from the textarea, enabled, approval.
  - Edit fills the form; the headers textarea starts empty with the placeholder `Leave empty to keep: <header names>`. On save, `PATCH` name, url, enabled, approval, and `headers` only when the textarea is not empty.
  - Show any request error in `#mcp-error` (the API returns `{"error": "..."}` with 400 for invalid input, e.g. a bad name or URL).
- `internal/web/web_test.go`: add `id="mcp-servers"`, `id="mcp-dialog"`, `id="mcp-list"`, `id="mcp-form"` to `TestIndexPage`'s list and `/api/mcp/servers` and `/tools` to `TestAppScript`'s list.

## Contract
API (served by another issue; build against this text):
- `GET /api/mcp/servers` → `[{"id","name","url","header_names":["..."],"enabled":true,"approval":"always"|"never","created_at","updated_at"}]`
- `POST /api/mcp/servers` body `{"name","url","headers":{"K":"V"},"enabled":bool,"approval":"always"|"never"}` → 201 view
- `PATCH /api/mcp/servers/{id}` body: any of the same fields (`headers` replaces all) → 200 view
- `DELETE /api/mcp/servers/{id}` → 204
- `GET /api/mcp/servers/{id}/tools` → `[{"name","description","parameters"}]`, or 502 `{"error"}` when the server cannot be reached
- Server names must match `^[a-z0-9][a-z0-9_-]{0,31}$`; tools reach the model as `<name>__<tool>`. Put that hint under the name input.

## Done when
- `go vet ./internal/web/ && go test ./internal/web/` passes.
- `node --check internal/web/app.js` passes, if `node` is installed.

## Out of scope
- Any Go code outside `internal/web/web_test.go`; the existing conversation UI.
~~~~

### 2.5 Document MCP servers and add a live Exa test

~~~~text
Epic: 2 Pieces
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`.
- Other workers are editing `internal/agent/`, `internal/api/`, `internal/web/` and creating `internal/mcptools/` at the same time. Change only the three files below.

## Goal
The README explains how to give llmchat's model tools from remote MCP servers such as Exa, and an opt-in live test checks the MCP client against the real Exa server.

## Read first
- `README.md`: the sections `Configuration`, `Tools`, `Approvals`, `API` (the endpoint table), `Layout` and `Test`.
- `Makefile`: the `live` target.
- `internal/mcpclient/mcpclient.go`: `New`, `ListTools`, `CallTool`.
- `internal/live/live_test.go`: `requireLive` — the skip pattern (`LLMCHAT_LIVE=1`). Do not import that package; copy the pattern.

## Change
- `internal/mcpclient/live_test.go` (new, package `mcpclient`): `TestLiveExa`, skipped unless `LLMCHAT_LIVE=1`. With `New("https://mcp.exa.ai/mcp", nil, &http.Client{Timeout: 60 * time.Second})`: `ListTools` includes `web_search_exa`; `CallTool(ctx, "web_search_exa", {"query":"Go programming language official site","objective":"Find the official Go website","numResults":1})` returns `IsError == false` and non-empty text. If Exa answers HTTP 429 or the network is unreachable, `t.Skip` with the reason.
- `Makefile`: the `live` target also runs `./internal/mcpclient/` with `-run Live`, e.g. `LLMCHAT_LIVE=1 go test -count=1 -v -timeout 20m ./internal/live/ && LLMCHAT_LIVE=1 go test -count=1 -v -run Live ./internal/mcpclient/`.
- `README.md`:
  - A new section `## MCP servers` after `## Approvals`: the model can use tools from remote MCP servers (Streamable HTTP). Add them with the page's "MCP servers" button or the API. Each server has a name (`^[a-z0-9][a-z0-9_-]{0,31}$`), a URL, optional headers (e.g. an API key), enabled, and an approval policy (`always`, the default, or `never`). Its tools reach the model as `<name>__<tool>`, are looked up at the start of each turn (changes apply on the next message, no restart), and like all tools are offered only in conversations with a workspace. A server that cannot be reached is skipped for that turn and logged. Header values are never returned by the API.
  - Example: Exa — name `exa`, URL `https://mcp.exa.ai/mcp`, approval `never` (its tools only read the web). It works without a key on a free rate limit; for more, add your Exa API key as header `x-api-key`. Tools: `exa__web_search_exa`, `exa__web_fetch_exa`. Show the `curl` for the POST:
    `curl -X POST localhost:8084/api/mcp/servers -H 'Content-Type: application/json' -d '{"name":"exa","url":"https://mcp.exa.ai/mcp","approval":"never"}'`
  - API table: add the six rows `GET /api/mcp/servers`, `POST /api/mcp/servers` (`{"name","url","headers","enabled","approval"}`, `enabled` defaults to true → 201), `GET /api/mcp/servers/{id}`, `PATCH /api/mcp/servers/{id}` (any of those fields; `headers` replaces all), `DELETE /api/mcp/servers/{id}` (204), `GET /api/mcp/servers/{id}/tools` (the tools as the model sees them; 502 if the server cannot be reached). Server views carry `header_names`, never values.
  - Layout: add `internal/mcpclient` (client for remote MCP servers over Streamable HTTP) and `internal/mcptools` (turns the enabled MCP servers into tools for the agent).
  - Test: mention that `make live` also runs the Exa test in `internal/mcpclient`, which needs internet access.

## Done when
- `go vet ./internal/mcpclient/ && go test ./internal/mcpclient/` passes (the live test skips).
- `LLMCHAT_LIVE=1 go test -count=1 -run Live ./internal/mcpclient/` passes or skips with a stated reason.

## Out of scope
- Any non-test Go code.
~~~~

## Epic 3: Wiring

### 3.1 Wire MCP servers into the llmchat app

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/llmchat/` under the repository root. It is a Go module named `llmchat`. Run every command from `samples/llmchat/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root; do not edit anything outside `samples/llmchat/`.
- Use the Go standard library only. Do not edit `go.mod` or `go.sum`.
- Rules: `internal/app` is the only package that wires implementations together. Tests build the app through `apptest.New` / `apptest.NewWith`, never by hand.
- Tests must never reach the network: the router and the MCP server are `net/http/httptest` fakes.
- You are the only worker in this epic.

## Goal
llmchat uses the MCP servers stored in its database: the agent offers their tools each turn, and the page's dialog works against the real endpoints. After this, adding Exa from the page gives the model `exa__web_search_exa` on the next message.

## Read first
- `internal/app/app.go`: `New` (where `tools`, `engine`, `apiHandler` and the mux are built), `defaultWorkspaceStore`.
- `internal/agent/agent.go`: `Options.MoreTools func(ctx context.Context) []chat.Tool`.
- `internal/mcptools/mcptools.go`: `NewSource(servers Lister, hc *http.Client, logger *log.Logger) *Source`, `(*Source).Tools`, `(*Source).ServerTools`.
- `internal/api/mcp.go`: `NewMCPHandler(servers chat.MCPServerStore, tools ServerTools, logger *log.Logger) http.Handler`.
- `internal/e2e/agent_test.go`: `newAgentRouter`, `agentToolCall`, `agentStart`, `agentCall`, `agentCreate`, `agentSend`, `agentGet` — the fake router and helpers to reuse (same package `e2e`).

## Change
- `internal/app/app.go`:
  - Build `logger` before the engine. Create `mcpSource := mcptools.NewSource(store, nil, logger)` (`*sqlitestore.Store` satisfies `mcptools.Lister` and `chat.MCPServerStore`).
  - Pass `MoreTools: mcpSource.Tools` in `agent.Options`.
  - Mount `api.NewMCPHandler(store, mcpSource.ServerTools, logger)` on the mux at `/api/mcp/`. Go's mux picks the longest pattern, so `/api/mcp/` wins over `/api/` for those paths. Check that `GET /api/mcp/servers` (no trailing slash) reaches it; if it does not, also mount the exact path `/api/mcp/servers`.
  - Extend `SystemPrompt` by one sentence: ` Tools whose names contain "__" come from remote services, such as web search.`
- `internal/app/app_test.go`: a test that `POST /api/mcp/servers` then `GET /api/mcp/servers` round-trips through the real store, and that `TestWorkspaceConversationOffersTools` still sees exactly the six built-in tools when no server is stored.
- `internal/e2e/mcp_test.go` (new): an httptest fake MCP server (answers `initialize`, `notifications/initialized` with 202, `tools/list` with one tool `search` taking `{"query": string}`, and `tools/call` returning `{"content":[{"type":"text","text":"result for <query>"}]}`) plus the fake router from `agent_test.go`.

## Contract
No new interfaces; this issue only connects the ones listed in "Read first".

## Done when
- `go vet ./... && go test ./...` passes.
- e2e: after `POST /api/mcp/servers {"name":"fake","url":<fake MCP URL>,"approval":"never"}`, a turn in a workspace conversation sends `fake__search` in the router request's tools; when the fake router makes the model call `fake__search` with `{"query":"go"}`, the tool result stored in the conversation is `result for go` and no approval was asked. With `"approval":"always"` (PATCH), the same call emits an `approval` event first.
- e2e: after `PATCH {"enabled": false}`, the next turn's router request has no `fake__search`.
- e2e: `GET /api/mcp/servers/{id}/tools` returns `[{"name":"fake__search",...}]`; with the fake MCP server closed it returns 502.

## Out of scope
- Changes inside `internal/mcpclient`, `internal/mcptools`, `internal/api`, `internal/agent` or `internal/web`. If one of them is wrong, block this issue with a comment saying what is wrong instead of editing it.
~~~~

