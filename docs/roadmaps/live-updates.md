# Roadmap: live updates for the web UI

## Goal

The tracker's web UI stops re-fetching every three seconds. Instead the server pushes a message when something changes, through a simple message bus with topics, and the page refreshes only then.

Tracker project: `PLAT`.

## Design

```
store (any process)          web server process                          browser
-------------------          ------------------                          -------
change + event row   --->    feed tails the event log (every 500ms)
in one transaction           publishes to bus topics    --->  SSE  --->  htmx re-fetches
                             project.<KEY>, issue.<ID>   /events?topic=  the region
```

- **Event log.** The CLI, the MCP server and the REST server are separate processes that write to the same SQLite file. The web server cannot hear about their changes in memory, so every store change also appends a row to an `events` table in the same transaction.
- **Feed.** The web server reads new rows from that log on a short interval and publishes each one. This is one small indexed query per tick, however many browsers are open.
- **Bus.** An in-process publish/subscribe bus with string topics: `project.<KEY>` for everything in a project, `issue.<ID>` for one issue.
- **Transport to the browser.** Server-Sent Events, consumed by the htmx SSE extension. It needs no hand-written JavaScript, and the browser reconnects by itself.
- **Safety net.** Each live region also re-fetches once a minute, in case a message was missed while the connection was down.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Parts | The event log, the bus and the SSE handler each pass their own tests. | 3, parallel |
| 2. Feed | Events written by a store come out of the bus under the right topics. | 1 |
| 3. Switch-over | The pages subscribe over SSE and no longer poll every three seconds. | 2, parallel |

No issue has a dependency inside its epic.

Why three epics: the feed needs the event log's types and the bus to compile (epic 2), and the switch-over needs the feed (epic 3).

Why epic 1 is parallel: the three parts live in packages that do not import one another. The SSE handler declares the one-method interface it needs instead of importing the bus, and the bus carries plain bytes instead of event types. So no issue's build check compiles a package another issue is editing.

Why 1.1 is one issue although it changes five files: adding methods to the `core.Store` interface breaks the build until both stores implement them.

---

## Epic 1: Parts

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
  - A database schema change is a new entry appended to the `migrations` slice in `internal/store/sqlite/sqlite.go`. Never edit or reorder the existing entries.
- The repository rule about exposing every use case in REST, CLI and MCP does not apply to this work: the event feed is internal to the web UI for now.
- Other workers are editing other packages at the same time. Create or edit only the files this issue lists, and run only the commands under "Done when": a repository-wide `make test` or `go build ./...` may fail because of someone else's half-finished file.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 1.1 Record an event for every change in both stores

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
Every change to a project, issue or comment also appends one row to an event log, in the same transaction as the change. Other processes will read this log to learn that something changed.

## Read first
- `internal/core/types.go`: the `Store` interface and the `Project`, `Issue` and `Comment` types.
- `internal/core/service.go`: the last methods of `Service`, to see how it passes calls to the store.
- `internal/store/memory/memory.go`: all of it.
- `internal/store/sqlite/sqlite.go`: the `migrations` slice, `CreateProject`, `CreateIssue`, `UpdateIssue`, `AddComment`, `formatTime` and `parseTime`.
- `internal/store/storetest/storetest.go`: the `comments` subtest, as the model for a new subtest.

## Change
- `internal/core/types.go`: add the `EventType` type and constants, the `Event` type, and two methods on the `Store` interface, exactly as under Contract.
- `internal/core/service.go`: add two methods that pass straight through to the store.
- `internal/store/memory/memory.go`: record events and implement the two methods.
- `internal/store/sqlite/sqlite.go`: add a migration, record events and implement the two methods.
- `internal/store/storetest/storetest.go` (tests): add an `events` subtest inside `Run`.
- `internal/core/service_test.go` (tests): add a check of the two Service methods.

## Contract
Add to `internal/core/types.go`:

type EventType string

const (
	EventProjectCreated EventType = "project.created"
	EventIssueCreated   EventType = "issue.created"
	EventIssueUpdated   EventType = "issue.updated"
	EventCommentAdded   EventType = "comment.added"
)

// Event records that something changed. Stores append one for every
// successful change, in the same transaction as the change.
type Event struct {
	ID         int64     `json:"id"`
	Type       EventType `json:"type"`
	ProjectKey string    `json:"project_key"`
	IssueID    string    `json:"issue_id,omitempty"`
	At         time.Time `json:"at"`
}

Add to the `Store` interface, after `ListComments`:

	// EventsSince returns the events whose ID is greater than after, oldest
	// first. It never returns nil.
	EventsSince(ctx context.Context, after int64) ([]Event, error)
	// LastEventID returns the ID of the newest event, or 0 if there are none.
	LastEventID(ctx context.Context) (int64, error)

Also extend the comment above the `Store` interface with one sentence: every successful CreateProject, CreateIssue, UpdateIssue and AddComment records exactly one Event.

Add to `internal/core/service.go`:

func (s *Service) EventsSince(ctx context.Context, after int64) ([]Event, error) {
	return s.store.EventsSince(ctx, after)
}

func (s *Service) LastEventID(ctx context.Context) (int64, error) {
	return s.store.LastEventID(ctx)
}

Which event each store method records, only when the method succeeds:

| Store method | Type | ProjectKey | IssueID | At |
|---|---|---|---|---|
| `CreateProject` | `project.created` | the project's key | empty | the project's `CreatedAt` |
| `CreateIssue` | `issue.created` | the issue's `ProjectKey` | the new issue's `ID` | the issue's `CreatedAt` |
| `UpdateIssue` | `issue.updated` | the issue's `ProjectKey` | the issue's `ID` | the issue's `UpdatedAt` after `fn` ran |
| `AddComment` | `comment.added` | the `ProjectKey` of the issue commented on | the comment's `IssueID` | the comment's `CreatedAt` |

A call that returns an error records nothing. That includes a duplicate project, an issue in a missing project, an `UpdateIssue` whose `fn` returns an error, and a comment on a missing issue.

Event IDs start at 1 and increase by 1 in the order the changes happened.

Memory store: keep events in a slice on the `Store` struct, appended under the existing mutex. The ID is the slice length after appending. `EventsSince` returns a new, non-nil slice.

SQLite store:
- Append this as the fourth entry of `migrations`:

	`CREATE TABLE events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		type        TEXT NOT NULL,
		project_key TEXT NOT NULL,
		issue_id    TEXT NOT NULL,
		at          TEXT NOT NULL
	);`,

- Record with `INSERT INTO events (type, project_key, issue_id, at) VALUES (?, ?, ?, ?)`, using `formatTime` for `at` and an empty string for a missing issue ID. Run it on the same `tx` as the change, before `tx.Commit()`.
- `CreateProject` currently runs one statement without a transaction. Give it a transaction like `CreateIssue` has (`s.db.BeginTx`, `defer tx.Rollback()`, `tx.Commit()`). Keep the `ON CONFLICT (key) DO NOTHING` insert and the `RowsAffected` check; when it reports a conflict, return `ErrConflict` as today without recording an event.
- `AddComment` needs the issue's project key. Inside its transaction, read it with `SELECT project_key FROM issues WHERE id = ?`. No row still means the same `ErrNotFound` error as today. Leave `issueExists` in place; `ListComments` uses it.
- `EventsSince`: `SELECT id, type, project_key, issue_id, at FROM events WHERE id > ? ORDER BY id`, parsing `at` with `parseTime`. Start from `out := []core.Event{}` so the result is never nil.
- `LastEventID`: `SELECT COALESCE(MAX(id), 0) FROM events`.

## Done when
- `go vet ./internal/core/ ./internal/store/... ./internal/app/ ./internal/apptest/` succeeds.
- `gofmt -l internal/core internal/store` prints nothing.
- `go test -race ./internal/core/ ./internal/store/...` passes.
- The `events` subtest in `storetest.go` runs against both stores (it does automatically, because both stores' tests call `storetest.Run`) and covers:
  - a new store has `LastEventID` 0 and `EventsSince(0)` empty and non-nil;
  - after creating a project, creating an issue in it, updating the issue and commenting on it, `EventsSince(0)` returns four events with IDs 1 to 4, each with the Type, ProjectKey, IssueID and At from the table above, and `LastEventID` is 4;
  - `EventsSince(2)` returns only the events with IDs 3 and 4;
  - each of the four failing calls listed above leaves `LastEventID` unchanged.
- The check in `service_test.go` creates a project and an issue through the Service from `apptest.New(t)` and confirms `LastEventID` is 2 and `EventsSince(1)` returns one `issue.created` event.

## Out of scope
Publishing events anywhere, HTTP, the web UI. Deleting old events. Exposing events in REST, the CLI or MCP. Any file not listed under Change.
```

### 1.2 Add an in-process message bus with topics

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
A small publish/subscribe bus: code publishes bytes to a named topic, and every current subscriber of that topic receives them. The web server will use it to tell open browser connections that something changed.

## Read first
Nothing. This is a new, self-contained package.

## Change
- `internal/bus/bus.go`: package `bus`.
- `internal/bus/bus_test.go` (tests): package `bus_test`.

## Contract
package bus

// Bus delivers messages published to a topic to that topic's subscribers.
// It is safe for concurrent use.
type Bus struct { /* unexported fields */ }

func New() *Bus

// Publish sends data to every current subscriber of topic. It never blocks:
// a subscriber whose buffer is full misses this message.
func (b *Bus) Publish(topic string, data []byte)

// Subscribe returns a channel that receives the data of every message
// published to topic from now on, and a cancel function that ends the
// subscription.
func (b *Bus) Subscribe(topic string) (<-chan []byte, func())

Details:
- A topic is any string, matched exactly. There are no wildcards.
- Each subscription has its own channel with a buffer of 16.
- `Publish` uses a non-blocking send (`select` with `default`), so one slow subscriber cannot hold up the publisher or the other subscribers.
- A message is delivered only to subscriptions that exist when `Publish` is called. Nothing is stored for later subscribers.
- The cancel function removes the subscription. It may be called more than once. It does not close the channel.
- When the last subscription of a topic is cancelled, remove the topic's entry so the map does not grow forever.
- Guard the state with a `sync.Mutex`.
- Do not import any other package of this repository.

## Done when
- `go test -race ./internal/bus/` passes.
- `go vet ./internal/bus/` succeeds and `gofmt -l internal/bus` prints nothing.
- Tests cover: a subscriber receives what is published to its topic and nothing published to another topic; two subscribers of one topic both receive a message; publishing with no subscribers does nothing; publishing 100 messages to a subscriber that never reads returns promptly and leaves exactly 16 in its channel; after cancel, a publish delivers nothing to that channel; cancel twice does not panic; concurrent publish, subscribe and cancel from several goroutines pass under `-race`.

## Out of scope
Persistence, wildcards, message types beyond bytes, anything about HTTP or events. Any file outside `internal/bus/`.
```

### 1.3 Add the Server-Sent Events handler

```
Epic: 1 Parts
Depends on: none

## Environment
(the Epic 1 Environment section above, pasted in full)

## Goal
An `http.Handler` that keeps a response open and streams the messages of one topic to the browser as Server-Sent Events. The web UI will connect to it so a page learns immediately when something changed.

## Read first
Nothing. This is a new, self-contained package.

## Change
- `internal/transport/web/sse/sse.go`: package `sse`.
- `internal/transport/web/sse/sse_test.go` (tests): package `sse`, so it can shorten `keepAlive`.

## Contract
package sse

// Subscriber is the source of messages. A message bus satisfies it.
type Subscriber interface {
	// Subscribe returns a channel of the messages for topic and a function
	// that ends the subscription.
	Subscribe(topic string) (<-chan []byte, func())
}

// keepAlive is how often a comment line is sent on an idle stream so that
// proxies keep the connection open. It is a variable so tests can shorten it.
var keepAlive = 25 * time.Second

func Handler(sub Subscriber) http.Handler

Behaviour of the handler, in order:
1. Read the topic from the query parameter `topic`. If it is empty, respond 400 with plain text `topic is required` and return.
2. If the `http.ResponseWriter` is not an `http.Flusher`, respond 500 with plain text `streaming unsupported` and return.
3. Call `sub.Subscribe(topic)` and `defer` the returned cancel function.
4. Set the headers `Content-Type: text/event-stream` and `Cache-Control: no-cache`, write the status 200, write the line `: connected` followed by a blank line, and flush.
5. Loop until the request's context is done:
   - on a message from the channel, write `event: changed`, then one `data: ` line for each line of the message (split the message on `\n`), then a blank line, and flush;
   - every `keepAlive`, write `: ping` followed by a blank line, and flush.

For a message `{"id":3}` the bytes written are exactly:
event: changed\ndata: {"id":3}\n\n

Do not import any other package of this repository. In particular do not import `codeplat/internal/bus`: the `Subscriber` interface is what keeps this package independent of it.

## Done when
- `go test -race ./internal/transport/web/sse/` passes.
- `go vet ./internal/transport/web/sse/` succeeds and `gofmt -l internal/transport/web/sse` prints nothing.
- Tests use `httptest.NewServer` and a fake `Subscriber` defined in the test file, and cover: a missing topic gives 400; the response has the two headers and starts with `: connected`; the fake is asked for exactly the topic in the query; a message sent on the fake's channel arrives as the exact bytes shown above; a message containing a newline arrives as two `data:` lines; with `keepAlive` shortened to a few milliseconds a `: ping` line arrives; after the client disconnects the cancel function is called (wait for it with a timeout, do not sleep a fixed time).

## Out of scope
Deciding which topics exist, authentication, replaying missed messages. Any file outside `internal/transport/web/sse/`. Do not edit `internal/transport/web/web.go` or `page.html`.
```

---

## Epic 2: Feed

Re-check against the code when epic 1 is done: `core.Event`, `Service.EventsSince`, `bus.Bus.Publish`.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- The repository rule about exposing every use case in REST, CLI and MCP does not apply to this work: the event feed is internal to the web UI for now.
- Create or edit only the files this issue lists.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 2.1 Feed store events onto the bus

```
Epic: 2 Feed
Depends on: none

## Environment
(the Epic 2 Environment section above, pasted in full)

## Goal
A loop that reads new events from the event log and publishes each one on the message bus under its topics. It is the bridge between changes made by any process and the browsers connected to this one.

## Read first
- `internal/core/types.go`: the `Event` type and its JSON field names.
- `internal/core/service.go`: `EventsSince` and `LastEventID`.
- `internal/bus/bus.go`: `New`, `Publish` and `Subscribe`.

## Change
- `internal/transport/web/feed.go`: package `web`.
- `internal/transport/web/feed_test.go` (tests): package `web`, because `feed` is unexported.

## Contract
// topicsFor returns the bus topics an event is published on: always
// "project.<ProjectKey>", and also "issue.<IssueID>" when the event has an
// issue ID.
func topicsFor(e core.Event) []string

// feed publishes every event with an ID greater than after onto b, checking
// for new events every interval, until ctx is done.
func feed(ctx context.Context, svc *core.Service, b *bus.Bus, after int64, interval time.Duration, log *slog.Logger)

Behaviour of `feed`:
- Use a `time.NewTicker(interval)` and stop it on return. Return when `ctx` is done.
- On each tick call `svc.EventsSince(ctx, after)`. If it returns an error, log it with `log.Error` and try again on the next tick; unless `ctx` is done, in which case return without logging.
- For each event, in order: marshal it with `encoding/json`, publish those bytes to every topic from `topicsFor`, then set `after` to the event's ID.

Examples for `topicsFor`:
- `{Type: "project.created", ProjectKey: "BOOK"}` gives `["project.BOOK"]`
- `{Type: "comment.added", ProjectKey: "BOOK", IssueID: "BOOK-3"}` gives `["project.BOOK", "issue.BOOK-3"]`

## Done when
- `go test -race ./internal/transport/web/` passes.
- `go vet ./internal/transport/web/` succeeds and `gofmt -l internal/transport/web` prints nothing.
- `make test` passes.
- Tests get the Service from `apptest.New(t)` and cover:
  - `topicsFor` for an event with and without an issue ID;
  - events that already exist are not published: create a project, read `LastEventID`, start `feed` with that value as `after`, and the project's creation never arrives;
  - after `feed` has started with a 5 millisecond interval, creating an issue delivers to a subscriber of `project.<KEY>` and to a subscriber of `issue.<ID>` a message that unmarshals to a `core.Event` with type `issue.created` and that issue ID;
  - a subscriber of another project's topic receives nothing;
  - `feed` returns after its context is cancelled.
  Wait for messages with a timeout of a second or two; do not sleep a fixed time and then check.

## Out of scope
The HTTP handler, the page template, starting `feed` from `NewHandler`. Do not edit `web.go`, `page.html` or `web_test.go`.
```

---

## Epic 3: Switch-over

Re-check against the code when epic 2 is done: `feed`, `sse.Handler`, and the current text of `page.html`.

```
## Environment
- Work at the repository root: the directory whose `go.mod` says `module codeplat`.
- If `go` is not found, run `export PATH=$HOME/.local/go/bin:$PATH` first.
- These rules of this repository apply to you:
  - Business rules live in `internal/core`. Transports do not validate.
  - A test that needs a `*core.Service` gets it from `apptest.New(t).Service` (package `codeplat/internal/apptest`). Do not construct a store or a service by hand in such a test.
- The repository rule about exposing every use case in REST, CLI and MCP does not apply to this work: the event feed is internal to the web UI for now.
- Another worker is editing other files at the same time. Create or edit only the files this issue lists.
- Do not run `make build`. Other workers are using the binaries in `bin/`.
- Use the Go standard library only. Do not edit `go.mod`.
```

### 3.1 Switch the web UI from polling to Server-Sent Events

```
Epic: 3 Switch-over
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
The web UI's board and issue pages stop re-fetching every three seconds. They open a Server-Sent Events connection for their topic and re-fetch when a message arrives.

## Read first
- `internal/transport/web/web.go`: `NewHandler`.
- `internal/transport/web/page.html`: the `<script>` tag in `<head>`, the `<section id="board"` element and the `<div id="live"` element.
- `internal/transport/web/feed.go`: the signature of `feed`.
- `internal/transport/web/sse/sse.go`: `Handler`.
- `cmd/web/main.go`: the call to `web.NewHandler`.

## Change
- `internal/transport/web/web.go`: `NewHandler` takes a context, creates the bus, starts the feed and registers the events route.
- `internal/transport/web/page.html`: load the htmx SSE extension and replace the two polling triggers.
- `cmd/web/main.go`: pass `ctx` to `web.NewHandler`.
- `internal/transport/web/web_test.go` (tests): the implementation worker changes only the existing `web.NewHandler(a.Service, a.Log)` call to `web.NewHandler(t.Context(), a.Service, a.Log)` so the package compiles. The testing worker updates and adds the assertions.

## Contract
In `web.go`:

// feedInterval is how often the web server looks for new events.
const feedInterval = 500 * time.Millisecond

func NewHandler(ctx context.Context, svc *core.Service, log *slog.Logger) http.Handler

`NewHandler` additionally does this, before returning the mux:
- `b := bus.New()` (import `codeplat/internal/bus`)
- `after, err := svc.LastEventID(ctx)`; on an error, log it with `log.Error` and continue with `after` as 0
- `go feed(ctx, svc, b, after, feedInterval, log)`
- `mux.Handle("GET /events", sse.Handler(b))` (import `codeplat/internal/transport/web/sse`)

The feed stops when `ctx` is done. `cmd/web/main.go` already has a `ctx` that is cancelled on shutdown; pass that one.

In `page.html`:
- Directly after the existing htmx `<script>` tag add:
  <script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>
- Wrap the board section in a new element and change its trigger. The section's opening tag becomes these two lines, and a closing `</div>` goes directly after the section's closing `</section>`:
  <div hx-ext="sse" sse-connect="/events?topic=project.{{.Active}}">
  <section id="board" class="board" hx-get="/projects/{{.Active}}" hx-trigger="sse:changed, every 60s" hx-select="#board" hx-target="#board" hx-swap="outerHTML">
- Do the same for the live region of the issue page. Its opening tag becomes these two lines, and a closing `</div>` goes directly after the `</div>` that closes `<div id="live"`:
  <div hx-ext="sse" sse-connect="/events?topic=issue.{{.ID}}">
  <div id="live" hx-get="/issues/{{.ID}}" hx-trigger="sse:changed, every 60s" hx-select="#live" hx-target="#live" hx-swap="outerHTML">
- The text `every 3s` must no longer appear anywhere in the file.

The wrapper element is not replaced when the region inside it is swapped, so the connection stays open across refreshes. `every 60s` is a safety net for a message missed while the connection was down.

## Done when
- `make test` passes.
- `go vet ./...` succeeds and `gofmt -l cmd internal` prints nothing.
- `web_test.go` covers:
  - the project page contains `sse-connect="/events?topic=project.AAA"`, `hx-trigger="sse:changed, every 60s"` and the `htmx-ext-sse` script tag, and does not contain `every 3s`;
  - the issue page contains `sse-connect="/events?topic=issue.AAA-3"`;
  - end to end: serve the handler with `httptest.NewServer`, open `GET /events?topic=project.AAA` with a request whose context the test cancels at the end, read until the `: connected` line, then create an issue in project AAA through the Service and read an `event: changed` line within two seconds;
  - `GET /events` without a topic returns 400.
- You cannot check this in a browser. Say so in your comment, so a person knows that step is still open.

## Out of scope
Changing `feed.go`, the `sse` package or the `bus` package. Any change to what the pages show. `ARCHITECTURE.md` and `README.md` (another issue owns them).
```

### 3.2 Document live updates in ARCHITECTURE.md

```
Epic: 3 Switch-over
Depends on: none

## Environment
(the Epic 3 Environment section above, pasted in full)

## Goal
`ARCHITECTURE.md` describes how the web UI now learns about changes, so the next person does not have to rediscover why there is an event log, a feed and a bus.

## Read first
- `ARCHITECTURE.md`: the code block under "Shape" and the bullet list under "Decisions".

## Change
- `ARCHITECTURE.md`

## Contract
1. In the code block under "Shape", add this line directly above the line that starts with `config/`:
internal/bus                   in-process message bus with topics, used by the web UI

2. In the "Web UI" bullet under "Decisions", replace the sentence that begins "It always renders whole pages;" up to and including "so there is no fragment-rendering code." with:
It always renders whole pages; htmx boosts the links, and a live region re-fetches its own URL and selects itself from the response, so there is no fragment-rendering code.

3. Directly after the "Web UI" bullet, add this bullet:
- **Live updates:** every change a store makes also appends a row to an event log, in the same transaction. The web server reads new rows from that log every 500ms (`feed`), publishes each on an in-process bus under the topics `project.<KEY>` and `issue.<ID>`, and streams a topic to the browser as Server-Sent Events at `/events?topic=...`. A live region re-fetches when its topic fires, and once a minute as a safety net. The log exists because the CLI, the MCP server and the REST server are separate processes: it is how their changes reach a browser connected to the web server.

4. In the "Not built yet" bullet, add `pruning the event log` to the list.

Change nothing else in the file.

## Done when
- `grep -c "internal/bus" ARCHITECTURE.md` prints 1.
- `grep -c "Live updates:" ARCHITECTURE.md` prints 1.
- `grep -c "poll their own URL" ARCHITECTURE.md` prints 0.
- `grep -c "pruning the event log" ARCHITECTURE.md` prints 1.
- No tests are needed.

## Out of scope
Any Go file, `README.md`, `CLAUDE.md`.
```
