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

const (
	stageSchema  = `{"type":"string","enum":["refinement","implementation","testing"],"description":"which agent loop the issue is waiting on"}`
	statusSchema = `{"type":"string","enum":["todo","in_progress","done"]}`
)

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
			Description: "Create an issue in a project. New issues start in status todo, in stage refinement unless you pass another stage.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"title":{"type":"string"},
				"description":{"type":"string"},
				"assignee":{"type":"string"},
				"stage":` + stageSchema + `},
				"required":["project_key","title"]}`),
			call: typed(func(ctx context.Context, in core.NewIssue) (any, error) {
				return svc.CreateIssue(ctx, in)
			}),
		},
		{
			Name:        "issue_list",
			Description: "List issues, optionally filtered by project, stage, status and assignee. A loop's queue is its stage with status todo.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"stage":` + stageSchema + `,
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
			Description: "Change an issue. Only the fields you pass are changed; pass an empty assignee to unassign. To hand an issue to the next loop, set the next stage, status todo and an empty assignee. To finish it, set status done.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"title":{"type":"string"},
				"description":{"type":"string"},
				"assignee":{"type":"string"},
				"stage":` + stageSchema + `,
				"status":` + statusSchema + `},
				"required":["id"]}`),
			call: typed(func(ctx context.Context, in struct {
				ID string `json:"id"`
				core.IssuePatch
			}) (any, error) {
				return svc.UpdateIssue(ctx, in.ID, in.IssuePatch)
			}),
		},
		{
			Name:        "issue_claim",
			Description: "Take a todo issue: sets it in_progress and assigns it to you. Fails if another worker already took it, in which case pick a different issue. Always claim before working.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"assignee":{"type":"string","description":"a name that identifies you, the worker"}},
				"required":["id","assignee"]}`),
			call: typed(func(ctx context.Context, in struct {
				ID       string `json:"id"`
				Assignee string `json:"assignee"`
			}) (any, error) {
				return svc.ClaimIssue(ctx, in.ID, in.Assignee)
			}),
		},
		{
			Name:        "comment_add",
			Description: "Add a comment to an issue. Use it to record progress, findings or questions.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"issue_id":{"type":"string","description":"e.g. PLAT-12"},
				"body":{"type":"string"},
				"author":{"type":"string","description":"who is commenting"}},
				"required":["issue_id","body"]}`),
			call: typed(func(ctx context.Context, in core.NewComment) (any, error) {
				return svc.AddComment(ctx, in)
			}),
		},
		{
			Name:        "comment_list",
			Description: "List an issue's comments, oldest first.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"issue_id":{"type":"string"}},
				"required":["issue_id"]}`),
			call: typed(func(ctx context.Context, in struct {
				IssueID string `json:"issue_id"`
			}) (any, error) {
				return svc.ListComments(ctx, in.IssueID)
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
