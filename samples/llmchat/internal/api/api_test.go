package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// fakeStore is an in-memory chat.Store.
type fakeStore struct {
	mu    sync.Mutex
	next  int
	convs map[string]chat.Conversation
	msgs  map[string][]chat.Message
}

func newFakeStore() *fakeStore {
	return &fakeStore{convs: map[string]chat.Conversation{}, msgs: map[string][]chat.Message{}}
}

func (s *fakeStore) CreateConversation(_ context.Context, c chat.Conversation) (chat.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	now := time.Now().UTC()
	c.ID = fmt.Sprintf("conv-%d", s.next)
	c.CreatedAt, c.UpdatedAt = now, now
	s.convs[c.ID] = c
	return c, nil
}

func (s *fakeStore) GetConversation(_ context.Context, id string) (chat.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.convs[id]
	if !ok {
		return chat.Conversation{}, fmt.Errorf("conversation %s: %w", id, chat.ErrNotFound)
	}
	return c, nil
}

func (s *fakeStore) ListConversations(_ context.Context) ([]chat.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []chat.Conversation{}
	for _, c := range s.convs {
		out = append(out, c)
	}
	return out, nil
}

func (s *fakeStore) UpdateConversation(_ context.Context, id string, p chat.ConversationPatch) (chat.Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.convs[id]
	if !ok {
		return chat.Conversation{}, fmt.Errorf("conversation %s: %w", id, chat.ErrNotFound)
	}
	if p.Title != nil {
		c.Title = *p.Title
	}
	if p.Model != nil {
		c.Model = *p.Model
	}
	if p.Workspace != nil {
		c.Workspace = *p.Workspace
	}
	c.UpdatedAt = time.Now().UTC()
	s.convs[id] = c
	return c, nil
}

func (s *fakeStore) DeleteConversation(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.convs[id]; !ok {
		return fmt.Errorf("conversation %s: %w", id, chat.ErrNotFound)
	}
	delete(s.convs, id)
	delete(s.msgs, id)
	return nil
}

func (s *fakeStore) AppendMessage(_ context.Context, id string, m chat.Message) (chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.convs[id]; !ok {
		return chat.Message{}, fmt.Errorf("conversation %s: %w", id, chat.ErrNotFound)
	}
	m.Seq = len(s.msgs[id]) + 1
	m.CreatedAt = time.Now().UTC()
	s.msgs[id] = append(s.msgs[id], m)
	return m, nil
}

func (s *fakeStore) ListMessages(_ context.Context, id string) ([]chat.Message, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.convs[id]; !ok {
		return nil, fmt.Errorf("conversation %s: %w", id, chat.ErrNotFound)
	}
	return append([]chat.Message{}, s.msgs[id]...), nil
}

// failingStore fails ListConversations with err and passes everything else through.
type failingStore struct {
	*fakeStore
	err error
}

func (s failingStore) ListConversations(context.Context) ([]chat.Conversation, error) {
	return nil, s.err
}

// fakeRunner runs a scripted function as the turn.
type fakeRunner struct {
	run func(ctx context.Context, id, text string, emit func(chat.Event)) error
}

func (f *fakeRunner) Run(ctx context.Context, id, text string, emit func(chat.Event)) error {
	return f.run(ctx, id, text, emit)
}

// doneRunner emits one content event and then done.
var doneRunner = &fakeRunner{run: func(_ context.Context, _, _ string, emit func(chat.Event)) error {
	emit(chat.Event{Type: chat.EventContent, Text: "hello"})
	emit(chat.Event{Type: chat.EventDone})
	return nil
}}

func newTestServer(t *testing.T, store chat.Store, runner Runner, approvals *Approvals, routes func(context.Context) ([]string, error)) *httptest.Server {
	t.Helper()
	if store == nil {
		store = newFakeStore()
	}
	if runner == nil {
		runner = doneRunner
	}
	if approvals == nil {
		approvals = NewApprovals()
	}
	if routes == nil {
		routes = func(context.Context) ([]string, error) { return []string{"auto", "fast"}, nil }
	}
	srv := httptest.NewServer(NewHandler(store, runner, approvals, routes, log.New(io.Discard, "", 0)))
	t.Cleanup(srv.Close)
	return srv
}

// send makes one request with a 10 second deadline. The response body is
// closed when the test ends.
func send(t *testing.T, srv *httptest.Server, method, path, body string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	req, err := http.NewRequestWithContext(ctx, method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// expectStatus checks the status and returns the body.
func expectStatus(t *testing.T, resp *http.Response, want int) []byte {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, want, body)
	}
	return body
}

// expectError checks an error status, a JSON content type and an {"error": ...} body.
func expectError(t *testing.T, resp *http.Response, want int) string {
	t.Helper()
	body := expectStatus(t, resp, want)
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &e); err != nil || e.Error == "" {
		t.Fatalf("error body %s: want {\"error\": ...}", body)
	}
	return e.Error
}

// parseEvents reads the rest of an event stream, one event per "data:" line,
// until the response ends. Use one reader for every read of a stream.
func parseEvents(t *testing.T, br *bufio.Reader) []chat.Event {
	t.Helper()
	var events []chat.Event
	for {
		line, err := br.ReadString('\n')
		if strings.HasPrefix(line, "data: ") {
			var ev chat.Event
			if uerr := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); uerr != nil {
				t.Fatalf("bad event %q: %v", line, uerr)
			}
			events = append(events, ev)
		}
		if errors.Is(err, io.EOF) {
			return events
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// unblocker closes a channel once, so a test can release a runner from
// both the test body and a defer.
type unblocker struct {
	once sync.Once
	ch   chan struct{}
}

func newUnblocker() *unblocker { return &unblocker{ch: make(chan struct{})} }
func (u *unblocker) release()  { u.once.Do(func() { close(u.ch) }) }

func createConv(t *testing.T, store chat.Store, title string) chat.Conversation {
	t.Helper()
	c, err := store.CreateConversation(context.Background(), chat.Conversation{Title: title, Model: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// --- conversations ---

func TestListConversations(t *testing.T) {
	store := newFakeStore()
	srv := newTestServer(t, store, nil, nil, nil)

	var empty []chat.Conversation
	body := expectStatus(t, send(t, srv, "GET", "/api/conversations", ""), http.StatusOK)
	if err := json.Unmarshal(body, &empty); err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list body = %s, want []", body)
	}

	createConv(t, store, "one")
	createConv(t, store, "two")
	var list []chat.Conversation
	body = expectStatus(t, send(t, srv, "GET", "/api/conversations", ""), http.StatusOK)
	if err := json.Unmarshal(body, &list); err != nil || len(list) != 2 {
		t.Fatalf("list body = %s, want 2 conversations", body)
	}
}

func TestCreateConversation(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, nil)

	resp := send(t, srv, "POST", "/api/conversations", `{"title":"hi","model":"auto","workspace":"/tmp/w"}`)
	var c chat.Conversation
	if err := json.Unmarshal(expectStatus(t, resp, http.StatusCreated), &c); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" || c.Title != "hi" || c.Model != "auto" || c.Workspace != "/tmp/w" {
		t.Fatalf("created = %+v", c)
	}

	// An empty body takes every default.
	resp = send(t, srv, "POST", "/api/conversations", "")
	if err := json.Unmarshal(expectStatus(t, resp, http.StatusCreated), &c); err != nil {
		t.Fatal(err)
	}
	if c.ID == "" || c.Title != "" || c.Model != "" || c.Workspace != "" {
		t.Fatalf("empty-body create = %+v, want defaults", c)
	}

	// Only a field the body names is set.
	resp = send(t, srv, "POST", "/api/conversations", `{}`)
	expectStatus(t, resp, http.StatusCreated)
}

func TestCreateConversationBadBody(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, nil)
	for name, body := range map[string]string{
		"invalid json":  `{"title":`,
		"unknown field": `{"titel":"typo"}`,
	} {
		t.Run(name, func(t *testing.T) {
			expectError(t, send(t, srv, "POST", "/api/conversations", body), http.StatusBadRequest)
		})
	}
}

func TestGetConversation(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "read me")
	ctx := context.Background()
	if _, err := store.AppendMessage(ctx, c.ID, chat.Message{Role: chat.RoleUser, Content: "q"}); err != nil {
		t.Fatal(err)
	}
	srv := newTestServer(t, store, nil, nil, nil)

	var got struct {
		Conversation chat.Conversation `json:"conversation"`
		Messages     []chat.Message    `json:"messages"`
	}
	body := expectStatus(t, send(t, srv, "GET", "/api/conversations/"+c.ID, ""), http.StatusOK)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Conversation.ID != c.ID || got.Conversation.Title != "read me" {
		t.Fatalf("conversation = %+v", got.Conversation)
	}
	if len(got.Messages) != 1 || got.Messages[0].Content != "q" || got.Messages[0].Seq != 1 {
		t.Fatalf("messages = %+v", got.Messages)
	}

	// No messages gives an empty array, not null.
	other := createConv(t, store, "empty")
	body = expectStatus(t, send(t, srv, "GET", "/api/conversations/"+other.ID, ""), http.StatusOK)
	if !strings.Contains(string(body), `"messages":[]`) {
		t.Fatalf("empty messages body = %s, want \"messages\":[]", body)
	}
}

func TestGetConversationNotFound(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, nil)
	expectError(t, send(t, srv, "GET", "/api/conversations/nope", ""), http.StatusNotFound)
}

func TestUpdateConversation(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "old")
	srv := newTestServer(t, store, nil, nil, nil)

	var got chat.Conversation
	body := expectStatus(t, send(t, srv, "PATCH", "/api/conversations/"+c.ID, `{"title":"new","workspace":"/w"}`), http.StatusOK)
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Workspace != "/w" || got.Model != "auto" {
		t.Fatalf("updated = %+v, want title new, workspace /w, model unchanged", got)
	}
}

func TestUpdateConversationErrors(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "x")
	srv := newTestServer(t, store, nil, nil, nil)

	expectError(t, send(t, srv, "PATCH", "/api/conversations/nope", `{"title":"y"}`), http.StatusNotFound)
	expectError(t, send(t, srv, "PATCH", "/api/conversations/"+c.ID, `{"colour":"red"}`), http.StatusBadRequest)
	expectError(t, send(t, srv, "PATCH", "/api/conversations/"+c.ID, `{"title":`), http.StatusBadRequest)
}

func TestDeleteConversation(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "bye")
	srv := newTestServer(t, store, nil, nil, nil)

	expectStatus(t, send(t, srv, "DELETE", "/api/conversations/"+c.ID, ""), http.StatusNoContent)
	expectError(t, send(t, srv, "GET", "/api/conversations/"+c.ID, ""), http.StatusNotFound)
	expectError(t, send(t, srv, "DELETE", "/api/conversations/"+c.ID, ""), http.StatusNotFound)
}

// failingStore errors map to statuses: ErrInvalid 400, ErrNotFound 404, anything else 500.
func TestErrorStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
	}{
		{"invalid", fmt.Errorf("bad: %w", chat.ErrInvalid), http.StatusBadRequest},
		{"not found", fmt.Errorf("gone: %w", chat.ErrNotFound), http.StatusNotFound},
		{"other", errors.New("disk on fire"), http.StatusInternalServerError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := failingStore{fakeStore: newFakeStore(), err: tc.err}
			srv := newTestServer(t, store, nil, nil, nil)
			expectError(t, send(t, srv, "GET", "/api/conversations", ""), tc.status)
		})
	}
}

func TestInternalErrorHidesDetail(t *testing.T) {
	store := failingStore{fakeStore: newFakeStore(), err: errors.New("disk on fire")}
	srv := newTestServer(t, store, nil, nil, nil)
	if msg := expectError(t, send(t, srv, "GET", "/api/conversations", ""), http.StatusInternalServerError); strings.Contains(msg, "disk on fire") {
		t.Errorf("500 message %q leaks the internal error", msg)
	}
}

func TestOversizedBodyIsBadRequest(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, nil)
	big := `{"title":"` + strings.Repeat("x", 1<<20) + `"}`
	expectError(t, send(t, srv, "POST", "/api/conversations", big), http.StatusBadRequest)
}

// --- messages and the stream ---

func TestSendMessageRejectsBeforeStreaming(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "chat")
	srv := newTestServer(t, store, nil, nil, nil)

	expectError(t, send(t, srv, "POST", "/api/conversations/nope/messages", `{"text":"hi"}`), http.StatusNotFound)
	expectError(t, send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"text":""}`), http.StatusBadRequest)
	expectError(t, send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"text":"   \n\t"}`), http.StatusBadRequest)
	expectError(t, send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"txt":"hi"}`), http.StatusBadRequest)
}

func TestSendMessageStreamsEvents(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "chat")
	var gotID, gotText string
	runner := &fakeRunner{run: func(_ context.Context, id, text string, emit func(chat.Event)) error {
		gotID, gotText = id, text
		emit(chat.Event{Type: chat.EventInfo, Route: "fast", Backend: "ollama", Model: "m"})
		emit(chat.Event{Type: chat.EventReasoning, Text: "think"})
		emit(chat.Event{Type: chat.EventContent, Text: "Hel"})
		emit(chat.Event{Type: chat.EventContent, Text: "lo"})
		emit(chat.Event{Type: chat.EventDone})
		return nil
	}}
	srv := newTestServer(t, store, runner, nil, nil)

	resp := send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"text":"hi"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q, want no-cache", cc)
	}
	events := parseEvents(t, bufio.NewReader(resp.Body))

	if gotID != c.ID || gotText != "hi" {
		t.Errorf("runner got (%q, %q), want (%q, \"hi\")", gotID, gotText, c.ID)
	}
	want := []chat.Event{
		{Type: chat.EventInfo, Route: "fast", Backend: "ollama", Model: "m"},
		{Type: chat.EventReasoning, Text: "think"},
		{Type: chat.EventContent, Text: "Hel"},
		{Type: chat.EventContent, Text: "lo"},
		{Type: chat.EventDone},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestRunnerErrorBecomesErrorThenDone(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "chat")
	runner := &fakeRunner{run: func(_ context.Context, _, _ string, emit func(chat.Event)) error {
		emit(chat.Event{Type: chat.EventContent, Text: "partial"})
		return errors.New("router down")
	}}
	srv := newTestServer(t, store, runner, nil, nil)

	events := parseEvents(t, bufio.NewReader(send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"text":"hi"}`).Body))
	want := []chat.Event{
		{Type: chat.EventContent, Text: "partial"},
		{Type: chat.EventError, Error: "router down"},
		{Type: chat.EventDone},
	}
	if len(events) != len(want) {
		t.Fatalf("events = %+v, want %+v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Errorf("event %d = %+v, want %+v", i, events[i], want[i])
		}
	}
}

func TestEventsArriveBeforeTurnEnds(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "chat")
	gate := newUnblocker()
	defer gate.release()
	runner := &fakeRunner{run: func(_ context.Context, _, _ string, emit func(chat.Event)) error {
		emit(chat.Event{Type: chat.EventContent, Text: "first"})
		<-gate.ch
		emit(chat.Event{Type: chat.EventDone})
		return nil
	}}
	srv := newTestServer(t, store, runner, nil, nil)

	// The first event is flushed with the headers, so this returns while the turn is blocked.
	resp := send(t, srv, "POST", "/api/conversations/"+c.ID+"/messages", `{"text":"hi"}`)
	br := bufio.NewReader(resp.Body)
	line, err := br.ReadString('\n')
	if err != nil {
		t.Fatalf("reading first event: %v", err)
	}
	if want := `data: {"type":"content","text":"first"}` + "\n"; line != want {
		t.Fatalf("first line = %q, want %q", line, want)
	}

	gate.release()
	rest := parseEvents(t, br)
	if len(rest) != 1 || rest[0].Type != chat.EventDone {
		t.Fatalf("after release: events = %+v, want done", rest)
	}
}

func TestSecondConcurrentTurnIsConflict(t *testing.T) {
	store := newFakeStore()
	c := createConv(t, store, "chat")
	gate := newUnblocker()
	defer gate.release()
	var calls int32
	runner := &fakeRunner{run: func(_ context.Context, _, _ string, emit func(chat.Event)) error {
		if atomic.AddInt32(&calls, 1) == 1 {
			emit(chat.Event{Type: chat.EventContent, Text: "busy"})
			<-gate.ch
		}
		emit(chat.Event{Type: chat.EventDone})
		return nil
	}}
	srv := newTestServer(t, store, runner, nil, nil)
	path := "/api/conversations/" + c.ID + "/messages"

	first := send(t, srv, "POST", path, `{"text":"one"}`)
	firstBody := bufio.NewReader(first.Body)
	if _, err := firstBody.ReadString('\n'); err != nil {
		t.Fatalf("first turn did not start: %v", err)
	}

	msg := expectError(t, send(t, srv, "POST", path, `{"text":"two"}`), http.StatusConflict)
	if msg == "" {
		t.Error("409 has an empty error")
	}

	// Another conversation is not blocked by this one.
	other := createConv(t, store, "other")
	expectStatus(t, send(t, srv, "POST", "/api/conversations/"+other.ID+"/messages", `{"text":"x"}`), http.StatusOK)

	// The first turn can still finish.
	gate.release()
	events := parseEvents(t, firstBody)
	if len(events) == 0 || events[len(events)-1].Type != chat.EventDone {
		t.Fatalf("first turn after release: events = %+v, want it to end with done", events)
	}

	// Once the turn has ended, the conversation accepts a new one.
	expectStatus(t, send(t, srv, "POST", path, `{"text":"three"}`), http.StatusOK)
}

// --- approvals ---

func TestApproveBlocksUntilResolved(t *testing.T) {
	a := NewApprovals()
	req := chat.ApprovalRequest{ConversationID: "c1", CallID: "call-1", Name: "shell", Arguments: `{}`}
	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, err := a.Approve(context.Background(), req)
		done <- result{ok, err}
	}()

	// Resolve fails until Approve has registered, so retry until it succeeds.
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := a.Resolve("c1", "call-1", true)
		if err == nil {
			break
		}
		if !errors.Is(err, chat.ErrNotFound) || time.Now().After(deadline) {
			t.Fatalf("Resolve: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
	select {
	case r := <-done:
		if r.err != nil || !r.ok {
			t.Fatalf("Approve = (%v, %v), want (true, nil)", r.ok, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Approve did not return after Resolve")
	}
}

func TestApproveDenied(t *testing.T) {
	a := NewApprovals()
	done := make(chan bool, 1)
	go func() {
		ok, err := a.Approve(context.Background(), chat.ApprovalRequest{ConversationID: "c", CallID: "k"})
		if err != nil {
			t.Errorf("Approve error: %v", err)
		}
		done <- ok
	}()
	waitResolve(t, a, "c", "k", false)
	if ok := <-done; ok {
		t.Fatal("Approve = true after Resolve(false)")
	}
}

// waitResolve retries Resolve until the request is registered.
func waitResolve(t *testing.T, a *Approvals, conv, call string, approve bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := a.Resolve(conv, call, approve)
		if err == nil {
			return
		}
		if !errors.Is(err, chat.ErrNotFound) || time.Now().After(deadline) {
			t.Fatalf("Resolve: %v", err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestApproveCancelled(t *testing.T) {
	a := NewApprovals()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		ok, err := a.Approve(ctx, chat.ApprovalRequest{ConversationID: "c", CallID: "k"})
		if ok {
			t.Error("Approve = true after cancel")
		}
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Approve error = %v, want context.Canceled", err)
	}
	// The request is no longer waiting.
	if err := a.Resolve("c", "k", true); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("Resolve after cancel = %v, want ErrNotFound", err)
	}
}

func TestResolveNothingWaiting(t *testing.T) {
	a := NewApprovals()
	if err := a.Resolve("c", "k", true); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("Resolve = %v, want ErrNotFound", err)
	}
}

// A cancelled Approve must not delete a newer waiter registered under the same key.
func TestCancelledApproveLeavesNewWaiterAlone(t *testing.T) {
	a := NewApprovals()
	key := approvalKey("c", "k")
	oldCh := make(chan bool, 1)
	newCh := make(chan bool, 1)

	a.mu.Lock()
	a.waiting[key] = newCh
	a.mu.Unlock()

	a.approvalCleanup(key, oldCh)

	a.mu.Lock()
	cur, ok := a.waiting[key]
	a.mu.Unlock()
	if !ok || cur != newCh {
		t.Fatalf("waiting[%q] after cleanup of old channel = %v, %v; want the new waiter", key, cur, ok)
	}

	// The newer waiter can still be answered.
	if err := a.Resolve("c", "k", true); err != nil {
		t.Fatalf("Resolve for new waiter = %v, want nil", err)
	}
	if got := <-newCh; !got {
		t.Fatalf("new waiter got false, want true")
	}
}

func TestSameCallIDInTwoConversations(t *testing.T) {
	a := NewApprovals()
	type result struct {
		conv string
		ok   bool
		err  error
	}
	done := make(chan result, 2)
	for _, conv := range []string{"a", "b"} {
		go func(conv string) {
			ok, err := a.Approve(context.Background(), chat.ApprovalRequest{ConversationID: conv, CallID: "x"})
			done <- result{conv, ok, err}
		}(conv)
	}

	waitResolve(t, a, "a", "x", true)
	select {
	case r := <-done:
		if r.conv != "a" || !r.ok || r.err != nil {
			t.Fatalf("first answer = %+v, want conversation a approved", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conversation a was not answered")
	}

	// Conversation b is still waiting and gets its own answer.
	waitResolve(t, a, "b", "x", false)
	select {
	case r := <-done:
		if r.conv != "b" || r.ok || r.err != nil {
			t.Fatalf("second answer = %+v, want conversation b denied", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("conversation b was not answered")
	}
}

func TestApprovalRoute(t *testing.T) {
	approvals := NewApprovals()
	srv := newTestServer(t, nil, nil, approvals, nil)

	done := make(chan bool, 1)
	go func() {
		ok, _ := approvals.Approve(context.Background(), chat.ApprovalRequest{ConversationID: "conv-9", CallID: "call-7"})
		done <- ok
	}()

	// The route answers 404 until the request is registered, so poll.
	path := "/api/conversations/conv-9/approvals/call-7"
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp := send(t, srv, "POST", path, `{"approve":true}`)
		if resp.StatusCode == http.StatusNoContent {
			break
		}
		expectError(t, resp, http.StatusNotFound)
		if time.Now().After(deadline) {
			t.Fatal("approval never registered")
		}
		time.Sleep(time.Millisecond)
	}
	if ok := <-done; !ok {
		t.Fatal("Approve = false, want true")
	}

	// Already answered, so nothing is waiting.
	expectError(t, send(t, srv, "POST", path, `{"approve":true}`), http.StatusNotFound)
}

func TestApprovalRouteDenyAndBadBody(t *testing.T) {
	approvals := NewApprovals()
	srv := newTestServer(t, nil, nil, approvals, nil)

	done := make(chan bool, 1)
	go func() {
		ok, _ := approvals.Approve(context.Background(), chat.ApprovalRequest{ConversationID: "c", CallID: "k"})
		done <- ok
	}()
	path := "/api/conversations/c/approvals/k"
	expectError(t, send(t, srv, "POST", path, `{"approve":"yes"}`), http.StatusBadRequest)

	deadline := time.Now().Add(5 * time.Second)
	for {
		resp := send(t, srv, "POST", path, `{"approve":false}`)
		if resp.StatusCode == http.StatusNoContent {
			break
		}
		expectError(t, resp, http.StatusNotFound)
		if time.Now().After(deadline) {
			t.Fatal("approval never registered")
		}
		time.Sleep(time.Millisecond)
	}
	if ok := <-done; ok {
		t.Fatal("Approve = true, want false")
	}
}

// --- routes and health ---

func TestHealthz(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, nil)
	var got map[string]string
	body := expectStatus(t, send(t, srv, "GET", "/healthz", ""), http.StatusOK)
	if err := json.Unmarshal(body, &got); err != nil || got["status"] != "ok" || len(got) != 1 {
		t.Fatalf("healthz body = %s, want {\"status\":\"ok\"}", body)
	}
}

func TestListRoutes(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, func(context.Context) ([]string, error) {
		return []string{"auto", "fast"}, nil
	})
	var got []string
	body := expectStatus(t, send(t, srv, "GET", "/api/routes", ""), http.StatusOK)
	if err := json.Unmarshal(body, &got); err != nil || len(got) != 2 || got[0] != "auto" || got[1] != "fast" {
		t.Fatalf("routes body = %s", body)
	}
}

func TestListRoutesEmpty(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, func(context.Context) ([]string, error) {
		return nil, nil
	})
	body := expectStatus(t, send(t, srv, "GET", "/api/routes", ""), http.StatusOK)
	if strings.TrimSpace(string(body)) != "[]" {
		t.Fatalf("empty routes body = %s, want []", body)
	}
}

func TestListRoutesFailure(t *testing.T) {
	srv := newTestServer(t, nil, nil, nil, func(context.Context) ([]string, error) {
		return nil, errors.New("router unreachable")
	})
	msg := expectError(t, send(t, srv, "GET", "/api/routes", ""), http.StatusBadGateway)
	if msg != "router unreachable" {
		t.Errorf("error = %q, want the routes error", msg)
	}
}
