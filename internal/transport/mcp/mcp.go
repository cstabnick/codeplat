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
	serverName    = "codeplat"
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
	log   *slog.Logger
	tools []tool
}

func NewServer(svc *core.Service, log *slog.Logger) *Server {
	return &Server{log: log, tools: tools(svc)}
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
			"serverInfo":      map[string]string{"name": serverName, "version": serverVersion},
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
		idx := slices.IndexFunc(s.tools, func(t tool) bool { return t.Name == params.Name })
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
func (s *Server) call(ctx context.Context, t tool, args json.RawMessage) callResult {
	out, err := t.call(ctx, args)
	if err != nil {
		s.log.Debug("tool failed", "tool", t.Name, "err", err)
		return callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	text, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true}
	}
	return callResult{Content: []content{{Type: "text", Text: string(text)}}}
}
