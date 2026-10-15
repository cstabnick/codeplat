package localdriver

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sandbox/internal/core"
)

func spec(name string) core.BoxSpec {
	return core.BoxSpec{Name: name, Image: "host", Workspace: "/tmp/ws", Network: "bridge"}
}

func TestEnsureIsIdempotent(t *testing.T) {
	ctx := context.Background()
	d := New()

	first, err := d.Ensure(ctx, spec("alpha"))
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if first.Name != "alpha" || first.Image != "host" || first.Workspace != "/tmp/ws" {
		t.Fatalf("Ensure returned %+v", first)
	}
	if first.ContainerID != "local-alpha" || first.State != "running" {
		t.Fatalf("ContainerID/State = %q/%q", first.ContainerID, first.State)
	}
	if first.CreatedAt.IsZero() || time.Since(first.CreatedAt) > time.Minute {
		t.Fatalf("CreatedAt = %v", first.CreatedAt)
	}

	second, err := d.Ensure(ctx, core.BoxSpec{Name: "alpha", Image: "other", Workspace: "/elsewhere"})
	if err != nil {
		t.Fatalf("second Ensure: %v", err)
	}
	if second != first {
		t.Fatalf("second Ensure = %+v, want the stored box %+v", second, first)
	}

	boxes, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(boxes) != 1 {
		t.Fatalf("List has %d boxes, want 1", len(boxes))
	}
}

func TestListIsSortedByName(t *testing.T) {
	ctx := context.Background()
	d := New()
	for _, name := range []string{"charlie", "alpha", "bravo"} {
		if _, err := d.Ensure(ctx, spec(name)); err != nil {
			t.Fatalf("Ensure %s: %v", name, err)
		}
	}

	boxes, err := d.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"alpha", "bravo", "charlie"}
	if len(boxes) != len(want) {
		t.Fatalf("List has %d boxes, want %d", len(boxes), len(want))
	}
	for i, name := range want {
		if boxes[i].Name != name {
			t.Fatalf("List[%d] = %q, want %q", i, boxes[i].Name, name)
		}
	}
}

func TestGetAndRemove(t *testing.T) {
	ctx := context.Background()
	d := New()

	if _, err := d.Get(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get unknown: err = %v, want ErrNotFound", err)
	}
	if err := d.Remove(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Remove unknown: err = %v, want ErrNotFound", err)
	}

	if _, err := d.Ensure(ctx, spec("beta")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	box, err := d.Get(ctx, "beta")
	if err != nil || box.Name != "beta" {
		t.Fatalf("Get = %+v, %v", box, err)
	}

	if err := d.Remove(ctx, "beta"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := d.Get(ctx, "beta"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get after Remove: err = %v, want ErrNotFound", err)
	}
	if err := d.Remove(ctx, "beta"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("second Remove: err = %v, want ErrNotFound", err)
	}
}

func TestExecReturnsOutputAndExitCode(t *testing.T) {
	ctx := context.Background()
	d := New()
	if _, err := d.Ensure(ctx, spec("gamma")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	var out bytes.Buffer
	code, err := d.Exec(ctx, "gamma", []string{"bash", "-c", "echo hi; exit 3"}, t.TempDir(), &out)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if out.String() != "hi\n" {
		t.Fatalf("output = %q, want %q", out.String(), "hi\n")
	}
	if code != 3 {
		t.Fatalf("exit code = %d, want 3", code)
	}

	out.Reset()
	code, err = d.Exec(ctx, "gamma", []string{"bash", "-c", "true"}, t.TempDir(), &out)
	if err != nil || code != 0 {
		t.Fatalf("Exec true = %d, %v; want 0, nil", code, err)
	}
}

func TestExecRunsInWorkdir(t *testing.T) {
	ctx := context.Background()
	d := New()
	if _, err := d.Ensure(ctx, spec("delta")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("here\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out bytes.Buffer
	code, err := d.Exec(ctx, "delta", []string{"cat", "marker"}, dir, &out)
	if err != nil || code != 0 {
		t.Fatalf("Exec cat = %d, %v", code, err)
	}
	if out.String() != "here\n" {
		t.Fatalf("output = %q, want %q", out.String(), "here\n")
	}
}

func TestExecUnknownBox(t *testing.T) {
	d := New()
	_, err := d.Exec(context.Background(), "nobody", []string{"true"}, t.TempDir(), &bytes.Buffer{})
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Exec unknown box: err = %v, want ErrNotFound", err)
	}
}

func TestExecEmptyArgv(t *testing.T) {
	ctx := context.Background()
	d := New()
	if _, err := d.Ensure(ctx, spec("epsilon")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	_, err := d.Exec(ctx, "epsilon", nil, t.TempDir(), &bytes.Buffer{})
	if !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("Exec with empty argv: err = %v, want ErrInvalid", err)
	}
}

func TestExecCancelStopsCommand(t *testing.T) {
	d := New()
	if _, err := d.Ensure(context.Background(), spec("zeta")); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	code, err := d.Exec(ctx, "zeta", []string{"sleep", "30"}, t.TempDir(), &bytes.Buffer{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Exec returned nil error after cancel, code %d", code)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want it to wrap context.DeadlineExceeded", err)
	}
	if code != -1 {
		t.Fatalf("exit code = %d, want -1", code)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("Exec took %v after cancel, want it to stop quickly", elapsed)
	}
}
