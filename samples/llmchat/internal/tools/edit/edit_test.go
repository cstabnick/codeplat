package edit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmchat/internal/chat"
)

func mustArgs(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func writeTemp(t *testing.T, path, content string, perm os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func TestSpecs(t *testing.T) {
	for _, tc := range []struct {
		tool chat.Tool
		name string
	}{
		{WriteFile(), "write_file"},
		{EditFile(), "edit_file"},
	} {
		spec := tc.tool.Spec()
		if spec.Name != tc.name {
			t.Errorf("name = %q, want %q", spec.Name, tc.name)
		}
		if spec.Description == "" {
			t.Errorf("%s: empty description", tc.name)
		}
		var schema map[string]any
		if err := json.Unmarshal(spec.Parameters, &schema); err != nil {
			t.Errorf("%s: parameters are not valid JSON: %v", tc.name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("%s: parameters type = %v, want object", tc.name, schema["type"])
		}
		if !tc.tool.NeedsApproval(json.RawMessage(`{}`)) {
			t.Errorf("%s: NeedsApproval = false, want true", tc.name)
		}
		if !tc.tool.NeedsApproval(nil) {
			t.Errorf("%s: NeedsApproval(nil) = false, want true", tc.name)
		}
	}
}

func TestWriteFileCreatesNewFileInNewDirectory(t *testing.T) {
	ws := t.TempDir()
	out, err := WriteFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":    "sub/dir/hello.txt",
		"content": "hello\n",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	path := filepath.Join(ws, "sub", "dir", "hello.txt")
	if got := readFile(t, path); got != "hello\n" {
		t.Errorf("content = %q, want %q", got, "hello\n")
	}
	if want := "created sub/dir/hello.txt (6 bytes)"; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
	if got := mode(t, path); got != 0o644 {
		t.Errorf("mode = %o, want 644", got)
	}
}

func TestWriteFileOverwriteKeepsMode(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "run.sh")
	writeTemp(t, path, "old", 0o755)

	out, err := WriteFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":    "run.sh",
		"content": "new content",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, path); got != "new content" {
		t.Errorf("content = %q, want %q", got, "new content")
	}
	if want := "wrote 11 bytes to run.sh"; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
	if got := mode(t, path); got != 0o755 {
		t.Errorf("mode = %o, want 755", got)
	}
}

func TestWriteFileRejectsDirectoryPath(t *testing.T) {
	ws := t.TempDir()
	if err := os.Mkdir(filepath.Join(ws, "adir"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := WriteFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":    "adir",
		"content": "x",
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if info, statErr := os.Stat(filepath.Join(ws, "adir")); statErr != nil || !info.IsDir() {
		t.Errorf("directory was changed: stat err %v", statErr)
	}
}

func TestWriteFileRejectsEscapingPaths(t *testing.T) {
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	outsideAbs := filepath.Join(base, "abs.txt")

	for _, p := range []string{"../x", "../../x", outsideAbs} {
		_, err := WriteFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
			"path":    p,
			"content": "x",
		}))
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("path %q: err = %v, want ErrInvalid", p, err)
		}
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "ws" {
			t.Errorf("file written outside workspace: %s", e.Name())
		}
	}
}

func TestWriteFileRejectsOversizeContent(t *testing.T) {
	ws := t.TempDir()
	big := strings.Repeat("a", (1<<20)+1)
	_, err := WriteFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":    "big.txt",
		"content": big,
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if _, statErr := os.Stat(filepath.Join(ws, "big.txt")); !os.IsNotExist(statErr) {
		t.Errorf("oversize file was written: stat err %v", statErr)
	}
}

func TestBadJSONIsInvalid(t *testing.T) {
	ws := t.TempDir()
	bad := json.RawMessage(`{"path": `)
	if _, err := WriteFile().Run(context.Background(), ws, bad); !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("write_file: err = %v, want ErrInvalid", err)
	}
	if _, err := EditFile().Run(context.Background(), ws, bad); !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("edit_file: err = %v, want ErrInvalid", err)
	}
}

func TestEditFileReplacesSingleOccurrence(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	writeTemp(t, path, "alpha beta gamma", 0o644)

	out, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "a.txt",
		"old_string": "beta",
		"new_string": "BETA",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, path); got != "alpha BETA gamma" {
		t.Errorf("content = %q", got)
	}
	if want := "replaced 1 occurrence(s) in a.txt"; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

func TestEditFileReplaceAll(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	writeTemp(t, path, "x-1 x-2 x-3", 0o644)

	out, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":        "a.txt",
		"old_string":  "x-",
		"new_string":  "y-",
		"replace_all": true,
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, path); got != "y-1 y-2 y-3" {
		t.Errorf("content = %q", got)
	}
	if want := "replaced 3 occurrence(s) in a.txt"; out != want {
		t.Errorf("result = %q, want %q", out, want)
	}
}

func TestEditFileAmbiguousLeavesFileUnchanged(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	writeTemp(t, path, "dup dup dup", 0o644)

	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "a.txt",
		"old_string": "dup",
		"new_string": "one",
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "3") {
		t.Errorf("error %q does not include the count 3", err)
	}
	if got := readFile(t, path); got != "dup dup dup" {
		t.Errorf("file changed: %q", got)
	}
}

func TestEditFileNotFound(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "a.txt")
	writeTemp(t, path, "hello", 0o644)

	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "a.txt",
		"old_string": "absent",
		"new_string": "x",
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
	if got := readFile(t, path); got != "hello" {
		t.Errorf("file changed: %q", got)
	}
}

func TestEditFileEmptyOldString(t *testing.T) {
	ws := t.TempDir()
	writeTemp(t, filepath.Join(ws, "a.txt"), "hello", 0o644)
	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "a.txt",
		"old_string": "",
		"new_string": "x",
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestEditFileIdenticalStrings(t *testing.T) {
	ws := t.TempDir()
	writeTemp(t, filepath.Join(ws, "a.txt"), "hello", 0o644)
	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "a.txt",
		"old_string": "hello",
		"new_string": "hello",
	}))
	if !errors.Is(err, chat.ErrInvalid) {
		t.Fatalf("err = %v, want ErrInvalid", err)
	}
}

func TestEditFileMissingFile(t *testing.T) {
	ws := t.TempDir()
	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "nope.txt",
		"old_string": "a",
		"new_string": "b",
	}))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if _, statErr := os.Stat(filepath.Join(ws, "nope.txt")); !os.IsNotExist(statErr) {
		t.Errorf("missing file was created: stat err %v", statErr)
	}
}

func TestEditFileRejectsEscapingPath(t *testing.T) {
	base := t.TempDir()
	ws := filepath.Join(base, "ws")
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(base, "secret.txt")
	writeTemp(t, outside, "secret", 0o644)

	for _, p := range []string{"../secret.txt", outside} {
		_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
			"path":       p,
			"old_string": "secret",
			"new_string": "changed",
		}))
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("path %q: err = %v, want ErrInvalid", p, err)
		}
	}
	if got := readFile(t, outside); got != "secret" {
		t.Errorf("file outside workspace changed: %q", got)
	}
}

func TestEditFileKeepsMode(t *testing.T) {
	ws := t.TempDir()
	path := filepath.Join(ws, "run.sh")
	writeTemp(t, path, "echo old", 0o755)

	_, err := EditFile().Run(context.Background(), ws, mustArgs(t, map[string]any{
		"path":       "run.sh",
		"old_string": "old",
		"new_string": "new",
	}))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := readFile(t, path); got != "echo new" {
		t.Errorf("content = %q", got)
	}
	if got := mode(t, path); got != 0o755 {
		t.Errorf("mode = %o, want 755", got)
	}
}
