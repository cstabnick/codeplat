package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// fakeStore is an in-memory chat.Store.
type fakeStore struct {
	nextID int
	convs  map[string]chat.Conversation
	msgs   map[string][]chat.Message
}

func newFakeStore() *fakeStore {
	return &fakeStore{convs: map[string]chat.Conversation{}, msgs: map[string][]chat.Message{}}
}

func (s *fakeStore) CreateConversation(ctx context.Context, c chat.Conversation) (chat.Conversation, error) {
	s.nextID++
	c.ID = fmt.Sprintf("c%d", s.nextID)
	now := time.Now()
	c.CreatedAt, c.UpdatedAt = now, now
	s.convs[c.ID] = c
	return c, nil
}

func (s *fakeStore) GetConversation(ctx context.Context, id string) (chat.Conversation, error) {
	c, ok := s.convs[id]
	if !ok {
		return chat.Conversation{}, chat.ErrNotFound
	}
	return c, nil
}

func (s *fakeStore) ListConversations(ctx context.Context) ([]chat.Conversation, error) {
	out := []chat.Conversation{}
	for _, c := range s.convs {
		out = append(out, c)
	}
	return out, nil
}

func (s *fakeStore) UpdateConversation(ctx context.Context, id string, p chat.ConversationPatch) (chat.Conversation, error) {
	c, err := s.GetConversation(ctx, id)
	if err != nil {
		return chat.Conversation{}, err
	}
	if p.Title != nil {
		c.Title = *p.Title
	}
	if p.Model != nil {
		c.Model = *p.Model
	}
	if p.Workspace != nil {
		c.Workspace = *p.Workspace
	}
	c.UpdatedAt = time.Now()
	s.convs[id] = c
	return c, nil
}

func (s *fakeStore) DeleteConversation(ctx context.Context, id string) error {
	delete(s.convs, id)
	delete(s.msgs, id)
	return nil
}

func (s *fakeStore) AppendMessage(ctx context.Context, conversationID string, m chat.Message) (chat.Message, error) {
	if _, err := s.GetConversation(ctx, conversationID); err != nil {
		return chat.Message{}, err
	}
	m.Seq = len(s.msgs[conversationID]) + 1
	m.CreatedAt = time.Now()
	s.msgs[conversationID] = append(s.msgs[conversationID], m)
	return m, nil
}

func (s *fakeStore) ListMessages(ctx context.Context, conversationID string) ([]chat.Message, error) {
	if _, err := s.GetConversation(ctx, conversationID); err != nil {
		return nil, err
	}
	out := []chat.Message{}
	out = append(out, s.msgs[conversationID]...)
	return out, nil
}

// fakeCompleter returns scripted replies in order and records each request.
type fakeCompleter struct {
	replies []chat.Message
	err     error
	reqs    []chat.CompletionRequest
}

func (c *fakeCompleter) Complete(ctx context.Context, req chat.CompletionRequest, emit func(chat.Event)) (chat.Message, error) {
	c.reqs = append(c.reqs, req)
	if c.err != nil {
		return chat.Message{}, c.err
	}
	if len(c.replies) == 0 {
		return chat.Message{}, errors.New("no scripted reply left")
	}
	r := c.replies[0]
	c.replies = c.replies[1:]
	r.Role = chat.RoleAssistant
	emit(chat.Event{Type: chat.EventInfo, Route: r.Route, Backend: r.Backend, Model: r.Model})
	emit(chat.Event{Type: chat.EventContent, Text: r.Content})
	return r, nil
}

// fakeApprover returns a set answer and records its requests.
type fakeApprover struct {
	answer bool
	err    error
	reqs   []chat.ApprovalRequest
}

func (a *fakeApprover) Approve(ctx context.Context, r chat.ApprovalRequest) (bool, error) {
	a.reqs = append(a.reqs, r)
	return a.answer, a.err
}

// fakeTool is a chat.Tool with configurable name, approval, result and error.
type fakeTool struct {
	name          string
	needsApproval bool
	result        string
	err           error
	runs          []toolRun
}

type toolRun struct {
	workspace string
	args      string
}

func (t *fakeTool) Spec() chat.ToolSpec {
	return chat.ToolSpec{Name: t.name, Description: "test tool", Parameters: json.RawMessage(`{"type":"object"}`)}
}

func (t *fakeTool) NeedsApproval(args json.RawMessage) bool {
	return t.needsApproval
}

func (t *fakeTool) Run(ctx context.Context, workspace string, args json.RawMessage) (string, error) {
	t.runs = append(t.runs, toolRun{workspace: workspace, args: string(args)})
	return t.result, t.err
}

type fixture struct {
	store     *fakeStore
	completer *fakeCompleter
	approver  *fakeApprover
	tool      *fakeTool
	eng       *Engine
}

func newFixture(opts Options, replies ...chat.Message) *fixture {
	f := &fixture{
		store:     newFakeStore(),
		completer: &fakeCompleter{replies: replies},
		approver:  &fakeApprover{},
		tool:      &fakeTool{name: "echo", result: "out"},
	}
	f.eng = New(f.store, f.completer, []chat.Tool{f.tool}, f.approver, opts)
	return f
}

func (f *fixture) newConv(t *testing.T, c chat.Conversation) string {
	t.Helper()
	conv, err := f.store.CreateConversation(context.Background(), c)
	if err != nil {
		t.Fatal(err)
	}
	return conv.ID
}

func (f *fixture) run(t *testing.T, id, text string) ([]chat.Event, error) {
	t.Helper()
	var evs []chat.Event
	err := f.eng.Run(context.Background(), id, text, func(e chat.Event) { evs = append(evs, e) })
	return evs, err
}

func (f *fixture) messages(t *testing.T, id string) []chat.Message {
	t.Helper()
	msgs, err := f.store.ListMessages(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return msgs
}

func eventTypes(evs []chat.Event) []string {
	var out []string
	for _, e := range evs {
		out = append(out, e.Type)
	}
	return out
}

func sameTypes(t *testing.T, got []chat.Event, want ...string) {
	t.Helper()
	g := eventTypes(got)
	if fmt.Sprint(g) != fmt.Sprint(want) {
		t.Fatalf("event types = %v, want %v", g, want)
	}
}

func toolCall(id, name, args string) chat.ToolCall {
	return chat.ToolCall{ID: id, Name: name, Arguments: args}
}

func TestPlainTurnStoresMessagesAndSetsTitle(t *testing.T) {
	f := newFixture(Options{}, chat.Message{Content: "hi there", Route: "r", Backend: "b", Model: "m"})
	id := f.newConv(t, chat.Conversation{Model: "auto"})

	evs, err := f.run(t, id, "hello")
	if err != nil {
		t.Fatal(err)
	}
	sameTypes(t, evs, chat.EventInfo, chat.EventContent, chat.EventDone)

	msgs := f.messages(t, id)
	if len(msgs) != 2 || msgs[0].Role != chat.RoleUser || msgs[0].Content != "hello" ||
		msgs[1].Role != chat.RoleAssistant || msgs[1].Content != "hi there" {
		t.Fatalf("stored messages = %+v", msgs)
	}
	conv, _ := f.store.GetConversation(context.Background(), id)
	if conv.Title != "hello" {
		t.Fatalf("title = %q, want %q", conv.Title, "hello")
	}
}

func TestTitleIsFirstLineCut(t *testing.T) {
	f := newFixture(Options{}, chat.Message{Content: "a"}, chat.Message{Content: "b"})
	id := f.newConv(t, chat.Conversation{Model: "auto"})
	long := strings.Repeat("x", 70)
	if _, err := f.run(t, id, long+"\nsecond line"); err != nil {
		t.Fatal(err)
	}
	conv, _ := f.store.GetConversation(context.Background(), id)
	if want := strings.Repeat("x", 60) + "…"; conv.Title != want {
		t.Fatalf("title = %q, want %q", conv.Title, want)
	}
}

func TestSecondTurnKeepsTitle(t *testing.T) {
	f := newFixture(Options{}, chat.Message{Content: "one"}, chat.Message{Content: "two"})
	id := f.newConv(t, chat.Conversation{Model: "auto"})
	if _, err := f.run(t, id, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, id, "something else"); err != nil {
		t.Fatal(err)
	}
	conv, _ := f.store.GetConversation(context.Background(), id)
	if conv.Title != "first" {
		t.Fatalf("title = %q, want %q", conv.Title, "first")
	}
}

func TestEmptyTextIsInvalid(t *testing.T) {
	f := newFixture(Options{})
	id := f.newConv(t, chat.Conversation{Model: "auto"})
	_, err := f.run(t, id, "   \n\t ")
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if n := len(f.messages(t, id)); n != 0 {
		t.Fatalf("stored %d messages for empty text", n)
	}
}

func TestUnknownConversationIsNotFound(t *testing.T) {
	f := newFixture(Options{})
	_, err := f.run(t, "nope", "hello")
	if !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestSystemPromptIsFirstAndNotStored(t *testing.T) {
	t.Run("no workspace", func(t *testing.T) {
		f := newFixture(Options{System: "be brief"}, chat.Message{Content: "ok"})
		id := f.newConv(t, chat.Conversation{Model: "auto"})
		if _, err := f.run(t, id, "hello"); err != nil {
			t.Fatal(err)
		}
		msgs := f.completer.reqs[0].Messages
		if len(msgs) < 2 || msgs[0].Role != chat.RoleSystem || msgs[0].Content != "be brief" {
			t.Fatalf("request messages = %+v", msgs)
		}
		for _, m := range f.messages(t, id) {
			if m.Role == chat.RoleSystem {
				t.Fatalf("system prompt was stored: %+v", m)
			}
		}
	})
	t.Run("with workspace", func(t *testing.T) {
		f := newFixture(Options{System: "be brief"}, chat.Message{Content: "ok"})
		id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})
		if _, err := f.run(t, id, "hello"); err != nil {
			t.Fatal(err)
		}
		msgs := f.completer.reqs[0].Messages
		want := "be brief\n\nWorkspace directory: /ws"
		if msgs[0].Role != chat.RoleSystem || msgs[0].Content != want {
			t.Fatalf("system = %+v, want %q", msgs[0], want)
		}
	})
}

func TestToolsOfferedOnlyWithWorkspace(t *testing.T) {
	f := newFixture(Options{}, chat.Message{Content: "a"}, chat.Message{Content: "b"})
	off := f.newConv(t, chat.Conversation{Model: "auto"})
	on := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})
	if _, err := f.run(t, off, "hi"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.run(t, on, "hi"); err != nil {
		t.Fatal(err)
	}
	if n := len(f.completer.reqs[0].Tools); n != 0 {
		t.Fatalf("tools without workspace = %d, want 0", n)
	}
	tools := f.completer.reqs[1].Tools
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools with workspace = %+v", tools)
	}
}

func TestToolCallWithoutApprovalRuns(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{"x":1}`)}},
		chat.Message{Content: "done"},
	)
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "run it")
	if err != nil {
		t.Fatal(err)
	}
	sameTypes(t, evs,
		chat.EventInfo, chat.EventContent, // first reply
		chat.EventToolCall, chat.EventToolResult,
		chat.EventInfo, chat.EventContent, // second reply
		chat.EventDone)
	if evs[2].CallID != "c1" || evs[2].Name != "echo" || evs[2].Arguments != `{"x":1}` {
		t.Fatalf("tool_call event = %+v", evs[2])
	}
	if evs[3].CallID != "c1" || evs[3].Text != "out" {
		t.Fatalf("tool_result event = %+v", evs[3])
	}

	msgs := f.messages(t, id)
	if len(msgs) != 4 {
		t.Fatalf("stored %d messages, want 4: %+v", len(msgs), msgs)
	}
	if msgs[1].Role != chat.RoleAssistant || len(msgs[1].ToolCalls) != 1 {
		t.Fatalf("assistant tool message = %+v", msgs[1])
	}
	if msgs[2].Role != chat.RoleTool || msgs[2].ToolCallID != "c1" || msgs[2].Content != "out" {
		t.Fatalf("tool message = %+v", msgs[2])
	}
	if msgs[3].Role != chat.RoleAssistant || msgs[3].Content != "done" {
		t.Fatalf("final message = %+v", msgs[3])
	}

	second := f.completer.reqs[1].Messages
	last := second[len(second)-1] // the tool message is the newest message in the second request
	if last.Role != chat.RoleTool || last.ToolCallID != "c1" || last.Content != "out" {
		t.Fatalf("second request tool message = %+v", last)
	}
	if len(f.approver.reqs) != 0 {
		t.Fatalf("approver asked for a tool that needs no approval")
	}
	if len(f.tool.runs) != 1 || f.tool.runs[0].workspace != "/ws" || f.tool.runs[0].args != `{"x":1}` {
		t.Fatalf("tool runs = %+v", f.tool.runs)
	}
}

func TestApprovalTrueRunsTool(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "done"},
	)
	f.tool.needsApproval = true
	f.approver.answer = true
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "run it")
	if err != nil {
		t.Fatal(err)
	}
	sameTypes(t, evs,
		chat.EventInfo, chat.EventContent,
		chat.EventToolCall, chat.EventApproval, chat.EventToolResult,
		chat.EventInfo, chat.EventContent,
		chat.EventDone)
	if len(f.approver.reqs) != 1 {
		t.Fatalf("approver requests = %+v", f.approver.reqs)
	}
	r := f.approver.reqs[0]
	if r.ConversationID != id || r.CallID != "c1" || r.Name != "echo" || r.Arguments != `{}` {
		t.Fatalf("approval request = %+v", r)
	}
	if len(f.tool.runs) != 1 {
		t.Fatalf("tool ran %d times, want 1", len(f.tool.runs))
	}
	if got := f.messages(t, id)[2].Content; got != "out" {
		t.Fatalf("tool result = %q, want out", got)
	}
}

func TestApprovalFalseDenies(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok"},
	)
	f.tool.needsApproval = true
	f.approver.answer = false
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "run it")
	if err != nil {
		t.Fatal(err)
	}
	sameTypes(t, evs,
		chat.EventInfo, chat.EventContent,
		chat.EventToolCall, chat.EventApproval, chat.EventToolResult,
		chat.EventInfo, chat.EventContent,
		chat.EventDone)
	if len(f.tool.runs) != 0 {
		t.Fatalf("denied tool ran %d times", len(f.tool.runs))
	}
	if got := evs[4].Text; got != "denied by the user" {
		t.Fatalf("tool_result text = %q", got)
	}
	if got := f.messages(t, id)[2].Content; got != "denied by the user" {
		t.Fatalf("stored tool result = %q", got)
	}
}

func TestApproverErrorIsReturned(t *testing.T) {
	boom := errors.New("boom")
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
	)
	f.tool.needsApproval = true
	f.approver.err = boom
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "run it")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	for _, e := range evs {
		if e.Type == chat.EventDone {
			t.Fatalf("EventDone emitted on failure")
		}
	}
	if len(f.tool.runs) != 0 {
		t.Fatalf("tool ran after approver error")
	}
}

func TestUnknownToolResult(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "nope", `{}`)}},
		chat.Message{Content: "ok"},
	)
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	if _, err := f.run(t, id, "run it"); err != nil {
		t.Fatal(err)
	}
	want := `error: unknown tool "nope"`
	if got := f.messages(t, id)[2].Content; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
	if len(f.tool.runs) != 0 || len(f.approver.reqs) != 0 {
		t.Fatalf("unknown tool triggered run or approval")
	}
}

func TestToolErrorResult(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok"},
	)
	f.tool.err = errors.New("bad path")
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	if _, err := f.run(t, id, "run it"); err != nil {
		t.Fatal(err)
	}
	if got := f.messages(t, id)[2].Content; got != "error: bad path" {
		t.Fatalf("tool result = %q", got)
	}
}

func TestLongResultIsCut(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok"},
	)
	f.tool.result = strings.Repeat("a", 25000)
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "run it")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("a", 20000) + "\n[truncated]"
	if got := f.messages(t, id)[2].Content; got != want {
		t.Fatalf("stored result length = %d, want %d", len(got), len(want))
	}
	for _, e := range evs {
		if e.Type == chat.EventToolResult && e.Text != want {
			t.Fatalf("tool_result event text length = %d, want %d", len(e.Text), len(want))
		}
	}
}

func TestToolsOffWithoutWorkspace(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok"},
	)
	f.tool.needsApproval = true
	id := f.newConv(t, chat.Conversation{Model: "auto"})

	if _, err := f.run(t, id, "run it"); err != nil {
		t.Fatal(err)
	}
	want := "error: tools are off for this conversation (no workspace)"
	if got := f.messages(t, id)[2].Content; got != want {
		t.Fatalf("tool result = %q, want %q", got, want)
	}
	if len(f.tool.runs) != 0 || len(f.approver.reqs) != 0 {
		t.Fatalf("tool ran or approval asked with tools off")
	}
}

func TestAutoPinsModelForTurn(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{Backend: "ollama", Model: "qwen", ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok", Backend: "other", Model: "other"},
	)
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})
	if _, err := f.run(t, id, "run it"); err != nil {
		t.Fatal(err)
	}
	if got := f.completer.reqs[0].Model; got != "auto" {
		t.Fatalf("first request model = %q, want auto", got)
	}
	if got := f.completer.reqs[1].Model; got != "ollama/qwen" {
		t.Fatalf("second request model = %q, want ollama/qwen", got)
	}
}

func TestCodeModelIsNotPinned(t *testing.T) {
	f := newFixture(Options{},
		chat.Message{Backend: "ollama", Model: "qwen", ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}},
		chat.Message{Content: "ok"},
	)
	id := f.newConv(t, chat.Conversation{Model: "code", Workspace: "/ws"})
	if _, err := f.run(t, id, "run it"); err != nil {
		t.Fatal(err)
	}
	for i, req := range f.completer.reqs {
		if req.Model != "code" {
			t.Fatalf("request %d model = %q, want code", i, req.Model)
		}
	}
}

func TestMaxStepsStopsTurn(t *testing.T) {
	calls := chat.Message{ToolCalls: []chat.ToolCall{toolCall("c1", "echo", `{}`)}}
	f := newFixture(Options{MaxSteps: 2}, calls, calls)
	id := f.newConv(t, chat.Conversation{Model: "auto", Workspace: "/ws"})

	evs, err := f.run(t, id, "loop")
	if err != nil {
		t.Fatal(err)
	}
	if n := len(f.completer.reqs); n != 2 {
		t.Fatalf("model calls = %d, want 2", n)
	}
	k := len(evs)
	if k < 2 || evs[k-2].Type != chat.EventError || evs[k-2].Error != "stopped after 2 steps" ||
		evs[k-1].Type != chat.EventDone {
		t.Fatalf("last events = %+v", evs[max(0, k-2):])
	}
}

func TestDefaultMaxStepsIs25(t *testing.T) {
	f := newFixture(Options{})
	if f.eng.opts.MaxSteps != 25 {
		t.Fatalf("MaxSteps = %d, want 25", f.eng.opts.MaxSteps)
	}
}

func TestCompleterErrorIsReturned(t *testing.T) {
	boom := errors.New("router down")
	f := newFixture(Options{})
	f.completer.err = boom
	id := f.newConv(t, chat.Conversation{Model: "auto"})

	evs, err := f.run(t, id, "hello")
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want router down", err)
	}
	for _, e := range evs {
		if e.Type == chat.EventDone {
			t.Fatalf("EventDone emitted on failure")
		}
	}
	msgs := f.messages(t, id)
	if len(msgs) != 1 || msgs[0].Role != chat.RoleUser {
		t.Fatalf("stored messages = %+v, want only the user message", msgs)
	}
}

func TestCancelledContextReturnsCanceled(t *testing.T) {
	f := newFixture(Options{}, chat.Message{Content: "ok"})
	id := f.newConv(t, chat.Conversation{Model: "auto"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := f.eng.Run(ctx, id, "hello", func(chat.Event) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n := len(f.completer.reqs); n != 0 {
		t.Fatalf("completer called %d times after cancel", n)
	}
}
