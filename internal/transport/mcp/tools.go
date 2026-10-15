package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"codeplat/internal/core"
)

// Tool is one MCP tool. Call returns a value that is sent as indented
// JSON, or as is when it is a string.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`

	Call func(ctx context.Context, args json.RawMessage) (any, error) `json:"-"`
}

const (
	stageSchema  = `{"type":"string","enum":["refinement","implementation","testing"],"description":"which agent loop the issue is waiting on"}`
	statusSchema = `{"type":"string","enum":["todo","in_progress","done","blocked"]}`
)

func tools(svc *core.Service) []Tool {
	return []Tool{
		{
			Name:        "project_create",
			Description: "Create a project. Issues in it are numbered KEY-1, KEY-2, and so on.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"key":{"type":"string","description":"2-10 letters or digits starting with a letter, e.g. PLAT"},
				"name":{"type":"string"}},
				"required":["key","name"]}`),
			Call: Typed(func(ctx context.Context, in core.NewProject) (any, error) {
				return svc.CreateProject(ctx, in)
			}),
		},
		{
			Name:        "project_list",
			Description: "List all projects.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
			Call: Typed(func(ctx context.Context, _ struct{}) (any, error) {
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
				"stage":` + stageSchema + `,
				"parent":{"type":"string","description":"ID of the goal this issue is part of, e.g. PLAT-2"},
				"epic":{"type":"integer","minimum":0,"description":"epic number within the issue's parent, 0 for none"},
				"depends_on":{"type":"array","items":{"type":"string"},"description":"IDs of issues that must be done first, e.g. PLAT-4"}},
				"required":["project_key","title"]}`),
			Call: Typed(func(ctx context.Context, in core.NewIssue) (any, error) {
				return svc.CreateIssue(ctx, in)
			}),
		},
		{
			Name:        "issue_list",
			Description: "List issues, optionally filtered by project, stage, status and assignee. A loop's queue is its stage with status todo and ready true. It returns 50 at a time by default; `limit` (at most 200) and `offset` read further pages.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"stage":` + stageSchema + `,
				"status":` + statusSchema + `,
				"assignee":{"type":"string"},
				"parent":{"type":"string","description":"ID of the goal this issue is part of, e.g. PLAT-2"},
				"limit":{"type":"integer","minimum":1,"maximum":200,"description":"at most this many issues; default 50"},
				"offset":{"type":"integer","minimum":0,"description":"skip this many issues"},
				"ready":{"type":"boolean","description":"true keeps only issues that nothing blocks"}}}`),
			Call: Typed(func(ctx context.Context, in core.IssueFilter) (any, error) {
				return svc.ListIssues(ctx, in)
			}),
		},
		{
			Name:        "issue_get",
			Description: "Get one issue by ID, e.g. PLAT-12.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"}},
				"required":["id"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				ID string `json:"id"`
			}) (any, error) {
				return svc.GetIssue(ctx, in.ID)
			}),
		},
		{
			Name:        "issue_update",
			Description: "Change an issue. Only the fields you pass are changed; pass an empty assignee to unassign. To hand an issue to the next loop, set the next stage, status todo and an empty assignee. To finish it, set status done. To mark an issue you cannot finish, use issue_block instead, which records why. Set epic to a number and depends_on to a list of issue IDs; pass an empty list ([]) for depends_on to clear them. Pass an empty parent to take the issue out of its goal.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"title":{"type":"string"},
				"description":{"type":"string"},
				"assignee":{"type":"string"},
				"stage":` + stageSchema + `,
				"status":` + statusSchema + `,
				"parent":{"type":"string","description":"ID of the goal this issue is part of, e.g. PLAT-2"},
				"epic":{"type":"integer","minimum":0,"description":"epic number within the issue's parent, 0 for none"},
				"depends_on":{"type":"array","items":{"type":"string"},"description":"IDs of issues that must be done first, e.g. PLAT-4"}},
				"required":["id"]}`),
			Call: Typed(func(ctx context.Context, in struct {
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
			Call: Typed(func(ctx context.Context, in struct {
				ID       string `json:"id"`
				Assignee string `json:"assignee"`
			}) (any, error) {
				return svc.ClaimIssue(ctx, in.ID, in.Assignee)
			}),
		},
		{
			Name:        "issue_block",
			Description: "Mark an issue you cannot finish as blocked. It keeps its assignee, so no other worker takes it, and it leaves every queue. The reason is recorded as a comment starting BLOCKED:. Use it instead of stopping without a trace.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"id":{"type":"string"},
				"author":{"type":"string","description":"the worker that is blocked"},
				"reason":{"type":"string","description":"why the issue cannot be finished"}},
				"required":["id","author","reason"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				ID     string `json:"id"`
				Author string `json:"author"`
				Reason string `json:"reason"`
			}) (any, error) {
				return svc.BlockIssue(ctx, in.ID, in.Author, in.Reason)
			}),
		},
		{
			Name:        "epic_set",
			Description: "Name an epic, or rename it. An epic is a numbered group of issues under one goal; pass the goal's ID as parent, or leave it out for a project's issues that have no goal.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"},
				"parent":{"type":"string","description":"ID of the goal the epic belongs to, e.g. PLAT-2; leave out for none"},
				"number":{"type":"integer","minimum":1,"description":"epic number within the parent"},
				"name":{"type":"string"}},
				"required":["project_key","number","name"]}`),
			Call: Typed(func(ctx context.Context, in core.Epic) (any, error) {
				return svc.SetEpic(ctx, in)
			}),
		},
		{
			Name:        "epic_list",
			Description: "List the named epics of a project, ordered by parent and number.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"project_key":{"type":"string"}},
				"required":["project_key"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				ProjectKey string `json:"project_key"`
			}) (any, error) {
				return svc.ListEpics(ctx, in.ProjectKey)
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
			Call: Typed(func(ctx context.Context, in core.NewComment) (any, error) {
				return svc.AddComment(ctx, in)
			}),
		},
		{
			Name:        "comment_list",
			Description: "List an issue's comments, oldest first. It returns 50 at a time by default; `limit` (at most 200) and `offset` read further pages.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"issue_id":{"type":"string"},
				"limit":{"type":"integer","minimum":1,"maximum":200,"description":"at most this many comments; default 50"},
				"offset":{"type":"integer","minimum":0,"description":"skip this many comments"}},
				"required":["issue_id"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				IssueID string `json:"issue_id"`
				core.Page
			}) (any, error) {
				return svc.ListCommentsPage(ctx, in.IssueID, in.Page)
			}),
		},
	}
}

// Typed adapts a handler with a concrete argument struct to the raw JSON
// arguments of a tools/call request.
func Typed[In any](fn func(context.Context, In) (any, error)) func(context.Context, json.RawMessage) (any, error) {
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
