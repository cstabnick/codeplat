package mcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"codeplat/internal/codenav"
)

const dirSchema = `{"type":"string","description":"directory relative to the repository root, e.g. internal/core or samples/llmchat; default the root"}`

// NewCodenavServer serves code navigation tools as the server "codenav".
func NewCodenavServer(nav *codenav.Navigator, log *slog.Logger) *Server {
	return NewToolServer("codenav", codenavTools(nav), log)
}

func codenavTools(nav *codenav.Navigator) []Tool {
	return []Tool{
		{
			Name: "code_map",
			Description: "Map the Go packages under a directory: each package's purpose and files, and every exported type, function, method, constant and variable with its signature, file:line and one-line doc. " +
				"Call it before reading files, to find which ones matter. Each module is mapped on its own, so map samples/<name> to see a sample.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{"dir":` + dirSchema + `}}`),
			Call: Typed(func(_ context.Context, in struct {
				Dir string `json:"dir"`
			}) (any, error) {
				return nav.Map(in.Dir)
			}),
		},
		{
			Name: "symbol_find",
			Description: "Find where Go declarations are, by name. Matches names containing the query, case-sensitively, exact matches first; qualify a method or field with its type, e.g. Service.ClaimIssue. " +
				"Returns file:line, kind, name and the declaring line. Searches the module that contains dir.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"query":{"type":"string","description":"a name or part of one, e.g. ClaimIssue or Service.ClaimIssue"},
				"dir":` + dirSchema + `},
				"required":["query"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				Query string `json:"query"`
				Dir   string `json:"dir"`
			}) (any, error) {
				syms, err := nav.Find(ctx, in.Dir, in.Query)
				if err != nil {
					return nil, err
				}
				return codenav.FormatSymbols(syms), nil
			}),
		},
		{
			Name: "symbol_refs",
			Description: "List every use of a Go declaration, type-checked, so unrelated names that merely match are left out: the declaration first, then each reference as file:line and the source line. " +
				"Use it to see what a change to a function, type or field would affect. If several declarations share the name, the error lists them; call again with `in`.",
			InputSchema: json.RawMessage(`{"type":"object","properties":{
				"symbol":{"type":"string","description":"the exact name, e.g. ClaimIssue or Service.ClaimIssue"},
				"in":{"type":"string","description":"part of the declaring file's path, to choose between declarations with the same name, e.g. store/sqlite"},
				"dir":` + dirSchema + `},
				"required":["symbol"]}`),
			Call: Typed(func(ctx context.Context, in struct {
				Symbol string `json:"symbol"`
				In     string `json:"in"`
				Dir    string `json:"dir"`
			}) (any, error) {
				locs, err := nav.References(ctx, in.Dir, in.Symbol, in.In)
				if err != nil {
					return nil, err
				}
				return codenav.FormatLocations(locs), nil
			}),
		},
	}
}
