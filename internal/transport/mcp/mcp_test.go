package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	}, "\n") + "\n"

	var out bytes.Buffer
	if err := mcp.NewServer(a.Service, a.Log).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	// Every line gets a reply except the notification.
	if len(lines) != 19 {
		t.Fatalf("got %d replies, want 19:\n%s", len(lines), &out)
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
	if len(tools) != 9 {
		t.Errorf("got %d tools, want 9", len(tools))
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
}
