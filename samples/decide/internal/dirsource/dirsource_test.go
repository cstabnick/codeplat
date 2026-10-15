package dirsource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"decide/internal/core"
)

const validModel = `{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "Request"},
    {"id": "out", "type": "outputNode", "name": "Response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "out"}
  ]
}`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGetValidModel(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "loan-check.json", validModel)

	g, err := New(dir).Get(context.Background(), "loan-check")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(g.Nodes) != 2 || len(g.Edges) != 1 {
		t.Fatalf("got %d nodes and %d edges, want 2 and 1", len(g.Nodes), len(g.Edges))
	}
	if g.Nodes[0].ID != "in" || g.Nodes[1].ID != "out" {
		t.Fatalf("got node IDs %q, %q", g.Nodes[0].ID, g.Nodes[1].ID)
	}
}

func TestGetSeesEditsWithoutRestart(t *testing.T) {
	dir := t.TempDir()
	src := New(dir)
	writeFile(t, dir, "m.json", validModel)
	if _, err := src.Get(context.Background(), "m"); err != nil {
		t.Fatalf("first Get: %v", err)
	}
	writeFile(t, dir, "m.json", `{"nodes": []}`)
	if _, err := src.Get(context.Background(), "m"); !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("after edit: err = %v, want ErrInvalid", err)
	}
}

func TestGetMissingKey(t *testing.T) {
	dir := t.TempDir()
	_, err := New(dir).Get(context.Background(), "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetPathTraversalKey(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "models")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A valid model outside dir must not be reachable through "../x".
	writeFile(t, root, "x.json", validModel)

	_, err := New(dir).Get(context.Background(), "../x")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetInvalidFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "broken.json", `{"nodes": []}`)
	writeFile(t, dir, "malformed.json", `{not json`)

	for _, key := range []string{"broken", "malformed"} {
		_, err := New(dir).Get(context.Background(), key)
		if !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Get(%q): err = %v, want ErrInvalid", key, err)
		}
		if errors.Is(err, core.ErrNotFound) {
			t.Errorf("Get(%q): err = %v, must not wrap ErrNotFound", key, err)
		}
	}
}

func TestList(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "zeta.json", validModel)
	writeFile(t, dir, "alpha.json", validModel)
	writeFile(t, dir, "mid-1.json", `{}`)
	writeFile(t, dir, "README.md", "docs")
	writeFile(t, dir, "Bad Name.json", validModel)
	if err := os.Mkdir(filepath.Join(dir, "sub.json"), 0o755); err != nil {
		t.Fatal(err)
	}

	keys, err := New(dir).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "mid-1", "zeta"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("List = %v, want %v", keys, want)
	}
}

func TestListEmptyDir(t *testing.T) {
	keys, err := New(t.TempDir()).List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if keys == nil || len(keys) != 0 {
		t.Fatalf("List = %#v, want empty non-nil slice", keys)
	}
}

func TestListMissingDir(t *testing.T) {
	_, err := New(filepath.Join(t.TempDir(), "absent")).List(context.Background())
	if err == nil {
		t.Fatal("List of missing directory returned no error")
	}
}

func TestSatisfiesModelSource(t *testing.T) {
	var _ core.ModelSource = New(t.TempDir())
}
