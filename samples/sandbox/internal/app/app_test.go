package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"sandbox/internal/app"
	"sandbox/internal/apptest"
	"sandbox/internal/config"
	"sandbox/internal/core"
)

func TestEnsureCreatesWorkspaceUnderRoot(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t)

	if _, err := svc.Ensure(ctx, "demo", "", ""); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	root := rootOf(t, svc)
	if fi, err := os.Stat(filepath.Join(root, "demo")); err != nil || !fi.IsDir() {
		t.Fatalf("workspace %s/demo: stat err %v", root, err)
	}
}

func TestExecRunsInWorkspace(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t)
	if _, err := svc.Ensure(ctx, "demo", "", ""); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	res, err := svc.Exec(ctx, "demo", "echo hi > f.txt && cat f.txt", 0)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.Output != "hi\n" {
		t.Errorf("Output = %q, want %q", res.Output, "hi\n")
	}
	if res.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", res.ExitCode)
	}
	if _, err := os.Stat(filepath.Join(rootOf(t, svc), "demo", "f.txt")); err != nil {
		t.Errorf("f.txt on host: %v", err)
	}
}

func TestExecReportsExitCode(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t)
	if _, err := svc.Ensure(ctx, "demo", "", ""); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	res, err := svc.Exec(ctx, "demo", "exit 3", 0)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if res.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", res.ExitCode)
	}
}

func TestRemoveThenBoxIsNotFound(t *testing.T) {
	ctx := context.Background()
	svc := apptest.New(t)
	if _, err := svc.Ensure(ctx, "demo", "", ""); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := svc.Remove(ctx, "demo"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := svc.Box(ctx, "demo")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Box after Remove: err = %v, want ErrNotFound", err)
	}
}

func TestNewRejectsEmptyRoot(t *testing.T) {
	cfg := testConfig(t)
	cfg.Root = ""
	if _, err := app.New(cfg); err == nil {
		t.Fatal("New with empty root: want error")
	}
}

func TestNewRejectsUnknownDriver(t *testing.T) {
	cfg := testConfig(t)
	cfg.Driver = "nope"
	if _, err := app.New(cfg); err == nil {
		t.Fatal("New with unknown driver: want error")
	}
}

func TestNewDockerDoesNotDialAtStartup(t *testing.T) {
	cfg, err := config.Load("dev")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root = t.TempDir()
	cfg.DockerHost = "unix:///nonexistent.sock"
	if _, err := app.New(cfg); err != nil {
		t.Fatalf("New with unreachable docker socket: %v", err)
	}
}

// testConfig returns the test config with Root set to a temporary directory.
func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root = t.TempDir()
	return cfg
}

// rootOf returns the temporary root that apptest chose. The workspace of a
// box named "demo" is Root/demo, so the root is found by asking the service
// for the box's workspace after Ensure.
func rootOf(t *testing.T, svc *core.Service) string {
	t.Helper()
	box, err := svc.Box(context.Background(), "demo")
	if err != nil {
		t.Fatalf("Box: %v", err)
	}
	return filepath.Dir(box.Workspace)
}
