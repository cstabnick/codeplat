# Roadmap: bookmarks sample

## Goal

A sample project in `samples/bookmarks/`: a JSON web API for saving bookmarks, and a small HTMX UI that is a separate server and talks to that API over HTTP. It is its own Go module, standard library only, with an in-memory store.

Tracker project: `BOOK`.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists and the shared types compile. | 1 |
| 2. Components | Store, API handler, API client and UI handler each pass their own tests. | 4, parallel |
| 3. Wiring | Both servers run and an end-to-end test passes. | 4, parallel |

No issue has a dependency inside its epic.

Why three epics and not fewer: the four components share one Go type and one interface, which must exist before any of them compiles (epic 1). The two `main` packages and the end-to-end test import all four components, so their build check cannot pass until epic 2 is finished (epic 3).

The shared interface is what makes epic 2 parallel. Store and client both implement `bookmark.Service`; API handler and UI handler both consume it. Each is tested against a fake or a hand-written test server, never against another component.

---

## Epic 1: Foundation

### 1.1 Create the bookmarks module and its shared types

```
Epic: 1 Foundation
Depends on: none

## Environment
- Work in `samples/bookmarks/` under the repository root. Create it; it does not exist yet.
- Run every command from `samples/bookmarks/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/bookmarks/`.
- Use the Go standard library only. Add no dependencies.

## Goal
Create the Go module for a bookmarks sample app and the one package every other part will import: the Bookmark type, two error values and the Service interface.

## Read first
Nothing. The directory is new.

## Change
- `samples/bookmarks/go.mod`: create it by running `go mod init bookmarks` in `samples/bookmarks/`.
- `samples/bookmarks/internal/bookmark/bookmark.go`: create it with exactly the content under Contract.

## Contract
package bookmark

import (
	"context"
	"errors"
	"time"
)

// Errors returned by a Service. Callers test for them with errors.Is.
var (
	ErrInvalid  = errors.New("invalid bookmark")
	ErrNotFound = errors.New("bookmark not found")
)

type Bookmark struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Service stores bookmarks. The in-memory store and the HTTP client both
// implement it; the API handler and the UI handler both consume it.
type Service interface {
	// List returns all bookmarks ordered by ID ascending. It never returns nil.
	List(ctx context.Context) ([]Bookmark, error)
	// Add returns an error wrapping ErrInvalid if title or url is not acceptable.
	Add(ctx context.Context, title, url string) (Bookmark, error)
	// Delete returns an error wrapping ErrNotFound if no bookmark has that ID.
	Delete(ctx context.Context, id int) error
}

## Done when
- `go build ./... && go vet ./...` succeeds.
- `gofmt -l .` prints nothing.
- No tests are needed: the package has no behaviour to test.

## Out of scope
Any other package, a README, a Makefile. Later issues add those.
```

---

## Epic 2: Components

All four issues begin with the same Environment section:

```
## Environment
- Work in `samples/bookmarks/` under the repository root. It is a Go module named `bookmarks`.
- Run every command from `samples/bookmarks/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/bookmarks/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Other workers are writing sibling packages under `internal/` at the same time. Create only the files this issue lists.
```

### 2.1 Add the in-memory bookmark store

```
Epic: 2 Components
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
An in-memory implementation of `bookmark.Service`. It holds the data for the API server and is where input is validated.

## Read first
- `internal/bookmark/bookmark.go`: the `Bookmark` type, `ErrInvalid`, `ErrNotFound` and the `Service` interface you implement.

## Change
- `internal/store/store.go`: package `store`, with `type Store struct` and `func New() *Store`. `*Store` implements `bookmark.Service`.
- `internal/store/store_test.go`: tests for the behaviour under Contract.

## Contract
- `func New() *Store`
- Safe for concurrent use (guard state with a `sync.Mutex`).
- `Add(ctx, title, url)`:
  - Trim surrounding whitespace from title and url before validating and storing.
  - Empty title: return `fmt.Errorf("%w: title is required", bookmark.ErrInvalid)`.
  - Title longer than 200 characters: `fmt.Errorf("%w: title must be at most 200 characters", bookmark.ErrInvalid)`.
  - url must parse with `net/url`, have scheme `http` or `https`, and a non-empty host. Otherwise: `fmt.Errorf("%w: url must be an http or https URL", bookmark.ErrInvalid)`.
  - IDs start at 1 and increase by 1. An ID is never reused after a delete.
  - `CreatedAt` is `time.Now().UTC()`.
- `List(ctx)`: all bookmarks ordered by ID ascending, as a new slice. With no bookmarks it returns an empty, non-nil slice.
- `Delete(ctx, id)`: unknown ID returns `fmt.Errorf("bookmark %d: %w", id, bookmark.ErrNotFound)`.
- Include this line in store.go so the compiler checks the interface: `var _ bookmark.Service = (*Store)(nil)`

## Done when
- `go test ./internal/store/` passes.
- `go vet ./internal/store/` succeeds and `gofmt -l internal/store` prints nothing.
- Tests cover: add then list; each of the three validation errors, checked with `errors.Is(err, bookmark.ErrInvalid)`; whitespace is trimmed; delete removes; delete of an unknown ID gives `ErrNotFound`; an ID is not reused after delete; empty list is non-nil.

## Out of scope
HTTP, JSON, persistence to disk. Any file outside `internal/store/`.
```

### 2.2 Add the JSON API handler

```
Epic: 2 Components
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
An `http.Handler` that exposes a `bookmark.Service` as a JSON API. The UI server calls this API over HTTP.

## Read first
- `internal/bookmark/bookmark.go`: the `Bookmark` type (its JSON field names are the wire format), `ErrInvalid`, `ErrNotFound` and the `Service` interface you call.

## Change
- `internal/api/api.go`: package `api`, with `func NewHandler(svc bookmark.Service) http.Handler`.
- `internal/api/api_test.go`: tests using `net/http/httptest` and a fake `bookmark.Service` defined in the test file.

## Contract
`func NewHandler(svc bookmark.Service) http.Handler`

Register routes on an `http.NewServeMux()` using method patterns such as `"GET /api/bookmarks"` and read the path value with `r.PathValue("id")`.

Every response has `Content-Type: application/json`, except 204 which has no body.

| Request | Calls | Success | Errors |
|---|---|---|---|
| `GET /api/bookmarks` | `svc.List` | 200, JSON array of bookmarks. An empty list is `[]`, never `null`. | |
| `POST /api/bookmarks` with body `{"title":"...","url":"..."}` | `svc.Add` | 201, the bookmark as JSON | Body is not valid JSON: 400 `{"error":"invalid JSON body"}`. `errors.Is(err, bookmark.ErrInvalid)`: 400 `{"error": err.Error()}`. |
| `DELETE /api/bookmarks/{id}` | `svc.Delete` | 204, no body | `{id}` is not an integer: 400 `{"error":"invalid id"}`. `errors.Is(err, bookmark.ErrNotFound)`: 404 `{"error": err.Error()}`. |

Any other error from the service: 500 `{"error":"internal error"}`.

Limit request bodies to 1 MB with `http.MaxBytesReader`.

A bookmark on the wire looks like:
{"id":1,"title":"Go","url":"https://go.dev","created_at":"2026-01-01T00:00:00Z"}

## Done when
- `go test ./internal/api/` passes.
- `go vet ./internal/api/` succeeds and `gofmt -l internal/api` prints nothing.
- Tests cover every row and every error in the table, including that an empty list encodes as `[]`.

## Out of scope
Validation of title or url (the service does that). HTML. A `main` function. Any file outside `internal/api/`. Do not import `bookmarks/internal/store`; test against a fake.
```

### 2.3 Add the HTTP client for the JSON API

```
Epic: 2 Components
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
A client that implements `bookmark.Service` by calling the JSON API over HTTP. The UI server uses it so that it can treat the remote API like a local service.

## Read first
- `internal/bookmark/bookmark.go`: the `Bookmark` type (its JSON field names are the wire format), `ErrInvalid`, `ErrNotFound` and the `Service` interface you implement.

## Change
- `internal/client/client.go`: package `client`, with `type Client struct` and `func New(baseURL string, httpClient *http.Client) *Client`. `*Client` implements `bookmark.Service`.
- `internal/client/client_test.go`: tests against an `httptest.NewServer` whose handler you write in the test file to behave as the table under Contract says.

## Contract
- `func New(baseURL string, httpClient *http.Client) *Client`
  - `baseURL` is like `http://127.0.0.1:8081`. Remove any trailing `/` from it.
  - A nil `httpClient` means `http.DefaultClient`.
- Build requests with `http.NewRequestWithContext` so the caller's context applies.
- Include this line in client.go: `var _ bookmark.Service = (*Client)(nil)`

The API the client talks to:

| Method | Request | Success | Errors |
|---|---|---|---|
| `List` | `GET {baseURL}/api/bookmarks` | 200, JSON array of bookmarks | |
| `Add` | `POST {baseURL}/api/bookmarks`, header `Content-Type: application/json`, body `{"title":"...","url":"..."}` | 201, the bookmark as JSON | 400 with body `{"error":"<message>"}` |
| `Delete` | `DELETE {baseURL}/api/bookmarks/{id}` | 204, no body | 404 with body `{"error":"<message>"}` |

A bookmark on the wire looks like:
{"id":1,"title":"Go","url":"https://go.dev","created_at":"2026-01-01T00:00:00Z"}

Mapping responses to return values:
- `List` returns an empty, non-nil slice when the array is empty.
- Status 400: read `<message>` from the body. If it starts with `"invalid bookmark: "`, remove that prefix. Return `fmt.Errorf("%w: %s", bookmark.ErrInvalid, message)`. Example: body `{"error":"invalid bookmark: title is required"}` becomes an error whose text is `invalid bookmark: title is required`.
- Status 404: return an error wrapping `bookmark.ErrNotFound`.
- Any other unexpected status: return `fmt.Errorf("bookmarks api: unexpected status %d", status)`.
- Always close the response body.

## Done when
- `go test ./internal/client/` passes.
- `go vet ./internal/client/` succeeds and `gofmt -l internal/client` prints nothing.
- Tests cover: list with items; list with `[]`; add success, checking the request body the server received; add with a 400, checking `errors.Is(err, bookmark.ErrInvalid)` and the exact error text; delete success; delete with a 404, checking `errors.Is(err, bookmark.ErrNotFound)`; an unexpected 500; a trailing slash on `baseURL`.

## Out of scope
Retries, timeouts, caching. Any file outside `internal/client/`. Do not import `bookmarks/internal/api`; write the test server by hand from the table.
```

### 2.4 Add the HTMX UI handler

```
Epic: 2 Components
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
An `http.Handler` that serves a one-page HTMX interface for listing, adding and deleting bookmarks, on top of a `bookmark.Service`.

## Read first
- `internal/bookmark/bookmark.go`: the `Bookmark` type, `ErrInvalid`, `ErrNotFound` and the `Service` interface you call.

## Change
- `internal/ui/ui.go`: package `ui`, with `func NewHandler(svc bookmark.Service) http.Handler`.
- `internal/ui/page.html`: the templates, loaded with `//go:embed page.html` and parsed with `html/template`.
- `internal/ui/ui_test.go`: tests using `net/http/httptest` and a fake `bookmark.Service` defined in the test file.

## Contract
`func NewHandler(svc bookmark.Service) http.Handler`

Use `html/template` (not `text/template`) so titles and URLs are escaped.

HTMX works by swapping HTML that the server returns into the page. Every action here returns the same fragment, the `app` section, and HTMX replaces the old one with it.

The `app` fragment, as a template named `app`:

<section id="app">
  <form hx-post="/bookmarks" hx-target="#app" hx-swap="outerHTML">
    <input name="title" value="{{.Title}}" placeholder="Title">
    <input name="url" value="{{.URL}}" placeholder="https://example.com">
    <button type="submit">Add</button>
  </form>
  {{if .Error}}<p class="error">{{.Error}}</p>{{end}}
  <ul>
    {{range .Bookmarks}}
    <li>
      <a href="{{.URL}}">{{.Title}}</a>
      <button hx-delete="/bookmarks/{{.ID}}" hx-target="#app" hx-swap="outerHTML">Delete</button>
    </li>
    {{end}}
  </ul>
</section>

Its data is a struct with fields `Title string`, `URL string`, `Error string`, `Bookmarks []bookmark.Bookmark`.

The full page is a complete HTML document with a `<title>` of `Bookmarks`, this tag in `<head>`:
<script src="https://unpkg.com/htmx.org@2.0.4"></script>
and the `app` fragment inside `<body>`. Add a few lines of CSS in a `<style>` tag so it is readable; keep it small.

Routes, registered on an `http.NewServeMux()` with method patterns:

| Request | Behaviour |
|---|---|
| `GET /{$}` | 200, the full page, with `Bookmarks` from `svc.List` and empty `Title`, `URL`, `Error`. |
| `POST /bookmarks` with form fields `title` and `url` | Call `svc.Add`. On success: 200, the `app` fragment only, with the fresh list and empty `Title`, `URL`, `Error`. If `errors.Is(err, bookmark.ErrInvalid)`: 200, the `app` fragment with the fresh list, `Title` and `URL` set to what was submitted, and `Error` set to `err.Error()`. |
| `DELETE /bookmarks/{id}` | `{id}` is not an integer: 400 plain text. Otherwise call `svc.Delete`. On success, or if `errors.Is(err, bookmark.ErrNotFound)`: 200, the `app` fragment with the fresh list. |

Validation errors answer 200, not 4xx, because HTMX does not swap 4xx responses by default and the user must see the message.

Any other error from the service, in any route: 502 with a plain-text message (`http.Error`). The service is a remote API in production, so this means it could not be reached.

All HTML responses have `Content-Type: text/html; charset=utf-8`.

## Done when
- `go test ./internal/ui/` passes.
- `go vet ./internal/ui/` succeeds and `gofmt -l internal/ui` prints nothing.
- Tests cover: `GET /` returns a full document containing the htmx script tag and existing bookmarks; `POST /bookmarks` success returns a fragment that starts with `<section id="app"` (no `<html>`), contains the new title and has empty inputs; `POST /bookmarks` with an invalid bookmark returns 200 with the error text and the submitted values still in the inputs; `DELETE /bookmarks/1` returns the fragment without that bookmark; `DELETE /bookmarks/abc` returns 400; a service failure returns 502; a title containing `<script>` is escaped in the output.

## Out of scope
JavaScript beyond the htmx script tag. Calling the JSON API directly (the handler only uses the `bookmark.Service` it is given). A `main` function. Any file outside `internal/ui/`.
```

---

## Epic 3: Wiring

Re-check these against the code when epic 2 is done, in particular the four constructor signatures.

All four issues begin with the same Environment section:

```
## Environment
- Work in `samples/bookmarks/` under the repository root. It is a Go module named `bookmarks`.
- Run every command from `samples/bookmarks/`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- This is a separate project from the code in the repository root. The root's CLAUDE.md and ARCHITECTURE.md describe that other project and do not apply here. Do not edit anything outside `samples/bookmarks/`.
- Use the Go standard library only. Do not edit `go.mod`.
- Other workers are adding sibling files at the same time. Create only the files this issue lists, and do not change any existing file.
```

The four constructors these issues use:

```
bookmarks/internal/store    func New() *Store                                          // implements bookmark.Service
bookmarks/internal/api      func NewHandler(svc bookmark.Service) http.Handler         // JSON API under /api/bookmarks
bookmarks/internal/client   func New(baseURL string, httpClient *http.Client) *Client  // implements bookmark.Service; nil client = http.DefaultClient
bookmarks/internal/ui       func NewHandler(svc bookmark.Service) http.Handler         // HTMX pages at / and /bookmarks
```

### 3.1 Add the API server entrypoint

```
Epic: 3 Wiring
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
A runnable server for the JSON API, backed by the in-memory store.

## Read first
- `internal/store/store.go`: confirm `func New() *Store`.
- `internal/api/api.go`: confirm `func NewHandler(svc bookmark.Service) http.Handler`.

## Change
- `cmd/api/main.go`: package `main`.

## Contract
- Listen address comes from the environment variable `BOOKMARKS_API_ADDR`, default `127.0.0.1:8081`.
- Handler: `api.NewHandler(store.New())`.
- Use an `http.Server` with `ReadHeaderTimeout: 10 * time.Second`.
- Before serving, log one line to stderr with the standard `log` package: `bookmarks api listening on <addr>`.
- If the server fails to start, log the error and exit with status 1.

## Done when
- `go build ./cmd/api` and `go vet ./cmd/api` succeed; `gofmt -l cmd/api` prints nothing.
- With the server running, `curl -s http://127.0.0.1:8081/api/bookmarks` prints `[]`.
- No test file is needed for this package.

## Out of scope
Flags, config files, graceful shutdown. The UI server. Any file outside `cmd/api/`.
```

### 3.2 Add the UI server entrypoint

```
Epic: 3 Wiring
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
A runnable server for the HTMX UI. It holds no data of its own: it reaches the bookmarks through the JSON API over HTTP.

## Read first
- `internal/client/client.go`: confirm `func New(baseURL string, httpClient *http.Client) *Client`.
- `internal/ui/ui.go`: confirm `func NewHandler(svc bookmark.Service) http.Handler`.

## Change
- `cmd/ui/main.go`: package `main`.

## Contract
- Listen address comes from `BOOKMARKS_UI_ADDR`, default `127.0.0.1:8082`.
- API base URL comes from `BOOKMARKS_API_URL`, default `http://127.0.0.1:8081`.
- Handler: `ui.NewHandler(client.New(apiURL, &http.Client{Timeout: 5 * time.Second}))`.
- Use an `http.Server` with `ReadHeaderTimeout: 10 * time.Second`.
- Before serving, log one line to stderr with the standard `log` package: `bookmarks ui listening on <addr>, api at <apiURL>`.
- If the server fails to start, log the error and exit with status 1.

## Done when
- `go build ./cmd/ui` and `go vet ./cmd/ui` succeed; `gofmt -l cmd/ui` prints nothing.
- With the UI server running and no API server running, `curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:8082/` prints `502`.
- No test file is needed for this package.

## Out of scope
Flags, config files, graceful shutdown. The API server. Any file outside `cmd/ui/`. Do not import `bookmarks/internal/store` or `bookmarks/internal/api`.
```

### 3.3 Add the end-to-end test

```
Epic: 3 Wiring
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
One test that runs the whole chain in-process, UI handler to HTTP client to API handler to store, to prove the four components agree on the wire format.

## Read first
- `internal/api/api.go`: the routes and status codes of the JSON API.
- `internal/ui/ui.go` and `internal/ui/page.html`: the routes of the UI and the HTML it returns.

## Change
- `internal/e2e/e2e_test.go`: package `e2e_test`. This directory holds only the test file.

## Contract
Build the chain with `net/http/httptest`:

apiServer := httptest.NewServer(api.NewHandler(store.New()))
uiServer := httptest.NewServer(ui.NewHandler(client.New(apiServer.URL, nil)))

Close both with `t.Cleanup`. Make every request to `uiServer.URL` unless stated.

Steps, in one test function:
1. `GET /` returns 200 and the body contains `id="app"`.
2. `POST /bookmarks` with form `title=Go`, `url=https://go.dev` returns 200 and the body contains `https://go.dev`.
3. `POST /bookmarks` with form `title=Bad`, `url=not-a-url` returns 200 and the body contains `url must be an http or https URL`.
4. `GET {apiServer.URL}/api/bookmarks` returns a JSON array with exactly one element, whose `title` is `Go`. Take its `id`.
5. `DELETE /bookmarks/{id}` returns 200 and the body does not contain `https://go.dev`.
6. `GET /` no longer contains `https://go.dev`.

## Done when
- `go test ./internal/e2e/` passes.
- `go vet ./internal/e2e/` succeeds and `gofmt -l internal/e2e` prints nothing.
- If a step fails because two components disagree, do not change them. Report which step failed and what each side sent, as a comment on this issue.

## Out of scope
Changing any existing file. Starting real processes or binding fixed ports.
```

### 3.4 Add the README and Makefile

```
Epic: 3 Wiring
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
Tell a newcomer what the sample is and how to run it, and give them one-word commands for it.

## Read first
Nothing is required. Everything you need is under Contract.

## Change
- `README.md` (in `samples/bookmarks/`)
- `Makefile` (in `samples/bookmarks/`)

## Contract
Facts for the README:
- The sample is two servers. The API server keeps bookmarks in memory and serves JSON. The UI server serves an HTMX page and reaches the bookmarks only by calling the API server over HTTP.
- Start the API with `go run ./cmd/api`, then the UI with `go run ./cmd/ui` in a second terminal, then open http://127.0.0.1:8082.
- Data is lost when the API server stops.
- Environment variables: `BOOKMARKS_API_ADDR` (API listen address, default `127.0.0.1:8081`), `BOOKMARKS_UI_ADDR` (UI listen address, default `127.0.0.1:8082`), `BOOKMARKS_API_URL` (where the UI finds the API, default `http://127.0.0.1:8081`).
- API endpoints: `GET /api/bookmarks` lists; `POST /api/bookmarks` with JSON `{"title":"...","url":"..."}` adds; `DELETE /api/bookmarks/{id}` removes.
- Layout: `internal/bookmark` shared types and the Service interface; `internal/store` in-memory store; `internal/api` JSON handler; `internal/client` HTTP client for the API; `internal/ui` HTMX handler; `cmd/api` and `cmd/ui` the two servers.

Keep the README under 60 lines.

Makefile targets, each declared `.PHONY`:
- `test`: `go test ./...`
- `run-api`: `go run ./cmd/api`
- `run-ui`: `go run ./cmd/ui`

Makefile recipe lines must start with a tab character, not spaces.

## Done when
- `make -n test run-api run-ui` prints the three commands without error.
- Every command and default in the README matches the Contract.

## Out of scope
Any Go file. Docker, CI, deployment.
```
