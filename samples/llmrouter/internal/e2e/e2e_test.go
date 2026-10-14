package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"llmrouter/internal/app"
	"llmrouter/internal/config"
	"llmrouter/internal/llm"
)

// recorder holds what the fake backends received. Its fields are guarded by mu.
type recorder struct {
	mu           sync.Mutex
	pulls        []string
	ollamaModels []string
	bonsaiModels []string
}

func (r *recorder) addPull(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pulls = append(r.pulls, name)
}

func (r *recorder) addOllamaModel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ollamaModels = append(r.ollamaModels, name)
}

func (r *recorder) addBonsaiModel(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.bonsaiModels = append(r.bonsaiModels, name)
}

func (r *recorder) snapshot() (pulls, ollama, bonsai []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.pulls...), append([]string(nil), r.ollamaModels...), append([]string(nil), r.bonsaiModels...)
}

// lastOf returns the last element of s, or "" when s is empty.
func lastOf(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[len(s)-1]
}

const (
	sseBody = "data: {\"choices\":[{\"delta\":{\"content\":\"o\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"k\"}}]}\n\n" +
		"data: [DONE]\n\n"
	listBody = `{"object":"list","data":[{"id":"bonsai-27b"}]}`
)

// completionBody is an OpenAI chat-completions reply naming model.
func completionBody(t *testing.T, model string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"id":     "x",
		"object": "chat.completion",
		"model":  model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": "ok"},
			"finish_reason": "stop",
		}},
	})
	if err != nil {
		t.Fatalf("encode completion: %v", err)
	}
	return string(b)
}

// writeJSON answers 200 with the given JSON text.
func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// fakeOllama answers the Ollama endpoints the router uses.
func fakeOllama(t *testing.T, rec *recorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/version", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"version":"fake"}`)
	})
	mux.HandleFunc("GET /api/tags", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"models":[{"name":"llama3:8b"}]}`)
	})
	mux.HandleFunc("GET /api/ps", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, `{"models":[]}`)
	})
	mux.HandleFunc("POST /api/pull", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, `{"error":"bad body"}`)
			return
		}
		rec.addPull(in.Model)
		writeJSON(w, http.StatusOK, `{"status":"success"}`)
	})
	mux.HandleFunc("DELETE /api/delete", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, `{"error":"bad body"}`)
			return
		}
		if in.Model == "llama3:8b" {
			writeJSON(w, http.StatusOK, `{"status":"success"}`)
			return
		}
		writeJSON(w, http.StatusNotFound, `{"error":"not found"}`)
	})
	mux.HandleFunc("POST /api/chat", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil || len(in.Messages) < 2 {
			writeJSON(w, http.StatusBadRequest, `{"error":"need a system and a user message"}`)
			return
		}
		prompt := in.Messages[1].Content
		route := "general"
		switch {
		case strings.Contains(prompt, "func"):
			route = "code"
		case strings.Contains(prompt, "prove"):
			route = "reason"
		}
		writeJSON(w, http.StatusOK, fmt.Sprintf(`{"message":{"role":"assistant","content":"{\"route\":\"%s\"}"},"done":true}`, route))
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, `{"error":"bad body"}`)
			return
		}
		rec.addOllamaModel(in.Model)
		if in.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, sseBody)
			return
		}
		writeJSON(w, http.StatusOK, completionBody(t, in.Model))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// fakeBonsai answers the OpenAI-compatible endpoints the router uses.
func fakeBonsai(t *testing.T, rec *recorder) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/models", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, listBody)
	})
	mux.HandleFunc("POST /v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, `{"error":"bad body"}`)
			return
		}
		rec.addBonsaiModel(in.Model)
		writeJSON(w, http.StatusOK, completionBody(t, in.Model))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// chat posts body to base's chat-completions endpoint and returns the
// response with its body read.
func chat(t *testing.T, base, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(base+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST chat completions: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read chat completions body: %v", err)
	}
	return resp, string(data)
}

// get fetches url and returns the status and body.
func get(t *testing.T, url string) (int, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s body: %v", url, err)
	}
	return resp.StatusCode, string(data)
}

// deleteModel sends DELETE to url and returns the status.
func deleteModel(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequest(http.MethodDelete, url, nil)
	if err != nil {
		t.Fatalf("new DELETE %s: %v", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", url, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

func TestEndToEnd(t *testing.T) {
	rec := &recorder{}
	ollamaSrv := fakeOllama(t, rec)
	bonsaiSrv := fakeBonsai(t, rec)

	cfg := config.Config{
		Addr: "127.0.0.1:0",
		Backends: []config.Backend{
			{Name: "ollama", Kind: llm.KindOllama, URL: ollamaSrv.URL, PullMissing: true},
			{Name: "bonsai", Kind: llm.KindOpenAI, URL: bonsaiSrv.URL},
		},
		Intent: config.Intent{Backend: "ollama", Model: "llama3.2:1b"},
		Routes: []llm.Route{
			{Name: "general", Backend: "ollama", Model: "llama3:8b"},
			{Name: "code", Backend: "ollama", Model: "qwen2.5-coder:1.5b"},
			{Name: "reason", Backend: "bonsai", Model: "bonsai-27b"},
		},
		DefaultRoute: "general",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config is invalid: %v", err)
	}

	a, err := app.New(cfg, nil)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	started := a.Start(ctx)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatalf("Start did not finish: %v", ctx.Err())
	}

	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	base := srv.URL

	// 1. The pulls: the intent model, then the code route's model. llama3:8b is already installed.
	pulls, _, _ := rec.snapshot()
	if want := []string{"llama3.2:1b", "qwen2.5-coder:1.5b"}; !equal(pulls, want) {
		t.Fatalf("pulls = %v, want %v", pulls, want)
	}

	// 2. A code prompt goes to the code route on ollama.
	resp, body := chat(t, base, `{"model":"auto","messages":[{"role":"user","content":"write a func that adds"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("code prompt: status %d, body %s", resp.StatusCode, body)
	}
	wantHeader(t, resp, "X-Llmrouter-Route", "code")
	wantHeader(t, resp, "X-Llmrouter-Backend", "ollama")
	wantHeader(t, resp, "X-Llmrouter-Model", "qwen2.5-coder:1.5b")
	if _, ollamaModels, _ := rec.snapshot(); lastOf(ollamaModels) != "qwen2.5-coder:1.5b" {
		t.Fatalf("ollama last model = %q, want qwen2.5-coder:1.5b (all: %v)", lastOf(ollamaModels), ollamaModels)
	}

	// 3. A reasoning prompt goes to the reason route on bonsai.
	resp, body = chat(t, base, `{"model":"auto","messages":[{"role":"user","content":"prove there are infinitely many primes"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reason prompt: status %d, body %s", resp.StatusCode, body)
	}
	wantHeader(t, resp, "X-Llmrouter-Route", "reason")
	wantHeader(t, resp, "X-Llmrouter-Backend", "bonsai")
	wantHeader(t, resp, "X-Llmrouter-Model", "bonsai-27b")
	if _, _, bonsaiModels := rec.snapshot(); lastOf(bonsaiModels) != "bonsai-27b" {
		t.Fatalf("bonsai last model = %q, want bonsai-27b (all: %v)", lastOf(bonsaiModels), bonsaiModels)
	}

	// 4. An explicit backend/model name bypasses routing.
	resp, body = chat(t, base, `{"model":"bonsai/other","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("explicit model: status %d, body %s", resp.StatusCode, body)
	}
	if got := resp.Header.Get("X-Llmrouter-Route"); got != "" {
		t.Fatalf("explicit model: X-Llmrouter-Route = %q, want none", got)
	}
	if _, _, bonsaiModels := rec.snapshot(); lastOf(bonsaiModels) != "other" {
		t.Fatalf("bonsai last model = %q, want other (all: %v)", lastOf(bonsaiModels), bonsaiModels)
	}

	// 5. A streamed general prompt is passed through as event-stream.
	resp, body = chat(t, base, `{"model":"auto","stream":true,"messages":[{"role":"user","content":"hello"}]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stream: status %d, body %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("stream: Content-Type = %q, want text/event-stream", ct)
	}
	if body != sseBody {
		t.Fatalf("stream: body = %q, want %q", body, sseBody)
	}

	// 6. The model list names auto, each route and each backend's models.
	status, body := get(t, base+"/v1/models")
	if status != http.StatusOK {
		t.Fatalf("GET /v1/models: status %d, body %s", status, body)
	}
	var models struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &models); err != nil {
		t.Fatalf("GET /v1/models: decode: %v (body %s)", err, body)
	}
	ids := make([]string, 0, len(models.Data))
	for _, m := range models.Data {
		ids = append(ids, m.ID)
	}
	for _, want := range []string{"auto", "code", "ollama/llama3:8b", "bonsai/bonsai-27b"} {
		if !contains(ids, want) {
			t.Errorf("GET /v1/models: %q missing from %v", want, ids)
		}
	}

	// 7. Three decisions, newest first, none a fallback.
	status, body = get(t, base+"/api/decisions")
	if status != http.StatusOK {
		t.Fatalf("GET /api/decisions: status %d, body %s", status, body)
	}
	var decisions []struct {
		Route    string `json:"route"`
		Fallback bool   `json:"fallback"`
	}
	if err := json.Unmarshal([]byte(body), &decisions); err != nil {
		t.Fatalf("GET /api/decisions: decode: %v (body %s)", err, body)
	}
	gotRoutes := make([]string, 0, len(decisions))
	for _, d := range decisions {
		gotRoutes = append(gotRoutes, d.Route)
		if d.Fallback {
			t.Errorf("decision for route %q is a fallback", d.Route)
		}
	}
	if want := []string{"general", "reason", "code"}; !equal(gotRoutes, want) {
		t.Fatalf("decision routes = %v, want %v", gotRoutes, want)
	}

	// 8. Both backends are reachable and not managed.
	status, body = get(t, base+"/api/backends")
	if status != http.StatusOK {
		t.Fatalf("GET /api/backends: status %d, body %s", status, body)
	}
	var backends []struct {
		Name      string `json:"name"`
		Reachable bool   `json:"reachable"`
		Managed   bool   `json:"managed"`
	}
	if err := json.Unmarshal([]byte(body), &backends); err != nil {
		t.Fatalf("GET /api/backends: decode: %v (body %s)", err, body)
	}
	if len(backends) != 2 {
		t.Fatalf("GET /api/backends: %d entries, want 2 (body %s)", len(backends), body)
	}
	for _, b := range backends {
		if !b.Reachable {
			t.Errorf("backend %s: not reachable", b.Name)
		}
		if b.Managed {
			t.Errorf("backend %s: managed, want not managed", b.Name)
		}
	}

	// 9. The page shows the code route's model and the reason route.
	status, body = get(t, base+"/")
	if status != http.StatusOK {
		t.Fatalf("GET /: status %d", status)
	}
	for _, want := range []string{"qwen2.5-coder:1.5b", "reason"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET /: page does not contain %q", want)
		}
	}

	// 10. Model deletion: 204 for an installed model, 404 for an unknown one, 400 for a backend that cannot manage models.
	if code := deleteModel(t, base+"/api/backends/ollama/models/llama3:8b"); code != http.StatusNoContent {
		t.Errorf("DELETE llama3:8b: status %d, want 204", code)
	}
	if code := deleteModel(t, base+"/api/backends/ollama/models/nope:1"); code != http.StatusNotFound {
		t.Errorf("DELETE nope:1: status %d, want 404", code)
	}
	if code := deleteModel(t, base+"/api/backends/bonsai/models/bonsai-27b"); code != http.StatusBadRequest {
		t.Errorf("DELETE bonsai-27b: status %d, want 400", code)
	}
}

func wantHeader(t *testing.T, resp *http.Response, name, want string) {
	t.Helper()
	if got := resp.Header.Get(name); got != want {
		t.Errorf("header %s = %q, want %q", name, got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
