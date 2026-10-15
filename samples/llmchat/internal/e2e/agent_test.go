package e2e_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"llmchat/internal/apptest"
	"llmchat/internal/chat"
)

// agentRequest is one completion request the agent fake router received.
type agentRequest struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCallID string `json:"tool_call_id"`
	} `json:"messages"`
	Tools []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tools"`
}

// agentRouter is a scripted fake router. It answers from the last message: a
// tool result is echoed as "done: ..."; a user message picks a tool call or
// the answer "plain".
type agentRouter struct {
	*httptest.Server

	mu   sync.Mutex
	reqs []agentRequest
}

func newAgentRouter(t *testing.T) *agentRouter {
	t.Helper()
	f := &agentRouter{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *agentRouter) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
		http.NotFound(w, r)
		return
	}
	var req agentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "no messages", http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Llmrouter-Route", "reason")
	w.Header().Set("X-Llmrouter-Backend", "bonsai")
	w.Header().Set("X-Llmrouter-Model", "b")
	w.WriteHeader(http.StatusOK)

	last := req.Messages[len(req.Messages)-1]
	if last.Role == chat.RoleTool {
		agentText(w, "done: "+last.Content)
	} else {
		switch last.Content {
		case "read hello.txt":
			agentToolCall(w, "read_file", `{"path":"hello.txt"}`)
		case "write note.txt":
			agentToolCall(w, "write_file", `{"path":"note.txt","content":"hi"}`)
		case "escape":
			agentToolCall(w, "read_file", `{"path":"../secret.txt"}`)
		case "run echo":
			agentToolCall(w, "bash", `{"command":"echo ran"}`)
		default:
			agentText(w, "plain")
		}
	}
	io.WriteString(w, "data: [DONE]\n\n")
}

// agentChunk writes one stream chunk holding a single choice.
func agentChunk(w io.Writer, choice map[string]any) {
	b, _ := json.Marshal(map[string]any{"choices": []map[string]any{choice}})
	io.WriteString(w, "data: "+string(b)+"\n\n")
}

// agentText streams text as content deltas, split after each space so the
// pieces join back to the exact text.
func agentText(w io.Writer, text string) {
	for _, piece := range strings.SplitAfter(text, " ") {
		if piece == "" {
			continue
		}
		agentChunk(w, map[string]any{"delta": map[string]any{"content": piece}})
	}
}

// agentToolCall streams one tool call the way a real server does: the name
// and the opening brace first, the arguments in two more pieces, then the
// finish reason.
func agentToolCall(w io.Writer, name, args string) {
	mid := args[1 : len(args)-1]
	last := args[len(args)-1:]
	piece := func(fn map[string]any, extra map[string]any) map[string]any {
		tc := map[string]any{"index": 0, "function": fn}
		for k, v := range extra {
			tc[k] = v
		}
		return map[string]any{"delta": map[string]any{"tool_calls": []map[string]any{tc}}}
	}
	agentChunk(w, piece(
		map[string]any{"name": name, "arguments": "{"},
		map[string]any{"id": "call_1", "type": "function"},
	))
	agentChunk(w, piece(map[string]any{"arguments": mid}, nil))
	agentChunk(w, piece(map[string]any{"arguments": last}, nil))
	agentChunk(w, map[string]any{"delta": map[string]any{}, "finish_reason": "tool_calls"})
}

func (f *agentRouter) requests() []agentRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]agentRequest(nil), f.reqs...)
}

// agentStart builds the app against routerURL on a fresh database and serves it.
func agentStart(t *testing.T, routerURL string) string {
	t.Helper()
	a := apptest.New(t, routerURL, filepath.Join(t.TempDir(), "llmchat.db"))
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return srv.URL
}

// agentCall sends a request with an optional JSON body and returns the status and body.
func agentCall(t *testing.T, method, url string, body any) (int, []byte) {
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

// agentCreate makes a conversation on the auto model, with a workspace when given.
func agentCreate(t *testing.T, base, workspace string) chat.Conversation {
	t.Helper()
	body := map[string]string{"model": "auto"}
	if workspace != "" {
		body["workspace"] = workspace
	}
	status, raw := agentCall(t, http.MethodPost, base+"/api/conversations", body)
	if status != http.StatusCreated {
		t.Fatalf("create conversation: status %d, body %s", status, raw)
	}
	var c chat.Conversation
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("decode conversation %s: %v", raw, err)
	}
	return c
}

// agentTurn posts text to a conversation and reads its events as they
// arrive, calling onEvent (if not nil) for each one. A non-200 status
// returns no events.
func agentTurn(ctx context.Context, base, id, text string, onEvent func(chat.Event)) (int, []chat.Event, error) {
	b, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/conversations/"+id+"/messages", bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, nil, nil
	}
	var events []chat.Event
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		data, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return resp.StatusCode, events, fmt.Errorf("decode event %q: %w", data, err)
		}
		events = append(events, ev)
		if onEvent != nil {
			onEvent(ev)
		}
	}
	return resp.StatusCode, events, sc.Err()
}

// agentAnswer answers a waiting approval. The engine announces an approval
// before it starts waiting, so a 404 is retried for up to 2 seconds.
func agentAnswer(t *testing.T, base, id, call string, approve bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		status, body := agentCall(t, http.MethodPost, base+"/api/conversations/"+id+"/approvals/"+call, map[string]bool{"approve": approve})
		if status == http.StatusNoContent {
			return
		}
		if status != http.StatusNotFound || time.Now().After(deadline) {
			t.Fatalf("answer approval %s: status %d, body %s", call, status, body)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// agentBool returns a pointer to b, for agentSend's approve argument.
func agentBool(b bool) *bool { return &b }

// agentSend posts text and returns its events. Each approval event is
// answered with *approve. A nil approve means no approval is expected: one
// that arrives is reported as an error and refused.
func agentSend(t *testing.T, base, id, text string, approve *bool) []chat.Event {
	t.Helper()
	onEvent := func(ev chat.Event) {
		if ev.Type != chat.EventApproval {
			return
		}
		if approve == nil {
			t.Errorf("unexpected approval for %s on %q", ev.Name, text)
			agentAnswer(t, base, id, ev.CallID, false)
			return
		}
		agentAnswer(t, base, id, ev.CallID, *approve)
	}
	status, events, err := agentTurn(context.Background(), base, id, text, onEvent)
	if err != nil {
		t.Fatalf("send %q: %v", text, err)
	}
	if status != http.StatusOK {
		t.Fatalf("send %q: status %d", text, status)
	}
	return events
}

// agentIndex returns the index of the first event of type typ, or -1.
func agentIndex(events []chat.Event, typ string) int {
	for i, ev := range events {
		if ev.Type == typ {
			return i
		}
	}
	return -1
}

// agentContentOf joins the text of the content events.
func agentContentOf(events []chat.Event) string {
	var b strings.Builder
	for _, ev := range events {
		if ev.Type == chat.EventContent {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

// agentLine is how read_file numbers a line.
func agentLine(n int, text string) string {
	return fmt.Sprintf("%6d\t%s", n, text)
}

// agentStoredMessages decodes a conversation's stored messages.
type agentStoredMessages struct {
	Conversation chat.Conversation `json:"conversation"`
	Messages     []chat.Message    `json:"messages"`
}

func agentGet(t *testing.T, base, id string) agentStoredMessages {
	t.Helper()
	status, raw := agentCall(t, http.MethodGet, base+"/api/conversations/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("get conversation %s: status %d, body %s", id, status, raw)
	}
	var detail agentStoredMessages
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatalf("decode conversation %s: %v", raw, err)
	}
	return detail
}

func TestAgentToolLoop(t *testing.T) {
	router := newAgentRouter(t)
	base := agentStart(t, router.URL)

	// The secret sits next to the workspace, so "../secret.txt" names it.
	parent := t.TempDir()
	ws := filepath.Join(parent, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "hello.txt"), []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte("top secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(ws, "note.txt")

	conv := agentCreate(t, base, ws)

	// 1. read_file runs without approval; the model gets the numbered lines
	// and answers with them.
	before := len(router.requests())
	events := agentSend(t, base, conv.ID, "read hello.txt", nil)
	if len(events) < 6 {
		t.Fatalf("events = %+v, want info, tool_call, tool_result, info, content and done", events)
	}
	wantTypes := []string{chat.EventInfo, chat.EventToolCall, chat.EventToolResult, chat.EventInfo}
	for i, want := range wantTypes {
		if events[i].Type != want {
			t.Fatalf("event %d = %+v, want type %s; events %+v", i, events[i], want, events)
		}
	}
	if events[0].Route != "reason" || events[0].Backend != "bonsai" || events[0].Model != "b" {
		t.Fatalf("first info = %+v, want route reason, backend bonsai, model b", events[0])
	}
	if call := events[1]; call.Name != "read_file" || !strings.Contains(call.Arguments, "hello.txt") {
		t.Fatalf("tool_call = %+v, want read_file on hello.txt", call)
	}
	result := events[2].Text
	if !strings.Contains(result, agentLine(1, "first line")) || !strings.Contains(result, "second line") {
		t.Fatalf("tool_result = %q, want numbered lines with first line and second line", result)
	}
	for _, ev := range events[4 : len(events)-1] {
		if ev.Type != chat.EventContent {
			t.Fatalf("event %+v after the second info, want only content", ev)
		}
	}
	if last := events[len(events)-1]; last.Type != chat.EventDone {
		t.Fatalf("last event = %+v, want done", last)
	}
	if got, want := agentContentOf(events), "done: "+result; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
	if i := agentIndex(events, chat.EventApproval); i >= 0 {
		t.Fatalf("read_file asked for approval: %+v", events[i])
	}

	turn := router.requests()[before:]
	if len(turn) != 2 {
		t.Fatalf("router got %d requests for the turn, want 2", len(turn))
	}
	first, second := turn[0], turn[1]
	if first.Model != "auto" {
		t.Fatalf("first request model = %q, want auto", first.Model)
	}
	var names []string
	for _, tool := range first.Tools {
		names = append(names, tool.Function.Name)
	}
	if want := []string{"read_file", "list_dir", "grep", "write_file", "edit_file", "bash"}; strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("first request tools = %v, want %v", names, want)
	}
	if len(first.Messages) == 0 || first.Messages[0].Role != chat.RoleSystem || !strings.HasPrefix(first.Messages[0].Content, "You are a coding assistant") {
		t.Fatalf("first request messages = %+v, want a leading system prompt", first.Messages)
	}
	if second.Model != "bonsai/b" {
		t.Fatalf("second request model = %q, want bonsai/b", second.Model)
	}
	if last := second.Messages[len(second.Messages)-1]; last.Role != chat.RoleTool {
		t.Fatalf("second request last message = %+v, want a tool message", last)
	}

	// 2. The stored history: user, assistant with the call, tool result, answer.
	detail := agentGet(t, base, conv.ID)
	if len(detail.Messages) != 4 {
		t.Fatalf("stored messages = %+v, want 4", detail.Messages)
	}
	m := detail.Messages
	if m[0].Role != chat.RoleUser || m[0].Content != "read hello.txt" {
		t.Fatalf("message 1 = %+v, want the user message", m[0])
	}
	if m[1].Role != chat.RoleAssistant || len(m[1].ToolCalls) != 1 || m[1].ToolCalls[0].ID != "call_1" || m[1].ToolCalls[0].Name != "read_file" {
		t.Fatalf("message 2 = %+v, want an assistant message with one read_file call call_1", m[1])
	}
	if m[2].Role != chat.RoleTool || m[2].ToolCallID != "call_1" || m[2].Content != result {
		t.Fatalf("message 3 = %+v, want the tool result for call_1", m[2])
	}
	if m[3].Role != chat.RoleAssistant || m[3].Content != "done: "+result {
		t.Fatalf("message 4 = %+v, want the final answer", m[3])
	}

	// 3. write_file asks for approval; approved, it writes the file.
	events = agentSend(t, base, conv.ID, "write note.txt", agentBool(true))
	ai, ri := agentIndex(events, chat.EventApproval), agentIndex(events, chat.EventToolResult)
	if ai < 0 || ri < 0 || ai > ri {
		t.Fatalf("events = %+v, want approval before tool_result", events)
	}
	if events[ai].Name != "write_file" || events[ai].CallID != events[ri].CallID {
		t.Fatalf("approval = %+v, tool_result = %+v, want write_file and the same call", events[ai], events[ri])
	}
	if data, err := os.ReadFile(note); err != nil || string(data) != "hi" {
		t.Fatalf("note.txt = %q, %v; want %q", data, err, "hi")
	}

	// 4. Denied, write_file does nothing.
	if err := os.Remove(note); err != nil {
		t.Fatal(err)
	}
	events = agentSend(t, base, conv.ID, "write note.txt", agentBool(false))
	ri = agentIndex(events, chat.EventToolResult)
	if ri < 0 || events[ri].Text != "denied by the user" {
		t.Fatalf("events = %+v, want tool_result %q", events, "denied by the user")
	}
	if _, err := os.Stat(note); err == nil {
		t.Fatal("note.txt exists after a denied write")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat note.txt: %v", err)
	}

	// 5. bash asks for approval; approved, it runs; denied, it does not.
	events = agentSend(t, base, conv.ID, "run echo", agentBool(true))
	ai, ri = agentIndex(events, chat.EventApproval), agentIndex(events, chat.EventToolResult)
	if ai < 0 || ri < 0 || events[ai].Name != "bash" {
		t.Fatalf("events = %+v, want a bash approval and a tool_result", events)
	}
	if out := events[ri].Text; !strings.Contains(out, "ran") || !strings.Contains(out, "exit status 0") {
		t.Fatalf("bash result = %q, want ran and exit status 0", out)
	}
	events = agentSend(t, base, conv.ID, "run echo", agentBool(false))
	if ri = agentIndex(events, chat.EventToolResult); ri < 0 || events[ri].Text != "denied by the user" {
		t.Fatalf("events = %+v, want a denied bash result", events)
	}

	// 6. Paths outside the workspace are refused without asking, and the
	// secret's content never reaches the events.
	events = agentSend(t, base, conv.ID, "escape", nil)
	ri = agentIndex(events, chat.EventToolResult)
	if ri < 0 || !strings.Contains(events[ri].Text, "outside the workspace") {
		t.Fatalf("events = %+v, want a tool_result saying outside the workspace", events)
	}
	raw, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "top secret") {
		t.Fatalf("events leak the secret file: %s", raw)
	}

	// 7. A conversation without a workspace offers no tools.
	plain := agentCreate(t, base, "")
	events = agentSend(t, base, plain.ID, "anything else", nil)
	if got := agentContentOf(events); got != "plain" {
		t.Fatalf("content = %q, want plain", got)
	}
	reqs := router.requests()
	req := reqs[len(reqs)-1]
	if len(req.Tools) != 0 {
		t.Fatalf("request without workspace has tools %+v", req.Tools)
	}
	for _, msg := range req.Messages {
		if msg.Role == chat.RoleSystem && strings.Contains(msg.Content, "Workspace directory: ") {
			t.Fatalf("system message %q names a workspace", msg.Content)
		}
	}

	// 8. Answering an approval that nobody waits for is 404.
	if status, body := agentCall(t, http.MethodPost, base+"/api/conversations/"+conv.ID+"/approvals/nope", map[string]bool{"approve": true}); status != http.StatusNotFound {
		t.Fatalf("answer with nothing waiting: status %d, body %s, want 404", status, body)
	}

	// 9. A client that abandons a turn waiting for approval ends that turn,
	// so the conversation accepts the next message and the call never runs.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sawApproval := false
	status, _, err := agentTurn(ctx, base, conv.ID, "write note.txt", func(ev chat.Event) {
		if ev.Type == chat.EventApproval {
			sawApproval = true
			cancel()
		}
	})
	if !sawApproval {
		t.Fatalf("write note.txt: no approval event before the stream ended (status %d, err %v)", status, err)
	}
	if err != nil && ctx.Err() == nil {
		t.Fatalf("abandoned stream: %v", err)
	}
	if _, err := os.Stat(note); err == nil {
		t.Fatal("note.txt exists after an abandoned approval")
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		status, events, err = agentTurn(context.Background(), base, conv.ID, "read hello.txt", nil)
		if err != nil {
			t.Fatalf("send after abandoned turn: %v", err)
		}
		if status != http.StatusConflict {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("conversation still busy 2 seconds after the abandoned turn")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status != http.StatusOK {
		t.Fatalf("send after abandoned turn: status %d, want 200", status)
	}
	if ri = agentIndex(events, chat.EventToolResult); ri < 0 || !strings.Contains(events[ri].Text, agentLine(1, "first line")) {
		t.Fatalf("events after abandoned turn = %+v, want the read_file result", events)
	}
	if last := events[len(events)-1]; last.Type != chat.EventDone {
		t.Fatalf("last event after abandoned turn = %+v, want done", last)
	}
}
