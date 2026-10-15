package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeplat/internal/codenav"
	"codeplat/internal/transport/mcp"
)

func TestCodenav(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"go.mod":     "module example.com/m\n\ngo 1.22\n",
		"pkg/pkg.go": "// Package pkg does one thing.\npackage pkg\n\n// Do does it.\nfunc Do() {}\n",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	nav, err := codenav.New(root, "")
	if err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"code_map","arguments":{"dir":"pkg"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"code_map","arguments":{"dir":".."}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"symbol_find","arguments":{"query":"Do"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"symbol_refs","arguments":{"symbol":"Do","bogus":1}}}`,
	}, "\n") + "\n"
	var out bytes.Buffer
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := mcp.NewCodenavServer(nav, log).Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}

	byID := map[int]reply{}
	var info struct {
		Result struct {
			ServerInfo struct{ Name string } `json:"serverInfo"`
		} `json:"result"`
	}
	for i, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r reply
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		if i == 0 {
			_ = json.Unmarshal([]byte(line), &info)
		}
		byID[*r.ID] = r
	}

	if info.Result.ServerInfo.Name != "codenav" {
		t.Errorf("server name = %q", info.Result.ServerInfo.Name)
	}
	var names []string
	for _, tool := range byID[2].Result.Tools {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, ","); got != "code_map,symbol_find,symbol_refs" {
		t.Errorf("tools = %s", got)
	}

	m := byID[3].Result
	if m.IsError || len(m.Content) != 1 || !strings.HasPrefix(m.Content[0].Text, "module example.com/m (.)\n") ||
		!strings.Contains(m.Content[0].Text, "func Do()  pkg.go:5  // Do does it.") {
		t.Errorf("code_map is not plain text: %+v", m)
	}
	if !byID[4].Result.IsError {
		t.Error("code_map outside the root succeeded")
	}
	if r := byID[5].Result; !r.IsError || !strings.Contains(r.Content[0].Text, "gopls is not installed") {
		t.Errorf("symbol_find without gopls: %+v", r)
	}
	if r := byID[6].Result; !r.IsError || !strings.Contains(r.Content[0].Text, "invalid arguments") {
		t.Errorf("symbol_refs with an unknown argument: %+v", r)
	}
}
