// Package agent runs one turn of a conversation: it saves the person's
// message, asks the router for a reply, runs the tools the model asks for
// (after approval where needed), and repeats until the model answers
// without tools. It streams what happens as chat.Events.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"llmchat/internal/chat"
)

const (
	defaultMaxSteps = 25
	maxResultBytes  = 20000
	maxTitleRunes   = 60
)

// Options configures an Engine.
type Options struct {
	// MaxSteps bounds the model calls in one turn. Zero means 25.
	MaxSteps int
	// System, if not empty, is sent as the first message of every request
	// (it is not stored). When the conversation has a Workspace, the
	// text "\n\nWorkspace directory: " + Workspace is appended to it.
	System string
}

// Engine runs turns of conversations.
type Engine struct {
	store     chat.Store
	completer chat.Completer
	tools     []chat.Tool
	approver  chat.Approver
	opts      Options
}

// New returns an Engine that uses the given store, completer, tools and approver.
func New(store chat.Store, completer chat.Completer, tools []chat.Tool, approver chat.Approver, opts Options) *Engine {
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = defaultMaxSteps
	}
	return &Engine{
		store:     store,
		completer: completer,
		tools:     tools,
		approver:  approver,
		opts:      opts,
	}
}

// Run performs one turn. It emits events as they happen, ends with
// exactly one chat.EventDone when it returns nil, and returns an error
// (without EventDone) when it fails.
func (e *Engine) Run(ctx context.Context, conversationID, text string, emit func(chat.Event)) error {
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: message is empty", chat.ErrInvalid)
	}
	conv, err := e.store.GetConversation(ctx, conversationID)
	if err != nil {
		return err
	}

	if _, err := e.store.AppendMessage(ctx, conversationID, chat.Message{Role: chat.RoleUser, Content: text}); err != nil {
		return err
	}
	if conv.Title == "" {
		title := titleFrom(text)
		if _, err := e.store.UpdateConversation(ctx, conversationID, chat.ConversationPatch{Title: &title}); err != nil {
			return err
		}
	}

	var specs []chat.ToolSpec
	if conv.Workspace != "" {
		for _, t := range e.tools {
			specs = append(specs, t.Spec())
		}
	}

	baseModel := conv.Model
	if baseModel == "" {
		baseModel = "auto"
	}
	// pinned is the backend/model of the first reply in this Run, used for
	// later requests when the conversation's model is "auto".
	pinned := ""

	for step := 0; step < e.opts.MaxSteps; step++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		msgs, err := e.store.ListMessages(ctx, conversationID)
		if err != nil {
			return err
		}
		if e.opts.System != "" {
			sys := e.opts.System
			if conv.Workspace != "" {
				sys += "\n\nWorkspace directory: " + conv.Workspace
			}
			msgs = append([]chat.Message{{Role: chat.RoleSystem, Content: sys}}, msgs...)
		}

		model := baseModel
		if pinned != "" && (conv.Model == "" || conv.Model == "auto") {
			model = pinned
		}
		req := chat.CompletionRequest{Model: model, Messages: msgs, Tools: specs}

		reply, err := e.completer.Complete(ctx, req, emit)
		if err != nil {
			return fmt.Errorf("complete: %w", err)
		}
		reply.Role = chat.RoleAssistant
		if _, err := e.store.AppendMessage(ctx, conversationID, reply); err != nil {
			return err
		}
		if pinned == "" && reply.Backend != "" && reply.Model != "" {
			pinned = reply.Backend + "/" + reply.Model
		}

		if len(reply.ToolCalls) == 0 {
			emit(chat.Event{Type: chat.EventDone})
			return nil
		}

		for _, call := range reply.ToolCalls {
			if err := ctx.Err(); err != nil {
				return err
			}
			result, err := e.runCall(ctx, conv, call, emit)
			if err != nil {
				return err
			}
			if _, err := e.store.AppendMessage(ctx, conversationID, chat.Message{
				Role:       chat.RoleTool,
				ToolCallID: call.ID,
				Content:    result,
			}); err != nil {
				return err
			}
			emit(chat.Event{Type: chat.EventToolResult, CallID: call.ID, Text: result})
		}
	}

	emit(chat.Event{Type: chat.EventError, Error: fmt.Sprintf("stopped after %d steps", e.opts.MaxSteps)})
	emit(chat.Event{Type: chat.EventDone})
	return nil
}

// runCall emits the tool_call event for call and returns the text to give
// the model as its result. It returns an error only when the approver fails
// or the context ends.
func (e *Engine) runCall(ctx context.Context, conv chat.Conversation, call chat.ToolCall, emit func(chat.Event)) (string, error) {
	emit(chat.Event{Type: chat.EventToolCall, CallID: call.ID, Name: call.Name, Arguments: call.Arguments})

	tool := e.findTool(call.Name)
	if tool == nil {
		return fmt.Sprintf("error: unknown tool %q", call.Name), nil
	}
	if conv.Workspace == "" {
		return "error: tools are off for this conversation (no workspace)", nil
	}

	args := json.RawMessage(call.Arguments)
	if tool.NeedsApproval(args) {
		emit(chat.Event{Type: chat.EventApproval, CallID: call.ID, Name: call.Name, Arguments: call.Arguments})
		ok, err := e.approver.Approve(ctx, chat.ApprovalRequest{
			ConversationID: conv.ID,
			CallID:         call.ID,
			Name:           call.Name,
			Arguments:      call.Arguments,
		})
		if err != nil {
			return "", fmt.Errorf("approve %s: %w", call.Name, err)
		}
		if !ok {
			return "denied by the user", nil
		}
	}

	out, err := tool.Run(ctx, conv.Workspace, args)
	if err != nil {
		return "error: " + err.Error(), nil
	}
	if len(out) > maxResultBytes {
		out = out[:maxResultBytes] + "\n[truncated]"
	}
	return out, nil
}

func (e *Engine) findTool(name string) chat.Tool {
	for _, t := range e.tools {
		if t.Spec().Name == name {
			return t
		}
	}
	return nil
}

// titleFrom returns the first line of text, cut to maxTitleRunes runes with
// "…" appended when it is cut.
func titleFrom(text string) string {
	line := strings.TrimSpace(strings.SplitN(text, "\n", 2)[0])
	runes := []rune(line)
	if len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes]) + "…"
	}
	return line
}
