package app_test

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"llmchat/internal/app"
	"llmchat/internal/apptest"
	"llmchat/internal/chat"
	"llmchat/internal/config"
)

const completionSSE = "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
	"data: [DONE]\n\n"

// requestLog keeps the bodies of the completion requests the fake router got.
type requestLog struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (l *requestLog) add(body map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.bodies = append(l.bodies, body)
}

// all returns the recorded request bodies, oldest first.
func (l *requestLog) all() []map[string]any {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]map[string]any(nil), l.bodies...)
}

// fakeRouter answers the two router endpoints llmchat uses. Nothing else is reached.
// It also returns a log of the completion request bodies.
func fakeRouter(t *testing.T) (*httptest.Server, *requestLog) {
	t.Helper()
	log := &requestLog{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/routes":
			io.WriteString(w, `{"default":"general","routes":[{"name":"general"},{"name":"code"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode completion request: %v", err)
			}
			log.add(body)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("X-Llmrouter-Route", "general")
			w.Header().Set("X-Llmrouter-Backend", "ollama")
			w.Header().Set("X-Llmrouter-Model", "m")
			io.WriteString(w, completionSSE)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, log
}

// sseTypes reads an event stream and returns the type of each event, in order.
func sseTypes(t *testing.T, body io.Reader) []string {
	t.Helper()
	var types []string
	sc := bufio.NewScanner(body)
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
		types = append(types, ev.Type)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	return types
}

func TestServerEndToEnd(t *testing.T) {
	fake, _ := fakeRouter(t)
	a := apptest.New(t, fake.URL, filepath.Join(t.TempDir(), "sub", "chat.db"))
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()

	get := func(path string) (*http.Response, string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return resp, string(b)
	}

	if resp, _ := get("/healthz"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /healthz: status %d, want 200", resp.StatusCode)
	}

	if resp, body := get("/"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", resp.StatusCode)
	} else if !strings.Contains(body, `id="composer"`) {
		t.Errorf("GET /: page does not contain id=\"composer\"")
	}

	if resp, body := get("/api/routes"); resp.StatusCode != http.StatusOK {
		t.Errorf("GET /api/routes: status %d, want 200", resp.StatusCode)
	} else {
		var names []string
		if err := json.Unmarshal([]byte(body), &names); err != nil {
			t.Fatalf("decode /api/routes %q: %v", body, err)
		}
		if want := []string{"general", "code"}; !reflect.DeepEqual(names, want) {
			t.Errorf("GET /api/routes = %v, want %v", names, want)
		}
	}

	resp, err := http.Post(srv.URL+"/api/conversations", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	var conv chat.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated || conv.ID == "" {
		t.Fatalf("create conversation: status %d, id %q", resp.StatusCode, conv.ID)
	}

	resp, err = http.Post(srv.URL+"/api/conversations/"+conv.ID+"/messages", "application/json", strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	types := sseTypes(t, resp.Body)
	resp.Body.Close()
	if want := []string{"info", "content", "content", "done"}; !reflect.DeepEqual(types, want) {
		t.Errorf("message events = %v, want %v", types, want)
	}

	_, body := get("/api/conversations/" + conv.ID)
	var got struct {
		Messages []chat.Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode conversation %q: %v", body, err)
	}
	if len(got.Messages) != 2 {
		t.Fatalf("messages = %d, want 2: %s", len(got.Messages), body)
	}
	if m := got.Messages[0]; m.Role != chat.RoleUser || m.Content != "hi" {
		t.Errorf("first message = role %q content %q, want user \"hi\"", m.Role, m.Content)
	}
	if m := got.Messages[1]; m.Role != chat.RoleAssistant || m.Content != "hello" || m.Route != "general" {
		t.Errorf("second message = role %q content %q route %q, want assistant \"hello\" route \"general\"", m.Role, m.Content, m.Route)
	}

}

// newWorkspaceApp builds the app with the given default workspace and step
// limit, against a fresh fake router, and serves it.
func newWorkspaceApp(t *testing.T, workspace string, maxSteps int) (*httptest.Server, *requestLog) {
	t.Helper()
	fake, log := fakeRouter(t)
	a, err := app.New(config.Config{
		Addr:      "127.0.0.1:0",
		DBPath:    filepath.Join(t.TempDir(), "chat.db"),
		RouterURL: fake.URL,
		MaxSteps:  maxSteps,
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return srv, log
}

// createConversation posts body to /api/conversations and returns the result.
func createConversation(t *testing.T, srv *httptest.Server, body string) chat.Conversation {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/conversations", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	defer resp.Body.Close()
	var conv chat.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&conv); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create conversation: status %d", resp.StatusCode)
	}
	return conv
}

// sendMessage posts a message and drains the event stream.
func sendMessage(t *testing.T, srv *httptest.Server, id string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/api/conversations/"+id+"/messages", "application/json", strings.NewReader(`{"text":"hi"}`))
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	defer resp.Body.Close()
	sseTypes(t, resp.Body)
}

func TestWorkspaceConversationOffersTools(t *testing.T) {
	dir := t.TempDir()
	srv, log := newWorkspaceApp(t, "", 0)
	conv := createConversation(t, srv, `{"workspace":"`+dir+`"}`)
	sendMessage(t, srv, conv.ID)

	reqs := log.all()
	if len(reqs) != 1 {
		t.Fatalf("completion requests = %d, want 1", len(reqs))
	}
	req := reqs[0]

	var names []string
	tools, _ := req["tools"].([]any)
	for _, raw := range tools {
		tool, _ := raw.(map[string]any)
		fn, _ := tool["function"].(map[string]any)
		name, _ := fn["name"].(string)
		names = append(names, name)
	}
	if want := []string{"read_file", "list_dir", "grep", "write_file", "edit_file", "bash"}; !reflect.DeepEqual(names, want) {
		t.Errorf("tool names = %v, want %v", names, want)
	}

	msgs, _ := req["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("request has no messages")
	}
	first, _ := msgs[0].(map[string]any)
	if role, _ := first["role"].(string); role != "system" {
		t.Errorf("first message role = %q, want system", role)
	}
	content, _ := first["content"].(string)
	if !strings.HasPrefix(content, "You are a coding assistant") {
		t.Errorf("system content = %q, want prefix %q", content, "You are a coding assistant")
	}
	if want := "Workspace directory: " + dir; !strings.HasSuffix(content, want) {
		t.Errorf("system content = %q, want suffix %q", content, want)
	}
}

func TestConversationWithoutWorkspaceOffersNoTools(t *testing.T) {
	srv, log := newWorkspaceApp(t, "", 0)
	conv := createConversation(t, srv, `{}`)
	sendMessage(t, srv, conv.ID)

	reqs := log.all()
	if len(reqs) != 1 {
		t.Fatalf("completion requests = %d, want 1", len(reqs))
	}
	if _, ok := reqs[0]["tools"]; ok {
		t.Errorf("request has tools key, want none: %v", reqs[0]["tools"])
	}
}

func TestConfiguredWorkspaceIsDefaultForNewConversations(t *testing.T) {
	dir := t.TempDir()
	srv, _ := newWorkspaceApp(t, dir, 0)
	conv := createConversation(t, srv, `{}`)
	if conv.Workspace != dir {
		t.Errorf("workspace = %q, want %q", conv.Workspace, dir)
	}
}
