// Package chat holds the types and interfaces shared by every part of
// llmchat. It imports only the standard library.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Errors returned by llmchat's parts. Callers test for them with errors.Is.
var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

// Message roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is a request from the model to run a tool. Arguments is the JSON
// text the model produced.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is one message of a conversation. Seq numbers a conversation's
// messages from 1. An assistant message records how the router chose its
// model in Route, Backend and Model (Route is empty when no route was
// used). A tool message answers the call whose ID is ToolCallID.
type Message struct {
	Seq        int        `json:"seq"`
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Route      string     `json:"route,omitempty"`
	Backend    string     `json:"backend,omitempty"`
	Model      string     `json:"model,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

// Conversation is a saved chat. Model is "auto", a route name, or
// "backend/model". Workspace is the directory the tools may use; empty
// means tools are off for this conversation.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Model     string    `json:"model"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ConversationPatch changes only the fields that are non-nil.
type ConversationPatch struct {
	Title     *string `json:"title,omitempty"`
	Model     *string `json:"model,omitempty"`
	Workspace *string `json:"workspace,omitempty"`
}

// Store is where conversations and their messages are kept.
type Store interface {
	// CreateConversation stores c with a new ID, CreatedAt and UpdatedAt set, and returns it.
	CreateConversation(ctx context.Context, c Conversation) (Conversation, error)
	// GetConversation returns ErrNotFound for an unknown id (also for the methods below).
	GetConversation(ctx context.Context, id string) (Conversation, error)
	// ListConversations returns every conversation, most recently updated first. Never nil.
	ListConversations(ctx context.Context) ([]Conversation, error)
	// UpdateConversation applies the non-nil fields of p and sets UpdatedAt.
	UpdateConversation(ctx context.Context, id string, p ConversationPatch) (Conversation, error)
	// DeleteConversation removes the conversation and its messages.
	DeleteConversation(ctx context.Context, id string) error
	// AppendMessage stores m as the conversation's next message (Seq is the
	// previous Seq plus 1; any Seq in m is ignored), sets CreatedAt, bumps
	// the conversation's UpdatedAt, and returns the stored message.
	AppendMessage(ctx context.Context, conversationID string, m Message) (Message, error)
	// ListMessages returns the conversation's messages oldest first. Never nil.
	ListMessages(ctx context.Context, conversationID string) ([]Message, error)
}

// ToolSpec describes a tool to the model. Parameters is a JSON Schema object.
type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Tool is something the model can ask to run.
type Tool interface {
	Spec() ToolSpec
	// NeedsApproval reports whether a call with these arguments needs the
	// person's approval before it runs.
	NeedsApproval(args json.RawMessage) bool
	// Run runs the call inside workspace, an absolute directory, and returns
	// the text to give the model. An error is shown to the model as the result.
	Run(ctx context.Context, workspace string, args json.RawMessage) (string, error)
}

// Event types.
const (
	EventInfo       = "info"        // routing for the assistant message about to stream
	EventContent    = "content"     // a piece of the answer
	EventReasoning  = "reasoning"   // a piece of the model's reasoning
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventApproval   = "approval"    // the person must approve that call
	EventToolResult = "tool_result" // the call finished (or was denied)
	EventError      = "error"
	EventStopped    = "stopped" // the person stopped the turn; followed by done
	EventDone       = "done"    // always the last event of a turn
)

// Event is one thing that happens during a turn. Type says which fields are set.
type Event struct {
	Type      string `json:"type"`
	Text      string `json:"text,omitempty"`      // content, reasoning (a delta); tool_result (the result)
	Route     string `json:"route,omitempty"`     // info
	Backend   string `json:"backend,omitempty"`   // info
	Model     string `json:"model,omitempty"`     // info
	CallID    string `json:"call_id,omitempty"`   // tool_call, approval, tool_result
	Name      string `json:"name,omitempty"`      // tool_call, approval
	Arguments string `json:"arguments,omitempty"` // tool_call, approval
	Error     string `json:"error,omitempty"`     // error
}

// CompletionRequest is one call to the router. Model is "auto", a route
// name, or "backend/model". No Tools means no tools are offered.
type CompletionRequest struct {
	Model    string
	Messages []Message
	Tools    []ToolSpec
}

// Completer asks the router for one assistant message. It calls emit for
// each event as it streams (info, then content and reasoning), and returns
// the assembled assistant message: Content, Reasoning, ToolCalls, and
// Route, Backend and Model as the router reported them.
type Completer interface {
	Complete(ctx context.Context, req CompletionRequest, emit func(Event)) (Message, error)
}

// ApprovalRequest asks the person to approve one tool call.
type ApprovalRequest struct {
	ConversationID string
	CallID         string
	Name           string
	Arguments      string
}

// Approver asks the person whether a tool call may run. It blocks until the
// person answers or ctx ends.
type Approver interface {
	Approve(ctx context.Context, r ApprovalRequest) (bool, error)
}
