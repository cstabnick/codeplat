// Package live holds an opt-in end-to-end test of the Docker driver. It needs
// a reachable Docker daemon (docker_host from dev.json) and runs only when
// SANDBOX_LIVE=1 is set, as `make live` does.
package live

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"sandbox/internal/app"
	"sandbox/internal/config"
)

func TestDockerLive(t *testing.T) {
	if os.Getenv("SANDBOX_LIVE") != "1" {
		t.Skip("set SANDBOX_LIVE=1 to run against a real Docker daemon")
	}

	cfg, err := config.FromEnvironment()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cfg.Driver = config.DriverDocker
	cfg.Root = t.TempDir()

	svc, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New with docker driver (is Docker reachable at %q?): %v", cfg.DockerHost, err)
	}

	name := fmt.Sprintf("live-%d", time.Now().Unix())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = svc.Remove(ctx, name) // already removed or never created is fine
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. Ensure creates and starts the box.
	box, err := svc.Ensure(ctx, name, "", "")
	if err != nil {
		t.Fatalf("ensure %s (is Docker reachable at %q?): %v", name, cfg.DockerHost, err)
	}
	if box.State != "running" {
		t.Fatalf("ensure: state = %q, want \"running\"", box.State)
	}

	// 2. A command writes into the workspace, and the host sees the file.
	res, err := svc.Exec(ctx, name, "echo hello > from-box.txt && id -u", 60)
	if err != nil {
		t.Fatalf("exec write: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exec write: exit %d, output %q", res.ExitCode, res.Output)
	}
	data, err := os.ReadFile(filepath.Join(cfg.Root, name, "from-box.txt"))
	if err != nil {
		t.Fatalf("read file written in box: %v", err)
	}
	if string(data) != "hello\n" {
		t.Fatalf("from-box.txt = %q, want \"hello\\n\"", data)
	}

	// 3. A command that outlives its timeout is reported as timed out.
	res, err = svc.Exec(ctx, name, "sleep 5", 1)
	if err != nil {
		t.Fatalf("exec timeout: %v", err)
	}
	if !res.TimedOut {
		t.Fatalf("exec timeout: TimedOut = false, exit %d, output %q", res.ExitCode, res.Output)
	}

	// 4. The box has a Linux userland; log what it reports.
	res, err = svc.Exec(ctx, name, "cat /etc/os-release", 60)
	if err != nil {
		t.Fatalf("exec os-release: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exec os-release: exit %d, output %q", res.ExitCode, res.Output)
	}
	t.Logf("os-release in box %s:\n%s", name, res.Output)
}
