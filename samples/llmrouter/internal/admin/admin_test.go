package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"llmrouter/internal/llm"
)

// fakeBackend is a plain llm.Backend. It is not a llm.ModelManager.
type fakeBackend struct {
	name      string
	healthErr error
	models    []llm.Model
	listErr   error
}

func (b *fakeBackend) Name() string { return b.name }
func (b *fakeBackend) Kind() string { return llm.KindOllama }
func (b *fakeBackend) Health(context.Context) (string, error) {
	if b.healthErr != nil {
		return "", b.healthErr
	}
	return "1.2.3", nil
}
func (b *fakeBackend) ListModels(context.Context) ([]llm.Model, error) {
	return b.models, b.listErr
}
func (b *fakeBackend) Classify(context.Context, string, string, string, []string) (string, error) {
	return "", errors.New("not used")
}
func (b *fakeBackend) ChatCompletions(context.Context, []byte) (*http.Response, error) {
	return nil, errors.New("not used")
}

// fakeManager is a backend that also implements llm.ModelManager.
type fakeManager struct {
	fakeBackend
	pullErr   error
	deleteErr error
	pulled    []string
	deleted   []string
}

func (m *fakeManager) LoadedModels(context.Context) ([]llm.LoadedModel, error) {
	return []llm.LoadedModel{{Name: "llama3.2:1b", SizeVRAM: 10, ExpiresAt: time.Unix(0, 0).UTC()}}, nil
}
func (m *fakeManager) PullModel(_ context.Context, name string) error {
	m.pulled = append(m.pulled, name)
	return m.pullErr
}
func (m *fakeManager) DeleteModel(_ context.Context, name string) error {
	m.deleted = append(m.deleted, name)
	return m.deleteErr
}

// fakeRouter is a canned llm.Router.
type fakeRouter struct {
	routes    []llm.Route
	def       string
	decisions []llm.Decision
	recentN   []int
	routed    []llm.Message
	decision  llm.Decision
}

func (r *fakeRouter) Route(_ context.Context, msgs []llm.Message) llm.Decision {
	r.routed = append(r.routed, msgs...)
	return r.decision
}
func (r *fakeRouter) Routes() []llm.Route  { return r.routes }
func (r *fakeRouter) DefaultRoute() string { return r.def }
func (r *fakeRouter) Recent(n int) []llm.Decision {
	r.recentN = append(r.recentN, n)
	return r.decisions
}

type fixture struct {
	srv    *httptest.Server
	plain  *fakeBackend
	mgr    *fakeManager
	down   *fakeBackend
	router *fakeRouter
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	fx := &fixture{
		plain: &fakeBackend{name: "plain", models: []llm.Model{{Name: "m1", Size: 5}}},
		mgr:   &fakeManager{fakeBackend: fakeBackend{name: "ollama"}},
		down:  &fakeBackend{name: "down", healthErr: fmt.Errorf("refused: %w", llm.ErrUnavailable)},
		router: &fakeRouter{
			routes: []llm.Route{{Name: "code", Backend: "ollama", Model: "qwen"}},
			def:    "code",
		},
	}
	fleet := llm.NewFleet([]llm.Backend{fx.plain, fx.mgr, fx.down}, func(name string) bool {
		return name == "ollama"
	})
	fx.srv = httptest.NewServer(NewHandler(fleet, fx.router))
	t.Cleanup(fx.srv.Close)
	return fx
}

// do sends a request and returns the status, content type and body.
func do(t *testing.T, fx *fixture, method, path, body string) (int, string, []byte) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, fx.srv.URL+path, rd)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header.Get("Content-Type"), b
}

// errorText returns the "error" field of an error response.
func errorText(t *testing.T, b []byte) string {
	t.Helper()
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("error body %q is not JSON: %v", b, err)
	}
	return m["error"]
}

func TestBackends(t *testing.T) {
	fx := newFixture(t)
	code, ct, body := do(t, fx, "GET", "/api/backends", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, body %s", code, body)
	}
	if ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var got []llm.BackendStatus
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d backends, want 3: %s", len(got), body)
	}
	if got[0].Name != "plain" || !got[0].Reachable || got[0].Version != "1.2.3" || got[0].Managed {
		t.Errorf("plain = %+v", got[0])
	}
	if got[1].Name != "ollama" || !got[1].Reachable || !got[1].Managed || len(got[1].Loaded) != 1 {
		t.Errorf("ollama = %+v", got[1])
	}
	if got[2].Name != "down" || got[2].Reachable || got[2].Error == "" {
		t.Errorf("down = %+v", got[2])
	}
}

func TestListModels(t *testing.T) {
	fx := newFixture(t)
	code, ct, body := do(t, fx, "GET", "/api/backends/plain/models", "")
	if code != http.StatusOK || ct != "application/json" {
		t.Fatalf("status %d, Content-Type %q, body %s", code, ct, body)
	}
	var got []llm.Model
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "m1" {
		t.Errorf("models = %+v", got)
	}
}

func TestListModelsEmptyIsArray(t *testing.T) {
	fx := newFixture(t)
	fx.plain.models = nil
	code, _, body := do(t, fx, "GET", "/api/backends/plain/models", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("body = %q, want []", body)
	}
}

func TestListModelsErrors(t *testing.T) {
	fx := newFixture(t)
	fx.plain.listErr = fmt.Errorf("boom: %w", llm.ErrUnavailable)
	code, ct, body := do(t, fx, "GET", "/api/backends/plain/models", "")
	if code != http.StatusServiceUnavailable || ct != "application/json" {
		t.Errorf("unavailable: status %d, Content-Type %q", code, ct)
	}
	if !strings.Contains(errorText(t, body), "boom") {
		t.Errorf("error body = %s", body)
	}

	fx.plain.listErr = errors.New("other")
	code, _, _ = do(t, fx, "GET", "/api/backends/plain/models", "")
	if code != http.StatusInternalServerError {
		t.Errorf("other error: status %d, want 500", code)
	}

	code, _, body = do(t, fx, "GET", "/api/backends/nope/models", "")
	if code != http.StatusNotFound || !strings.Contains(errorText(t, body), "nope") {
		t.Errorf("unknown backend: status %d, body %s", code, body)
	}
}

func TestPull(t *testing.T) {
	fx := newFixture(t)
	code, ct, body := do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":"llama3.2:1b"}`)
	if code != http.StatusOK || ct != "application/json" {
		t.Fatalf("status %d, Content-Type %q, body %s", code, ct, body)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil || got["status"] != "success" {
		t.Errorf("body = %s", body)
	}
	if len(fx.mgr.pulled) != 1 || fx.mgr.pulled[0] != "llama3.2:1b" {
		t.Errorf("pulled = %v", fx.mgr.pulled)
	}
}

func TestPullErrors(t *testing.T) {
	fx := newFixture(t)

	// Plain backend is not a ModelManager: 400 even with a valid name.
	code, _, body := do(t, fx, "POST", "/api/backends/plain/models/pull", `{"name":"x"}`)
	if code != http.StatusBadRequest || !strings.Contains(errorText(t, body), "cannot manage") {
		t.Errorf("plain backend: status %d, body %s", code, body)
	}
	// Not a ModelManager wins over an empty name.
	code, _, _ = do(t, fx, "POST", "/api/backends/plain/models/pull", `{"name":""}`)
	if code != http.StatusBadRequest {
		t.Errorf("plain backend with empty name: status %d", code)
	}
	// Unknown backend is 404.
	code, _, _ = do(t, fx, "POST", "/api/backends/nope/models/pull", `{"name":"x"}`)
	if code != http.StatusNotFound {
		t.Errorf("unknown backend: status %d", code)
	}
	// Empty name is 400 and never reaches PullModel.
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":""}`)
	if code != http.StatusBadRequest {
		t.Errorf("empty name: status %d", code)
	}
	// Bad JSON and unknown fields are 400.
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":`)
	if code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d", code)
	}
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":"x","extra":1}`)
	if code != http.StatusBadRequest {
		t.Errorf("unknown field: status %d", code)
	}
	if len(fx.mgr.pulled) != 0 {
		t.Errorf("PullModel called for rejected requests: %v", fx.mgr.pulled)
	}

	// PullModel errors map through writeError.
	fx.mgr.pullErr = fmt.Errorf("bad: %w", llm.ErrNotFound)
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":"x"}`)
	if code != http.StatusNotFound {
		t.Errorf("PullModel ErrNotFound: status %d", code)
	}
	fx.mgr.pullErr = fmt.Errorf("down: %w", llm.ErrUnavailable)
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":"x"}`)
	if code != http.StatusServiceUnavailable {
		t.Errorf("PullModel ErrUnavailable: status %d", code)
	}
	fx.mgr.pullErr = errors.New("other")
	code, _, _ = do(t, fx, "POST", "/api/backends/ollama/models/pull", `{"name":"x"}`)
	if code != http.StatusInternalServerError {
		t.Errorf("PullModel other: status %d", code)
	}
}

func TestDelete(t *testing.T) {
	fx := newFixture(t)
	code, _, body := do(t, fx, "DELETE", "/api/backends/ollama/models/hf.co/user/model:Q4", "")
	if code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", code, body)
	}
	if len(body) != 0 {
		t.Errorf("204 has body %q", body)
	}
	if len(fx.mgr.deleted) != 1 || fx.mgr.deleted[0] != "hf.co/user/model:Q4" {
		t.Errorf("deleted = %v, want the name intact", fx.mgr.deleted)
	}

	code, _, _ = do(t, fx, "DELETE", "/api/backends/ollama/models/llama3.2:1b", "")
	if code != http.StatusNoContent {
		t.Errorf("simple name: status %d", code)
	}
}

func TestDeleteErrors(t *testing.T) {
	fx := newFixture(t)

	code, _, _ := do(t, fx, "DELETE", "/api/backends/plain/models/x", "")
	if code != http.StatusBadRequest {
		t.Errorf("plain backend: status %d, want 400", code)
	}
	code, _, _ = do(t, fx, "DELETE", "/api/backends/nope/models/x", "")
	if code != http.StatusNotFound {
		t.Errorf("unknown backend: status %d, want 404", code)
	}
	code, _, _ = do(t, fx, "DELETE", "/api/backends/ollama/models/", "")
	if code != http.StatusBadRequest {
		t.Errorf("empty name: status %d, want 400", code)
	}

	fx.mgr.deleteErr = fmt.Errorf("no such model: %w", llm.ErrNotFound)
	code, _, body := do(t, fx, "DELETE", "/api/backends/ollama/models/ghost:1b", "")
	if code != http.StatusNotFound {
		t.Errorf("unknown model: status %d, want 404", code)
	}
	if !strings.Contains(errorText(t, body), "no such model") {
		t.Errorf("error body = %s", body)
	}
}

func TestRoutes(t *testing.T) {
	fx := newFixture(t)
	code, ct, body := do(t, fx, "GET", "/api/routes", "")
	if code != http.StatusOK || ct != "application/json" {
		t.Fatalf("status %d, Content-Type %q", code, ct)
	}
	var got struct {
		Default string      `json:"default"`
		Routes  []llm.Route `json:"routes"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Default != "code" || len(got.Routes) != 1 || got.Routes[0].Model != "qwen" {
		t.Errorf("routes = %+v", got)
	}
}

func TestRoutesEmptyIsArray(t *testing.T) {
	fx := newFixture(t)
	fx.router.routes = nil
	_, _, body := do(t, fx, "GET", "/api/routes", "")
	if !strings.Contains(string(body), `"routes":[]`) {
		t.Errorf("body = %s, want routes encoded as []", body)
	}
}

func TestDecisions(t *testing.T) {
	fx := newFixture(t)
	fx.router.decisions = []llm.Decision{{Prompt: "hi", Route: "code", Backend: "ollama", Model: "qwen"}}

	code, ct, body := do(t, fx, "GET", "/api/decisions?n=20", "")
	if code != http.StatusOK || ct != "application/json" {
		t.Fatalf("status %d, Content-Type %q", code, ct)
	}
	if fx.router.recentN[len(fx.router.recentN)-1] != 20 {
		t.Errorf("Recent called with %v, want 20", fx.router.recentN)
	}
	var got []llm.Decision
	if err := json.Unmarshal(body, &got); err != nil || len(got) != 1 || got[0].Route != "code" {
		t.Errorf("body = %s", body)
	}
	if !strings.Contains(string(body), `"prompt":"hi"`) {
		t.Errorf("decision JSON tags not applied: %s", body)
	}

	// Default n is 50.
	do(t, fx, "GET", "/api/decisions", "")
	if fx.router.recentN[len(fx.router.recentN)-1] != 50 {
		t.Errorf("default n: Recent called with %v, want 50", fx.router.recentN)
	}

	// Zero and negative n pass through unchanged.
	do(t, fx, "GET", "/api/decisions?n=0", "")
	do(t, fx, "GET", "/api/decisions?n=-3", "")
	if got := fx.router.recentN[len(fx.router.recentN)-2:]; got[0] != 0 || got[1] != -3 {
		t.Errorf("Recent called with %v, want [0 -3] at the end", fx.router.recentN)
	}
}

func TestDecisionsEmptyIsArray(t *testing.T) {
	fx := newFixture(t)
	fx.router.decisions = nil
	code, _, body := do(t, fx, "GET", "/api/decisions", "")
	if code != http.StatusOK || strings.TrimSpace(string(body)) != "[]" {
		t.Errorf("status %d, body %q, want []", code, body)
	}
}

func TestDecisionsBadN(t *testing.T) {
	fx := newFixture(t)
	code, ct, body := do(t, fx, "GET", "/api/decisions?n=abc", "")
	if code != http.StatusBadRequest || ct != "application/json" {
		t.Errorf("status %d, Content-Type %q", code, ct)
	}
	if errorText(t, body) == "" {
		t.Errorf("no error text: %s", body)
	}
	if len(fx.router.recentN) != 0 {
		t.Errorf("Recent called for bad n: %v", fx.router.recentN)
	}
}

func TestRoute(t *testing.T) {
	fx := newFixture(t)
	fx.router.decision = llm.Decision{Prompt: "write code", Route: "code", Backend: "ollama", Model: "qwen", Fallback: false}

	code, ct, body := do(t, fx, "POST", "/api/route", `{"prompt":"write code"}`)
	if code != http.StatusOK || ct != "application/json" {
		t.Fatalf("status %d, Content-Type %q, body %s", code, ct, body)
	}
	var got llm.Decision
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Route != "code" || got.Model != "qwen" {
		t.Errorf("decision = %+v", got)
	}
	if len(fx.router.routed) != 1 || fx.router.routed[0] != (llm.Message{Role: "user", Content: "write code"}) {
		t.Errorf("routed = %+v", fx.router.routed)
	}
}

func TestRouteErrors(t *testing.T) {
	fx := newFixture(t)
	code, _, _ := do(t, fx, "POST", "/api/route", `{"prompt":""}`)
	if code != http.StatusBadRequest {
		t.Errorf("empty prompt: status %d, want 400", code)
	}
	code, _, _ = do(t, fx, "POST", "/api/route", `{"prompt":`)
	if code != http.StatusBadRequest {
		t.Errorf("bad JSON: status %d, want 400", code)
	}
	code, _, _ = do(t, fx, "POST", "/api/route", `{"text":"x"}`)
	if code != http.StatusBadRequest {
		t.Errorf("unknown field: status %d, want 400", code)
	}
	if len(fx.router.routed) != 0 {
		t.Errorf("Route called for rejected requests: %+v", fx.router.routed)
	}
}
