// Package e2e_test runs the whole llmchat server, built by app.New, against a
// fake llmrouter, and checks that chat, streaming, saving and restarting work
// together. Nothing here reaches a real router, Ollama or a model.
package e2e_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"

	"llmchat/internal/app"
	"llmchat/internal/apptest"
	"llmchat/internal/chat"
)

// routerRequest is what the fake router decoded from one completion request.
type routerRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	HasTools bool   `json:"-"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
}

// fakeRouter answers the two router endpoints llmchat uses and records every
// completion request it receives.
type fakeRouter struct {
	*httptest.Server

	mu   sync.Mutex
	reqs []routerRequest
}

func newFakeRouter(t *testing.T) *fakeRouter {
	t.Helper()
	f := &fakeRouter{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeRouter) serve(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/routes":
		io.WriteString(w, `{"default":"general","routes":[{"name":"general"},{"name":"code"}]}`)
	case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
		f.completion(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeRouter) completion(w http.ResponseWriter, r *http.Request) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req routerRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_, req.HasTools = keys["tools"]

	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()

	last := ""
	if n := len(req.Messages); n > 0 {
		last = req.Messages[n-1].Content
	}
	if strings.Contains(last, "fail") {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "router down")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Llmrouter-Route", "code")
	w.Header().Set("X-Llmrouter-Backend", "ollama")
	w.Header().Set("X-Llmrouter-Model", "qwen")
	w.WriteHeader(http.StatusOK)

	if strings.Contains(last, "think") {
		writeChunk(w, map[string]any{"reasoning_content": "hmm"})
	}
	// The last user message is the one this turn answers; split after each
	// space so the pieces join back to the exact text.
	answer := "answer to: " + lastUserContent(req)
	for _, piece := range strings.SplitAfter(answer, " ") {
		if piece == "" {
			continue
		}
		writeChunk(w, map[string]any{"content": piece})
	}
	io.WriteString(w, "data: [DONE]\n\n")
}

// lastUserContent returns the content of the last user message in req.
func lastUserContent(req routerRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == chat.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}

// writeChunk writes one OpenAI-style stream chunk with a single delta.
func writeChunk(w io.Writer, delta map[string]any) {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"delta": delta}},
	})
	io.WriteString(w, "data: "+string(b)+"\n\n")
}

func (f *fakeRouter) requests() []routerRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]routerRequest(nil), f.reqs...)
}

func (f *fakeRouter) last(t *testing.T) routerRequest {
	t.Helper()
	reqs := f.requests()
	if len(reqs) == 0 {
		t.Fatal("fake router received no completion request")
	}
	return reqs[len(reqs)-1]
}

// start builds the app on dbPath against routerURL and serves it. The caller
// closes the returned app (and the server) to simulate a restart.
func start(t *testing.T, dbPath, routerURL string) (*app.App, *httptest.Server) {
	t.Helper()
	a := apptest.New(t, routerURL, dbPath)
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(func() {
		srv.Close()
		a.Close()
	})
	return a, srv
}

// do sends a request with an optional JSON body and returns the status and body.
func do(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, url, err)
	}
	return resp.StatusCode, out
}

// mustJSON checks the status and decodes the body into v.
func mustJSON(t *testing.T, status, want int, body []byte, v any) {
	t.Helper()
	if status != want {
		t.Fatalf("status = %d, want %d; body %s", status, want, body)
	}
	if err := json.Unmarshal(body, v); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

func createConversation(t *testing.T, base string) chat.Conversation {
	t.Helper()
	status, body := do(t, http.MethodPost, base+"/api/conversations", map[string]string{"model": "auto"})
	var c chat.Conversation
	mustJSON(t, status, http.StatusCreated, body, &c)
	return c
}

func getConversation(t *testing.T, base, id string) (status int, body []byte) {
	t.Helper()
	return do(t, http.MethodGet, base+"/api/conversations/"+id, nil)
}

// send posts text to a conversation, reads the stream to its end and returns
// each event.
func send(t *testing.T, base, id, text string) []chat.Event {
	t.Helper()
	status, body := do(t, http.MethodPost, base+"/api/conversations/"+id+"/messages", map[string]string{"text": text})
	if status != http.StatusOK {
		t.Fatalf("send %q: status %d, body %s", text, status, body)
	}
	var events []chat.Event
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("decode event %q: %v", data, err)
		}
		events = append(events, ev)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	return events
}

// contentOf joins the text of the content events.
func contentOf(events []chat.Event) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Type == chat.EventContent {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{12}$`)

func TestServerEndToEnd(t *testing.T) {
	router := newFakeRouter(t)
	dbPath := filepath.Join(t.TempDir(), "llmchat.db")
	a, srv := start(t, dbPath, router.URL)
	base := srv.URL

	// 1. Routes and the page.
	status, body := do(t, http.MethodGet, base+"/api/routes", nil)
	var routes []string
	mustJSON(t, status, http.StatusOK, body, &routes)
	if want := []string{"general", "code"}; !reflect.DeepEqual(routes, want) {
		t.Fatalf("routes = %v, want %v", routes, want)
	}
	status, body = do(t, http.MethodGet, base+"/", nil)
	if status != http.StatusOK || !strings.Contains(string(body), `id="composer"`) {
		t.Fatalf("GET /: status %d, page does not contain composer", status)
	}

	// 2. Create a conversation.
	conv := createConversation(t, base)
	if !idPattern.MatchString(conv.ID) {
		t.Fatalf("conversation id = %q, want 12 lowercase hex characters", conv.ID)
	}

	// 3. A plain turn streams routing, content and done.
	events := send(t, base, conv.ID, "hello world")
	if len(events) < 3 {
		t.Fatalf("events = %+v, want info, content and done", events)
	}
	info := events[0]
	if info.Type != chat.EventInfo || info.Route != "code" || info.Backend != "ollama" || info.Model != "qwen" {
		t.Fatalf("first event = %+v, want info route code backend ollama model qwen", info)
	}
	for _, ev := range events[1 : len(events)-1] {
		if ev.Type != chat.EventContent {
			t.Fatalf("event %+v between info and done, want only content", ev)
		}
	}
	if got := contentOf(events); got != "answer to: hello world" {
		t.Fatalf("content = %q, want %q", got, "answer to: hello world")
	}
	if done := events[len(events)-1]; done.Type != chat.EventDone {
		t.Fatalf("last event = %+v, want done", done)
	}
	req := router.last(t)
	if req.Model != "auto" || !req.Stream || req.HasTools {
		t.Fatalf("router request model=%q stream=%v tools=%v, want auto, true, false", req.Model, req.Stream, req.HasTools)
	}
	// The server sends the system prompt first (app.SystemPrompt, CHAT-17),
	// then the one user message. Skip the leading system message and check the rest.
	msgs := req.Messages
	if len(msgs) > 0 && msgs[0].Role == chat.RoleSystem {
		msgs = msgs[1:]
	}
	if len(msgs) != 1 || msgs[0].Role != chat.RoleUser || msgs[0].Content != "hello world" {
		t.Fatalf("router messages = %+v, want system prompt then one user message %q", req.Messages, "hello world")
	}

	// 4. The stored conversation has both messages and the title.
	status, body = getConversation(t, base, conv.ID)
	var detail struct {
		Conversation chat.Conversation `json:"conversation"`
		Messages     []chat.Message    `json:"messages"`
	}
	mustJSON(t, status, http.StatusOK, body, &detail)
	if detail.Conversation.Title != "hello world" {
		t.Fatalf("title = %q, want %q", detail.Conversation.Title, "hello world")
	}
	if len(detail.Messages) != 2 {
		t.Fatalf("messages = %+v, want 2", detail.Messages)
	}
	u, as := detail.Messages[0], detail.Messages[1]
	if u.Seq != 1 || u.Role != chat.RoleUser || u.Content != "hello world" {
		t.Fatalf("message 1 = %+v, want seq 1 user %q", u, "hello world")
	}
	if as.Seq != 2 || as.Role != chat.RoleAssistant || as.Content != "answer to: hello world" ||
		as.Route != "code" || as.Backend != "ollama" || as.Model != "qwen" {
		t.Fatalf("message 2 = %+v, want seq 2 assistant with route code, backend ollama, model qwen", as)
	}

	// 5. A turn with reasoning streams it and sends the earlier history.
	events = send(t, base, conv.ID, "think about it")
	var sawReasoning bool
	for _, ev := range events {
		if ev.Type == chat.EventReasoning && ev.Text == "hmm" {
			sawReasoning = true
		}
	}
	if !sawReasoning {
		t.Fatalf("events = %+v, want a reasoning event with %q", events, "hmm")
	}
	if got := contentOf(events); got != "answer to: think about it" {
		t.Fatalf("content = %q, want %q", got, "answer to: think about it")
	}
	req = router.last(t)
	// Same as the first turn: skip the leading system prompt (CHAT-17), then check the history.
	msgs = req.Messages
	if len(msgs) > 0 && msgs[0].Role == chat.RoleSystem {
		msgs = msgs[1:]
	}
	if len(msgs) != 3 ||
		msgs[0].Role != chat.RoleUser || msgs[0].Content != "hello world" ||
		msgs[1].Role != chat.RoleAssistant ||
		msgs[2].Role != chat.RoleUser || msgs[2].Content != "think about it" {
		t.Fatalf("router messages = %+v, want user, assistant, then the new user message", req.Messages)
	}
	status, body = getConversation(t, base, conv.ID)
	mustJSON(t, status, http.StatusOK, body, &detail)
	if got := detail.Messages[len(detail.Messages)-1]; got.Role != chat.RoleAssistant || got.Reasoning != "hmm" {
		t.Fatalf("stored last message = %+v, want assistant with reasoning %q", got, "hmm")
	}

	// 6. A router failure is an error event, then done, and saves no reply.
	before := len(detail.Messages)
	events = send(t, base, conv.ID, "fail please")
	if len(events) != 2 {
		t.Fatalf("events = %+v, want an error event then done", events)
	}
	if e := events[0]; e.Type != chat.EventError || !strings.Contains(e.Error, "503") || !strings.Contains(e.Error, "router down") {
		t.Fatalf("first event = %+v, want error mentioning 503 and router down", events[0])
	}
	if events[1].Type != chat.EventDone {
		t.Fatalf("second event = %+v, want done", events[1])
	}
	status, body = getConversation(t, base, conv.ID)
	mustJSON(t, status, http.StatusOK, body, &detail)
	if len(detail.Messages) != before+1 {
		t.Fatalf("messages = %d after failed turn, want %d", len(detail.Messages), before+1)
	}
	if last := detail.Messages[len(detail.Messages)-1]; last.Role != chat.RoleUser || last.Content != "fail please" {
		t.Fatalf("last message = %+v, want the user message %q", last, "fail please")
	}

	// 7. Changing the model is used by the next turn.
	status, body = do(t, http.MethodPatch, base+"/api/conversations/"+conv.ID, map[string]string{"model": "code"})
	var patched chat.Conversation
	mustJSON(t, status, http.StatusOK, body, &patched)
	if patched.Model != "code" {
		t.Fatalf("patched model = %q, want code", patched.Model)
	}
	send(t, base, conv.ID, "switch model now")
	if req = router.last(t); req.Model != "code" {
		t.Fatalf("router model = %q after patch, want code", req.Model)
	}

	// 8. Two conversations, the one just used first.
	other := createConversation(t, base)
	send(t, base, other.ID, "second chat")
	status, body = do(t, http.MethodGet, base+"/api/conversations", nil)
	var list []chat.Conversation
	mustJSON(t, status, http.StatusOK, body, &list)
	if len(list) != 2 || list[0].ID != other.ID || list[1].ID != conv.ID {
		t.Fatalf("list = %+v, want [%s, %s]", list, other.ID, conv.ID)
	}

	// 9. Delete the one just used; it is gone and the list has one.
	if status, body = do(t, http.MethodDelete, base+"/api/conversations/"+other.ID, nil); status != http.StatusNoContent {
		t.Fatalf("delete: status %d, body %s", status, body)
	}
	if status, _ = getConversation(t, base, other.ID); status != http.StatusNotFound {
		t.Fatalf("get deleted conversation: status %d, want 404", status)
	}
	status, body = do(t, http.MethodGet, base+"/api/conversations", nil)
	mustJSON(t, status, http.StatusOK, body, &list)
	if len(list) != 1 || list[0].ID != conv.ID {
		t.Fatalf("list after delete = %+v, want only %s", list, conv.ID)
	}

	// 10. Restart on the same database: the conversation and its messages remain.
	srv.Close()
	if err := a.Close(); err != nil {
		t.Fatalf("close app: %v", err)
	}
	_, srv2 := start(t, dbPath, router.URL)
	status, body = getConversation(t, srv2.URL, conv.ID)
	mustJSON(t, status, http.StatusOK, body, &detail)
	if detail.Conversation.ID != conv.ID || detail.Conversation.Model != "code" {
		t.Fatalf("after restart conversation = %+v, want %s with model code", detail.Conversation, conv.ID)
	}
	if len(detail.Messages) != before+3 {
		t.Fatalf("after restart messages = %d, want %d", len(detail.Messages), before+3)
	}
	if got := detail.Messages[len(detail.Messages)-1].Content; got != "answer to: switch model now" {
		t.Fatalf("after restart last message = %q, want %q", got, "answer to: switch model now")
	}

	// 11. Unknown conversations are 404; an empty text is 400.
	if status, body = do(t, http.MethodPost, srv2.URL+"/api/conversations/000000000000/messages", map[string]string{"text": "hi"}); status != http.StatusNotFound {
		t.Fatalf("message to unknown conversation: status %d, body %s, want 404", status, body)
	}
	if status, body = do(t, http.MethodPost, srv2.URL+"/api/conversations/"+conv.ID+"/messages", map[string]string{"text": ""}); status != http.StatusBadRequest {
		t.Fatalf("empty text: status %d, body %s, want 400", status, body)
	}
}
