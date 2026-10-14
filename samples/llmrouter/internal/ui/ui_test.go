package ui

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"llmrouter/internal/llm"
)

// fakeBackend is a Backend that answers from fields the test sets.
type fakeBackend struct {
	name      string
	kind      string
	version   string
	healthErr error
	models    []llm.Model
	listErr   error
}

func (b *fakeBackend) Name() string { return b.name }
func (b *fakeBackend) Kind() string { return b.kind }

func (b *fakeBackend) Health(ctx context.Context) (string, error) {
	return b.version, b.healthErr
}

func (b *fakeBackend) ListModels(ctx context.Context) ([]llm.Model, error) {
	return b.models, b.listErr
}

func (b *fakeBackend) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	return "", errors.New("not used")
}

func (b *fakeBackend) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	return nil, errors.New("not used")
}

// fakeManager is a Backend that also manages models.
type fakeManager struct {
	*fakeBackend
	loaded    []llm.LoadedModel
	pullErr   error
	deleteErr error
	pulled    []string
	deleted   []string
}

func (m *fakeManager) LoadedModels(ctx context.Context) ([]llm.LoadedModel, error) {
	return m.loaded, nil
}

func (m *fakeManager) PullModel(ctx context.Context, name string) error {
	m.pulled = append(m.pulled, name)
	return m.pullErr
}

func (m *fakeManager) DeleteModel(ctx context.Context, name string) error {
	m.deleted = append(m.deleted, name)
	return m.deleteErr
}

// fakeRouter is a Router that returns next from Route and records the calls.
type fakeRouter struct {
	routes    []llm.Route
	def       string
	decisions []llm.Decision
	next      llm.Decision
	routed    [][]llm.Message
}

func (r *fakeRouter) Route(ctx context.Context, messages []llm.Message) llm.Decision {
	r.routed = append(r.routed, messages)
	return r.next
}

func (r *fakeRouter) Routes() []llm.Route  { return r.routes }
func (r *fakeRouter) DefaultRoute() string { return r.def }

func (r *fakeRouter) Recent(n int) []llm.Decision {
	if n > 0 && n < len(r.decisions) {
		return r.decisions[:n]
	}
	return r.decisions
}

type setup struct {
	plain   *fakeBackend
	manager *fakeManager
	router  *fakeRouter
	handler http.Handler
}

func newSetup(t *testing.T) setup {
	t.Helper()
	plain := &fakeBackend{
		name:    "llama",
		kind:    llm.KindOpenAI,
		version: "b4000",
		models:  []llm.Model{{Name: "qwen-coder", Size: 1024, ModifiedAt: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)}},
	}
	manager := &fakeManager{
		fakeBackend: &fakeBackend{
			name:    "ollama",
			kind:    llm.KindOllama,
			version: "0.5.1",
			models:  []llm.Model{{Name: "llama3", Size: 2048, ModifiedAt: time.Date(2026, 2, 3, 4, 5, 0, 0, time.UTC)}},
		},
		loaded: []llm.LoadedModel{{Name: "llama3", SizeVRAM: 4096}},
	}
	router := &fakeRouter{
		routes: []llm.Route{
			{Name: "code", Description: "programming questions", Backend: "llama", Model: "qwen-coder"},
			{Name: "chat", Description: "everything else", Backend: "ollama", Model: "llama3"},
		},
		def: "chat",
		decisions: []llm.Decision{{
			At:      time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC),
			Prompt:  "hello decision",
			Route:   "chat",
			Backend: "ollama",
			Model:   "llama3",
			Took:    15 * time.Millisecond,
		}},
		next: llm.Decision{
			Prompt:  "write a sort",
			Route:   "code",
			Backend: "llama",
			Model:   "qwen-coder",
			Took:    time.Millisecond,
		},
	}
	f := llm.NewFleet([]llm.Backend{plain, manager}, func(name string) bool { return name == "ollama" })
	return setup{plain: plain, manager: manager, router: router, handler: NewHandler(f, router)}
}

func (s setup) get(t *testing.T) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec.Code, rec.Body.String()
}

func (s setup) post(t *testing.T, path string, form url.Values) (int, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	s.handler.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func mustContain(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
}

func TestPageListsBackendsRoutesModelsAndDecisions(t *testing.T) {
	s := newSetup(t)
	code, body := s.get(t)
	if code != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", code)
	}
	mustContain(t, body,
		`id="backends"`, `id="routes"`, `id="models"`, `id="try"`, `id="decisions"`,
		"llama", "ollama", llm.KindOpenAI, llm.KindOllama, "b4000", "0.5.1",
		// Managed: only "ollama" is managed by the fleet.
		"Loaded models",
		"code", "programming questions", "qwen-coder",
		"chat", "everything else", "llama3",
		"(default)",
		"hello decision", "05:06:07",
	)
	if n := strings.Count(body, "(default)"); n != 1 {
		t.Errorf("(default) appears %d times, want 1", n)
	}
	mustContain(t, body, `chat <span class="muted">(default)</span>`)
}

func TestPageHasChat(t *testing.T) {
	s := newSetup(t)
	_, body := s.get(t)
	mustContain(t, body, `id="chat"`, `id="chat-log"`, `id="chat-form"`, `id="chat-model"`,
		"/v1/chat/completions", "/api/routes", `option value="auto"`)
	chat := strings.Index(body, `id="chat"`)
	backends := strings.Index(body, `id="backends"`)
	backendsEnd := backends + strings.Index(body[backends:], "</section>")
	decisions := strings.Index(body, `id="decisions"`)
	if !(backendsEnd < chat && chat < decisions) {
		t.Errorf("chat section at %d, want between the end of backends (%d) and decisions (%d)", chat, backendsEnd, decisions)
	}
}

func TestPageShowsManagerControlsOnlyForModelManager(t *testing.T) {
	s := newSetup(t)
	_, body := s.get(t)
	if n := strings.Count(body, `action="/models/delete"`); n != 1 {
		t.Errorf("delete forms = %d, want 1 (only for the managing backend)", n)
	}
	if n := strings.Count(body, `action="/models/pull"`); n != 1 {
		t.Errorf("pull forms = %d, want 1 (only for the managing backend)", n)
	}
	if !strings.Contains(body, `name="backend" value="ollama"`) {
		t.Errorf("delete/pull forms do not name the managing backend")
	}
	if strings.Contains(body, `name="backend" value="llama"`) {
		t.Errorf("plain backend has model management controls")
	}
}

func TestPageShowsListModelsError(t *testing.T) {
	s := newSetup(t)
	s.plain.listErr = errors.New("list exploded")
	_, body := s.get(t)
	mustContain(t, body, "list exploded")
}

func TestTryShowsRouteDecision(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/try", url.Values{"prompt": {"write a sort"}})
	if code != http.StatusOK {
		t.Fatalf("POST /try status = %d, want 200", code)
	}
	if len(s.router.routed) != 1 {
		t.Fatalf("Route calls = %d, want 1", len(s.router.routed))
	}
	msgs := s.router.routed[0]
	if len(msgs) != 1 || msgs[0].Role != "user" || msgs[0].Content != "write a sort" {
		t.Errorf("Route got %+v, want one user message with the prompt", msgs)
	}
	mustContain(t, body, `id="try"`, "Route", "code", "qwen-coder", "llama")
}

func TestTryEmptyPromptShowsError(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/try", url.Values{"prompt": {""}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(s.router.routed) != 0 {
		t.Errorf("Route called %d times for an empty prompt, want 0", len(s.router.routed))
	}
	mustContain(t, body, "Enter a prompt.")
}

func TestTryEscapesHTML(t *testing.T) {
	s := newSetup(t)
	payload := "<script>alert(1)</script>"
	_, body := s.post(t, "/try", url.Values{"prompt": {payload}})
	if strings.Contains(body, payload) {
		t.Errorf("prompt was not escaped in the page")
	}
	mustContain(t, body, "&lt;script&gt;")
}

func TestPullCallsManagerAndShowsMessage(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/models/pull", url.Values{"backend": {"ollama"}, "name": {"phi4"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(s.manager.pulled) != 1 || s.manager.pulled[0] != "phi4" {
		t.Errorf("pulled = %v, want [phi4]", s.manager.pulled)
	}
	mustContain(t, body, `id="models"`, "Pulled phi4.")
}

func TestPullErrorIsShown(t *testing.T) {
	s := newSetup(t)
	s.manager.pullErr = errors.New("pull failed hard")
	code, body := s.post(t, "/models/pull", url.Values{"backend": {"ollama"}, "name": {"phi4"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	mustContain(t, body, "pull failed hard")
	if strings.Contains(body, "Pulled phi4.") {
		t.Errorf("success message shown after a failed pull")
	}
}

func TestDeleteCallsManagerAndShowsMessage(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/models/delete", url.Values{"backend": {"ollama"}, "name": {"llama3"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if len(s.manager.deleted) != 1 || s.manager.deleted[0] != "llama3" {
		t.Errorf("deleted = %v, want [llama3]", s.manager.deleted)
	}
	mustContain(t, body, `id="models"`, "Deleted llama3.")
}

func TestDeleteOnPlainBackendShowsError(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/models/delete", url.Values{"backend": {"llama"}, "name": {"qwen-coder"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	if strings.Contains(body, "Deleted qwen-coder.") {
		t.Errorf("success shown for a backend that cannot delete")
	}
	mustContain(t, body, "does not manage models")
}

func TestDeleteUnknownBackendShowsError(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/models/delete", url.Values{"backend": {"nope"}, "name": {"x"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	mustContain(t, body, "unknown backend")
}

func TestPullUnknownBackendShowsError(t *testing.T) {
	s := newSetup(t)
	code, body := s.post(t, "/models/pull", url.Values{"backend": {"nope"}, "name": {"x"}})
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	mustContain(t, body, "unknown backend")
	if len(s.manager.pulled) != 0 {
		t.Errorf("pull reached a manager for an unknown backend")
	}
}
