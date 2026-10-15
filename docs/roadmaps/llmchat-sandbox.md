# Roadmap: run llmchat's shell commands in sandbox containers

## Goal

llmchat can run each conversation's `bash` tool calls inside a Docker container (a "box") managed by the sandbox server (`samples/sandbox`, port 8087, project `BOX`), instead of on the host. Every new conversation can get its own project directory, so a conversation becomes a project being built in its own container. This is the first step to llmchat as a project-building app.

The choice is a port chosen by config: `CHAT_EXEC=local` (today's behaviour, the default) or `CHAT_EXEC=sandbox`. The box mounts the workspace at the same path as on the host, so the file tools (read, list, grep, write, edit) keep working on the host and see the same files the container does.

Tracker project: `CHAT`, a new goal beside CHAT-1. It is built in parallel with BOX, against the sandbox HTTP contract pasted into issue 1.2.

Note: `samples/llmchat` has uncommitted changes from CHAT-22 (app, api, web, e2e, live). Commit them before these issues start, because the workers edit `internal/app/app.go` and `internal/tools/shell/shell.go`.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Parts | The shell tool takes a Runner; the sandbox client and the new config fields pass their tests. | 3, parallel |
| 2. Wiring | The app picks the runner and gives each new conversation a project directory; the README and a live test describe and check it. | 2, parallel |

No issue has a dependency inside its epic. The sandbox client satisfies `shell.Runner` without importing `shell`: the interface uses only built-in types, so the two packages are written at the same time.

---

## Epic 1: Parts

### 1.1 Let the bash tool run commands through a Runner

~~~~text
Epic: 1 Parts
Depends on: none

## Environment
- Work in `samples/llmchat/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are adding `internal/sandboxclient` and editing `internal/config` right now. Change only the files below. `internal/app/app.go` calls `shell.Bash()`; keep that working unchanged.
- Rule: ports are declared by the package that uses them, so the Runner interface lives in shell.

## Goal
The bash tool keeps its spec, approval and output format, but where the command runs becomes a constructor argument.

## Read first
- samples/llmchat/internal/tools/shell/shell.go and shell_test.go.

## Change
- samples/llmchat/internal/tools/shell/shell.go and shell_test.go.

## Contract
```go
package shell

// Runner runs one command for the bash tool. exitCode is -1 when the process was killed by a signal.
type Runner interface {
	Run(ctx context.Context, workspace, command string, timeout time.Duration) (output string, exitCode int, timedOut bool, err error)
}

// Local runs commands with bash on this machine, in the workspace (today's behaviour:
// sandbox.Resolve, process group, kill on timeout, head/tail capture).
type Local struct{}
func (Local) Run(ctx context.Context, workspace, command string, timeout time.Duration) (string, int, bool, error)

func Bash() chat.Tool              // = BashWith(Local{})
func BashWith(r Runner) chat.Tool
```
The tool still decodes args, rejects an empty command, and applies the timeout default 60s and cap 300s. It calls r.Run and formats the result exactly as today: output, a newline if missing, then `exit status N`, `terminated by signal` (exitCode -1) or `timed out after Ns`. A Runner error is returned as the tool's error, as cancellation is today.

## Done when
- `go test ./internal/tools/shell/` passes: the existing tests still pass unchanged against Bash(); a new test passes a fake Runner to BashWith and checks the workspace, command and timeout it receives and the formatted text for exit 0, exit 2, -1 and timed out.
- `go build -o /dev/null ./cmd/llmchat` succeeds.

## Out of scope
The sandbox client, config, app wiring.
~~~~

### 1.2 Add the sandbox client

~~~~text
Epic: 1 Parts
Depends on: none

## Environment
- Work in `samples/llmchat/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are editing `internal/tools/shell` and `internal/config` right now. Do not import shell; match the method signature below. Change only the files below.

## Goal
A client for the sandbox server that runs a command in the workspace's box, creating the box first if needed.

## Read first
- samples/llmchat/internal/routerclient/routerclient.go: error-message style (`router unreachable`, status plus 512 bytes of body).

## Change
- samples/llmchat/internal/sandboxclient/sandboxclient.go and sandboxclient_test.go.

## Contract
```go
package sandboxclient

// New: baseURL like "http://127.0.0.1:8087" (trailing slash ignored); hc nil means &http.Client{} (callers bound calls with ctx).
func New(baseURL string, hc *http.Client) *Client
// Run satisfies shell.Runner (do not import shell).
func (c *Client) Run(ctx context.Context, workspace, command string, timeout time.Duration) (output string, exitCode int, timedOut bool, err error)
// BoxName derives the box name from the workspace: lower-case filepath.Base, every character outside [a-z0-9-]
// becomes "-", leading "-" trimmed, cut to 63 characters; empty becomes "workspace".
func BoxName(workspace string) string
```
The sandbox server's API (the sandbox sample, built at the same time):
- `PUT /api/boxes/{name}` with `{"workspace": "<absolute host path>"}` returns 200 with the Box `{"name", "image", "workspace", "container_id", "state", "created_at"}`. It creates and starts the box when needed. The workspace is mounted at the same path inside it.
- `POST /api/boxes/{name}/exec` with `{"command": "...", "timeout_seconds": N}` returns 200 `{"output": "...", "exit_code": 0, "timed_out": false}`. The output is already trimmed by the server.
- Errors are `{"error": "..."}` with 400, 404, 503 (Docker unavailable) or 500.
Run: PUT the box (name from BoxName(workspace)), then POST exec with timeout_seconds = int(timeout / time.Second) (at least 1). Non-2xx: `sandbox returned <status>: <first 512 bytes of body>`. Connection failure: `sandbox unreachable: <err>`, or ctx.Err() if the context ended.

## Done when
- `go test ./internal/sandboxclient/` passes with an httptest fake: the PUT path and body, then the exec body; output, exit code and timed_out returned; a 503 on PUT gives the status error; unreachable; BoxName for "/x/0a1b2c", "/x/My Project!", "/".

## Out of scope
The shell tool, config, app wiring.
~~~~

### 1.3 Add the exec settings to the llmchat config

~~~~text
Epic: 1 Parts
Depends on: none

## Environment
- Work in `samples/llmchat/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Other workers are editing `internal/tools/shell` and adding `internal/sandboxclient` right now. Change only the files below.
- Rule: config only describes; it never constructs. Only `internal/app` (a later issue) does.

## Goal
Settings that say where bash runs and where new conversations' project directories go.

## Read first
- samples/llmchat/internal/config/config.go and config_test.go.

## Change
- samples/llmchat/internal/config/config.go and config_test.go.

## Contract
Add to Config:
```go
	Exec        string // "local" or "sandbox"
	SandboxURL  string // sandbox server base URL
	ProjectsDir string // when set, each new conversation without a workspace gets <ProjectsDir>/<id>; empty means none
```
FromEnvironment reads:
- CHAT_EXEC: default "local". Any value other than local or sandbox is an error from FromEnvironment.
- CHAT_SANDBOX_URL: default "http://127.0.0.1:8087".
- CHAT_PROJECTS_DIR: default "", except that with CHAT_EXEC=sandbox it defaults to $XDG_DATA_HOME/sandbox/workspaces (else ~/.local/share/sandbox/workspaces), which is the sandbox server's default root.
An empty variable counts as unset, as for the others. Update the doc comment.

## Done when
- `go test ./internal/config/` passes: the existing tests updated for the new fields (defaults: Exec "local", SandboxURL as above, ProjectsDir ""), plus sandbox mode with and without CHAT_PROJECTS_DIR, and CHAT_EXEC=docker being an error.

## Out of scope
Using the settings (app wiring is a later issue).
~~~~

---

## Epic 2: Wiring

### 2.1 Run bash in the sandbox and give each conversation a project directory

~~~~text
Epic: 2 Wiring
Depends on: none

## Environment
- Work in `samples/llmchat/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Another worker is editing README.md and adding internal/live/live_sandbox_test.go right now. Change only the files below.
- Rule: `internal/app` is the only package that chooses an implementation from config.

## Goal
With CHAT_EXEC=sandbox, the bash tool runs in the conversation's box. With CHAT_PROJECTS_DIR set, a new conversation without a workspace gets its own project directory.

## Read first
- samples/llmchat/internal/app/app.go: New, SystemPrompt, defaultWorkspaceStore.
- samples/llmchat/internal/config/config.go: Exec, SandboxURL, ProjectsDir.
- samples/llmchat/internal/tools/shell/shell.go: Bash, BashWith, Local. samples/llmchat/internal/sandboxclient/sandboxclient.go: New.
- samples/llmchat/internal/e2e/agent_test.go: how a fake router drives a tool call and an approval.

## Change
- samples/llmchat/internal/app/app.go and app_test.go.
- samples/llmchat/internal/e2e/sandbox_test.go (new).

## Contract
- Runner: Exec "" or "local" -> shell.Local{}; "sandbox" -> sandboxclient.New(cfg.SandboxURL, nil); anything else is an error from New. The bash tool is shell.BashWith(runner).
- In sandbox mode, append to the system prompt: ` Shell commands run in a Linux container where the workspace is mounted at the same path. Install what you need there.`
- defaultWorkspaceStore.CreateConversation: if the workspace is empty, use cfg.Workspace when set. Otherwise, if cfg.ProjectsDir is set, create the conversation, then `<ProjectsDir>/<id>` with os.MkdirAll(0o755), then UpdateConversation with that Workspace, and return the updated conversation.

## Done when
- `go vet ./... && go test ./...` passes.
- app_test: an unknown Exec fails; ProjectsDir (a t.TempDir()) gives a new conversation the workspace `<dir>/<id>`, which exists; CHAT_WORKSPACE-style cfg.Workspace still wins.
- e2e/sandbox_test.go: an httptest fake sandbox server plus the fake router from agent_test.go. A bash call, once approved, reaches the fake's PUT and exec with the conversation's workspace, and the tool result contains the fake's output and `exit status 0`.

## Out of scope
The sandbox server itself; README; the web page.
~~~~

### 2.2 Document sandbox mode and add a live sandbox test

~~~~text
Epic: 2 Wiring
Depends on: none

## Environment
- Work in `samples/llmchat/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod or go.sum; never run `go mod tidy`.
- Another worker is editing internal/app and adding internal/e2e/sandbox_test.go right now. Change only the files below.

## Goal
Tell a reader how to run llmchat with its commands in containers, and add a test that checks a real sandbox server when asked to.

## Read first
- samples/llmchat/README.md: the Configuration table and the agent section.
- samples/llmchat/internal/live/live_test.go: how live tests skip.
- samples/llmchat/internal/sandboxclient/sandboxclient.go: New, Run.

## Change
- samples/llmchat/README.md: add CHAT_EXEC, CHAT_SANDBOX_URL and CHAT_PROJECTS_DIR to the Configuration table, and a "Sandbox mode" section covering:
  - Start `samples/sandbox` (`make run`, port 8087; Docker must be reachable from WSL, see that README), then `CHAT_EXEC=sandbox make run`.
  - Each new conversation gets `<projects dir>/<id>`, and its bash calls run in the box named after that directory.
  - File tools still work on the host, on the same files.
  - Approval still applies to every bash call.
- samples/llmchat/internal/live/live_sandbox_test.go: skip unless `LLMCHAT_SANDBOX_LIVE=1`. Server URL from `CHAT_SANDBOX_URL` (default http://127.0.0.1:8087). The workspace is a new directory under `$XDG_DATA_HOME/sandbox/workspaces` (else ~/.local/share/sandbox/workspaces) named `live-<unix seconds>`, removed in t.Cleanup. Run `echo hi > f.txt && cat f.txt && id -u`: exit 0, output starts with "hi", and the file exists on the host.

## Done when
- `go vet ./internal/live/` succeeds and `go test ./internal/live/` passes (it skips).
- Every variable and path in the README exists in the code or in this issue.

## Out of scope
Go code outside internal/live.
~~~~
