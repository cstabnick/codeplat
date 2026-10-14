package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"codeplat/internal/apptest"
	"codeplat/internal/core"
	"codeplat/internal/transport/mcp"
)

type reply struct {
	ID     *int `json:"id"`
	Result struct {
		ProtocolVersion string `json:"protocolVersion"`
		Tools           []struct {
			Name        string         `json:"name"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	} `json:"result"`
	Error *struct {
		Code int `json:"code"`
	} `json:"error"`
}

func TestMCP(t *testing.T) {
	a := apptest.New(t)

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"project_create","arguments":{"key":"plat","name":"Platform"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"issue_create","arguments":{"project_key":"PLAT","title":"First"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"issue_update","arguments":{"id":"PLAT-1","status":"done","assignee":"sam"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"issue_list","arguments":{"status":"done"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"issue_get","arguments":{"id":"PLAT-9"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"project_list"}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"comment_add","arguments":{"issue_id":"plat-1","body":"on it","author":"haiku"}}}`,
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"comment_list","arguments":{"issue_id":"PLAT-1"}}}`,
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"comment_add","arguments":{"issue_id":"PLAT-1","body":" "}}}`,
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"issue_create","arguments":{"project_key":"PLAT","title":"Second","stage":"implementation"}}}`,
		`{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"issue_claim","arguments":{"id":"PLAT-2","assignee":"haiku-1"}}}`,
		`{"jsonrpc":"2.0","id":17,"method":"tools/call","params":{"name":"issue_claim","arguments":{"id":"PLAT-2","assignee":"haiku-2"}}}`,
		`{"jsonrpc":"2.0","id":18,"method":"tools/call","params":{"name":"issue_list","arguments":{"stage":"implementation","status":"in_progress"}}}`,
		`not json`,
		`{"jsonrpc":"2.0","id":11,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":19,"method":"tools/call","params":{"name":"issue_block","arguments":{"id":"PLAT-2","author":"sam","reason":"no spec"}}}`,
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := mcp.NewServer(a.Service, a.Log).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// Every line gets a reply except the notification.
	if len(lines) != 20 {
		t.Fatalf("got %d replies, want 20:\n%s", len(lines), &out)
	}
	replies := make([]reply, len(lines))
	for n, line := range lines {
		if err := json.Unmarshal([]byte(line), &replies[n]); err != nil {
			t.Fatalf("reply %d %q: %v", n, line, err)
		}
	}
	byID := func(id int) reply {
		for _, r := range replies {
			if r.ID != nil && *r.ID == id {
				return r
			}
		}
		t.Fatalf("no reply with id %d", id)
		return reply{}
	}
	text := func(id int) string {
		r := byID(id)
		if r.Error != nil || len(r.Result.Content) != 1 || r.Result.Content[0].Type != "text" {
			t.Fatalf("reply %d is not a single text result: %+v", id, r)
		}
		return r.Result.Content[0].Text
	}

	if v := byID(1).Result.ProtocolVersion; v != "2025-03-26" {
		t.Errorf("negotiated version = %q", v)
	}

	tools := byID(2).Result.Tools
	if len(tools) != 10 {
		t.Errorf("got %d tools, want 10", len(tools))
	}
	for _, tool := range tools {
		if tool.Name != "issue_update" {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		status, _ := props["status"].(map[string]any)
		enum, _ := status["enum"].([]any)
		found := false
		for _, v := range enum {
			if v == "blocked" {
				found = true
			}
		}
		if !found {
			t.Errorf("issue_update status enum = %v, want it to contain blocked", enum)
		}
	}
	for _, tool := range tools {
		if tool.InputSchema["type"] != "object" {
			t.Errorf("tool %s: schema type = %v", tool.Name, tool.InputSchema["type"])
		}
	}

	var issue core.Issue
	if err := json.Unmarshal([]byte(text(5)), &issue); err != nil {
		t.Fatal(err)
	}
	if issue.ID != "PLAT-1" || issue.Status != core.StatusDone || issue.Assignee != "sam" || issue.Title != "First" {
		t.Errorf("updated issue = %+v", issue)
	}

	var issues []core.Issue
	if err := json.Unmarshal([]byte(text(6)), &issues); err != nil || len(issues) != 1 {
		t.Errorf("issue_list = %v, %v", issues, err)
	}

	if r := byID(7); !r.Result.IsError || !strings.Contains(text(7), "not found") {
		t.Errorf("missing issue should be a tool error: %+v", r)
	}
	if !strings.Contains(text(8), "PLAT") {
		t.Errorf("project_list without arguments = %s", text(8))
	}
	if r := byID(9); r.Error == nil || r.Error.Code != -32602 {
		t.Errorf("unknown tool = %+v", r)
	}
	if r := byID(10); r.Error == nil || r.Error.Code != -32601 {
		t.Errorf("unknown method = %+v", r)
	}
	var comments []core.Comment
	if err := json.Unmarshal([]byte(text(13)), &comments); err != nil || len(comments) != 1 || comments[0].Author != "haiku" || comments[0].Number != 1 {
		t.Errorf("comment_list = %+v, %v", comments, err)
	}
	if r := byID(14); !r.Result.IsError || !strings.Contains(text(14), "comment body is required") {
		t.Errorf("blank comment should be a tool error: %+v", r)
	}
	if r := byID(16); r.Result.IsError || !strings.Contains(text(16), `"assignee": "haiku-1"`) {
		t.Errorf("first claim = %s", text(16))
	}
	if r := byID(17); !r.Result.IsError || !strings.Contains(text(17), "not todo") {
		t.Errorf("second claim should be a tool error: %+v", r)
	}
	var inProgress []core.Issue
	if err := json.Unmarshal([]byte(text(18)), &inProgress); err != nil || len(inProgress) != 1 || inProgress[0].ID != "PLAT-2" {
		t.Errorf("stage filter = %+v, %v", inProgress, err)
	}
	if r := replies[17]; r.ID != nil || r.Error == nil || r.Error.Code != -32700 {
		t.Errorf("parse error reply = %+v", r)
	}
	if r := byID(11); r.Error != nil {
		t.Errorf("ping = %+v", r)
	}
	var blocked core.Issue
	if err := json.Unmarshal([]byte(text(19)), &blocked); err != nil {
		t.Fatal(err)
	}
	if blocked.ID != "PLAT-2" || blocked.Status != core.StatusBlocked || blocked.Assignee != "haiku-1" {
		t.Errorf("blocked issue = %+v", blocked)
	}
}

// pagingText returns the single text content of a successful reply and fails
// the test otherwise.
func pagingText(t *testing.T, r reply) string {
	t.Helper()
	if r.Error != nil || r.Result.IsError || len(r.Result.Content) != 1 || r.Result.Content[0].Type != "text" {
		t.Fatalf("reply %v is not a single successful text result: %+v", r.ID, r)
	}
	return r.Result.Content[0].Text
}

func TestMCPPaging(t *testing.T) {
	a := apptest.New(t)

	call := func(id int, name, args string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, args)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		call(3, "project_create", `{"key":"PLAT","name":"Platform"}`),
		call(4, "issue_create", `{"project_key":"PLAT","title":"One"}`),
		call(5, "issue_create", `{"project_key":"PLAT","title":"Two"}`),
		call(6, "issue_create", `{"project_key":"PLAT","title":"Three"}`),
		call(7, "comment_add", `{"issue_id":"PLAT-1","body":"c1","author":"sam"}`),
		call(8, "comment_add", `{"issue_id":"PLAT-1","body":"c2","author":"sam"}`),
		call(9, "comment_add", `{"issue_id":"PLAT-1","body":"c3","author":"sam"}`),
		call(10, "issue_list", `{"limit":1,"offset":1}`),
		call(11, "comment_list", `{"issue_id":"PLAT-1","limit":1,"offset":1}`),
		call(12, "issue_list", `{"limit":201}`),
		call(13, "comment_list", `{"issue_id":"PLAT-1","limit":0,"offset":-1}`),
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := mcp.NewServer(a.Service, a.Log).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 13 {
		t.Fatalf("got %d replies, want 13:\n%s", len(lines), &out)
	}
	replies := make([]reply, len(lines))
	for n, line := range lines {
		if err := json.Unmarshal([]byte(line), &replies[n]); err != nil {
			t.Fatalf("reply %d %q: %v", n, line, err)
		}
	}
	// Replies are written in request order, so reply n answers request n+1.
	r := func(id int) reply { return replies[id-1] }

	// issue_list with limit 1 and offset 1 returns only PLAT-2.
	var issues []core.Issue
	if err := json.Unmarshal([]byte(pagingText(t, r(10))), &issues); err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].ID != "PLAT-2" {
		t.Errorf("issue_list limit 1 offset 1 = %+v, want only PLAT-2", issues)
	}

	// comment_list with limit 1 and offset 1 returns only comment number 2.
	var comments []core.Comment
	if err := json.Unmarshal([]byte(pagingText(t, r(11))), &comments); err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 || comments[0].Number != 2 || comments[0].Body != "c2" {
		t.Errorf("comment_list limit 1 offset 1 = %+v, want only comment 2", comments)
	}

	// A limit above the maximum is a tool error from core.
	if got := r(12); got.Error != nil || !got.Result.IsError {
		t.Errorf("issue_list limit 201 should be a tool error: %+v", got)
	} else if !strings.HasPrefix(got.Result.Content[0].Text, "invalid input: ") {
		t.Errorf("issue_list limit 201 text = %q, want prefix %q", got.Result.Content[0].Text, "invalid input: ")
	}

	// A negative offset to comment_list is a tool error from core.
	if got := r(13); got.Error != nil || !got.Result.IsError {
		t.Errorf("comment_list offset -1 should be a tool error: %+v", got)
	}

	// tools/list advertises limit and offset on both tools.
	for _, tool := range r(2).Result.Tools {
		if tool.Name != "issue_list" && tool.Name != "comment_list" {
			continue
		}
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for _, name := range []string{"limit", "offset"} {
			if _, ok := props[name]; !ok {
				t.Errorf("%s schema has no %q property", tool.Name, name)
			}
		}
	}
}

func TestMCPEpicsAndReady(t *testing.T) {
	a := apptest.New(t)

	call := func(id int, name, args string) string {
		return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, args)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		call(3, "project_create", `{"key":"PLAT","name":"Platform"}`),
		call(4, "issue_create", `{"project_key":"PLAT","title":"First"}`),
		call(5, "issue_create", `{"project_key":"PLAT","title":"Second","epic":2,"depends_on":["PLAT-1"]}`),
		call(6, "issue_create", `{"project_key":"PLAT","title":"Third","depends_on":["PLAT-99"]}`),
		call(7, "issue_list", `{"ready":true}`),
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := mcp.NewServer(a.Service, a.Log).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("got %d replies, want 7:\n%s", len(lines), &out)
	}
	replies := make([]reply, len(lines))
	for n, line := range lines {
		if err := json.Unmarshal([]byte(line), &replies[n]); err != nil {
			t.Fatalf("reply %d %q: %v", n, line, err)
		}
	}
	// Replies are written in request order, so reply n answers request n+1.
	r := func(id int) reply { return replies[id-1] }

	// The three tools advertise epic, depends_on and, for issue_list, ready.
	want := map[string][]string{
		"issue_create": {"epic", "depends_on"},
		"issue_update": {"epic", "depends_on"},
		"issue_list":   {"ready"},
	}
	seen := map[string]bool{}
	for _, tool := range r(2).Result.Tools {
		names, ok := want[tool.Name]
		if !ok {
			continue
		}
		seen[tool.Name] = true
		props, _ := tool.InputSchema["properties"].(map[string]any)
		for _, name := range names {
			if _, ok := props[name]; !ok {
				t.Errorf("%s schema has no %q property", tool.Name, name)
			}
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("tools/list has no %s tool", name)
		}
	}

	// An epic and dependencies come back in the created issue.
	var created core.Issue
	if err := json.Unmarshal([]byte(pagingText(t, r(5))), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID != "PLAT-2" || created.Epic != 2 || len(created.DependsOn) != 1 || created.DependsOn[0] != "PLAT-1" {
		t.Errorf("issue_create with epic and depends_on = %+v", created)
	}

	// A dependency that does not exist is a tool error from core.
	if got := r(6); got.Error != nil || !got.Result.IsError || len(got.Result.Content) != 1 || !strings.Contains(got.Result.Content[0].Text, "dependency PLAT-99 does not exist") {
		t.Errorf("issue_create with missing dependency = %+v", got)
	}

	// PLAT-2 waits for PLAT-1, so only PLAT-1 is ready.
	var ready []core.Issue
	if err := json.Unmarshal([]byte(pagingText(t, r(7))), &ready); err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != "PLAT-1" {
		t.Errorf("issue_list ready = %+v, want only PLAT-1", ready)
	}
}
