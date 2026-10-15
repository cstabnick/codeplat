package live_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"llmchat/internal/chat"
)

// agentModel returns the route the agent tests pin. It skips the test when
// LLMCHAT_LIVE_AGENT_MODEL (default "reason") is not a route name the router
// offers, or when the router cannot be reached.
func agentModel(t *testing.T, router string) string {
	t.Helper()
	model := os.Getenv("LLMCHAT_LIVE_AGENT_MODEL")
	if model == "" {
		model = "reason"
	}
	if strings.Contains(model, "/") {
		t.Skipf("LLMCHAT_LIVE_AGENT_MODEL %q is a backend/model string, not a route name", model)
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(strings.TrimRight(router, "/") + "/api/routes")
	if err != nil {
		t.Skipf("router not reachable at %s: %v", router, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("router not reachable at %s: GET /api/routes returned %d", router, resp.StatusCode)
	}
	var list struct {
		Routes []struct {
			Name string `json:"name"`
		} `json:"routes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Skipf("router /api/routes: decode: %v", err)
	}
	names := make([]string, 0, len(list.Routes))
	for _, rt := range list.Routes {
		names = append(names, rt.Name)
	}
	if !slices.Contains(names, model) {
		t.Skipf("route %q is not one of the router's routes %v", model, names)
	}
	return model
}

// createAgentConversation makes a conversation with a model and a workspace.
func createAgentConversation(t *testing.T, client *http.Client, base, model, workspace string) string {
	t.Helper()
	resp := postJSON(t, client, base+"/api/conversations", map[string]string{"model": model, "workspace": workspace})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create conversation: status %d", resp.StatusCode)
	}
	var c chat.Conversation
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode conversation: %v", err)
	}
	return c.ID
}

// streamTurn posts text and returns the turn's events in order. It answers
// every approval event with approve=true while the stream is still open.
func streamTurn(t *testing.T, client *http.Client, base, id, text string) []chat.Event {
	t.Helper()
	resp := postJSON(t, client, base+"/api/conversations/"+id+"/messages", map[string]string{"text": text})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("send message: status %d", resp.StatusCode)
	}
	var events []chat.Event
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		payload, ok := strings.CutPrefix(sc.Text(), "data: ")
		if !ok {
			continue
		}
		var ev chat.Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			t.Fatalf("decode event %q: %v", payload, err)
		}
		events = append(events, ev)
		if ev.Type == chat.EventApproval {
			answer := postJSON(t, client, base+"/api/conversations/"+id+"/approvals/"+ev.CallID, map[string]bool{"approve": true})
			answer.Body.Close()
			if answer.StatusCode != http.StatusNoContent {
				t.Fatalf("approve call %s: status %d", ev.CallID, answer.StatusCode)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read events: %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Type != chat.EventDone {
		t.Fatalf("turn did not end with %q; events: %s", chat.EventDone, describeEvents(events))
	}
	return events
}

// describeEvents lists the events by type and tool name, for failure messages.
func describeEvents(events []chat.Event) string {
	parts := make([]string, 0, len(events))
	for _, ev := range events {
		s := ev.Type
		if ev.Name != "" {
			s += "(" + ev.Name + ")"
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " ")
}

// finalAnswer joins the content events after the last info event.
func finalAnswer(events []chat.Event) string {
	start := 0
	for i, ev := range events {
		if ev.Type == chat.EventInfo {
			start = i
		}
	}
	var b strings.Builder
	for _, ev := range events[start:] {
		if ev.Type == chat.EventContent {
			b.WriteString(ev.Text)
		}
	}
	return b.String()
}

// firstInfo returns the first info event, or fails the test if there is none.
func firstInfo(t *testing.T, events []chat.Event) chat.Event {
	t.Helper()
	for _, ev := range events {
		if ev.Type == chat.EventInfo {
			return ev
		}
	}
	t.Fatalf("no info event; events: %s", describeEvents(events))
	return chat.Event{}
}

func TestAgentReadsFile(t *testing.T) {
	if os.Getenv("LLMCHAT_LIVE") != "1" {
		t.Skip("set LLMCHAT_LIVE=1 to run against a real router")
	}
	router := routerURL()
	model := agentModel(t, router)

	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "secret.txt"), []byte("The magic word is plugh-7421."), 0o644); err != nil {
		t.Fatalf("write secret.txt: %v", err)
	}

	srv := startApp(t, router)
	client := &http.Client{Timeout: 10 * time.Minute}
	id := createAgentConversation(t, client, srv.URL, model, workspace)
	events := streamTurn(t, client, srv.URL, id, "Read the file secret.txt and tell me the magic word.")

	info := firstInfo(t, events)
	var readCallID string
	for _, ev := range events {
		switch ev.Type {
		case chat.EventError:
			t.Errorf("error event: %s", ev.Error)
		case chat.EventApproval:
			t.Errorf("unexpected approval for %s; read_file should not need one", ev.Name)
		case chat.EventToolCall:
			if ev.Name == "read_file" && readCallID == "" {
				readCallID = ev.CallID
			}
		}
	}
	if readCallID == "" {
		t.Fatalf("model did not call read_file; events: %s", describeEvents(events))
	}
	var sawResult bool
	for _, ev := range events {
		if ev.Type == chat.EventToolResult && ev.CallID == readCallID && strings.Contains(ev.Text, "plugh-7421") {
			sawResult = true
		}
	}
	if !sawResult {
		t.Fatalf("no tool_result for read_file contained plugh-7421; events: %s", describeEvents(events))
	}
	answer := finalAnswer(events)
	if !strings.Contains(answer, "plugh-7421") {
		t.Fatalf("final answer %q does not contain plugh-7421", answer)
	}
	t.Logf("route=%s backend=%s model=%s", info.Route, info.Backend, info.Model)
	t.Logf("answer: %s", answer)
}

func TestAgentWritesFile(t *testing.T) {
	if os.Getenv("LLMCHAT_LIVE") != "1" {
		t.Skip("set LLMCHAT_LIVE=1 to run against a real router")
	}
	router := routerURL()
	model := agentModel(t, router)

	workspace := t.TempDir()
	srv := startApp(t, router)
	client := &http.Client{Timeout: 10 * time.Minute}
	id := createAgentConversation(t, client, srv.URL, model, workspace)
	events := streamTurn(t, client, srv.URL, id, "Create a file named out.txt containing exactly the text ok.")

	info := firstInfo(t, events)
	var sawWriteApproval bool
	for _, ev := range events {
		switch ev.Type {
		case chat.EventError:
			t.Errorf("error event: %s", ev.Error)
		case chat.EventApproval:
			if ev.Name == "write_file" {
				sawWriteApproval = true
			}
		}
	}
	if !sawWriteApproval {
		t.Fatalf("model did not ask to run write_file; events: %s", describeEvents(events))
	}
	data, err := os.ReadFile(filepath.Join(workspace, "out.txt"))
	if err != nil {
		t.Fatalf("read out.txt: %v; events: %s", err, describeEvents(events))
	}
	if got := strings.TrimSpace(string(data)); got != "ok" {
		t.Fatalf("out.txt contains %q, want %q", got, "ok")
	}
	t.Logf("route=%s backend=%s model=%s", info.Route, info.Backend, info.Model)
	t.Logf("answer: %s", finalAnswer(events))
}
