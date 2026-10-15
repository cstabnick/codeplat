package fs

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

// writeFile creates name (relative to dir) with content, making parent directories.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func runTool(t *testing.T, tool chat.Tool, ws string, args string) (string, error) {
	t.Helper()
	var raw json.RawMessage
	if args != "" {
		raw = json.RawMessage(args)
	}
	return tool.Run(context.Background(), ws, raw)
}

func TestSpecs(t *testing.T) {
	for _, tc := range []struct {
		tool chat.Tool
		name string
	}{
		{ReadFile(), "read_file"},
		{ListDir(), "list_dir"},
	} {
		spec := tc.tool.Spec()
		if spec.Name != tc.name {
			t.Errorf("Spec().Name = %q, want %q", spec.Name, tc.name)
		}
		if spec.Description == "" {
			t.Errorf("%s: empty description", tc.name)
		}
		if !json.Valid(spec.Parameters) {
			t.Errorf("%s: Parameters is not valid JSON: %s", tc.name, spec.Parameters)
		}
		var schema map[string]any
		if err := json.Unmarshal(spec.Parameters, &schema); err != nil {
			t.Errorf("%s: Parameters is not a JSON object: %v", tc.name, err)
		}
		if schema["type"] != "object" {
			t.Errorf("%s: schema type = %v, want object", tc.name, schema["type"])
		}
		if tc.tool.NeedsApproval(nil) {
			t.Errorf("%s: NeedsApproval(nil) = true, want false", tc.name)
		}
		if tc.tool.NeedsApproval(json.RawMessage(`{"path":"x"}`)) {
			t.Errorf("%s: NeedsApproval(args) = true, want false", tc.name)
		}
	}
}

func TestReadFileSmall(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "notes.txt", "one\ntwo\nthree\n")

	got, err := runTool(t, ReadFile(), ws, `{"path":"notes.txt"}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "     1\tone\n     2\ttwo\n     3\tthree\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadFileSubdirPath(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "pkg/a.go", "package a\n")

	got, err := runTool(t, ReadFile(), ws, `{"path":"pkg/a.go"}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "     1\tpackage a\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestReadFileOffsetLimit(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "five.txt", "one\ntwo\nthree\nfour\nfive\n")

	got, err := runTool(t, ReadFile(), ws, `{"path":"five.txt","offset":2,"limit":2}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "     2\ttwo\n     3\tthree\n[more lines follow; use offset 4]\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Reading to the end has no footer.
	got, err = runTool(t, ReadFile(), ws, `{"path":"five.txt","offset":4}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "     4\tfour\n     5\tfive\n"; got != want {
		t.Errorf("offset 4: got %q, want %q", got, want)
	}

	// A limit that exactly covers the rest has no footer either.
	got, err = runTool(t, ReadFile(), ws, `{"path":"five.txt","limit":5}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "[more lines follow") {
		t.Errorf("limit 5 of 5 lines: unexpected footer in %q", got)
	}

	// Limit alone shows the footer with the next line number.
	got, err = runTool(t, ReadFile(), ws, `{"path":"five.txt","limit":1}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "     1\tone\n[more lines follow; use offset 2]\n"; got != want {
		t.Errorf("limit 1: got %q, want %q", got, want)
	}
}

func TestReadFileOffsetPastEnd(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "three.txt", "a\nb\nc\n")

	_, err := runTool(t, ReadFile(), ws, `{"path":"three.txt","offset":4}`)
	if err == nil || err.Error() != "offset 4 is past the end of the file" {
		t.Errorf("offset 4 of 3 lines: err = %v", err)
	}
}

func TestReadFileBadOffsetOrLimit(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "a.txt", "a\n")

	for _, args := range []string{
		`{"path":"a.txt","offset":0}`,
		`{"path":"a.txt","offset":-1}`,
		`{"path":"a.txt","limit":0}`,
		`{"path":"a.txt","limit":-5}`,
	} {
		_, err := runTool(t, ReadFile(), ws, args)
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", args, err)
		}
	}
}

func TestReadFileEmpty(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "empty.txt", "")

	for _, args := range []string{
		`{"path":"empty.txt"}`,
		`{"path":"empty.txt","offset":1}`,
		`{"path":"empty.txt","offset":5,"limit":3}`,
	} {
		got, err := runTool(t, ReadFile(), ws, args)
		if err != nil {
			t.Errorf("%s: %v", args, err)
			continue
		}
		if got != "(empty file)" {
			t.Errorf("%s: got %q, want (empty file)", args, got)
		}
	}
}

func TestReadFileDirectory(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "sub/x.txt", "x\n")

	_, err := runTool(t, ReadFile(), ws, `{"path":"sub"}`)
	if err == nil || err.Error() != "path is a directory" {
		t.Errorf("err = %v, want path is a directory", err)
	}
}

func TestReadFileBinary(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "data.bin", "header\x00\x01\x02 more")

	_, err := runTool(t, ReadFile(), ws, `{"path":"data.bin"}`)
	if err == nil || err.Error() != "binary file" {
		t.Errorf("err = %v, want binary file", err)
	}
}

func TestReadFileMissing(t *testing.T) {
	ws := t.TempDir()

	_, err := runTool(t, ReadFile(), ws, `{"path":"missing.txt"}`)
	if err == nil {
		t.Fatal("missing file: no error")
	}
	if !strings.Contains(err.Error(), "missing.txt") {
		t.Errorf("error %q does not mention the path", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("error %q does not wrap os.ErrNotExist", err)
	}
}

func TestReadFileOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	secret := writeFile(t, outside, "secret.txt", "secret\n")
	writeFile(t, filepath.Dir(ws), "sibling.txt", "sibling\n")

	for _, args := range []string{
		`{"path":"../` + filepath.Base(outside) + `/secret.txt"}`,
		`{"path":"../sibling.txt"}`,
		`{"path":"` + secret + `"}`,
	} {
		got, err := runTool(t, ReadFile(), ws, args)
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("%s: err = %v (output %q), want ErrInvalid", args, err, got)
		}
		if strings.Contains(got, "secret") || strings.Contains(got, "sibling") {
			t.Errorf("%s: leaked content %q", args, got)
		}
	}
}

func TestReadFileInvalidJSON(t *testing.T) {
	ws := t.TempDir()

	_, err := runTool(t, ReadFile(), ws, `{"path":`)
	if !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
	_, err = runTool(t, ListDir(), ws, `not json`)
	if !errors.Is(err, chat.ErrInvalid) {
		t.Errorf("list_dir: err = %v, want ErrInvalid", err)
	}
}

func TestReadFileLarge(t *testing.T) {
	ws := t.TempDir()
	// 2049 lines of 1024 bytes each: 2,098,176 bytes, just over 2 MiB.
	line := strings.Repeat("x", 1023) + "\n"
	writeFile(t, ws, "big.txt", strings.Repeat(line, 2049))

	_, err := runTool(t, ReadFile(), ws, `{"path":"big.txt"}`)
	if err == nil || !strings.HasPrefix(err.Error(), "file too large (") {
		t.Errorf("no limit: err = %v, want file too large", err)
	}

	got, err := runTool(t, ReadFile(), ws, `{"path":"big.txt","limit":3}`)
	if err != nil {
		t.Fatalf("with limit: %v", err)
	}
	if !strings.HasPrefix(got, "     1\t"+strings.Repeat("x", 1023)+"\n") {
		t.Errorf("with limit: unexpected start %q", got[:min(len(got), 40)])
	}
	if want := "[more lines follow; use offset 4]\n"; !strings.HasSuffix(got, want) {
		t.Errorf("with limit: missing footer, got tail %q", got[max(0, len(got)-60):])
	}

	got, err = runTool(t, ReadFile(), ws, `{"path":"big.txt","offset":2049}`)
	if err != nil {
		t.Fatalf("with offset: %v", err)
	}
	if want := "  2049\t" + strings.Repeat("x", 1023) + "\n"; got != want {
		t.Errorf("with offset: got %d bytes, want the last line", len(got))
	}
}

func TestListDirWorkspace(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "b.txt", "")
	writeFile(t, ws, "sub/x.txt", "")
	writeFile(t, ws, "Beta.txt", "")
	writeFile(t, ws, "alpha.txt", "")

	// Byte order: uppercase sorts before lowercase.
	got, err := runTool(t, ListDir(), ws, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	want := "Beta.txt\nalpha.txt\nb.txt\nsub/\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// Empty arguments list the workspace too.
	got2, err := runTool(t, ListDir(), ws, "")
	if err != nil {
		t.Fatal(err)
	}
	if got2 != want {
		t.Errorf("empty args: got %q, want %q", got2, want)
	}
}

func TestListDirSubdirectory(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "sub/zeta.go", "")
	writeFile(t, ws, "sub/deep/inner.go", "")
	writeFile(t, ws, "sub/alpha.go", "")

	got, err := runTool(t, ListDir(), ws, `{"path":"sub"}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "alpha.go\ndeep/\nzeta.go\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestListDirFile(t *testing.T) {
	ws := t.TempDir()
	writeFile(t, ws, "a.txt", "a\n")

	_, err := runTool(t, ListDir(), ws, `{"path":"a.txt"}`)
	if err == nil || err.Error() != "not a directory" {
		t.Errorf("err = %v, want not a directory", err)
	}
}

func TestListDirEmpty(t *testing.T) {
	ws := t.TempDir()
	if err := os.Mkdir(filepath.Join(ws, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := runTool(t, ListDir(), ws, `{"path":"empty"}`)
	if err != nil {
		t.Fatal(err)
	}
	if got != "(empty directory)" {
		t.Errorf("got %q, want (empty directory)", got)
	}
}

func TestListDirTruncated(t *testing.T) {
	ws := t.TempDir()
	for i := 0; i < 600; i++ {
		writeFile(t, ws, fmt.Sprintf("f%03d.txt", i), "")
	}

	got, err := runTool(t, ListDir(), ws, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != 501 {
		t.Fatalf("got %d lines, want 501 (500 entries and a footer)", len(lines))
	}
	if lines[0] != "f000.txt" || lines[499] != "f499.txt" {
		t.Errorf("first/last shown = %q/%q, want f000.txt/f499.txt", lines[0], lines[499])
	}
	if want := "[100 more entries not shown]"; lines[500] != want {
		t.Errorf("footer = %q, want %q", lines[500], want)
	}
}

func TestListDirSymlink(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	writeFile(t, ws, "real/inside.txt", "")
	if err := os.Symlink(filepath.Join(ws, "real"), filepath.Join(ws, "linkdir")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if err := os.Symlink(filepath.Join(ws, "real", "inside.txt"), filepath.Join(ws, "linkfile")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "gone"), filepath.Join(ws, "dangling")); err != nil {
		t.Fatal(err)
	}

	got, err := runTool(t, ListDir(), ws, `{}`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "dangling\nlinkdir/\nlinkfile\nreal/\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestListDirOutsideWorkspace(t *testing.T) {
	ws := t.TempDir()
	outside := t.TempDir()
	writeFile(t, outside, "x.txt", "")

	for _, args := range []string{
		`{"path":".."}`,
		`{"path":"` + outside + `"}`,
	} {
		got, err := runTool(t, ListDir(), ws, args)
		if !errors.Is(err, chat.ErrInvalid) {
			t.Errorf("%s: err = %v (output %q), want ErrInvalid", args, err, got)
		}
	}
}
