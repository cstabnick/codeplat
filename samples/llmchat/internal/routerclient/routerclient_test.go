package routerclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// sse writes each payload as a "data:" line and flushes after each.
func sseHandler(t *testing.T, got *map[string]any, lines ...string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if got != nil {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode request body: %v", err)
			}
			*got = body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		f, _ := w.(http.Flusher)
		for _, l := range lines {
			io.WriteString(w, l)
			if f != nil {
				f.Flush()
			}
		}
	}
}

func dataLine(payload string) string {
	return "data: " + payload + "\n\n"
}

func chunk(delta string) string {
	return dataLine(`{"choices":[{"delta":` + delta + `}]}`)
}

func newServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func collect(events *[]chat.Event) func(chat.Event) {
	return func(e chat.Event) { *events = append(*events, e) }
}

func TestCompleterInterface(t *testing.T) {
	var _ chat.Completer = (*Client)(nil)
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	var path string
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		io.WriteString(w, dataLine("[DONE]"))
	})
	c := New(srv.URL+"/", nil)
	if _, err := c.Complete(context.Background(), chat.CompletionRequest{Model: "m"}, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", path)
	}
}

func TestRequestBodyPlainConversation(t *testing.T) {
	var got map[string]any
	srv := newServer(t, sseHandler(t, &got, dataLine("[DONE]")))
	c := New(srv.URL, nil)
	_, err := c.Complete(context.Background(), chat.CompletionRequest{
		Model: "general",
		Messages: []chat.Message{
			{Role: chat.RoleSystem, Content: "be brief"},
			{Role: chat.RoleUser, Content: "hi"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got["model"] != "general" {
		t.Errorf("model = %v, want general", got["model"])
	}
	if got["stream"] != true {
		t.Errorf("stream = %v, want true", got["stream"])
	}
	if _, ok := got["tools"]; ok {
		t.Errorf("tools key present for a request without tools: %v", got["tools"])
	}
	want := []any{
		map[string]any{"role": "system", "content": "be brief"},
		map[string]any{"role": "user", "content": "hi"},
	}
	if !reflect.DeepEqual(got["messages"], want) {
		t.Errorf("messages = %#v, want %#v", got["messages"], want)
	}
}

func TestEmptyModelBecomesAuto(t *testing.T) {
	var got map[string]any
	srv := newServer(t, sseHandler(t, &got, dataLine("[DONE]")))
	c := New(srv.URL, nil)
	if _, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if got["model"] != "auto" {
		t.Errorf("model = %v, want auto", got["model"])
	}
}

func TestRequestBodyToolsAndToolMessages(t *testing.T) {
	var got map[string]any
	srv := newServer(t, sseHandler(t, &got, dataLine("[DONE]")))
	c := New(srv.URL, nil)
	req := chat.CompletionRequest{
		Model: "auto",
		Messages: []chat.Message{
			{Role: chat.RoleUser, Content: "read a.txt"},
			{
				Role:      chat.RoleAssistant,
				Content:   "",
				Reasoning: "secret thinking",
				ToolCalls: []chat.ToolCall{{ID: "call_1", Name: "read_file", Arguments: `{"path":"a.txt"}`}},
			},
			{Role: chat.RoleTool, ToolCallID: "call_1", Content: "hello"},
			{Role: chat.RoleAssistant, Content: "It says hello."},
		},
		Tools: []chat.ToolSpec{{
			Name:        "read_file",
			Description: "Read a file",
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`),
		}},
	}
	if _, err := c.Complete(context.Background(), req, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	wantTools := []any{map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        "read_file",
			"description": "Read a file",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
			},
		},
	}}
	if !reflect.DeepEqual(got["tools"], wantTools) {
		t.Errorf("tools = %#v, want %#v", got["tools"], wantTools)
	}

	wantMsgs := []any{
		map[string]any{"role": "user", "content": "read a.txt"},
		map[string]any{
			"role":    "assistant",
			"content": "",
			"tool_calls": []any{map[string]any{
				"id":       "call_1",
				"type":     "function",
				"function": map[string]any{"name": "read_file", "arguments": `{"path":"a.txt"}`},
			}},
		},
		map[string]any{"role": "tool", "tool_call_id": "call_1", "content": "hello"},
		map[string]any{"role": "assistant", "content": "It says hello."},
	}
	if !reflect.DeepEqual(got["messages"], wantMsgs) {
		t.Errorf("messages = %#v, want %#v", got["messages"], wantMsgs)
	}
	if strings.Contains(strings.ToLower(mustJSON(t, got)), "secret thinking") {
		t.Errorf("reasoning was sent back to the router: %v", got)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestContentAndReasoningStreamAndMessage(t *testing.T) {
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"reasoning_content":"think "}`),
		chunk(`{"reasoning_content":"more"}`),
		chunk(`{"content":"Hel"}`),
		chunk(`{"content":"lo"}`),
		dataLine("[DONE]"),
	))
	c := New(srv.URL, nil)
	var events []chat.Event
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{Model: "m"}, collect(&events))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := []chat.Event{
		{Type: chat.EventInfo},
		{Type: chat.EventReasoning, Text: "think "},
		{Type: chat.EventReasoning, Text: "more"},
		{Type: chat.EventContent, Text: "Hel"},
		{Type: chat.EventContent, Text: "lo"},
	}
	if !reflect.DeepEqual(events, want) {
		t.Errorf("events = %#v, want %#v", events, want)
	}
	if msg.Role != chat.RoleAssistant {
		t.Errorf("Role = %q", msg.Role)
	}
	if msg.Content != "Hello" {
		t.Errorf("Content = %q, want Hello", msg.Content)
	}
	if msg.Reasoning != "think more" {
		t.Errorf("Reasoning = %q, want %q", msg.Reasoning, "think more")
	}
	if msg.ToolCalls != nil {
		t.Errorf("ToolCalls = %v, want nil", msg.ToolCalls)
	}
}

func TestNullAndAbsentDeltasIgnored(t *testing.T) {
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"content":null,"reasoning_content":null}`),
		chunk(`{"role":"assistant"}`),
		chunk(`{"content":"ok"}`),
		dataLine(`{"choices":[]}`),
		dataLine("[DONE]"),
	))
	c := New(srv.URL, nil)
	var events []chat.Event
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, collect(&events))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Content != "ok" || msg.Reasoning != "" {
		t.Errorf("message = %+v, want content ok and no reasoning", msg)
	}
	if len(events) != 2 || events[1] != (chat.Event{Type: chat.EventContent, Text: "ok"}) {
		t.Errorf("events = %#v", events)
	}
}

func TestToolCallAssembledFromChunks(t *testing.T) {
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"read_file","arguments":"{"}}]}`),
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"\"path\":\""}}]}`),
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"a.txt"}}]}`),
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]}`),
		dataLine("[DONE]"),
	))
	c := New(srv.URL, nil)
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := []chat.ToolCall{{ID: "call_a", Name: "read_file", Arguments: `{"path":"a.txt"}`}}
	if !reflect.DeepEqual(msg.ToolCalls, want) {
		t.Errorf("ToolCalls = %#v, want %#v", msg.ToolCalls, want)
	}
}

func TestTwoToolCallsInIndexOrder(t *testing.T) {
	// Index 1 arrives first; the result must still list index 0 first.
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"tool_calls":[{"index":1,"id":"call_b","function":{"name":"list","arguments":"{}"}}]}`),
		chunk(`{"tool_calls":[{"index":0,"id":"call_a","function":{"name":"read_file","arguments":"{"}}]}`),
		chunk(`{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}`),
		dataLine("[DONE]"),
	))
	c := New(srv.URL, nil)
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := []chat.ToolCall{
		{ID: "call_a", Name: "read_file", Arguments: "{}"},
		{ID: "call_b", Name: "list", Arguments: "{}"},
	}
	if !reflect.DeepEqual(msg.ToolCalls, want) {
		t.Errorf("ToolCalls = %#v, want %#v", msg.ToolCalls, want)
	}
}

// TestLineSplitAcrossWritesParses sends one SSE line in two writes with a pause.
func TestLineSplitAcrossWritesParses(t *testing.T) {
	line := chunk(`{"content":"split line"}`)
	half := len(line) / 2
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		io.WriteString(w, line[:half])
		f.Flush()
		time.Sleep(20 * time.Millisecond)
		io.WriteString(w, line[half:])
		f.Flush()
		io.WriteString(w, dataLine("[DONE]"))
		f.Flush()
	})
	c := New(srv.URL, nil)
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Content != "split line" {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestStreamWithoutDoneIsNotAnError(t *testing.T) {
	srv := newServer(t, sseHandler(t, nil, chunk(`{"content":"partial"}`)))
	c := New(srv.URL, nil)
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Content != "partial" {
		t.Errorf("Content = %q", msg.Content)
	}
}

func TestRoutingHeaders(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Llmrouter-Route", "code")
		w.Header().Set("X-Llmrouter-Backend", "ollama")
		w.Header().Set("X-Llmrouter-Model", "qwen")
		io.WriteString(w, chunk(`{"content":"x"}`))
		io.WriteString(w, dataLine("[DONE]"))
	})
	c := New(srv.URL, nil)
	var events []chat.Event
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, collect(&events))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	info := events[0]
	if info.Type != chat.EventInfo || info.Route != "code" || info.Backend != "ollama" || info.Model != "qwen" {
		t.Errorf("info event = %+v", info)
	}
	if msg.Route != "code" || msg.Backend != "ollama" || msg.Model != "qwen" {
		t.Errorf("message routing = %q/%q/%q", msg.Route, msg.Backend, msg.Model)
	}
}

func TestMissingRouteHeaderLeavesRouteEmpty(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Llmrouter-Backend", "ollama")
		io.WriteString(w, dataLine("[DONE]"))
	})
	c := New(srv.URL, nil)
	var events []chat.Event
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{}, collect(&events))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Route != "" || events[0].Route != "" {
		t.Errorf("Route = %q in message, %q in info; want empty", msg.Route, events[0].Route)
	}
	if msg.Backend != "ollama" {
		t.Errorf("Backend = %q", msg.Backend)
	}
}

func TestNon200ReturnsStatusAndBody(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "no backend ready")
	})
	c := New(srv.URL, nil)
	_, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "503") || !strings.Contains(err.Error(), "no backend ready") {
		t.Errorf("error = %v, want 503 and the body", err)
	}
}

func TestNon200BodyTruncatedTo512Bytes(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, strings.Repeat("x", 2000))
	})
	c := New(srv.URL, nil)
	_, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err == nil {
		t.Fatal("want error")
	}
	if n := strings.Count(err.Error(), "x"); n != 512 {
		t.Errorf("body bytes in error = %d, want 512", n)
	}
}

func TestClosedServerIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New(url, nil)
	_, err := c.Complete(context.Background(), chat.CompletionRequest{}, nil)
	if err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Errorf("error = %v, want unreachable", err)
	}
}

func TestCancelledContext(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	ctx, cancel := context.WithCancel(context.Background())
	c := New(srv.URL, nil)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	_, err := c.Complete(ctx, chat.CompletionRequest{}, nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestRoutesReturnsNamesInOrder(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/routes" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"default":"general","routes":[{"name":"general"},{"name":"code"},{"name":"vision"}]}`)
	})
	c := New(srv.URL, nil)
	names, err := c.Routes(context.Background())
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	want := []string{"general", "code", "vision"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("names = %v, want %v", names, want)
	}
}

func TestRoutesServerErrorIsError(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, "boom")
	})
	c := New(srv.URL, nil)
	if _, err := c.Routes(context.Background()); err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("error = %v, want 500", err)
	}
}
