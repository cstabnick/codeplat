// Package mcp exposes core.Service as Model Context Protocol tools over a
// newline-delimited JSON-RPC stream (the MCP stdio transport).
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"slices"

	"codeplat/internal/core"
)

const (
	serverVersion = "0.1.0"
	maxLineBytes  = 4 << 20

	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Newest first; the first entry is offered when the client asks for a
// version we do not know.
var protocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type Server struct {
	name  string
	log   *slog.Logger
	tools []Tool
}

// NewServer serves the tracker's tools as the server "codeplat".
func NewServer(svc *core.Service, log *slog.Logger) *Server {
	return NewToolServer("codeplat", tools(svc), log)
}

// NewToolServer serves the given tools under the given server name.
func NewToolServer(name string, tools []Tool, log *slog.Logger) *Server {
	return &Server{name: name, log: log, tools: tools}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// Serve handles requests from in until it reaches EOF or ctx is cancelled.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64<<10), maxLineBytes)
	enc := json.NewEncoder(out)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		resp := s.handleLine(ctx, line)
		if resp == nil {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// handleLine returns nil when the message is a notification.
func (s *Server) handleLine(ctx context.Context, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{codeParseError, "parse error"}}
	}
	isNotification := len(req.ID) == 0
	if req.Method == "" {
		if isNotification {
			return nil
		}
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{codeInvalidRequest, "method is required"}}
	}

	result, rpcErr := s.dispatch(ctx, req)
	if isNotification {
		return nil
	}
	return &response{JSONRPC: "2.0", ID: req.ID, Result: result, Error: rpcErr}
}

func (s *Server) dispatch(ctx context.Context, req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		version := protocolVersions[0]
		if slices.Contains(protocolVersions, params.ProtocolVersion) {
			version = params.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": s.name, "version": serverVersion},
		}, nil

	case "ping":
		return map[string]any{}, nil

	case "tools/list":
		return map[string]any{"tools": s.tools}, nil

	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &rpcError{codeInvalidParams, "invalid params"}
		}
		idx := slices.IndexFunc(s.tools, func(t Tool) bool { return t.Name == params.Name })
		if idx < 0 {
			return nil, &rpcError{codeInvalidParams, "unknown tool: " + params.Name}
		}
		return s.call(ctx, s.tools[idx], params.Arguments), nil

	default:
		return nil, &rpcError{codeMethodNotFound, "method not found: " + req.Method}
	}
}

// call runs a tool. Failures are reported inside the result, as MCP
// requires, so the model can read them and try again.
func (s *Server) call(ctx context.Context, t Tool, args json.RawMessage) callResult {
	out, err := t.Call(ctx, args)
	if err != nil {
		s.log.Debug("tool failed", "tool", t.Name, "err", err)
		return callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	if text, ok := out.(string); ok {
		return callResult{Content: []content{{Type: "text", Text: text}}}
	}
	text, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return callResult{Content: []content{{Type: "text", Text: string(text)}}}
}
