// Stop-turn tests: POST /stop with no turn, stop during a running turn,
// and stop while an approval is waiting.
package e2e_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// slowRouter streams content chunks, sleeps long enough that a /stop can land
// while the turn is running, then finishes the stream. The test can wait for
// the router's completion handler to start before posting /stop.
type slowRouter struct {
	*httptest.Server

	mu      sync.Mutex
	reqs    []routerRequest
	started sync.Once
}

func (f *slowRouter) ready() {
	f.started.Do(func() {})
}

// readStream drains a body that emits "data: ..." events and returns them.
func readStream(body io.Reader) []chat.Event {
	var events []chat.Event
	sc := bufio.NewScanner(body)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			// Ignore malformed lines; they should not happen.
			continue
		}
		events = append(events, ev)
	}
	return events
}

func hasEvent(events []chat.Event, typ string) bool {
	for _, ev := range events {
		if ev.Type == typ {
			return true
		}
	}
	return false
}

// stopPost does a POST to url with no body and returns (status, raw body).
func stopPost(t *testing.T, url string) (int, []byte) {
	t.Helper()
	r, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

// TestStopNoRunningTurnReturnsNotFound checks that POST /stop is 404 when no
// turn is running, both for an existing conversation and an unknown one.
func TestStopNoRunningTurnReturnsNotFound(t *testing.T) {
	router := newSlowRouter()
	dbPath := t.TempDir() + "/llmchat.db"
	a, srv := start(t, dbPath, router.URL)
	defer a.Close()
	defer srv.Close()
	defer router.Close()
	base := srv.URL

	conv := createConversation(t, base)

	// No turn running: /stop is 404.
	status, _ := stopPost(t, base+"/api/conversations/"+conv.ID+"/stop")
	if status != http.StatusNotFound {
		t.Fatalf("stop with no running turn: status %d, want 404", status)
	}

	// Unknown conversation: /stop is 404.
	status, _ = stopPost(t, base+"/api/conversations/000000000000/stop")
	if status != http.StatusNotFound {
		t.Fatalf("stop for unknown conversation: status %d, want 404", status)
	}
}

// TestStopDuringRunningTurnEmitsStoppedThenDone starts a turn against a slow
// router, posts /stop while it is in flight, and verifies the open stream
// delivers a stopped event, then a done event, with no error event.
func TestStopDuringRunningTurnEmitsStoppedThenDone(t *testing.T) {
	router := newSlowRouter()
	dbPath := t.TempDir() + "/llmchat.db"
	a, srv := start(t, dbPath, router.URL)
	defer a.Close()
	defer srv.Close()
	defer router.Close()
	base := srv.URL

	conv := createConversation(t, base)

	// Open the turn. The response stream stays open while the router sleeps.
	req, err := http.NewRequest(
		http.MethodPost,
		base+"/api/conversations/"+conv.ID+"/messages",
		strings.NewReader(`{"text":"hi"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send: status %d, want 200", resp.StatusCode)
	}

	// Stop while the turn is running.
	status, _ := stopPost(t, base+"/api/conversations/"+conv.ID+"/stop")
	if status != http.StatusNoContent {
		t.Fatalf("stop during a running turn: status %d, want 204", status)
	}

	// The open stream now delivers a stopped event, then done, no error.
	events := readStream(resp.Body)
	if !hasEvent(events, chat.EventStopped) {
		t.Fatalf("events after stop = %+v, want a stopped event", events)
	}
	if hasEvent(events, chat.EventError) {
		t.Fatalf("stop produced an error event: %+v", events)
	}
	if last := events[len(events)-1]; last.Type != chat.EventDone {
		t.Fatalf("last event after stop = %+v, want done", last)
	}

	// A second /stop while no turn runs is 404 again.
	status, _ = stopPost(t, base+"/api/conversations/"+conv.ID+"/stop")
	if status != http.StatusNotFound {
		t.Fatalf("second stop (no turn): status %d, want 404", status)
	}
}

// TestStopWhileApprovalIsWaiting drives a turn that stops to ask for a
// tool approval (via agentRouter, which maps "write note.txt" to write_file),
// posts /stop while the engine is blocked on Approve, and verifies the open
// stream delivers stopped then done, with no error event.
func TestStopWhileApprovalIsWaiting(t *testing.T) {
	router := newAgentRouter(t)
	base := agentStart(t, router.URL)

	ws := t.TempDir()
	conv := agentCreate(t, base, ws)

	// We need to post /stop while the engine is waiting on Approve.
	// agentTurn streams events into onEvent. We post /stop from inside onEvent
	// when the approval event arrives, so the turn is still running.
	var (
		mu       sync.Mutex
		events   []chat.Event
		stopSent bool
	)
	done := make(chan []chat.Event)

	// Start the turn; agentTurn blocks reading the stream until it ends.
	go func() {
		status, evts, err := agentTurn(context.Background(), base, conv.ID, "write note.txt", func(ev chat.Event) {
			mu.Lock()
			defer mu.Unlock()
			events = append(events, ev)
			if ev.Type == chat.EventApproval && !stopSent {
				stopSent = true
				// Post /stop while the engine is blocked on Approve.
				go func() {
					stopPost(t, base+"/api/conversations/"+conv.ID+"/stop")
				}()
			}
		})
		_ = status
		_ = err
		done <- evts
	}()

	// Wait for the stream to finish.
	events := <-done

	if !hasEvent(events, chat.EventApproval) {
		t.Fatalf("no approval event in stream: %+v", events)
	}
	if !hasEvent(events, chat.EventStopped) {
		t.Fatalf("events after stop = %+v, want a stopped event", events)
	}
	if hasEvent(events, chat.EventError) {
		t.Fatalf("stop while waiting for approval produced an error event: %+v", events)
	}
	if last := events[len(events)-1]; last.Type != chat.EventDone {
		t.Fatalf("last event = %+v, want done", last)
	}
}

// newSlowRouter returns a slowRouter whose completion handler is triggered by
// the engine calling /v1/chat/completions.
func newSlowRouter() *slowRouter {
	f := &slowRouter{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	return f
}

func (f *slowRouter) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/routes":
		io.WriteString(w, `{"default":"general","routes":[{"name":"general"}]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		f.completion(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *slowRouter) completion(w http.ResponseWriter, r *http.Request) {
	f.ready()
	f.mu.Lock()
	f.reqs = append(f.reqs, routerRequest{Model: "auto", Stream: true})
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	writeChunk(w, map[string]any{"content": "hi "})
	writeChunk(w, map[string]any{"content": "there"})
	// Sleep so a /stop can land while the turn is still running.
	time.Sleep(30 * time.Second)
	writeChunk(w, map[string]any{"content": "!!"})
	io.WriteString(w, "data: [DONE]\n\n")
}
