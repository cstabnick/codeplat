package search

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmchat/internal/chat"
)

// longCut is what a long line looks like after it is cut to 300 runes:
// "Hello" followed by 295 two-byte runes.
var longCut = "Hello" + strings.Repeat("é", 295)

// newWorkspace builds the fixture workspace and returns its path.
func newWorkspace(t *testing.T) string {
	t.Helper()
	ws := t.TempDir()
	files := map[string]string{
		"a.go":                      "package a\n\nfunc Hello() {}\n",
		"bin.dat":                   "Hello\x00\x01binary",
		"crlf.txt":                  "Hello\r\nworld\r\n",
		"long.txt":                  "Hello" + strings.Repeat("é", 1000) + "\n",
		"notes.txt":                 "hello lowercase\n",
		"sub/b.go":                  "package sub\n// Hello again\n",
		"sub/deep/c.txt":            "nothing here\n",
		"z.go":                      "Hello from z\n",
		".git/config":               "Hello git\n",
		"node_modules/lib/index.js": "Hello module\n",
		"vendor/v.go":               "Hello vendor\n",
	}
	for name, body := range files {
		p := filepath.Join(ws, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(ws, "a.go"), filepath.Join(ws, "link.go")); err != nil {
		t.Fatal(err)
	}
	return ws
}

func run(t *testing.T, ws, args string) (string, error) {
	t.Helper()
	return Grep().Run(context.Background(), ws, json.RawMessage(args))
}

func mustRun(t *testing.T, ws, args string) string {
	t.Helper()
	out, err := run(t, ws, args)
	if err != nil {
		t.Fatalf("run %s: %v", args, err)
	}
	return out
}

func TestSpec(t *testing.T) {
	var tool chat.Tool = Grep()
	spec := tool.Spec()
	if spec.Name != "grep" {
		t.Errorf("name = %q, want grep", spec.Name)
	}
	var params struct {
		Type       string                     `json:"type"`
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(spec.Parameters, &params); err != nil {
		t.Fatalf("parameters are not valid JSON: %v", err)
	}
	if params.Type != "object" {
		t.Errorf("parameters type = %q, want object", params.Type)
	}
	for _, k := range []string{"pattern", "path", "glob", "ignore_case"} {
		if _, ok := params.Properties[k]; !ok {
			t.Errorf("parameters missing property %q", k)
		}
	}
	if len(params.Required) != 1 || params.Required[0] != "pattern" {
		t.Errorf("required = %v, want [pattern]", params.Required)
	}
	if tool.NeedsApproval(json.RawMessage(`{"pattern":"x"}`)) {
		t.Error("NeedsApproval = true, want false")
	}
}

func TestMatchesAcrossFilesInOrder(t *testing.T) {
	ws := newWorkspace(t)
	got := mustRun(t, ws, `{"pattern":"Hello"}`)
	want := strings.Join([]string{
		"a.go:3:func Hello() {}",
		"crlf.txt:1:Hello",
		"long.txt:1:" + longCut,
		"sub/b.go:2:// Hello again",
		"z.go:1:Hello from z",
	}, "\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCarriageReturnTrimmed(t *testing.T) {
	ws := newWorkspace(t)
	got := mustRun(t, ws, `{"pattern":"Hello$","path":"crlf.txt"}`)
	if got != "crlf.txt:1:Hello" {
		t.Errorf("got %q, want %q", got, "crlf.txt:1:Hello")
	}
}

func TestGlobFilters(t *testing.T) {
	ws := newWorkspace(t)
	if got := mustRun(t, ws, `{"pattern":"Hello","glob":"b.*"}`); got != "sub/b.go:2:// Hello again" {
		t.Errorf("glob b.*: got %q", got)
	}
	if got := mustRun(t, ws, `{"pattern":"Hello","glob":"*.txt"}`); got != "crlf.txt:1:Hello\nlong.txt:1:"+longCut {
		t.Errorf("glob *.txt: got %q", got)
	}
	if got := mustRun(t, ws, `{"pattern":"Hello","glob":"*.md"}`); got != "no matches" {
		t.Errorf("glob *.md: got %q, want no matches", got)
	}
}

func TestIgnoreCase(t *testing.T) {
	ws := newWorkspace(t)
	got := mustRun(t, ws, `{"pattern":"hello","glob":"notes.txt","ignore_case":true}`)
	if got != "notes.txt:1:hello lowercase" {
		t.Errorf("got %q", got)
	}
	if got := mustRun(t, ws, `{"pattern":"hello","glob":"notes.txt"}`); got != "notes.txt:1:hello lowercase" {
		// Lowercase text matches a lowercase pattern without ignore_case.
		t.Errorf("case-sensitive lowercase match: got %q", got)
	}
	if got := mustRun(t, ws, `{"pattern":"HELLO","glob":"notes.txt"}`); got != "no matches" {
		t.Errorf("case-sensitive uppercase pattern: got %q, want no matches", got)
	}
}

func TestSingleFilePath(t *testing.T) {
	ws := newWorkspace(t)
	if got := mustRun(t, ws, `{"pattern":"Hello","path":"sub/b.go"}`); got != "sub/b.go:2:// Hello again" {
		t.Errorf("file path: got %q", got)
	}
	// The glob filter does not apply to an explicit file path.
	if got := mustRun(t, ws, `{"pattern":"hello","path":"notes.txt","glob":"*.go","ignore_case":true}`); got != "notes.txt:1:hello lowercase" {
		t.Errorf("file path with non-matching glob: got %q", got)
	}
}

func TestDirectoryPath(t *testing.T) {
	ws := newWorkspace(t)
	if got := mustRun(t, ws, `{"pattern":"Hello","path":"sub"}`); got != "sub/b.go:2:// Hello again" {
		t.Errorf("dir path: got %q", got)
	}
}

func TestSkippedDirectoriesBinaryAndSymlinks(t *testing.T) {
	ws := newWorkspace(t)
	got := mustRun(t, ws, `{"pattern":"Hello"}`)
	for _, banned := range []string{".git", "node_modules", "vendor", "bin.dat", "link.go"} {
		if strings.Contains(got, banned) {
			t.Errorf("output mentions %q, which must be skipped:\n%s", banned, got)
		}
	}
	if got := mustRun(t, ws, `{"pattern":"Hello","path":"bin.dat"}`); got != "no matches" {
		t.Errorf("binary file searched explicitly: got %q, want no matches", got)
	}
}

func TestLongLineCutToRunes(t *testing.T) {
	ws := newWorkspace(t)
	got := mustRun(t, ws, `{"pattern":"Hello","path":"long.txt"}`)
	want := "long.txt:1:" + longCut
	if got != want {
		t.Errorf("long line: got %d bytes, want %d bytes", len(got), len(want))
	}
	if n := len([]rune(strings.TrimPrefix(got, "long.txt:1:"))); n != 300 {
		t.Errorf("text is %d runes, want 300", n)
	}
}

func TestMatchLimit(t *testing.T) {
	ws := newWorkspace(t)
	var b strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&b, "Hello %03d\n", i)
	}
	if err := os.WriteFile(filepath.Join(ws, "many.txt"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	got := mustRun(t, ws, `{"pattern":"Hello","path":"many.txt"}`)
	lines := strings.Split(got, "\n")
	if len(lines) != 201 {
		t.Fatalf("got %d lines, want 200 matches and a footer", len(lines))
	}
	for i := 0; i < 200; i++ {
		want := fmt.Sprintf("many.txt:%d:Hello %03d", i+1, i)
		if lines[i] != want {
			t.Fatalf("line %d = %q, want %q", i, lines[i], want)
		}
	}
	if lines[200] != "[stopped after 200 matches]" {
		t.Errorf("footer = %q", lines[200])
	}
}

func TestNoMatches(t *testing.T) {
	ws := newWorkspace(t)
	if got := mustRun(t, ws, `{"pattern":"absent-text-xyz"}`); got != "no matches" {
		t.Errorf("got %q, want no matches", got)
	}
}

func TestInvalidInput(t *testing.T) {
	ws := newWorkspace(t)
	cases := map[string]string{
		"invalid regexp":  `{"pattern":"("}`,
		"empty pattern":   `{"pattern":""}`,
		"missing pattern": `{}`,
		"bad glob":        `{"pattern":"x","glob":"["}`,
		"bad JSON":        `{"pattern":`,
		"parent path":     `{"pattern":"x","path":"../outside"}`,
		"absolute path":   `{"pattern":"x","path":"/"}`,
	}
	for name, args := range cases {
		_, err := run(t, ws, args)
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestCancelledContext(t *testing.T) {
	ws := newWorkspace(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Grep().Run(ctx, ws, json.RawMessage(`{"pattern":"Hello"}`))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
