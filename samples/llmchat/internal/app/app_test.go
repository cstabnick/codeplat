package app

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"llmchat/internal/chat"
)

const completionSSE = "data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\n" +
	"data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n" +
	"data: [DONE]\n\n"

// fakeRouter answers the two router endpoints llmchat uses. Nothing else is reached.
func fakeRouter(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/routes":
			io.WriteString(w, `{"default":"general","routes":[{"name":"general"},{"name":"code"}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
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
	return srv
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
	fake := fakeRouter(t)
	a, err := New(Config{
		Addr:      "127.0.0.1:0",
		DBPath:    filepath.Join(t.TempDir(), "sub", "chat.db"),
		RouterURL: fake.URL,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
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

	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestConfigFromEnvSet(t *testing.T) {
	t.Setenv("CHAT_ADDR", "0.0.0.0:9000")
	t.Setenv("CHAT_DB", "/var/lib/x.db")
	t.Setenv("CHAT_ROUTER_URL", "http://router:8083")

	got := ConfigFromEnv()
	want := Config{Addr: "0.0.0.0:9000", DBPath: "/var/lib/x.db", RouterURL: "http://router:8083"}
	if got != want {
		t.Errorf("ConfigFromEnv() = %+v, want %+v", got, want)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("CHAT_ADDR", "")
	t.Setenv("CHAT_DB", "")
	t.Setenv("CHAT_ROUTER_URL", "")
	t.Setenv("XDG_DATA_HOME", dataHome)

	got := ConfigFromEnv()
	want := Config{
		Addr:      "127.0.0.1:8084",
		DBPath:    filepath.Join(dataHome, "llmchat", "llmchat.db"),
		RouterURL: "http://127.0.0.1:8083",
	}
	if got != want {
		t.Errorf("ConfigFromEnv() = %+v, want %+v", got, want)
	}
}
