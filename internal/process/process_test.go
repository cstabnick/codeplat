package process_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codeplat/internal/process"
)

func TestStartEmptyArgv(t *testing.T) {
	wait, err := process.Start("", []string{})
	if err == nil {
		t.Fatal("Start with empty argv: want error, got nil")
	}
	if err.Error() != "process: empty command" {
		t.Errorf("Start with empty argv: error = %q, want %q", err.Error(), "process: empty command")
	}
	if wait != nil {
		t.Error("Start with empty argv: want nil wait function")
	}
}

func TestStartSuccessWaitsNil(t *testing.T) {
	wait, err := process.Start("", []string{"true"})
	if err != nil {
		t.Fatalf("Start(true): unexpected error: %v", err)
	}
	if err := wait(); err != nil {
		t.Errorf("wait for true: got %v, want nil", err)
	}
}

func TestStartNonZeroExitWaitError(t *testing.T) {
	wait, err := process.Start("", []string{"sh", "-c", "exit 3"})
	if err != nil {
		t.Fatalf("Start(sh -c exit 3): unexpected error: %v", err)
	}
	if err := wait(); err == nil {
		t.Error("wait for exit 3: got nil, want non-nil error")
	}
}

func TestStartHonoursDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	wait, err := process.Start(dir, []string{"sh", "-c", "test -f marker"})
	if err != nil {
		t.Fatalf("Start in dir: unexpected error: %v", err)
	}
	if err := wait(); err != nil {
		t.Errorf("wait for test -f marker in dir: got %v, want nil", err)
	}
}

func TestStartMissingProgram(t *testing.T) {
	wait, err := process.Start("", []string{"codeplat-no-such-program"})
	if err == nil {
		t.Fatal("Start with missing program: want error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "process: start codeplat-no-such-program") {
		t.Errorf("Start with missing program: error = %q, want prefix %q", err.Error(), "process: start codeplat-no-such-program")
	}
	if wait != nil {
		t.Error("Start with missing program: want nil wait function")
	}
}
