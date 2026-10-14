package app

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"llmrouter/internal/config"
	"llmrouter/internal/llm"
)

// fakeOllama answers the Ollama endpoints the app uses and records the
// names of the models it is asked to pull.
type fakeOllama struct {
	mu     sync.Mutex
	pulled []string
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/version":
		io.WriteString(w, `{"version":"test"}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/tags":
		io.WriteString(w, `{"models":[{"name":"llama3:8b"}]}`)
	case r.Method == http.MethodGet && r.URL.Path == "/api/ps":
		io.WriteString(w, `{"models":[]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/api/pull":
		var in struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		f.pulled = append(f.pulled, in.Model)
		f.mu.Unlock()
		io.WriteString(w, `{"status":"success"}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeOllama) pulls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.pulled...)
}

func testConfig(url string) config.Config {
	return config.Config{
		Addr: "127.0.0.1:0",
		Backends: []config.Backend{{
			Name:        "ollama",
			Kind:        llm.KindOllama,
			URL:         url,
			PullMissing: true,
		}},
		Intent: config.Intent{Backend: "ollama", Model: "small:1b"},
		Routes: []llm.Route{
			{Name: "general", Backend: "ollama", Model: "llama3:8b"},
			{Name: "code", Backend: "ollama", Model: "coder:1b"},
		},
		DefaultRoute: "general",
	}
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestAppLifecycle(t *testing.T) {
	fake := &fakeOllama{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	logger := log.New(io.Discard, "", 0)
	a, err := New(testConfig(srv.URL), logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.Handler == nil || a.Fleet == nil || a.Router == nil {
		t.Fatalf("New left a part unset: handler=%v fleet=%v router=%v", a.Handler, a.Fleet, a.Router)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := a.Start(ctx)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Start: background pulls did not finish")
	}

	pulled := fake.pulls()
	got := make(map[string]bool, len(pulled))
	for _, m := range pulled {
		got[m] = true
	}
	if !got["small:1b"] || !got["coder:1b"] {
		t.Errorf("pulled %v, want small:1b and coder:1b", pulled)
	}
	if got["llama3:8b"] {
		t.Errorf("pulled llama3:8b, which the backend already has: %v", pulled)
	}

	if code, _ := get(t, a.Handler, "/"); code != http.StatusOK {
		t.Errorf("GET /: status %d, want 200", code)
	}

	code, body := get(t, a.Handler, "/api/routes")
	if code != http.StatusOK {
		t.Errorf("GET /api/routes: status %d, want 200", code)
	}
	for _, name := range []string{"general", "code"} {
		if !strings.Contains(body, name) {
			t.Errorf("GET /api/routes: body %q does not mention route %q", body, name)
		}
	}

	code, body = get(t, a.Handler, "/v1/models")
	if code != http.StatusOK {
		t.Errorf("GET /v1/models: status %d, want 200", code)
	}
	if !strings.Contains(body, llm.AutoModel) {
		t.Errorf("GET /v1/models: body %q does not list %q", body, llm.AutoModel)
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

func TestNewRejectsUnknownDefaultRoute(t *testing.T) {
	fake := &fakeOllama{}
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

	cfg := testConfig(srv.URL)
	cfg.DefaultRoute = "nope"
	if _, err := New(cfg, log.New(io.Discard, "", 0)); err == nil {
		t.Fatal("New with unknown DefaultRoute: want error, got nil")
	}
}
