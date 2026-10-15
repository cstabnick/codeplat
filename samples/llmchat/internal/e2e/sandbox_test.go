package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"llmchat/internal/apptest"
	"llmchat/internal/chat"
	"llmchat/internal/config"
	"llmchat/internal/sandboxclient"
)

// sbxCall is one request the fake sandbox server received.
type sbxCall struct {
	method string
	path   string
	body   map[string]any
}

// sbxServer is a fake sandbox server. It records every request and answers
// each exec with a fixed output and exit status 0.
type sbxServer struct {
	*httptest.Server

	mu    sync.Mutex
	calls []sbxCall
}

func newSbxServer(t *testing.T) *sbxServer {
	t.Helper()
	f := &sbxServer{}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *sbxServer) serve(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	if len(raw) > 0 {
		json.Unmarshal(raw, &body)
	}
	f.mu.Lock()
	f.calls = append(f.calls, sbxCall{method: r.Method, path: r.URL.Path, body: body})
	f.mu.Unlock()

	switch {
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/api/boxes/"):
		io.WriteString(w, `{}`)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/boxes/") && strings.HasSuffix(r.URL.Path, "/exec"):
		io.WriteString(w, `{"output":"sandbox out\n","exit_code":0,"timed_out":false}`)
	default:
		http.NotFound(w, r)
	}
}

func (f *sbxServer) recorded() []sbxCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]sbxCall(nil), f.calls...)
}

func TestBashRunsInSandboxBox(t *testing.T) {
	sandbox := newSbxServer(t)
	router := newAgentRouter(t)
	a := apptest.NewWith(t, config.Config{
		Addr:       "127.0.0.1:0",
		DBPath:     filepath.Join(t.TempDir(), "llmchat.db"),
		RouterURL:  router.URL,
		Exec:       "sandbox",
		SandboxURL: sandbox.URL,
	})
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)

	ws := t.TempDir()
	conv := agentCreate(t, srv.URL, ws)

	events := agentSend(t, srv.URL, conv.ID, "run echo", agentBool(true))
	ai, ri := agentIndex(events, chat.EventApproval), agentIndex(events, chat.EventToolResult)
	if ai < 0 || ri < 0 || events[ai].Name != "bash" {
		t.Fatalf("events = %+v, want a bash approval and a tool_result", events)
	}
	if out := events[ri].Text; !strings.Contains(out, "sandbox out") || !strings.Contains(out, "exit status 0") {
		t.Fatalf("bash result = %q, want sandbox out and exit status 0", out)
	}

	// The system prompt says where shell commands run.
	reqs := router.requests()
	if len(reqs) == 0 || len(reqs[0].Messages) == 0 {
		t.Fatalf("router got no requests")
	}
	if sys := reqs[0].Messages[0].Content; !strings.Contains(sys, "Shell commands run in a Linux container") {
		t.Errorf("system prompt = %q, want the sandbox note", sys)
	}

	// The box is created for the conversation's workspace, then the command runs in it.
	box := sandboxclient.BoxName(ws)
	var put, exec *sbxCall
	for _, c := range sandbox.recorded() {
		c := c
		switch {
		case c.method == http.MethodPut && c.path == "/api/boxes/"+box:
			put = &c
		case c.method == http.MethodPost && c.path == "/api/boxes/"+box+"/exec":
			exec = &c
		}
	}
	if put == nil {
		t.Fatalf("no PUT /api/boxes/%s; calls %+v", box, sandbox.recorded())
	}
	if got := put.body["workspace"]; got != ws {
		t.Errorf("PUT workspace = %v, want %q", got, ws)
	}
	if exec == nil {
		t.Fatalf("no POST /api/boxes/%s/exec; calls %+v", box, sandbox.recorded())
	}
	if got := exec.body["command"]; got != "echo ran" {
		t.Errorf("exec command = %v, want %q", got, "echo ran")
	}
}
