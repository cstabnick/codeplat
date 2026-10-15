package live_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"llmchat/internal/sandboxclient"
)

// The sandbox test runs a command in a real sandbox server's box. It skips
// unless LLMCHAT_SANDBOX_LIVE=1 is set and the server answers /healthz. Start
// the server first (samples/sandbox, port 8087 by default), then run:
//
//	LLMCHAT_SANDBOX_LIVE=1 go test ./internal/live/ -run Sandbox

const defaultSandbox = "http://127.0.0.1:8087"

func requireSandboxLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LLMCHAT_SANDBOX_LIVE") != "1" {
		t.Skip("set LLMCHAT_SANDBOX_LIVE=1 to run against a real sandbox server")
	}
}

func sandboxURL() string {
	if v := os.Getenv("CHAT_SANDBOX_URL"); v != "" {
		return v
	}
	return defaultSandbox
}

// skipIfSandboxUnreachable skips the test when the sandbox server's health
// check cannot be reached.
func skipIfSandboxUnreachable(t *testing.T, base string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(strings.TrimRight(base, "/") + "/healthz")
	if err != nil {
		t.Skipf("sandbox not reachable at %s: %v", base, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("sandbox not reachable at %s: GET /healthz returned %d", base, resp.StatusCode)
	}
}

// sandboxWorkspaceRoot is the sandbox server's default workspaces root:
// $XDG_DATA_HOME/sandbox/workspaces when XDG_DATA_HOME is an absolute path,
// else ~/.local/share/sandbox/workspaces.
func sandboxWorkspaceRoot(t *testing.T) string {
	t.Helper()
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "sandbox", "workspaces")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("home directory: %v", err)
	}
	return filepath.Join(home, ".local", "share", "sandbox", "workspaces")
}

func TestSandboxRun(t *testing.T) {
	requireSandboxLive(t)
	base := sandboxURL()
	skipIfSandboxUnreachable(t, base)

	root := sandboxWorkspaceRoot(t)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create workspaces root: %v", err)
	}
	ws := filepath.Join(root, "live-"+strconv.FormatInt(time.Now().Unix(), 10))
	if err := os.Mkdir(ws, 0o755); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(ws) })

	c := sandboxclient.New(base, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	out, code, timedOut, err := c.Run(ctx, ws, "echo hi > f.txt && cat f.txt && id -u", 60*time.Second)
	if err != nil {
		t.Fatalf("run in sandbox: %v", err)
	}
	if timedOut {
		t.Fatalf("command timed out; output %q", out)
	}
	if code != 0 {
		t.Fatalf("exit code %d, want 0; output %q", code, out)
	}
	if !strings.HasPrefix(out, "hi") {
		t.Fatalf("output %q does not start with %q", out, "hi")
	}
	t.Logf("output: %q", out)

	data, err := os.ReadFile(filepath.Join(ws, "f.txt"))
	if err != nil {
		t.Fatalf("file written in the box is not on the host: %v", err)
	}
	if strings.TrimSpace(string(data)) != "hi" {
		t.Fatalf("host file content %q, want %q", data, "hi")
	}
}
