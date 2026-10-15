# Roadmap: sandbox sample

## Goal

A sample project in `samples/sandbox/`: a server that gives each project a Docker container (a "box") to work in. A box bind-mounts a host directory at the same path inside the container, so a file written by either side is the same file. Callers create a box with `PUT /api/boxes/{name}` and run commands in it with `POST /api/boxes/{name}/exec`. llmchat (project `CHAT`, goal "Run llmchat's shell commands in sandbox containers") uses it so the model's `bash` calls run in a container instead of on the host.

The server talks to Docker through the Docker Engine HTTP API on its socket, using only the standard library: no Docker CLI and no SDK. The driver is a port chosen by config: `docker`, or `local` (runs commands on the host with no isolation, used by tests and by machines without Docker).

Its own Go module, `sandbox`, standard library only. Tracker project: `BOX`. It follows the platform's dependency rules: config chooses, `app.New` wires, pure constructor injection, thin `main`, business rules in `core.Service`, ports declared in `core`, tests through `apptest.New`.

Facts checked on this machine (2026-10-14): Docker Desktop runs on Windows, but its WSL integration is off for this distro. `docker` on PATH is the Windows stub ("could not be found in this WSL 2 distro"), and `/var/run/docker.sock` does not exist. The proxy socket under `/mnt/wsl/docker-desktop/shared-sockets/guest-services/` is root-only. Enabling Docker Desktop → Settings → Resources → WSL integration for this distro creates `/var/run/docker.sock`. Until then, only the opt-in live test (issue 4.3) cannot run; every other test uses the `local` driver or a fake Engine API.

## Shape

| Epic | Done when | Issues |
|---|---|---|
| 1. Foundation | The module exists; `core` (types, Driver port, Service rules) passes its tests. | 1 |
| 2. Components | The Docker driver, the local driver and the config each pass their own tests. | 3, parallel |
| 3. Wiring | `app.New` builds the service from config; `apptest.New` exists. | 1 |
| 4. Serve | The HTTP server works through `apptest`; README and ARCHITECTURE exist; an opt-in live Docker test exists. | 3, parallel |

No issue has a dependency inside its epic. The HTTP contract in issue 4.1 is the one llmchat's client is built against.

---

## Epic 1: Foundation

### 1.1 Create the sandbox module and its core package

~~~~text
Epic: 1 Foundation
Depends on: none

## Environment
- Create `samples/sandbox/` under the repository root and run every command from it. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Its own Go module, standard library only. The root CLAUDE.md and ARCHITECTURE.md describe another project; do not edit anything outside `samples/sandbox/`.
- Rules: dependencies are constructor arguments (no globals, no init registration); business rules live in `internal/core`; ports are declared in core, which uses them.

## Goal
The module and the core package: box types, the Driver port, and the Service holding every rule about names, workspaces, timeouts and output size.

## Read first
- samples/llmrouter/Makefile: copy its shape.
- samples/llmchat/internal/tools/shell/shell.go: its `capture` type keeps the head and tail of long output; reuse the idea.

## Change
- samples/sandbox/go.mod (`module sandbox`, `go 1.27.2`), Makefile (.PHONY `test`, `vet`, `run` = `go run ./cmd/sandbox-server`, `live` = `SANDBOX_LIVE=1 go test -count=1 -v -timeout 10m ./internal/live/`).
- samples/sandbox/internal/core/core.go, service.go, service_test.go.

## Contract
```go
package core

var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid")
	ErrUnavailable = errors.New("driver unavailable") // drivers wrap connection failures in it
)

var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

type BoxSpec struct {
	Name      string
	Image     string
	Workspace string // absolute host path, mounted at the same path in the box
	MemoryMB  int     // 0 = no limit
	CPUs      float64 // 0 = no limit
	Network   string  // "bridge" or "none"
	User      string  // "uid:gid", "" = image default
}

type Box struct {
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	Workspace   string    `json:"workspace"`
	ContainerID string    `json:"container_id"`
	State       string    `json:"state"` // "running", "exited", "created", ...
	CreatedAt   time.Time `json:"created_at"`
}

type Driver interface {
	// Ensure creates the box if missing and starts it if not running; it returns the box as it now is.
	Ensure(ctx context.Context, spec BoxSpec) (Box, error)
	Get(ctx context.Context, name string) (Box, error) // ErrNotFound
	List(ctx context.Context) ([]Box, error)
	Remove(ctx context.Context, name string) error // ErrNotFound
	// Exec runs argv in the box with workdir as its directory, writing stdout and stderr to out.
	Exec(ctx context.Context, name string, argv []string, workdir string, out io.Writer) (exitCode int, err error)
}

type Options struct {
	Root     string // absolute; workspaces must be inside it
	Image    string // default image
	MemoryMB int
	CPUs     float64
	Network  string
	User     string
}

type ExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}

func NewService(d Driver, opts Options) *Service
func (s *Service) Ensure(ctx context.Context, name, workspace, image string) (Box, error)
func (s *Service) Box(ctx context.Context, name string) (Box, error)
func (s *Service) Boxes(ctx context.Context) ([]Box, error) // never nil
func (s *Service) Remove(ctx context.Context, name string) error
func (s *Service) Exec(ctx context.Context, name, command string, timeoutSeconds int) (ExecResult, error)
```
Service rules:
- A name not matching NamePattern is ErrInvalid.
- Ensure: workspace "" means `<Root>/<name>`. It must be absolute. Create it with os.MkdirAll(0o755), then resolve symlinks on it and on Root. It must be strictly inside Root, else ErrInvalid. Image "" means opts.Image. If the driver already has a box with this name and a different Workspace, that is ErrInvalid (`box "x" already uses workspace "/..."`). Otherwise call driver.Ensure with a BoxSpec built from opts.
- Exec: an empty command is ErrInvalid. timeoutSeconds 0 means 60, negative is ErrInvalid, above 600 becomes 600. Get the box (ErrNotFound passes through). If State is not "running", driver.Ensure it again from its own Workspace and Image. Run argv `["timeout", "-k", "5", "<t>", "bash", "-c", command]` with workdir = box.Workspace, under a context with timeout t+15s. Keep the first and last 15000 bytes of output, joined by "\n[... output truncated ...]\n" when longer. Exit code 124 sets TimedOut.

## Done when
- `go vet ./... && go test ./...` passes in samples/sandbox.
- service_test uses a fake Driver written in the test and `t.TempDir()` roots, and covers: bad name, default workspace created under Root, workspace outside Root (including through a symlink), workspace conflict, default image, Exec argv and workdir, timeout defaults and cap, restart of a stopped box, output truncation, exit 124 means TimedOut.

## Out of scope
Any Driver implementation, config, HTTP.
~~~~

---

## Epic 2: Components

### 2.1 Add the Docker Engine API driver

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod; never run `go mod tidy`.
- Other workers are writing `internal/localdriver` and `internal/config` right now. Change only the files below.
- Docker is not reachable from this machine's tests. Test against a fake Engine API served by your test on a unix socket.

## Goal
core.Driver backed by the Docker Engine HTTP API, so boxes are real containers.

## Read first
- samples/sandbox/internal/core/core.go: Driver, BoxSpec, Box, ErrNotFound, ErrUnavailable.

## Change
- samples/sandbox/internal/docker/docker.go and docker_test.go (split into engine.go and driver.go if you like).

## Contract
```go
package docker

// New accepts host "unix:///path/to.sock" (dial that socket) or "tcp://h:p" / "http://h:p".
// hc nil means a new http.Client with that transport and no timeout. Other schemes are an error.
func New(host string, hc *http.Client) (*Driver, error)

var _ core.Driver = (*Driver)(nil)
```
Requests go to `http://docker` plus the paths below, with no API version prefix. Connection failures wrap core.ErrUnavailable. Other non-2xx responses become errors carrying the status and Docker's `{"message"}`.
- Container name `sandbox-<name>`. Labels `codeplat.sandbox=1`, `codeplat.sandbox.name=<name>`, `codeplat.sandbox.workspace=<workspace>`.
- Get: `GET /containers/sandbox-<name>/json`. 404 means core.ErrNotFound. The Box comes from `Id`, `State.Status`, `Created` (RFC 3339), `Config.Image` and the workspace label.
- Ensure: Get. If not found, `POST /containers/create?name=sandbox-<name>` with `{"Image", "Cmd": ["sleep", "infinity"], "WorkingDir": ws, "User", "Labels", "HostConfig": {"Binds": ["<ws>:<ws>"], "Memory": MemoryMB<<20, "NanoCpus": int64(CPUs*1e9), "NetworkMode": Network}}`. If that returns 404 (image missing), `POST /images/create?fromImage=<repo>&tag=<tag>` (tag defaults to "latest"), read the JSON-lines stream to the end and fail on any line with "error", then create again. Then, if not running, `POST /containers/<id>/start` (204 or 304 are fine). Return Get.
- List: `GET /containers/json?all=1&filters=<url-encoded {"label":["codeplat.sandbox=1"]}>`. Map `Names[0]` without "/sandbox-", `State`, `Image`, `Created` (unix seconds) and the labels.
- Remove: `DELETE /containers/sandbox-<name>?force=1`. 404 means core.ErrNotFound.
- Exec: `POST /containers/sandbox-<name>/exec` with `{"AttachStdout": true, "AttachStderr": true, "Cmd": argv, "WorkingDir": workdir}` returns `{"Id"}`. Then `POST /exec/<id>/start` with `{"Detach": false, "Tty": false}`. Its body is multiplexed frames: an 8-byte header (byte 0 = stream 1 or 2, bytes 4..7 = big-endian payload length) followed by the payload; write payloads to out. Then `GET /exec/<id>/json` gives `ExitCode`. Cancelling ctx closes the stream.

## Done when
- `go test ./internal/docker/` passes. The fake Engine listens with `net.Listen("unix", filepath.Join(t.TempDir(), "d.sock"))` and `http.Serve`. Cover: Ensure creating (body fields checked), Ensure pulling after a 404, Ensure on an existing stopped box starting it, Get 404, List, Remove, Exec demultiplexing two frames and returning the exit code, an unreachable socket wrapping core.ErrUnavailable, New rejecting "ftp://x".

## Out of scope
The CLI, Docker SDKs, every other package.
~~~~

### 2.2 Add the local driver

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod; never run `go mod tidy`.
- Other workers are writing `internal/docker` and `internal/config` right now. Change only the files below.

## Goal
core.Driver that runs commands directly on the host, for tests and for machines without Docker. It isolates nothing; say so in the package comment.

## Read first
- samples/sandbox/internal/core/core.go: Driver, BoxSpec, Box, ErrNotFound.
- samples/llmchat/internal/tools/shell/shell.go: process-group kill on cancel (Setpgid, syscall.Kill(-pid, SIGKILL)).

## Change
- samples/sandbox/internal/localdriver/localdriver.go and localdriver_test.go.

## Contract
```go
package localdriver

// New returns a driver that keeps boxes in memory (guarded by a mutex) and runs Exec with os/exec.
func New() *Driver

var _ core.Driver = (*Driver)(nil)
```
- Ensure stores Box{Name, Image, Workspace, ContainerID: "local-<name>", State: "running", CreatedAt: time.Now()} if missing, and returns the stored box.
- Get/Remove of an unknown name wrap core.ErrNotFound. List is sorted by Name.
- Exec on an unknown name wraps core.ErrNotFound. Otherwise it runs `exec.Command(argv[0], argv[1:]...)` with Dir = workdir, stdout and stderr to out, its own process group, killed when ctx ends. It returns the exit code (an ExitError's code; 0 on success).

## Done when
- `go test ./internal/localdriver/` passes: Ensure is idempotent, List order, Remove, Exec `bash -c "echo hi; exit 3"` gives "hi\n" and 3, Exec runs in workdir, ctx cancel stops `sleep 30` quickly.

## Out of scope
Every other package.
~~~~

### 2.3 Add the sandbox config

~~~~text
Epic: 2 Components
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Standard library only. Do not edit go.mod; never run `go mod tidy`.
- Other workers are writing `internal/docker` and `internal/localdriver` right now. Change only the files below.
- Rule: config only describes which implementation to use; it never constructs one. Only `internal/app` (a later issue) does.

## Goal
Per-environment JSON config embedded in the binary, with environment overrides.

## Read first
- config/config.go at the repository root: the platform embeds its configs with `//go:embed *.json`; follow its shape.

## Change
- samples/sandbox/internal/config/config.go, config_test.go, dev.json, test.json.

## Contract
```go
package config

type Config struct {
	Addr       string  `json:"addr"`
	Driver     string  `json:"driver"`                // "docker" | "local"
	DockerHost string  `json:"docker_host,omitempty"` // required for docker
	Root       string  `json:"root"`                  // workspaces root; "" is filled by FromEnvironment
	Image      string  `json:"image"`
	MemoryMB   int     `json:"memory_mb"`
	CPUs       float64 `json:"cpus"`
	Network    string  `json:"network"`        // "bridge" | "none"
	User       string  `json:"user,omitempty"` // "" is filled by FromEnvironment
}

func Load(name string) (Config, error) // embedded <name>.json, DisallowUnknownFields, then Validate
// FromEnvironment loads SANDBOX_ENV (default "dev"). SANDBOX_ADDR, SANDBOX_DRIVER, DOCKER_HOST, SANDBOX_ROOT and SANDBOX_IMAGE
// replace their field when non-empty. Then an empty Root becomes $XDG_DATA_HOME/sandbox/workspaces
// (else ~/.local/share/sandbox/workspaces), and an empty User becomes "<os.Getuid()>:<os.Getgid()>". Then Validate.
func FromEnvironment() (Config, error)
func (c Config) Validate() error // Addr and Image non-empty; known Driver and Network; DockerHost for docker; MemoryMB and CPUs not negative; Root empty or absolute
```
dev.json: `{"addr": "127.0.0.1:8087", "driver": "docker", "docker_host": "unix:///var/run/docker.sock", "root": "", "image": "debian:bookworm-slim", "memory_mb": 2048, "cpus": 2, "network": "bridge"}`
test.json: `{"addr": "127.0.0.1:0", "driver": "local", "root": "", "image": "local", "memory_mb": 0, "cpus": 0, "network": "none"}`

## Done when
- `go test ./internal/config/` passes: both files load; unknown name; unknown field; each Validate error; each variable and both defaults via t.Setenv.

## Out of scope
Every other package.
~~~~

---

## Epic 3: Wiring

### 3.1 Wire the sandbox app and its test harness

~~~~text
Epic: 3 Wiring
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod; never run `go mod tidy`.
- Rules: `internal/app` is the only package that chooses an implementation from config. Dependencies are constructor arguments. No globals.

## Goal
One function that turns a config into the real service, and a test harness that builds it with the test config.

## Read first
- samples/sandbox/internal/config/config.go, samples/sandbox/internal/core/core.go (NewService, Options).
- Constructors: `docker.New(host, nil)` in internal/docker, `localdriver.New()` in internal/localdriver.

## Change
- samples/sandbox/internal/app/app.go and app_test.go.
- samples/sandbox/internal/apptest/apptest.go.

## Contract
```go
package app
// New: cfg.Root must be non-empty and absolute; create it with os.MkdirAll(0o755).
// Driver "docker" -> docker.New(cfg.DockerHost, nil); "local" -> localdriver.New(); anything else is an error.
// Returns core.NewService(driver, core.Options{Root, Image, MemoryMB, CPUs, Network, User} from cfg).
func New(cfg config.Config) (*core.Service, error)

package apptest
// New builds the app from config.Load("test") with Root = t.TempDir(); t.Fatal on error.
func New(t testing.TB) *core.Service
```

## Done when
- `go vet ./... && go test ./...` passes.
- app_test via apptest.New: Ensure "demo" creates `<root>/demo`; Exec "echo hi > f.txt && cat f.txt" gives "hi\n", exit 0, and the file exists on the host; an exit 3 command; Remove then Box is ErrNotFound. app.New with an empty Root or an unknown driver fails. With driver "docker" and an unreachable socket, New still succeeds (it starts nothing).

## Out of scope
HTTP, `cmd/`.
~~~~

---

## Epic 4: Serve

### 4.1 Add the sandbox HTTP server

~~~~text
Epic: 4 Serve
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod; never run `go mod tidy`.
- Other workers are adding README.md, ARCHITECTURE.md and internal/live right now. Change only the files below.
- Rules: a transport decodes its input and maps errors; it validates nothing else. A `main` is: load config, `app.New`, hand the service to the transport. Tests go through `apptest.New(t)`.

## Goal
Serve boxes over HTTP. llmchat's sandbox client is built against exactly this contract.

## Read first
- samples/sandbox/internal/core/core.go: Service, Box, ExecResult, errors.
- samples/sandbox/internal/apptest/apptest.go, samples/sandbox/internal/config/config.go (FromEnvironment).

## Change
- samples/sandbox/internal/httpapi/httpapi.go and httpapi_test.go.
- samples/sandbox/cmd/sandbox-server/main.go.

## Contract
```go
package httpapi
func NewHandler(svc *core.Service) http.Handler
```
| Route | Body | Returns |
|---|---|---|
| `PUT /api/boxes/{name}` | `{"workspace": "", "image": ""}`, both optional | 200 Box |
| `GET /api/boxes` | | 200 array of Box (`[]`, never null) |
| `GET /api/boxes/{name}` | | 200 Box |
| `DELETE /api/boxes/{name}` | | 204 |
| `POST /api/boxes/{name}/exec` | `{"command": "...", "timeout_seconds": 60}` | 200 `{"output", "exit_code", "timed_out"}` |
| `GET /healthz` | | 200 `{"status": "ok"}` |
Box JSON: `{"name", "image", "workspace", "container_id", "state", "created_at"}`. Errors `{"error": "<message>"}`: 404 core.ErrNotFound; 400 core.ErrInvalid or a bad body; 503 core.ErrUnavailable; 500 otherwise. Body limit 1 MiB. JSON content type throughout.
main: `config.FromEnvironment()`, `app.New(cfg)`, `http.Server{Addr, Handler: httpapi.NewHandler(svc), ReadHeaderTimeout: 10 * time.Second}` (no write timeout: exec can take 10 minutes); log `sandbox listening on http://<addr> (driver <driver>, root <root>)`; graceful shutdown on SIGINT/SIGTERM.

## Done when
- `go test ./internal/httpapi/` passes via apptest.New + httptest: PUT then exec `echo hi` (output "hi\n", exit_code 0), PUT twice is fine, bad name 400, exec on an unknown box 404, list, delete 204 then get 404, healthz.
- `go build -o /dev/null ./cmd/sandbox-server` succeeds.

## Out of scope
README, live test.
~~~~

### 4.2 Write the sandbox README and ARCHITECTURE

~~~~text
Epic: 4 Serve
Depends on: none

## Environment
- Work in `samples/sandbox/`. Other workers are adding internal/httpapi, cmd/sandbox-server and internal/live right now. Change only the two files below.

## Goal
Tell a reader how to run and call the sandbox server, how to make Docker reachable from WSL, and state its dependency rules.

## Read first
- samples/llmrouter/README.md: tone and layout.
- samples/sandbox/internal/config/config.go: variables and defaults.

## Change
- samples/sandbox/README.md:
  - What it is: a box is a container with a host directory mounted at the same path.
  - Docker on WSL: Docker Desktop → Settings → Resources → WSL integration → enable this distro → Apply & restart. Then `ls -l /var/run/docker.sock` and `curl --unix-socket /var/run/docker.sock http://docker/_ping` should print OK. If permission is denied, add yourself to the `docker` group (`sudo usermod -aG docker $USER`, then a new shell).
  - Run: `make run` on 127.0.0.1:8087.
  - Configuration: SANDBOX_ENV, SANDBOX_ADDR, SANDBOX_DRIVER, DOCKER_HOST, SANDBOX_ROOT, SANDBOX_IMAGE and their defaults. Workspaces must be under the root. The default image is debian:bookworm-slim; for Go projects use for example `SANDBOX_IMAGE=golang:1.27-bookworm`.
  - HTTP API: the table of routes and a curl example (PUT a box, exec `uname -a`).
  - The `local` driver isolates nothing.
  - `make live`.
- samples/sandbox/ARCHITECTURE.md: a Rules section: 1. config chooses, `app.New` wires; 2. pure DI, no globals or init registration; 3. entrypoints are thin; 4. business rules (names, workspace confinement, timeouts, output limits) live in core.Service, transports only decode and map errors; 5. ports are declared in core (Driver); tests go through apptest.New. Then a Layout list of packages, one line each.

## Done when
- Both files exist and every path, variable and route they mention exists in the code or in this issue.

## Out of scope
All Go code.
~~~~

### 4.3 Add an opt-in live Docker test

~~~~text
Epic: 4 Serve
Depends on: none

## Environment
- Work in `samples/sandbox/`; run commands from there. If `go` is not found: `export PATH=$HOME/.local/go/bin:$PATH`.
- Do not edit go.mod; never run `go mod tidy`.
- Other workers are adding internal/httpapi, cmd/sandbox-server, README.md and ARCHITECTURE.md right now. Change only the file below.
- Docker may not be reachable here. Without SANDBOX_LIVE=1 the test must skip; with it and no Docker it must fail with a clear message, not hang.

## Goal
Prove the real Docker driver works end to end when someone asks for it.

## Read first
- samples/sandbox/internal/app/app.go, samples/sandbox/internal/config/config.go, samples/sandbox/internal/core/core.go.

## Change
- samples/sandbox/internal/live/live_test.go.

## Contract
Skip unless `SANDBOX_LIVE=1`. Use `config.FromEnvironment()` with Driver forced to "docker" and Root = t.TempDir(), then `app.New`. Box name `live-<unix seconds>`; `t.Cleanup` removes it. Steps:
1. Ensure gives State "running".
2. Exec `echo hello > from-box.txt && id -u` gives exit 0, and `<root>/<name>/from-box.txt` reads "hello\n" on the host.
3. Exec `sleep 5` with timeout 1 gives TimedOut.
4. Exec `cat /etc/os-release` gives exit 0; log the output.
Use a 5-minute context for the whole test (the first run may pull the image).

## Done when
- `go vet ./internal/live/` succeeds and `go test ./internal/live/` passes (it skips).
- If Docker is reachable (`curl --unix-socket /var/run/docker.sock http://docker/_ping` prints OK), `make live` passes; if it is not, say so in a comment on this issue rather than marking it blocked.

## Out of scope
Every other package.
~~~~
