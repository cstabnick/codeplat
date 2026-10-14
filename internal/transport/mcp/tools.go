package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"codeplat/internal/core"
)

type tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`

	call func(ctx context.Context, args json.RawMessage) (any, error)
}

const statusSchema = `{"type":"string","enum":["todo","in_progress","done"]}`

func tools(svc *core.Service) []tool {
	return []tool{
		{
			Name:        "project_create",
			Description: "Create a project. Issues in it are numbered KEY-1, KEY-2, and so on.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"key":{"type":"string","description":"2-10 letters or digits starting with a letter, e.g. PLAT"},
				"name":{"type":"string"}},
				"required":["key","name"]}`),
			call: typed(func(ctx context.Context, in core.NewProject) (any, error) {
				return svc.CreateProject(ctx, in)
			}),
		},
		{
			Name:        "project_list",
			Description: "List all projects.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			call: typed(func(ctx context.Context, _ struct{}) (any, error) {
				return svc.ListProjects(ctx)
			}),
		},
		{
			Name:        "issue_create",
			Description: "Create an issue in a project. New issues start in status todo.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"title":{"type":"string"},
				"description":{"type":"string"},
				"assignee":{"type":"string"}},
				"required":["project_key","title"]}`),
			call: typed(func(ctx context.Context, in core.NewIssue) (any, error) {
				return svc.CreateIssue(ctx, in)
			}),
		},
		{
			Name:        "issue_list",
			Description: "List issues, optionally filtered by project, status and assignee.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"status":` + statusSchema + `,
				"assignee":{"type":"string"}}}`),
			call: typed(func(ctx context.Context, in core.IssueFilter) (any, error) {
				return svc.ListIssues(ctx, in)
			}),
		},
		{
			Name:        "issue_get",
			Description: "Get one issue by ID, e.g. PLAT-12.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"}},
				"required":["id"]}`),
			call: typed(func(ctx context.Context, in struct {
				ID string `json:"id"`
			}) (any, error) {
				return svc.GetIssue(ctx, in.ID)
			}),
		},
		{
			Name:        "issue_update",
			Description: "Change an issue. Only the fields you pass are changed; pass an empty assignee to unassign.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"title":{"type":"string"},
				"description":{"type":"string"},
				"assignee":{"type":"string"},
				"status":` + statusSchema + `},
				"required":["id"]}`),
			call: typed(func(ctx context.Context, in struct {
				ID string `json:"id"`
				core.IssuePatch
			}) (any, error) {
				return svc.UpdateIssue(ctx, in.ID, in.IssuePatch)
			}),
		},
	}
}

// typed adapts a handler with a concrete argument struct to the raw JSON
// arguments of a tools/call request.
func typed[In any](fn func(context.Context, In) (any, error)) func(context.Context, json.RawMessage) (any, error) {
	return func(ctx context.Context, raw json.RawMessage) (any, error) {
		var in In
		if len(raw) > 0 {
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
		}
		return fn(ctx, in)
	}
}
