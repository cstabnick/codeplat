package e2e_test

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmchat/internal/chat"
)

// agentTextCall streams a tool call written as reply text, the way a model
// writes it when it does not use the structured tool_calls field. params holds
// key, value pairs. The text reaches the client as content events.
func agentTextCall(w io.Writer, name string, params ...string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Calling %s.\n<tool_call>\n<function=%s>\n", name, name)
	for i := 0; i+1 < len(params); i += 2 {
		fmt.Fprintf(&b, "<parameter=%s>\n%s\n</parameter>\n", params[i], params[i+1])
	}
	b.WriteString("</function>\n</tool_call>")
	agentText(w, b.String())
}

// A write_file call written as reply text goes through the same approval and
// tool path as a structured call: approved, it writes the file.
func TestAgentTextToolCallRunsThroughApproval(t *testing.T) {
	router := newAgentRouter(t)
	base := agentStart(t, router.URL)

	ws := t.TempDir()
	conv := agentCreate(t, base, ws)

	events := agentSend(t, base, conv.ID, "write text note.txt", agentBool(true))
	if i := agentIndex(events, chat.EventError); i >= 0 {
		t.Fatalf("events = %+v, want no error event for a well-formed text call", events)
	}
	ai, ri := agentIndex(events, chat.EventApproval), agentIndex(events, chat.EventToolResult)
	if ai < 0 || ri < 0 || ai > ri {
		t.Fatalf("events = %+v, want approval before tool_result", events)
	}
	if events[ai].Name != "write_file" || events[ai].CallID != events[ri].CallID {
		t.Fatalf("approval = %+v, tool_result = %+v, want write_file and the same call", events[ai], events[ri])
	}
	if data, err := os.ReadFile(filepath.Join(ws, "note.txt")); err != nil || string(data) != "hi" {
		t.Fatalf("note.txt = %q, %v; want %q", data, err, "hi")
	}
	if got := agentContentOf(events); !strings.Contains(got, "<tool_call>") {
		t.Fatalf("content = %q, want the reply text kept", got)
	}

	// The stored assistant message keeps the text and carries the converted call.
	detail := agentGet(t, base, conv.ID)
	var assistant *chat.Message
	for i := range detail.Messages {
		if detail.Messages[i].Role == chat.RoleAssistant && len(detail.Messages[i].ToolCalls) > 0 {
			assistant = &detail.Messages[i]
			break
		}
	}
	if assistant == nil {
		t.Fatalf("stored messages = %+v, want an assistant message with a tool call", detail.Messages)
	}
	if assistant.ToolCalls[0].Name != "write_file" || !strings.Contains(assistant.Content, "<tool_call>") {
		t.Fatalf("stored assistant = %+v, want write_file and the reply text", assistant)
	}
}
