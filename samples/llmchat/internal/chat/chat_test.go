package chat

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Compile-time checks that a fake of each interface can be written.
type fakeStore struct{}

func (fakeStore) CreateConversation(ctx context.Context, c Conversation) (Conversation, error) {
	return Conversation{}, nil
}
func (fakeStore) GetConversation(ctx context.Context, id string) (Conversation, error) {
	return Conversation{}, nil
}
func (fakeStore) ListConversations(ctx context.Context) ([]Conversation, error) { return nil, nil }
func (fakeStore) UpdateConversation(ctx context.Context, id string, p ConversationPatch) (Conversation, error) {
	return Conversation{}, nil
}
func (fakeStore) DeleteConversation(ctx context.Context, id string) error { return nil }
func (fakeStore) AppendMessage(ctx context.Context, conversationID string, m Message) (Message, error) {
	return Message{}, nil
}
func (fakeStore) ListMessages(ctx context.Context, conversationID string) ([]Message, error) {
	return nil, nil
}

type fakeCompleter struct{}

func (fakeCompleter) Complete(ctx context.Context, req CompletionRequest, emit func(Event)) (Message, error) {
	return Message{}, nil
}

type fakeApprover struct{}

func (fakeApprover) Approve(ctx context.Context, r ApprovalRequest) (bool, error) {
	return false, nil
}

type fakeTool struct{}

func (fakeTool) Spec() ToolSpec                          { return ToolSpec{} }
func (fakeTool) NeedsApproval(args json.RawMessage) bool { return false }
func (fakeTool) Run(ctx context.Context, workspace string, args json.RawMessage) (string, error) {
	return "", nil
}

var (
	_ Store     = fakeStore{}
	_ Completer = fakeCompleter{}
	_ Approver  = fakeApprover{}
	_ Tool      = fakeTool{}
)

func TestMessageMinimalMarshalOmitsOptionalFields(t *testing.T) {
	m := Message{
		Role:      RoleUser,
		Content:   "hello",
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"seq", "role", "content", "created_at"} {
		if _, ok := got[k]; !ok {
			t.Errorf("key %q missing from %s", k, b)
		}
	}
	for _, k := range []string{"reasoning", "tool_calls", "tool_call_id", "route", "backend", "model"} {
		if _, ok := got[k]; ok {
			t.Errorf("key %q should be omitted, got %s", k, b)
		}
	}
}

func TestMessageToolCallsRoundTrip(t *testing.T) {
	m := Message{
		Seq:       3,
		Role:      RoleAssistant,
		Content:   "",
		ToolCalls: []ToolCall{{ID: "c1", Name: "read_file", Arguments: `{"path":"a.txt"}`}},
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	want := `"tool_calls":[{"id":"c1","name":"read_file","arguments":"{\"path\":\"a.txt\"}"}]`
	if !strings.Contains(string(b), want) {
		t.Errorf("marshal = %s, want it to contain %s", b, want)
	}
	var back Message
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m, back) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", back, m)
	}
}

func TestEventMarshal(t *testing.T) {
	cases := []struct {
		name string
		ev   Event
		want string
	}{
		{"done", Event{Type: EventDone}, `{"type":"done"}`},
		{"content", Event{Type: EventContent, Text: "hi"}, `{"type":"content","text":"hi"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.ev)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != c.want {
				t.Errorf("got %s, want %s", b, c.want)
			}
		})
	}
}

func TestConversationPatchMarshalTitleOnly(t *testing.T) {
	title := "x"
	b, err := json.Marshal(ConversationPatch{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"title":"x"}` {
		t.Errorf("got %s, want {\"title\":\"x\"}", b)
	}
}
