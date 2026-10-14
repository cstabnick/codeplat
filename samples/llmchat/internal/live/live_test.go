// Package live_test runs llmchat against a real llmrouter. The tests skip
// unless LLMCHAT_LIVE=1 is set, so that `go test ./...` never reaches a
// model. Use `make live` to run them.
package live_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"llmchat/internal/app"
	"llmchat/internal/chat"
)

const defaultRouter = "http://127.0.0.1:8083"

func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LLMCHAT_LIVE") != "1" {
		t.Skip("set LLMCHAT_LIVE=1 to run against a real router")
	}
}

func routerURL() string {
	if v := os.Getenv("LLMCHAT_LIVE_ROUTER"); v != "" {
		return v
	}
	return defaultRouter
}

// startApp builds the real app against the router and serves it over HTTP.
func startApp(t *testing.T, router string) *httptest.Server {
	t.Helper()
	a, err := app.New(app.Config{
		Addr:      "127.0.0.1:0",
		DBPath:    filepath.Join(t.TempDir(), "chat.db"),
		RouterURL: router,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(func() { a.Close() })
	t.Cleanup(srv.Close)
	return srv
}

// skipIfUnreachable skips the test when the router's route list cannot be fetched.
func skipIfUnreachable(t *testing.T, router string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(strings.TrimRight(router, "/") + "/api/routes")
	if err != nil {
		t.Skipf("router not reachable at %s: %v", router, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("router not reachable at %s: GET /api/routes returned %d", router, resp.StatusCode)
	}
}

// postJSON posts a JSON body and returns the response, which the caller closes.
func postJSON(t *testing.T, client *http.Client, url string, body any) *http.Response {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

// createConversation makes a conversation with the given model and returns its ID.
func createConversation(t *testing.T, client *http.Client, base, model string) string {
	t.Helper()
	resp := postJSON(t, client, base+"/api/conversations", map[string]string{"model": model})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create conversation: status %d", resp.StatusCode)
	}
	var c chat.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	return c.ID
}

// sendMessage posts a user message and returns the events of the turn in order.
func sendMessage(t *testing.T, client *http.Client, base, id, text string) []chat.Event {
	t.Helper()
	resp := postJSON(t, client, base+"/api/conversations/"+id+"/messages", map[string]string{"text": text})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send message: status %d", resp.StatusCode)
	}
	var events []chat.Event
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			t.Fatalf("decode event %q: %v", payload, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read events: %v", err)
	}
	return events
}

func TestChat(t *testing.T) {
	requireLive(t)
	router := routerURL()
	skipIfUnreachable(t, router)

	srv := startApp(t, router)
	client := &http.Client{Timeout: 5 * time.Minute}

	id := createConversation(t, client, srv.URL, "auto")
	events := sendMessage(t, client, srv.URL, id, "Reply with the single word: pong")

	if len(events) == 0 {
		t.Fatal("no events")
	}
	var info *chat.Event
	var content []string
	var sawError bool
	for i := range events {
		switch events[i].Type {
		case chat.EventInfo:
			if info == nil {
				info = &events[i]
			}
		case chat.EventContent:
			content = append(content, events[i].Text)
		case chat.EventError:
			sawError = true
			t.Errorf("error event: %s", events[i].Error)
		}
	}
	if info == nil {
		t.Fatal("no info event")
	}
	if len(content) == 0 {
		t.Fatal("no content events")
	}
	if sawError {
		t.FailNow()
	}
	if last := events[len(events)-1]; last.Type != chat.EventDone {
		t.Fatalf("last event is %q, want %q", last.Type, chat.EventDone)
	}

	answer := strings.Join(content, "")
	t.Logf("route=%s backend=%s model=%s", info.Route, info.Backend, info.Model)
	t.Logf("content: %s", answer)

	resp, err := client.Get(srv.URL + "/api/conversations/" + id)
	if err != nil {
		t.Fatalf("GET conversation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET conversation: status %d", resp.StatusCode)
	}
	var got struct {
		Messages []chat.Message `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("got %d messages, want 2", len(got.Messages))
	}
	if got.Messages[1].Content == "" {
		t.Fatal("assistant message is empty")
	}
}

func TestRoutePinned(t *testing.T) {
	requireLive(t)
	route := os.Getenv("LLMCHAT_LIVE_ROUTE")
	if route == "" {
		t.Skip("set LLMCHAT_LIVE_ROUTE to a route name to run this test")
	}
	router := routerURL()
	skipIfUnreachable(t, router)

	srv := startApp(t, router)
	client := &http.Client{Timeout: 5 * time.Minute}

	id := createConversation(t, client, srv.URL, route)
	events := sendMessage(t, client, srv.URL, id, "Say hello.")

	var info *chat.Event
	for i := range events {
		if events[i].Type == chat.EventInfo {
			info = &events[i]
			break
		}
	}
	if info == nil {
		t.Fatal("no info event")
	}
	if info.Route != route {
		t.Fatalf("info route = %q, want %q", info.Route, route)
	}
}
