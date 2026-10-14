package sqlitestore

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// fakeClock advances one second per call.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time {
	c.t = c.t.Add(time.Second)
	return c.t
}

func openTest(t *testing.T) (*Store, *fakeClock) {
	t.Helper()
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	s, err := Open(filepath.Join(t.TempDir(), "chat.db"), clk.now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, clk
}

func strp(s string) *string { return &s }

func TestCreateThenGet(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	c, err := s.CreateConversation(ctx, chat.Conversation{Title: "hi", Model: "m1", Workspace: "/w"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(c.ID) {
		t.Fatalf("ID = %q, want 12 lowercase hex characters", c.ID)
	}
	if c.CreatedAt.IsZero() || !c.UpdatedAt.Equal(c.CreatedAt) {
		t.Fatalf("CreatedAt %v, UpdatedAt %v: want both set and equal", c.CreatedAt, c.UpdatedAt)
	}
	got, err := s.GetConversation(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.ID != c.ID || got.Title != "hi" || got.Model != "m1" || got.Workspace != "/w" {
		t.Fatalf("got %+v, want fields of %+v", got, c)
	}
	if !got.CreatedAt.Equal(c.CreatedAt) || !got.UpdatedAt.Equal(c.UpdatedAt) {
		t.Fatalf("times: got %v/%v, want %v/%v", got.CreatedAt, got.UpdatedAt, c.CreatedAt, c.UpdatedAt)
	}
}

func TestEmptyModelBecomesAuto(t *testing.T) {
	s, _ := openTest(t)
	c, err := s.CreateConversation(context.Background(), chat.Conversation{Title: "x"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if c.Model != "auto" {
		t.Fatalf("Model = %q, want auto", c.Model)
	}
	got, err := s.GetConversation(context.Background(), c.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.Model != "auto" {
		t.Fatalf("stored Model = %q, want auto", got.Model)
	}
}

func TestListOrderFollowsUpdates(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	a, _ := s.CreateConversation(ctx, chat.Conversation{Title: "a"})
	b, _ := s.CreateConversation(ctx, chat.Conversation{Title: "b"})
	c, _ := s.CreateConversation(ctx, chat.Conversation{Title: "c"})

	list, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if got := ids(list); !equal(got, []string{c.ID, b.ID, a.ID}) {
		t.Fatalf("order = %v, want c,b,a", got)
	}

	if _, err := s.AppendMessage(ctx, a.ID, chat.Message{Role: chat.RoleUser, Content: "hello"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	list, err = s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if got := ids(list); !equal(got, []string{a.ID, c.ID, b.ID}) {
		t.Fatalf("after append order = %v, want a,c,b", got)
	}
}

func TestUpdateFields(t *testing.T) {
	ctx := context.Background()
	base := chat.Conversation{Title: "t", Model: "m", Workspace: "/w"}

	cases := []struct {
		name  string
		patch chat.ConversationPatch
		want  chat.Conversation
	}{
		{"title", chat.ConversationPatch{Title: strp("new")}, chat.Conversation{Title: "new", Model: "m", Workspace: "/w"}},
		{"model", chat.ConversationPatch{Model: strp("route")}, chat.Conversation{Title: "t", Model: "route", Workspace: "/w"}},
		{"workspace", chat.ConversationPatch{Workspace: strp("")}, chat.Conversation{Title: "t", Model: "m", Workspace: ""}},
		{"all", chat.ConversationPatch{Title: strp("T"), Model: strp("M"), Workspace: strp("/x")}, chat.Conversation{Title: "T", Model: "M", Workspace: "/x"}},
		{"none", chat.ConversationPatch{}, base},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := openTest(t)
			c, err := s.CreateConversation(ctx, base)
			if err != nil {
				t.Fatalf("CreateConversation: %v", err)
			}
			got, err := s.UpdateConversation(ctx, c.ID, tc.patch)
			if err != nil {
				t.Fatalf("UpdateConversation: %v", err)
			}
			if got.Title != tc.want.Title || got.Model != tc.want.Model || got.Workspace != tc.want.Workspace {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if !got.UpdatedAt.After(c.UpdatedAt) {
				t.Fatalf("UpdatedAt %v not after %v", got.UpdatedAt, c.UpdatedAt)
			}
			if !got.CreatedAt.Equal(c.CreatedAt) {
				t.Fatalf("CreatedAt changed: %v -> %v", c.CreatedAt, got.CreatedAt)
			}
			stored, err := s.GetConversation(ctx, c.ID)
			if err != nil {
				t.Fatalf("GetConversation: %v", err)
			}
			if stored.Title != tc.want.Title || stored.Model != tc.want.Model || stored.Workspace != tc.want.Workspace {
				t.Fatalf("stored %+v, want %+v", stored, tc.want)
			}
		})
	}
}

func TestUnknownIDsWrapNotFound(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	const missing = "nope"
	check := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, chat.ErrNotFound) {
			t.Errorf("%s: err = %v, want chat.ErrNotFound", name, err)
		}
	}
	_, err := s.GetConversation(ctx, missing)
	check("Get", err)
	_, err = s.UpdateConversation(ctx, missing, chat.ConversationPatch{Title: strp("x")})
	check("Update", err)
	check("Delete", s.DeleteConversation(ctx, missing))
	_, err = s.AppendMessage(ctx, missing, chat.Message{Role: chat.RoleUser})
	check("Append", err)
	_, err = s.ListMessages(ctx, missing)
	check("ListMessages", err)
}

func TestAppendAssignsSeq(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	c, _ := s.CreateConversation(ctx, chat.Conversation{})
	for want := 1; want <= 3; want++ {
		m, err := s.AppendMessage(ctx, c.ID, chat.Message{Seq: 99, Role: chat.RoleUser, Content: "x"})
		if err != nil {
			t.Fatalf("AppendMessage: %v", err)
		}
		if m.Seq != want {
			t.Fatalf("Seq = %d, want %d", m.Seq, want)
		}
		if m.CreatedAt.IsZero() {
			t.Fatalf("CreatedAt not set")
		}
	}
}

func TestMessageRoundTrip(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	c, _ := s.CreateConversation(ctx, chat.Conversation{})
	in := chat.Message{
		Role:       chat.RoleAssistant,
		Content:    "answer",
		Reasoning:  "thinking",
		ToolCalls:  []chat.ToolCall{{ID: "c1", Name: "read", Arguments: `{"path":"a"}`}, {ID: "c2", Name: "list", Arguments: `{}`}},
		ToolCallID: "tc",
		Route:      "fast",
		Backend:    "bonsai",
		Model:      "bonsai-8b",
	}
	stored, err := s.AppendMessage(ctx, c.ID, in)
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	msgs, err := s.ListMessages(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1", len(msgs))
	}
	got := msgs[0]
	if got.Seq != stored.Seq || got.Role != in.Role || got.Content != in.Content || got.Reasoning != in.Reasoning ||
		got.ToolCallID != in.ToolCallID || got.Route != in.Route || got.Backend != in.Backend || got.Model != in.Model {
		t.Fatalf("round trip: got %+v, want fields of %+v", got, in)
	}
	if !got.CreatedAt.Equal(stored.CreatedAt) {
		t.Fatalf("CreatedAt %v, want %v", got.CreatedAt, stored.CreatedAt)
	}
	if len(got.ToolCalls) != 2 || got.ToolCalls[0] != in.ToolCalls[0] || got.ToolCalls[1] != in.ToolCalls[1] {
		t.Fatalf("ToolCalls = %+v, want %+v", got.ToolCalls, in.ToolCalls)
	}
}

func TestMessageWithoutToolCallsIsNil(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	c, _ := s.CreateConversation(ctx, chat.Conversation{})
	if _, err := s.AppendMessage(ctx, c.ID, chat.Message{Role: chat.RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	msgs, err := s.ListMessages(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if msgs[0].ToolCalls != nil {
		t.Fatalf("ToolCalls = %+v, want nil", msgs[0].ToolCalls)
	}
}

func TestDeleteRemovesMessages(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	c, _ := s.CreateConversation(ctx, chat.Conversation{})
	if _, err := s.AppendMessage(ctx, c.ID, chat.Message{Role: chat.RoleUser, Content: "hi"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := s.DeleteConversation(ctx, c.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if _, err := s.GetConversation(ctx, c.ID); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("Get after delete: err = %v, want ErrNotFound", err)
	}
	if _, err := s.ListMessages(ctx, c.ID); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("ListMessages after delete: err = %v, want ErrNotFound", err)
	}
	if err := s.DeleteConversation(ctx, c.ID); !errors.Is(err, chat.ErrNotFound) {
		t.Fatalf("second delete: err = %v, want ErrNotFound", err)
	}
}

func TestDataSurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "chat.db")
	clk := &fakeClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}

	s, err := Open(path, clk.now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	c, err := s.CreateConversation(ctx, chat.Conversation{Title: "keep", Model: "auto"})
	if err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if _, err := s.AppendMessage(ctx, c.ID, chat.Message{Role: chat.RoleUser, Content: "persist me"}); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := Open(path, clk.now)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer s2.Close()
	got, err := s2.GetConversation(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConversation after reopen: %v", err)
	}
	if got.Title != "keep" {
		t.Fatalf("Title = %q, want keep", got.Title)
	}
	msgs, err := s2.ListMessages(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListMessages after reopen: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "persist me" {
		t.Fatalf("messages after reopen = %+v", msgs)
	}
}

func TestIDsAreUnique(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		c, err := s.CreateConversation(ctx, chat.Conversation{})
		if err != nil {
			t.Fatalf("CreateConversation: %v", err)
		}
		if len(c.ID) != 12 {
			t.Fatalf("ID %q has length %d, want 12", c.ID, len(c.ID))
		}
		if seen[c.ID] {
			t.Fatalf("duplicate ID %q", c.ID)
		}
		seen[c.ID] = true
	}
}

func TestEmptyListsAreNonNil(t *testing.T) {
	s, _ := openTest(t)
	ctx := context.Background()
	convs, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if convs == nil || len(convs) != 0 {
		t.Fatalf("ListConversations on empty store = %#v, want non-nil empty", convs)
	}
	c, _ := s.CreateConversation(ctx, chat.Conversation{})
	msgs, err := s.ListMessages(ctx, c.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if msgs == nil || len(msgs) != 0 {
		t.Fatalf("ListMessages on empty conversation = %#v, want non-nil empty", msgs)
	}
}

func TestSubSecondOrderAndRoundTrip(t *testing.T) {
	ctx := context.Background()
	stamps := []time.Time{
		time.Date(2026, 1, 1, 10, 0, 5, 0, time.UTC),           // A created
		time.Date(2026, 1, 1, 10, 0, 5, 500_000_000, time.UTC), // B created
		time.Date(2026, 1, 1, 10, 0, 6, 0, time.UTC),           // A updated
	}
	i := 0
	now := func() time.Time {
		ts := stamps[i]
		i++
		return ts
	}
	s, err := Open(filepath.Join(t.TempDir(), "chat.db"), now)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	a, err := s.CreateConversation(ctx, chat.Conversation{Title: "A"})
	if err != nil {
		t.Fatalf("create A: %v", err)
	}
	b, err := s.CreateConversation(ctx, chat.Conversation{Title: "B"})
	if err != nil {
		t.Fatalf("create B: %v", err)
	}
	if want := stamps[0]; !a.CreatedAt.Equal(want) {
		t.Fatalf("A.CreatedAt = %v, want %v", a.CreatedAt, want)
	}
	got, err := s.GetConversation(ctx, b.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if !got.CreatedAt.Equal(stamps[1]) {
		t.Fatalf("B.CreatedAt read back = %v, want %v", got.CreatedAt, stamps[1])
	}

	list, err := s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if want := []string{b.ID, a.ID}; !equal(ids(list), want) {
		t.Fatalf("order before update = %v, want %v (B at 05.500 is newer than A at 05.000)", ids(list), want)
	}

	if _, err := s.UpdateConversation(ctx, a.ID, chat.ConversationPatch{Title: strp("A2")}); err != nil {
		t.Fatalf("update A: %v", err)
	}
	list, err = s.ListConversations(ctx)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if want := []string{a.ID, b.ID}; !equal(ids(list), want) {
		t.Fatalf("order after update = %v, want %v", ids(list), want)
	}
}

func ids(cs []chat.Conversation) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.ID
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
