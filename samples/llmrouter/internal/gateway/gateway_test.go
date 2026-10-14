package gateway

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
	"testing"
	"time"

	"llmrouter/internal/llm"
)

// fakeBackend is a llm.Backend that records what it was sent and returns
// a canned response.
type fakeBackend struct {
	name    string
	models  []llm.Model
	listErr error
	chatErr error
	resp    *http.Response // returned by ChatCompletions; nil means 200 {"ok":true}
	gotBody []byte
	calls   int
}

func (b *fakeBackend) Name() string { return b.name }
func (b *fakeBackend) Kind() string { return llm.KindOpenAI }

func (b *fakeBackend) Health(ctx context.Context) (string, error) { return "", nil }

func (b *fakeBackend) ListModels(ctx context.Context) ([]llm.Model, error) {
	return b.models, b.listErr
}

func (b *fakeBackend) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	return "", llm.ErrUnsupported
}

func (b *fakeBackend) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	b.calls++
	b.gotBody = body
	if b.chatErr != nil {
		return nil, b.chatErr
	}
	if b.resp != nil {
		return b.resp, nil
	}
	return jsonResponse(http.StatusOK, `{"ok":true}`), nil
}

// fakeRouter is a llm.Router with a fixed decision.
type fakeRouter struct {
	routes   []llm.Route
	def      string
	decision llm.Decision
	routed   int
	gotMsgs  []llm.Message
}

func (r *fakeRouter) Route(ctx context.Context, messages []llm.Message) llm.Decision {
	r.routed++
	r.gotMsgs = messages
	return r.decision
}

func (r *fakeRouter) Routes() []llm.Route  { return r.routes }
func (r *fakeRouter) DefaultRoute() string { return r.def }
func (r *fakeRouter) Recent(n int) []llm.Decision {
	return nil
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// newServer serves the handler over the given backends and router.
func newServer(t *testing.T, r llm.Router, backends ...*fakeBackend) *httptest.Server {
	t.Helper()
	bs := make([]llm.Backend, 0, len(backends))
	for _, b := range backends {
		bs = append(bs, b)
	}
	srv := httptest.NewServer(NewHandler(llm.NewFleet(bs, nil), r, log.New(io.Discard, "", 0)))
	t.Cleanup(srv.Close)
	return srv
}

// post sends body to /v1/chat/completions and returns the response with its body read.
func post(t *testing.T, srv *httptest.Server, body string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp, string(raw)
}

// chatJSON builds a request body with one user message and extra fields.
func chatJSON(t *testing.T, fields map[string]any) string {
	t.Helper()
	if _, ok := fields["messages"]; !ok {
		fields["messages"] = []map[string]any{{"role": "user", "content": "hi"}}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// decodeBody decodes a JSON object that a backend received.
func decodeBody(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("backend body is not a JSON object: %v: %s", err, raw)
	}
	return m
}

// errorEnvelope decodes an OpenAI-style error body and checks its content type.
func errorEnvelope(t *testing.T, resp *http.Response, raw string) (msg, typ string) {
	t.Helper()
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("error Content-Type = %q, want application/json", ct)
	}
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("error body is not JSON: %v: %s", err, raw)
	}
	return env.Error.Message, env.Error.Type
}

func TestModelChoice(t *testing.T) {
	routes := []llm.Route{
		{Name: "code", Backend: "ollama", Model: "qwen-coder"},
		{Name: "general", Backend: "openai", Model: "llama-8b"},
	}
	cases := []struct {
		name        string
		model       string
		decision    llm.Decision
		wantBackend string
		wantModel   string
		wantRoute   string // "" means the header must be absent
		routed      bool   // whether the router must be asked
	}{
		{
			name:        "auto uses the router",
			model:       "auto",
			decision:    llm.Decision{Route: "code", Backend: "ollama", Model: "qwen"},
			wantBackend: "ollama", wantModel: "qwen", wantRoute: "code", routed: true,
		},
		{
			name:        "empty model uses the router",
			model:       "",
			decision:    llm.Decision{Route: "general", Backend: "openai", Model: "llama-8b"},
			wantBackend: "openai", wantModel: "llama-8b", wantRoute: "general", routed: true,
		},
		{
			name:        "route name uses that route",
			model:       "code",
			wantBackend: "ollama", wantModel: "qwen-coder", wantRoute: "code",
		},
		{
			name:        "backend prefix uses that backend with no route",
			model:       "openai/mistral",
			wantBackend: "openai", wantModel: "mistral",
		},
		{
			name:        "backend prefix splits at the first slash",
			model:       "ollama/library/llama3",
			wantBackend: "ollama", wantModel: "library/llama3",
		},
		{
			name:        "unknown name goes to the default route's backend with no route",
			model:       "gpt-x",
			wantBackend: "openai", wantModel: "gpt-x",
		},
		{
			name:        "unknown prefix goes to the default route's backend unchanged",
			model:       "ghost/x",
			wantBackend: "openai", wantModel: "ghost/x",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := &fakeBackend{name: "ollama"}
			openai := &fakeBackend{name: "openai"}
			router := &fakeRouter{routes: routes, def: "general", decision: tc.decision}
			srv := newServer(t, router, ollama, openai)

			resp, raw := post(t, srv, chatJSON(t, map[string]any{"model": tc.model}))
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body %s", resp.StatusCode, raw)
			}

			backends := map[string]*fakeBackend{"ollama": ollama, "openai": openai}
			want := backends[tc.wantBackend]
			if want.calls != 1 {
				t.Fatalf("%s got %d calls, want 1", tc.wantBackend, want.calls)
			}
			for name, b := range backends {
				if name != tc.wantBackend && b.calls != 0 {
					t.Errorf("%s got %d calls, want 0", name, b.calls)
				}
			}
			if got := decodeBody(t, want.gotBody)["model"]; got != tc.wantModel {
				t.Errorf("backend received model %v, want %q", got, tc.wantModel)
			}
			if got := resp.Header.Get("X-Llmrouter-Backend"); got != tc.wantBackend {
				t.Errorf("X-Llmrouter-Backend = %q, want %q", got, tc.wantBackend)
			}
			if got := resp.Header.Get("X-Llmrouter-Model"); got != tc.wantModel {
				t.Errorf("X-Llmrouter-Model = %q, want %q", got, tc.wantModel)
			}
			if got := resp.Header.Get("X-Llmrouter-Route"); got != tc.wantRoute {
				t.Errorf("X-Llmrouter-Route = %q, want %q", got, tc.wantRoute)
			}
			if tc.routed {
				if router.routed != 1 {
					t.Errorf("router asked %d times, want 1", router.routed)
				}
			} else if router.routed != 0 {
				t.Errorf("router asked %d times, want 0", router.routed)
			}
		})
	}
}

func TestRouterGetsMessages(t *testing.T) {
	ollama := &fakeBackend{name: "ollama"}
	router := &fakeRouter{def: "x", decision: llm.Decision{Route: "x", Backend: "ollama", Model: "m"}}
	srv := newServer(t, router, ollama)

	body := chatJSON(t, map[string]any{"model": "auto", "messages": []map[string]any{
		{"role": "system", "content": "be brief"},
		{"role": "user", "content": "write a sort"},
	}})
	post(t, srv, body)
	want := []llm.Message{{Role: "system", Content: "be brief"}, {Role: "user", Content: "write a sort"}}
	if len(router.gotMsgs) != len(want) {
		t.Fatalf("router got %v, want %v", router.gotMsgs, want)
	}
	for i := range want {
		if router.gotMsgs[i] != want[i] {
			t.Errorf("message %d = %+v, want %+v", i, router.gotMsgs[i], want[i])
		}
	}
}

func TestUnroutableTargetsAre500(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		router *fakeRouter
	}{
		{
			name:  "router picks a backend not in the fleet",
			model: "auto",
			router: &fakeRouter{def: "general", decision: llm.Decision{
				Route: "broken", Backend: "missing", Model: "m"}},
		},
		{
			name:   "default route is not configured",
			model:  "gpt-x",
			router: &fakeRouter{def: "nope", routes: []llm.Route{{Name: "code", Backend: "ollama", Model: "m"}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := &fakeBackend{name: "ollama"}
			srv := newServer(t, tc.router, ollama)
			resp, raw := post(t, srv, chatJSON(t, map[string]any{"model": tc.model}))
			if resp.StatusCode != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500; body %s", resp.StatusCode, raw)
			}
			if _, typ := errorEnvelope(t, resp, raw); typ != "server_error" {
				t.Errorf("error type = %q, want server_error", typ)
			}
			if ollama.calls != 0 {
				t.Errorf("ollama got %d calls, want 0", ollama.calls)
			}
		})
	}
}

func TestContentForms(t *testing.T) {
	cases := []struct {
		name    string
		content string // raw JSON for the message's content
		want    string
	}{
		{"string", `"hello"`, "hello"},
		{"null is empty", `null`, ""},
		{"array joins text parts and skips others", `[{"type":"text","text":"a"},{"type":"image_url","image_url":{"url":"x"}},{"type":"text","text":"b"}]`, "a\nb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := &fakeBackend{name: "ollama"}
			router := &fakeRouter{def: "x", decision: llm.Decision{Route: "x", Backend: "ollama", Model: "m"}}
			srv := newServer(t, router, ollama)
			body := `{"model":"auto","messages":[{"role":"user","content":` + tc.content + `}]}`
			resp, raw := post(t, srv, body)
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, body %s", resp.StatusCode, raw)
			}
			if len(router.gotMsgs) != 1 || router.gotMsgs[0].Content != tc.want {
				t.Errorf("router got %+v, want one message with content %q", router.gotMsgs, tc.want)
			}
		})
	}
}

func TestBadRequests(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"invalid JSON", `{"model":`},
		{"body is not an object", `[]`},
		{"messages missing", `{"model":"auto"}`},
		{"messages not an array", `{"model":"auto","messages":"hi"}`},
		{"content is a number", `{"model":"auto","messages":[{"role":"user","content":5}]}`},
		{"content is an object", `{"model":"auto","messages":[{"role":"user","content":{"x":1}}]}`},
		{"model is not a string", `{"model":5,"messages":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := &fakeBackend{name: "ollama"}
			router := &fakeRouter{def: "x", decision: llm.Decision{Route: "x", Backend: "ollama", Model: "m"}}
			srv := newServer(t, router, ollama)
			resp, raw := post(t, srv, tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body %s", resp.StatusCode, raw)
			}
			if _, typ := errorEnvelope(t, resp, raw); typ != "invalid_request_error" {
				t.Errorf("error type = %q, want invalid_request_error", typ)
			}
			if ollama.calls != 0 || router.routed != 0 {
				t.Errorf("backend calls = %d, router calls = %d; want 0 and 0", ollama.calls, router.routed)
			}
		})
	}
}

func TestBackendErrors(t *testing.T) {
	cases := []struct {
		name       string
		chatErr    error
		wantStatus int
	}{
		{"unavailable", fmt.Errorf("dial ollama: %w", llm.ErrUnavailable), http.StatusServiceUnavailable},
		{"other error", errors.New("boom"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ollama := &fakeBackend{name: "ollama", chatErr: tc.chatErr}
			srv := newServer(t, &fakeRouter{}, ollama)
			resp, raw := post(t, srv, chatJSON(t, map[string]any{"model": "ollama/llama3"}))
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body %s", resp.StatusCode, tc.wantStatus, raw)
			}
			if _, typ := errorEnvelope(t, resp, raw); typ != "server_error" {
				t.Errorf("error type = %q, want server_error", typ)
			}
		})
	}
}

func TestBackendStatusAndBodyPassThrough(t *testing.T) {
	ollama := &fakeBackend{name: "ollama", resp: &http.Response{
		StatusCode: http.StatusNotFound,
		Header: http.Header{
			"Content-Type":  {"application/json"},
			"Cache-Control": {"no-store"},
		},
		Body: io.NopCloser(strings.NewReader(`{"error":{"message":"model not found"}}`)),
	}}
	srv := newServer(t, &fakeRouter{}, ollama)
	resp, raw := post(t, srv, chatJSON(t, map[string]any{"model": "ollama/nope"}))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if raw != `{"error":{"message":"model not found"}}` {
		t.Errorf("body = %q, want the backend's body unchanged", raw)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
}

func TestOtherFieldsReachBackendUnchanged(t *testing.T) {
	ollama := &fakeBackend{name: "ollama"}
	srv := newServer(t, &fakeRouter{}, ollama)
	body := chatJSON(t, map[string]any{
		"model":       "ollama/llama3",
		"temperature": 0.2,
		"stream":      true,
		"max_tokens":  5,
	})
	if resp, raw := post(t, srv, body); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body %s", resp.StatusCode, raw)
	}
	got := decodeBody(t, ollama.gotBody)
	if got["model"] != "llama3" {
		t.Errorf("model = %v, want llama3", got["model"])
	}
	if got["temperature"] != 0.2 {
		t.Errorf("temperature = %v, want 0.2", got["temperature"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	if got["max_tokens"] != float64(5) {
		t.Errorf("max_tokens = %v, want 5", got["max_tokens"])
	}
	msgs, ok := got["messages"].([]any)
	if !ok || len(msgs) != 1 {
		t.Errorf("messages = %v, want one message", got["messages"])
	}
}

func TestStreamedResponseReachesClientAsItArrives(t *testing.T) {
	pr, pw := io.Pipe()
	release := make(chan struct{})
	go func() {
		defer pw.Close()
		fmt.Fprint(pw, "data: {\"n\":1}\n\n")
		<-release
		fmt.Fprint(pw, "data: {\"n\":2}\n\n")
	}()
	ollama := &fakeBackend{name: "ollama", resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"text/event-stream"}},
		Body:       pr,
	}}
	srv := newServer(t, &fakeRouter{}, ollama)

	// Post returns once the headers arrive, which happens on the first flush.
	type result struct {
		resp *http.Response
		err  error
	}
	posted := make(chan result, 1)
	go func() {
		resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json",
			strings.NewReader(chatJSON(t, map[string]any{"model": "ollama/llama3", "stream": true})))
		posted <- result{resp, err}
	}()
	var resp *http.Response
	select {
	case r := <-posted:
		if r.err != nil {
			t.Fatal(r.err)
		}
		resp = r.resp
	case <-time.After(5 * time.Second):
		t.Fatal("response headers did not arrive before the backend finished streaming")
	}
	defer resp.Body.Close()

	reader := bufio.NewReader(resp.Body)
	lines := make(chan string, 1)
	go func() {
		line, _ := reader.ReadString('\n')
		lines <- line
	}()
	select {
	case line := <-lines:
		if line != "data: {\"n\":1}\n" {
			t.Fatalf("first line = %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first streamed line did not reach the client before the backend wrote the second")
	}
	close(release)

	rest, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rest), "data: {\"n\":2}\n\n") {
		t.Errorf("rest of stream = %q, want the second event", rest)
	}
}

func TestListModels(t *testing.T) {
	ollama := &fakeBackend{name: "ollama", models: []llm.Model{{Name: "llama3"}, {Name: "qwen"}}}
	broken := &fakeBackend{name: "broken", listErr: errors.New("down")}
	openai := &fakeBackend{name: "openai", models: []llm.Model{{Name: "mistral"}}}
	router := &fakeRouter{routes: []llm.Route{{Name: "code"}, {Name: "general"}}, def: "general"}
	srv := newServer(t, router, ollama, broken, openai)

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		Object string `json:"object"`
		Data   []struct {
			ID      string `json:"id"`
			Object  string `json:"object"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Object != "list" {
		t.Errorf("object = %q, want list", out.Object)
	}
	want := []struct{ id, owner string }{
		{"auto", "llmrouter"},
		{"code", "llmrouter"},
		{"general", "llmrouter"},
		{"ollama/llama3", "ollama"},
		{"ollama/qwen", "ollama"},
		{"openai/mistral", "openai"},
	}
	if len(out.Data) != len(want) {
		t.Fatalf("got %d models %+v, want %d", len(out.Data), out.Data, len(want))
	}
	for i, w := range want {
		got := out.Data[i]
		if got.ID != w.id || got.OwnedBy != w.owner || got.Object != "model" {
			t.Errorf("entry %d = %+v, want id %q owned by %q", i, got, w.id, w.owner)
		}
	}
}
