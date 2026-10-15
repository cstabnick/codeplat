package core_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"sandbox/internal/core"
)

// execCall records one driver Exec call.
type execCall struct {
	name    string
	argv    []string
	workdir string
	left    time.Duration // time left on the context when the driver ran
}

// fakeDriver is an in-memory Driver for the service tests.
type fakeDriver struct {
	boxes    map[string]core.Box
	ensures  []core.BoxSpec
	execs    []execCall
	output   string // written to the caller in 1000-byte chunks
	exitCode int
}

func newFakeDriver() *fakeDriver {
	return &fakeDriver{boxes: map[string]core.Box{}}
}

func (f *fakeDriver) Ensure(_ context.Context, spec core.BoxSpec) (core.Box, error) {
	f.ensures = append(f.ensures, spec)
	box, ok := f.boxes[spec.Name]
	if !ok {
		box = core.Box{
			Name:        spec.Name,
			Image:       spec.Image,
			Workspace:   spec.Workspace,
			ContainerID: "c-" + spec.Name,
			CreatedAt:   time.Now(),
		}
	}
	box.State = "running"
	f.boxes[spec.Name] = box
	return box, nil
}

func (f *fakeDriver) Get(_ context.Context, name string) (core.Box, error) {
	box, ok := f.boxes[name]
	if !ok {
		return core.Box{}, core.ErrNotFound
	}
	return box, nil
}

func (f *fakeDriver) List(_ context.Context) ([]core.Box, error) {
	var out []core.Box
	for _, box := range f.boxes {
		out = append(out, box)
	}
	return out, nil
}

func (f *fakeDriver) Remove(_ context.Context, name string) error {
	if _, ok := f.boxes[name]; !ok {
		return core.ErrNotFound
	}
	delete(f.boxes, name)
	return nil
}

func (f *fakeDriver) Exec(ctx context.Context, name string, argv []string, workdir string, out io.Writer) (int, error) {
	call := execCall{name: name, argv: argv, workdir: workdir}
	if deadline, ok := ctx.Deadline(); ok {
		call.left = time.Until(deadline)
	}
	f.execs = append(f.execs, call)
	for rest := f.output; rest != ""; {
		n := min(1000, len(rest))
		if _, err := io.WriteString(out, rest[:n]); err != nil {
			return 0, err
		}
		rest = rest[n:]
	}
	return f.exitCode, nil
}

func newService(t *testing.T) (*core.Service, *fakeDriver, string) {
	t.Helper()
	root := t.TempDir()
	d := newFakeDriver()
	svc := core.NewService(d, core.Options{
		Root:     root,
		Image:    "alpine:3",
		MemoryMB: 512,
		CPUs:     1.5,
		Network:  "none",
		User:     "1000:1000",
	})
	return svc, d, root
}

func resolved(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestBadNamesAreInvalid(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	for _, name := range []string{"", "Alpha", "-alpha", "a_b", "a/b", strings.Repeat("a", 64)} {
		if _, err := svc.Ensure(ctx, name, "", ""); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Ensure(%q) = %v, want ErrInvalid", name, err)
		}
		if _, err := svc.Box(ctx, name); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Box(%q) = %v, want ErrInvalid", name, err)
		}
		if err := svc.Remove(ctx, name); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Remove(%q) = %v, want ErrInvalid", name, err)
		}
		if _, err := svc.Exec(ctx, name, "true", 0); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Exec(%q) = %v, want ErrInvalid", name, err)
		}
	}
	if len(d.ensures) != 0 || len(d.execs) != 0 {
		t.Errorf("driver was called for an invalid name: %d ensures, %d execs", len(d.ensures), len(d.execs))
	}
	if _, err := svc.Ensure(ctx, strings.Repeat("a", 63), "", ""); err != nil {
		t.Errorf("63-character name rejected: %v", err)
	}
}

func TestEnsureCreatesDefaultWorkspaceUnderRoot(t *testing.T) {
	svc, _, root := newService(t)
	box, err := svc.Ensure(context.Background(), "alpha", "", "")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(resolved(t, root), "alpha")
	if box.Workspace != want {
		t.Errorf("workspace = %q, want %q", box.Workspace, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Errorf("workspace directory not created: %v", err)
	}
}

func TestEnsureRejectsWorkspaceOutsideRoot(t *testing.T) {
	svc, d, root := newService(t)
	ctx := context.Background()
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, workspace string
	}{
		{"a1", outside},
		{"a2", root},
		{"a3", link},
		{"a4", "relative/dir"},
	}
	for _, tc := range cases {
		if _, err := svc.Ensure(ctx, tc.name, tc.workspace, ""); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Ensure(%q, %q) = %v, want ErrInvalid", tc.name, tc.workspace, err)
		}
	}
	if len(d.ensures) != 0 {
		t.Errorf("driver Ensure called for a rejected workspace")
	}
}

func TestEnsureRejectsWorkspaceConflict(t *testing.T) {
	svc, _, root := newService(t)
	ctx := context.Background()
	one := filepath.Join(root, "one")
	two := filepath.Join(root, "two")
	if _, err := svc.Ensure(ctx, "x", one, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ensure(ctx, "x", one, ""); err != nil {
		t.Errorf("same workspace again: %v", err)
	}
	_, err := svc.Ensure(ctx, "x", two, "")
	if !errors.Is(err, core.ErrInvalid) || !strings.Contains(err.Error(), "already uses workspace") {
		t.Errorf("conflicting workspace = %v, want ErrInvalid about the existing workspace", err)
	}
}

func TestEnsureUsesDefaultImageAndOptions(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Ensure(ctx, "beta", "", "debian:12"); err != nil {
		t.Fatal(err)
	}
	first := d.ensures[0]
	if first.Image != "alpine:3" {
		t.Errorf("default image = %q, want alpine:3", first.Image)
	}
	if first.MemoryMB != 512 || first.CPUs != 1.5 || first.Network != "none" || first.User != "1000:1000" {
		t.Errorf("spec limits = %+v, want the service options", first)
	}
	if d.ensures[1].Image != "debian:12" {
		t.Errorf("explicit image = %q, want debian:12", d.ensures[1].Image)
	}
}

func TestExecRunsCommandInWorkspace(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	box, err := svc.Ensure(ctx, "alpha", "", "")
	if err != nil {
		t.Fatal(err)
	}
	d.output = "hi\n"
	res, err := svc.Exec(ctx, "alpha", "echo hi", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.execs) != 1 {
		t.Fatalf("driver Exec calls = %d, want 1", len(d.execs))
	}
	call := d.execs[0]
	wantArgv := []string{"timeout", "-k", "5", "60", "bash", "-c", "echo hi"}
	if !slices.Equal(call.argv, wantArgv) {
		t.Errorf("argv = %q, want %q", call.argv, wantArgv)
	}
	if call.workdir != box.Workspace {
		t.Errorf("workdir = %q, want %q", call.workdir, box.Workspace)
	}
	if res.Output != "hi\n" || res.ExitCode != 0 || res.TimedOut {
		t.Errorf("result = %+v, want output hi, exit 0, not timed out", res)
	}
}

func TestExecTimeoutDefaultAndCap(t *testing.T) {
	cases := []struct {
		in, want int
	}{
		{0, 60},
		{30, 30},
		{600, 600},
		{1000, 600},
	}
	for _, tc := range cases {
		svc, d, _ := newService(t)
		ctx := context.Background()
		if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Exec(ctx, "alpha", "true", tc.in); err != nil {
			t.Fatalf("Exec timeout %d: %v", tc.in, err)
		}
		call := d.execs[len(d.execs)-1]
		if got := call.argv[3]; got != strconv.Itoa(tc.want) {
			t.Errorf("timeout %d: argv limit = %s, want %d", tc.in, got, tc.want)
		}
		want := time.Duration(tc.want+15) * time.Second
		if call.left > want || call.left < want-time.Second {
			t.Errorf("timeout %d: context left %v, want about %v", tc.in, call.left, want)
		}
	}
}

func TestExecRejectsBadInput(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		command string
		timeout int
	}{
		{"", 0},
		{"   ", 0},
		{"true", -1},
	}
	for _, tc := range cases {
		if _, err := svc.Exec(ctx, "alpha", tc.command, tc.timeout); !errors.Is(err, core.ErrInvalid) {
			t.Errorf("Exec(%q, %d) = %v, want ErrInvalid", tc.command, tc.timeout, err)
		}
	}
	if len(d.execs) != 0 {
		t.Errorf("driver Exec called for rejected input")
	}
}

func TestExecRestartsStoppedBox(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	box, err := svc.Ensure(ctx, "alpha", "", "")
	if err != nil {
		t.Fatal(err)
	}
	box.State = "exited"
	d.boxes["alpha"] = box
	before := len(d.ensures)

	if _, err := svc.Exec(ctx, "alpha", "true", 0); err != nil {
		t.Fatal(err)
	}
	if len(d.ensures) != before+1 {
		t.Fatalf("driver Ensure calls = %d, want %d", len(d.ensures), before+1)
	}
	last := d.ensures[len(d.ensures)-1]
	if last.Workspace != box.Workspace || last.Image != box.Image {
		t.Errorf("restart spec = %+v, want workspace %q and image %q", last, box.Workspace, box.Image)
	}
	if d.boxes["alpha"].State != "running" {
		t.Errorf("box state after exec = %q, want running", d.boxes["alpha"].State)
	}
	if len(d.execs) != 1 {
		t.Fatalf("driver Exec calls = %d, want 1", len(d.execs))
	}

	// A running box is not restarted again.
	if _, err := svc.Exec(ctx, "alpha", "true", 0); err != nil {
		t.Fatal(err)
	}
	if len(d.ensures) != before+1 {
		t.Errorf("running box was restarted: Ensure calls = %d", len(d.ensures))
	}
}

func TestExecTruncatesLongOutput(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}

	d.output = strings.Repeat("q", 30000)
	res, err := svc.Exec(ctx, "alpha", "short", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != d.output {
		t.Errorf("30000 bytes of output changed: got %d bytes", len(res.Output))
	}

	d.output = strings.Repeat("a", 20000) + strings.Repeat("m", 20000) + strings.Repeat("z", 20000)
	res, err = svc.Exec(ctx, "alpha", "long", 0)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("a", 15000) + "\n[... output truncated ...]\n" + strings.Repeat("z", 15000)
	if res.Output != want {
		t.Errorf("truncated output: got %d bytes, want %d bytes made of the first and last 15000", len(res.Output), len(want))
	}
}

func TestExecExitStatus(t *testing.T) {
	svc, d, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}

	d.exitCode = 124
	res, err := svc.Exec(ctx, "alpha", "sleep 999", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.ExitCode != 124 {
		t.Errorf("exit 124 result = %+v, want TimedOut and code 124", res)
	}

	d.exitCode = 1
	res, err = svc.Exec(ctx, "alpha", "false", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.TimedOut || res.ExitCode != 1 {
		t.Errorf("exit 1 result = %+v, want not TimedOut and code 1", res)
	}
}

func TestUnknownBoxIsNotFound(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Box(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Box = %v, want ErrNotFound", err)
	}
	if err := svc.Remove(ctx, "ghost"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Remove = %v, want ErrNotFound", err)
	}
	if _, err := svc.Exec(ctx, "ghost", "true", 0); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Exec = %v, want ErrNotFound", err)
	}
}

func TestBoxesIsNeverNil(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	boxes, err := svc.Boxes(ctx)
	if err != nil || boxes == nil || len(boxes) != 0 {
		t.Fatalf("empty Boxes = %v, %v; want a non-nil empty slice", boxes, err)
	}
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}
	boxes, err = svc.Boxes(ctx)
	if err != nil || len(boxes) != 1 {
		t.Errorf("Boxes after Ensure = %d boxes, %v; want 1", len(boxes), err)
	}
}

func TestRemoveDeletesBox(t *testing.T) {
	svc, _, _ := newService(t)
	ctx := context.Background()
	if _, err := svc.Ensure(ctx, "alpha", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(ctx, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Box(ctx, "alpha"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Box after Remove = %v, want ErrNotFound", err)
	}
}
