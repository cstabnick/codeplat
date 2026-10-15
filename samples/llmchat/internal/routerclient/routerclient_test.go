package routerclient

import (
	"bytes"
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

func TestDebugLogRecordsFinishReasonLength(t *testing.T) {
	stream := sseHandler(t, nil,
		dataLine(`{"choices":[{"delta":{"content":"abc"},"finish_reason":null}]}`),
		dataLine(`{"choices":[{"delta":{},"finish_reason":"length"}]}`),
		dataLine("[DONE]"),
	)
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Llmrouter-Route", "code")
		w.Header().Set("X-Llmrouter-Backend", "bonsai")
		w.Header().Set("X-Llmrouter-Model", "Ternary-Bonsai")
		stream(w, r)
	})
	var out bytes.Buffer
	c := New(srv.URL, nil).WithDebugLog(&out)
	if _, err := c.Complete(context.Background(), chat.CompletionRequest{Model: "m"}, nil); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if !strings.Contains(out.String(), `"finish_reason":"length"`) {
		t.Errorf("debug log has no finish_reason length:\n%s", out.String())
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 1 {
		t.Fatalf("debug log has %d lines, want 1:\n%s", len(lines), out.String())
	}
	var rec debugRecord
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("debug line is not JSON: %v", err)
	}
	if rec.Route != "code" || rec.Backend != "bonsai" || rec.Model != "Ternary-Bonsai" {
		t.Errorf("route/backend/model = %q/%q/%q", rec.Route, rec.Backend, rec.Model)
	}
	if len(rec.Chunks) != 3 {
		t.Fatalf("chunks = %d, want 3", len(rec.Chunks))
	}
	if rec.Chunks[1].FinishReason == nil || *rec.Chunks[1].FinishReason != "length" {
		t.Errorf("chunk 1 finish_reason = %v, want length", rec.Chunks[1].FinishReason)
	}
	if rec.ContentLength != 3 || rec.ReasoningLength != 0 || rec.ToolCalls != 0 {
		t.Errorf("sizes = content %d, reasoning %d, tool calls %d", rec.ContentLength, rec.ReasoningLength, rec.ToolCalls)
	}
}

func TestNoDebugWriterWritesNothing(t *testing.T) {
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"content":"hi"}`),
		dataLine("[DONE]"),
	))
	c := New(srv.URL, nil).WithDebugLog(nil)
	if c.debug != nil {
		t.Fatal("WithDebugLog(nil) turned the debug log on")
	}
	msg, err := c.Complete(context.Background(), chat.CompletionRequest{Model: "m"}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.Content != "hi" {
		t.Errorf("Content = %q, want hi", msg.Content)
	}
}

// textTools is the request's tool list for the text-call tests: write_file as
// the chat tools declare it, and resize with one property of each typed kind.
var textTools = []chat.ToolSpec{
	{
		Name:        "write_file",
		Description: "Create a file.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`),
	},
	{
		Name:        "resize",
		Description: "Resize.",
		Parameters:  json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"},"force":{"type":"boolean"},"ratio":{"type":"number"}}}`),
	},
}

// textReply streams text as the reply content, with no structured tool calls,
// and completes a request that offers tools.
func textReply(t *testing.T, text string, tools []chat.ToolSpec) (chat.Message, []chat.Event, error) {
	t.Helper()
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"content":`+mustJSON(t, text)+`}`),
		dataLine("[DONE]"),
	))
	var events []chat.Event
	msg, err := New(srv.URL, nil).Complete(context.Background(), chat.CompletionRequest{Model: "m", Tools: tools}, collect(&events))
	return msg, events, err
}

func errorEvents(events []chat.Event) []chat.Event {
	var out []chat.Event
	for _, ev := range events {
		if ev.Type == chat.EventError {
			out = append(out, ev)
		}
	}
	return out
}

func TestWellFormedTextCallConvertsToToolCall(t *testing.T) {
	text := "Let me write it.\n<tool_call>\n<function=write_file>\n<parameter=path>\nnote.txt\n</parameter>\n<parameter=content>\nline one\nline two\n</parameter>\n</function>\n</tool_call>"
	msg, events, err := textReply(t, text, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %+v, want one call", msg.ToolCalls)
	}
	tc := msg.ToolCalls[0]
	if tc.Name != "write_file" || tc.ID != "text-1" {
		t.Errorf("call = name %q id %q, want write_file text-1", tc.Name, tc.ID)
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Arguments), &args); err != nil {
		t.Fatalf("Arguments %q is not JSON: %v", tc.Arguments, err)
	}
	if want := map[string]any{"path": "note.txt", "content": "line one\nline two"}; !reflect.DeepEqual(args, want) {
		t.Errorf("Arguments = %v, want %v", args, want)
	}
	if msg.Content != text {
		t.Errorf("Content = %q, want the reply text kept unchanged", msg.Content)
	}
	if len(errorEvents(events)) != 0 {
		t.Errorf("events = %+v, want no error event", events)
	}
}

func TestTextCallValueTrimsOnlyOneNewline(t *testing.T) {
	cases := map[string]string{
		"no newline":   "<parameter=content>x</parameter>",
		"one newline":  "<parameter=content>\nx\n</parameter>",
		"two newlines": "<parameter=content>\n\nx\n\n</parameter>",
		"spaces kept":  "<parameter=content>  x  </parameter>",
	}
	want := map[string]string{
		"no newline":   "x",
		"one newline":  "x",
		"two newlines": "\nx\n",
		"spaces kept":  "  x  ",
	}
	for name, param := range cases {
		text := "<tool_call>\n<function=write_file>\n<parameter=path>\na.txt\n</parameter>\n" + param + "\n</function>\n</tool_call>"
		msg, _, err := textReply(t, text, textTools)
		if err != nil {
			t.Fatalf("%s: Complete: %v", name, err)
		}
		if len(msg.ToolCalls) != 1 {
			t.Fatalf("%s: ToolCalls = %+v, want one call", name, msg.ToolCalls)
		}
		var args map[string]string
		if err := json.Unmarshal([]byte(msg.ToolCalls[0].Arguments), &args); err != nil {
			t.Fatalf("%s: Arguments: %v", name, err)
		}
		if args["content"] != want[name] {
			t.Errorf("%s: content = %q, want %q", name, args["content"], want[name])
		}
	}
}

func TestMalformedTextCallConvertsNothingAndEmitsError(t *testing.T) {
	text := "Let me check the directory contents:\n<tool_call>\n<function=ls -la\n</parameter>\n</tool_call>"
	msg, events, err := textReply(t, text, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.ToolCalls != nil {
		t.Errorf("ToolCalls = %+v, want none", msg.ToolCalls)
	}
	if msg.Content != text {
		t.Errorf("Content = %q, want the reply text kept unchanged", msg.Content)
	}
	errs := errorEvents(events)
	if len(errs) != 1 || errs[0].Error != "could not read a tool call in the reply; no tool was run" {
		t.Fatalf("error events = %+v, want exactly one with the stated text", errs)
	}
	if last := events[len(events)-1]; last.Type != chat.EventError {
		t.Errorf("last event = %+v, want the error after the content", last)
	}
}

func TestMalformedTextCallVariants(t *testing.T) {
	bad := map[string]string{
		"unclosed function":  "<tool_call><function=write_file><parameter=path>a</parameter></tool_call>",
		"unclosed parameter": "<tool_call><function=write_file><parameter=path>a</function></tool_call>",
		"unnamed parameter":  "<tool_call><function=write_file><parameter=>a</parameter></function></tool_call>",
		"text between tags":  "<tool_call>oops<function=write_file></function></tool_call>",
		"bad integer":        "<tool_call><function=resize><parameter=count>x</parameter></function></tool_call>",
		"bad boolean":        "<tool_call><function=resize><parameter=force>yes</parameter></function></tool_call>",
		"repeated key":       "<tool_call><function=write_file><parameter=path>a</parameter><parameter=path>b</parameter></function></tool_call>",
	}
	for name, text := range bad {
		msg, events, err := textReply(t, text, textTools)
		if err != nil {
			t.Fatalf("%s: Complete: %v", name, err)
		}
		if msg.ToolCalls != nil {
			t.Errorf("%s: ToolCalls = %+v, want none", name, msg.ToolCalls)
		}
		if len(errorEvents(events)) != 1 {
			t.Errorf("%s: events = %+v, want one error event", name, events)
		}
	}
}

func TestWellFormedTextCallForUnknownNameIsIgnored(t *testing.T) {
	text := "<tool_call>\n<function=bash>\n<parameter=command>\necho hi\n</parameter>\n</function>\n</tool_call>"
	msg, events, err := textReply(t, text, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if msg.ToolCalls != nil {
		t.Errorf("ToolCalls = %+v, want none", msg.ToolCalls)
	}
	if len(errorEvents(events)) != 0 {
		t.Errorf("events = %+v, want no error event", events)
	}
}

func TestUnknownNameSkippedKnownNameConverts(t *testing.T) {
	text := "<tool_call><function=bash><parameter=command>ls</parameter></function></tool_call>" +
		"<tool_call><function=write_file><parameter=path>a.txt</parameter><parameter=content>b</parameter></function></tool_call>"
	msg, events, err := textReply(t, text, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].ID != "text-1" || msg.ToolCalls[0].Name != "write_file" {
		t.Fatalf("ToolCalls = %+v, want one write_file as text-1", msg.ToolCalls)
	}
	if len(errorEvents(events)) != 0 {
		t.Errorf("events = %+v, want no error event", events)
	}
}

func TestTwoTextCallsNumberedInOrder(t *testing.T) {
	text := "<tool_call><function=write_file><parameter=path>a</parameter><parameter=content>1</parameter></function></tool_call>\n" +
		"<tool_call><function=write_file><parameter=path>b</parameter><parameter=content>2</parameter></function></tool_call>"
	msg, _, err := textReply(t, text, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(msg.ToolCalls) != 2 {
		t.Fatalf("ToolCalls = %+v, want two", msg.ToolCalls)
	}
	for i, id := range []string{"text-1", "text-2"} {
		if msg.ToolCalls[i].ID != id {
			t.Errorf("call %d id = %q, want %q", i, msg.ToolCalls[i].ID, id)
		}
	}
}

func TestTypedTextArguments(t *testing.T) {
	ok := "<tool_call><function=resize><parameter=count>3</parameter><parameter=force>true</parameter><parameter=ratio>0.5</parameter></function></tool_call>"
	msg, _, err := textReply(t, ok, textTools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Arguments != `{"count":3,"force":true,"ratio":0.5}` {
		t.Fatalf("ToolCalls = %+v, want resize with typed arguments", msg.ToolCalls)
	}
}

func TestStructuredCallWinsOverTextCall(t *testing.T) {
	text := "<tool_call><function=write_file><parameter=path>a</parameter><parameter=content>b</parameter></function></tool_call>"
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"content":`+mustJSON(t, text)+`}`),
		chunk(`{"tool_calls":[{"index":0,"id":"call_s","function":{"name":"read_file","arguments":"{\"path\":\"a\"}"}}]}`),
		dataLine("[DONE]"),
	))
	var events []chat.Event
	msg, err := New(srv.URL, nil).Complete(context.Background(), chat.CompletionRequest{Model: "m", Tools: textTools}, collect(&events))
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	want := []chat.ToolCall{{ID: "call_s", Name: "read_file", Arguments: `{"path":"a"}`}}
	if !reflect.DeepEqual(msg.ToolCalls, want) {
		t.Fatalf("ToolCalls = %+v, want only the structured call %+v", msg.ToolCalls, want)
	}
}

func TestStreamErrorSkipsTextFallback(t *testing.T) {
	text := "<tool_call><function=write_file><parameter=path>a</parameter><parameter=content>b</parameter></function></tool_call>"
	srv := newServer(t, sseHandler(t, nil,
		chunk(`{"content":`+mustJSON(t, text)+`}`),
		dataLine("{not json"),
	))
	var events []chat.Event
	msg, err := New(srv.URL, nil).Complete(context.Background(), chat.CompletionRequest{Model: "m", Tools: textTools}, collect(&events))
	if err == nil {
		t.Fatal("Complete: no error for a broken stream chunk")
	}
	if msg.ToolCalls != nil {
		t.Errorf("ToolCalls = %+v, want none after a stream error", msg.ToolCalls)
	}
	if len(errorEvents(events)) != 0 {
		t.Errorf("events = %+v, want no text-call error after a stream error", events)
	}
}
