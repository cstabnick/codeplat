// Package routerclient talks to an llmrouter server over HTTP. It implements
// chat.Completer by streaming its OpenAI-compatible chat endpoint.
package routerclient

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"llmchat/internal/chat"
)

// Client talks to an llmrouter server.
type Client struct {
	base  string
	hc    *http.Client
	debug *streamLog // nil unless WithDebugLog was called
}

// streamLog writes one JSON line per completion. The mutex keeps lines whole
// when completions overlap.
type streamLog struct {
	mu sync.Mutex
	w  io.Writer
}

// WithDebugLog returns a copy of c that writes one JSON line per completion
// to w: the route, backend and model, each raw stream payload with its
// finish_reason, and the sizes of the assembled message. A nil w returns c
// unchanged, so completions write nothing.
func (c *Client) WithDebugLog(w io.Writer) *Client {
	if w == nil {
		return c
	}
	cp := *c
	cp.debug = &streamLog{w: w}
	return &cp
}

var _ chat.Completer = (*Client)(nil)

// New returns a client for the router at baseURL, such as
// "http://127.0.0.1:8083". A trailing slash is ignored. hc nil means a
// new http.Client with no timeout; callers bound calls with ctx.
func New(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Complete implements chat.Completer.
func (c *Client) Complete(ctx context.Context, req chat.CompletionRequest, emit func(chat.Event)) (msg chat.Message, err error) {
	if emit == nil {
		emit = func(chat.Event) {}
	}
	var rec *debugRecord
	if c.debug != nil {
		rec = &debugRecord{Time: time.Now().UTC().Format(time.RFC3339Nano), Chunks: []debugChunk{}}
		defer func() {
			rec.finish(msg, err)
			c.debug.write(rec)
		}()
	}
	body, err := requestBody(req)
	if err != nil {
		return chat.Message{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return chat.Message{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return chat.Message{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return chat.Message{}, statusError(resp)
	}

	h := resp.Header
	route := h.Get("X-Llmrouter-Route")
	backend := h.Get("X-Llmrouter-Backend")
	model := h.Get("X-Llmrouter-Model")
	emit(chat.Event{Type: chat.EventInfo, Route: route, Backend: backend, Model: model})

	s := streamState{rec: rec}
	streamErr := s.read(ctx, resp.Body, emit)
	calls := s.toolCalls()
	var textErr error
	if streamErr == nil && calls == nil {
		calls, textErr = textToolCalls(s.content.String(), req.Tools)
	}
	msg = chat.Message{
		Role:      chat.RoleAssistant,
		Content:   s.content.String(),
		Reasoning: s.reasoning.String(),
		ToolCalls: calls,
		Route:     route,
		Backend:   backend,
		Model:     model,
	}
	if textErr != nil {
		emit(chat.Event{Type: chat.EventError, Error: textCallError})
	}
	return msg, streamErr
}

// Routes returns the router's route names in the order the router lists them.
func (c *Client) Routes(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/routes", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.do(ctx, httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}
	var out struct {
		Routes []struct {
			Name string `json:"name"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode routes: %w", err)
	}
	names := make([]string, 0, len(out.Routes))
	for _, r := range out.Routes {
		names = append(names, r.Name)
	}
	return names, nil
}

// do sends req. A failure reports the context's error when the context
// ended, and otherwise says the router is unreachable.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("router unreachable: %w", err)
	}
	return resp, nil
}

// statusError reads the start of a non-200 body into an error.
func statusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("router returned %s: %s", resp.Status, body)
}

// requestBody encodes the chat completion request.
func requestBody(req chat.CompletionRequest) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = "auto"
	}
	body := map[string]any{
		"model":    model,
		"stream":   true,
		"messages": wireMessages(req.Messages),
	}
	if len(req.Tools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools))
		for _, t := range req.Tools {
			tools = append(tools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  t.Parameters,
				},
			})
		}
		body["tools"] = tools
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}
	return b, nil
}

// wireMessages converts conversation messages to the router's message shape.
// Reasoning is never sent back.
func wireMessages(msgs []chat.Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.Role == chat.RoleTool:
			out = append(out, map[string]any{
				"role":         chat.RoleTool,
				"tool_call_id": m.ToolCallID,
				"content":      m.Content,
			})
		case m.Role == chat.RoleAssistant && len(m.ToolCalls) > 0:
			calls := make([]map[string]any, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				calls = append(calls, map[string]any{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]any{
						"name":      tc.Name,
						"arguments": tc.Arguments,
					},
				})
			}
			out = append(out, map[string]any{
				"role":       chat.RoleAssistant,
				"content":    m.Content,
				"tool_calls": calls,
			})
		default:
			out = append(out, map[string]any{
				"role":    m.Role,
				"content": m.Content,
			})
		}
	}
	return out
}

// streamChunk is one "data:" payload of the router's stream.
type streamChunk struct {
	Choices []struct {
		FinishReason *string `json:"finish_reason"`
		Delta        struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
	} `json:"choices"`
}

// finishReason returns the first finish_reason the chunk carries, or nil.
func (c streamChunk) finishReason() *string {
	for _, ch := range c.Choices {
		if ch.FinishReason != nil {
			return ch.FinishReason
		}
	}
	return nil
}

// streamState collects the assistant message as its stream is read.
type streamState struct {
	content   strings.Builder
	reasoning strings.Builder
	calls     map[int]*chat.ToolCall
	rec       *debugRecord // nil when the debug log is off
}

// debugRecord is one line of the debug log: a single completion.
type debugRecord struct {
	Time            string       `json:"time"`
	Route           string       `json:"route"`
	Backend         string       `json:"backend"`
	Model           string       `json:"model"`
	Chunks          []debugChunk `json:"chunks"`
	ContentLength   int          `json:"content_length"`
	ReasoningLength int          `json:"reasoning_length"`
	ToolCalls       int          `json:"tool_calls"`
	Error           string       `json:"error,omitempty"`
}

// debugChunk is one raw "data:" payload, as received, with its finish_reason
// (null when the chunk has none).
type debugChunk struct {
	Data         string  `json:"data"`
	FinishReason *string `json:"finish_reason"`
}

// addChunk appends a payload to the record. A nil record does nothing.
func (r *debugRecord) addChunk(data string, finish *string) {
	if r == nil {
		return
	}
	r.Chunks = append(r.Chunks, debugChunk{Data: data, FinishReason: finish})
}

// finish fills the record's summary from the completion's result.
func (r *debugRecord) finish(msg chat.Message, err error) {
	r.Route, r.Backend, r.Model = msg.Route, msg.Backend, msg.Model
	r.ContentLength = len(msg.Content)
	r.ReasoningLength = len(msg.Reasoning)
	r.ToolCalls = len(msg.ToolCalls)
	if err != nil {
		r.Error = err.Error()
	}
}

// write appends the record as one JSON line. A failed write is ignored: the
// log is a diagnostic and must not fail the completion.
func (l *streamLog) write(r *debugRecord) {
	b, err := json.Marshal(r)
	if err != nil {
		return
	}
	b = append(b, '\n')
	l.mu.Lock()
	defer l.mu.Unlock()
	l.w.Write(b)
}

// read consumes the SSE body until [DONE] or the end of the body. A stream
// that ends without [DONE] is not an error.
func (s *streamState) read(ctx context.Context, body io.Reader, emit func(chat.Event)) error {
	br := bufio.NewReader(body)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, rerr := br.ReadString('\n')
		if data, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "data: "); ok {
			done, err := s.apply(data, emit)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			if err := ctx.Err(); err != nil {
				return err
			}
			return fmt.Errorf("reading router stream: %w", rerr)
		}
	}
}

// apply handles one data payload and reports whether the stream is done.
func (s *streamState) apply(data string, emit func(chat.Event)) (bool, error) {
	if data == "[DONE]" {
		s.rec.addChunk(data, nil)
		return true, nil
	}
	var chunk streamChunk
	err := json.Unmarshal([]byte(data), &chunk)
	s.rec.addChunk(data, chunk.finishReason())
	if err != nil {
		return false, fmt.Errorf("decode router stream chunk: %w", err)
	}
	for _, ch := range chunk.Choices {
		d := ch.Delta
		if d.Content != "" {
			s.content.WriteString(d.Content)
			emit(chat.Event{Type: chat.EventContent, Text: d.Content})
		}
		if d.ReasoningContent != "" {
			s.reasoning.WriteString(d.ReasoningContent)
			emit(chat.Event{Type: chat.EventReasoning, Text: d.ReasoningContent})
		}
		for _, tc := range d.ToolCalls {
			if s.calls == nil {
				s.calls = map[int]*chat.ToolCall{}
			}
			call := s.calls[tc.Index]
			if call == nil {
				call = &chat.ToolCall{}
				s.calls[tc.Index] = call
			}
			if tc.ID != "" {
				call.ID = tc.ID
			}
			if tc.Function.Name != "" {
				call.Name = tc.Function.Name
			}
			call.Arguments += tc.Function.Arguments
		}
	}
	return false, nil
}

// toolCalls returns the assembled calls in index order, or nil when there are none.
func (s *streamState) toolCalls() []chat.ToolCall {
	if len(s.calls) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(s.calls))
	for i := range s.calls {
		indexes = append(indexes, i)
	}
	sort.Ints(indexes)
	out := make([]chat.ToolCall, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, *s.calls[i])
	}
	return out
}

// textCallError is the error event for a reply whose text holds a tool call
// that could not be read. The text stays in the reply and no tool runs.
const textCallError = "could not read a tool call in the reply; no tool was run"

const (
	textCallOpen   = "<tool_call>"
	textCallClose  = "</tool_call>"
	textFuncOpen   = "<function="
	textFuncClose  = "</function>"
	textParamOpen  = "<parameter="
	textParamClose = "</parameter>"
)

// textToolCalls reads the tool calls the model wrote as reply text, in the
// Qwen form <tool_call><function=NAME><parameter=KEY>VALUE</parameter>...
// </function></tool_call>. Only a call whose name is in tools converts; a
// well-formed call for another name is skipped. A malformed call returns an
// error and no calls at all. Calls are numbered text-1, text-2, ... in the
// order they converted.
func textToolCalls(content string, tools []chat.ToolSpec) ([]chat.ToolCall, error) {
	var calls []chat.ToolCall
	rest := content
	for {
		i := strings.Index(rest, textCallOpen)
		if i < 0 {
			return calls, nil
		}
		parsed, after, ok := parseTextCall(rest[i+len(textCallOpen):])
		if !ok {
			return nil, errMalformedTextCall
		}
		rest = after
		spec, known := findTool(tools, parsed.name)
		if !known {
			continue
		}
		args, ok := textArguments(spec, parsed.params)
		if !ok {
			return nil, errMalformedTextCall
		}
		b, err := json.Marshal(args)
		if err != nil {
			return nil, errMalformedTextCall
		}
		calls = append(calls, chat.ToolCall{
			ID:        fmt.Sprintf("text-%d", len(calls)+1),
			Name:      parsed.name,
			Arguments: string(b),
		})
	}
}

var errMalformedTextCall = errors.New("malformed text tool call")

// textCall is a well-formed call read from reply text, before its name is checked.
type textCall struct {
	name   string
	params []textParam
}

// textParam is one parameter of a text call. value has its one surrounding
// newline removed.
type textParam struct {
	key   string
	value string
}

// parseTextCall reads one call from s, which starts just after "<tool_call>".
// It returns the call and the text after "</tool_call>". Only whitespace may
// sit between the tags; any other structure is malformed.
func parseTextCall(s string) (textCall, string, bool) {
	var c textCall
	s = strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(s, textFuncOpen) {
		return c, "", false
	}
	s = s[len(textFuncOpen):]
	end := strings.IndexByte(s, '>')
	if end < 0 || !isIdent(s[:end]) {
		return c, "", false
	}
	c.name = s[:end]
	s = s[end+1:]
	for {
		s = strings.TrimLeft(s, " \t\r\n")
		if strings.HasPrefix(s, textFuncClose) {
			s = s[len(textFuncClose):]
			break
		}
		if !strings.HasPrefix(s, textParamOpen) {
			return c, "", false
		}
		s = s[len(textParamOpen):]
		end := strings.IndexByte(s, '>')
		if end < 0 || !isIdent(s[:end]) {
			return c, "", false
		}
		key := s[:end]
		s = s[end+1:]
		j := strings.Index(s, textParamClose)
		if j < 0 {
			return c, "", false
		}
		value := strings.TrimSuffix(strings.TrimPrefix(s[:j], "\n"), "\n")
		c.params = append(c.params, textParam{key: key, value: value})
		s = s[j+len(textParamClose):]
	}
	s = strings.TrimLeft(s, " \t\r\n")
	if !strings.HasPrefix(s, textCallClose) {
		return c, "", false
	}
	return c, s[len(textCallClose):], true
}

// isIdent reports whether s is a non-empty run of ASCII letters, digits and underscores.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_':
		default:
			return false
		}
	}
	return true
}

// findTool returns the spec named name.
func findTool(tools []chat.ToolSpec, name string) (chat.ToolSpec, bool) {
	for _, t := range tools {
		if t.Name == name {
			return t, true
		}
	}
	return chat.ToolSpec{}, false
}

// textArguments builds the JSON arguments of a text call. A parameter's value
// is typed by the schema property of the same name: boolean, integer and
// number values must parse as that type. A property without a type, or a key
// the schema does not name, takes the value as a string. A repeated key is
// malformed.
func textArguments(spec chat.ToolSpec, params []textParam) (map[string]any, bool) {
	var schema struct {
		Properties map[string]struct {
			Type string `json:"type"`
		} `json:"properties"`
	}
	// An unreadable schema leaves every property untyped, so values are strings.
	_ = json.Unmarshal(spec.Parameters, &schema)
	args := make(map[string]any, len(params))
	for _, p := range params {
		if _, dup := args[p.key]; dup {
			return nil, false
		}
		v, ok := typedValue(schema.Properties[p.key].Type, p.value)
		if !ok {
			return nil, false
		}
		args[p.key] = v
	}
	return args, true
}

// typedValue parses value as the JSON type typ. An unknown type keeps the string.
func typedValue(typ, value string) (any, bool) {
	switch typ {
	case "boolean":
		b, err := strconv.ParseBool(value)
		return b, err == nil
	case "integer":
		n, err := strconv.ParseInt(value, 10, 64)
		return n, err == nil
	case "number":
		f, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
			return nil, false
		}
		return f, true
	default:
		return value, true
	}
}
