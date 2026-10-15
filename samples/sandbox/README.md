# sandbox

sandbox is a small HTTP server that runs commands for you in isolated boxes.
A box is a container with a host directory mounted at the same path, so a
command run in the box works on the same files you see on the host. Each box
has a name. You create or reuse a box with a workspace directory, run bash
commands in it, and remove it when you are done.

## Docker on WSL

The default driver runs boxes as Docker containers, so Docker must be reachable
from this WSL distro.

1. In Docker Desktop, open Settings → Resources → WSL integration, enable this
   distro, and click Apply & restart.
2. Check the socket:

        ls -l /var/run/docker.sock
        curl --unix-socket /var/run/docker.sock http://docker/_ping

   The curl command should print `OK`.
3. If you get "permission denied" on the socket, add yourself to the `docker`
   group, then open a new shell:

        sudo usermod -aG docker $USER

## Run

Run it from this directory:

    make run
    # or
    go run ./cmd/sandbox-server

It listens on http://127.0.0.1:8087 with the default config. The server logs
its address, driver and root when it starts. Stop it with Ctrl+C.

## Configuration

The config comes from `internal/config/dev.json` (or the embedded config named
by `SANDBOX_ENV`, such as `test`), and these environment variables override it. An empty variable
changes nothing.

| Variable | Overrides | Default |
|----------|-----------|---------|
| `SANDBOX_ENV` | which embedded config to load | `dev` |
| `SANDBOX_ADDR` | listen address | `127.0.0.1:8087` |
| `SANDBOX_DRIVER` | `docker` or `local` | `docker` |
| `DOCKER_HOST` | Docker daemon address | `unix:///var/run/docker.sock` |
| `SANDBOX_ROOT` | workspaces root | `$XDG_DATA_HOME/sandbox/workspaces`, or `~/.local/share/sandbox/workspaces` |
| `SANDBOX_IMAGE` | default image for new boxes | `debian:bookworm-slim` |

Workspaces must be inside the root: a workspace that is the root itself, or
is outside it, is rejected. A box created without a workspace gets
`<root>/<name>`. The default image is `debian:bookworm-slim`. For Go projects,
use an image with the toolchain, for example:

    SANDBOX_IMAGE=golang:1.27-bookworm make run

Memory, CPU and network limits come from the config file: 2048 MB, 2 CPUs and
the `bridge` network in `dev.json`.

## HTTP API

Every request and response is JSON. Errors are `{"error": "..."}`. A
workspace or image left out of a request uses the default.

| Method and path | Body | Returns |
|-----------------|------|---------|
| `PUT /api/boxes/{name}` | `{"workspace": "/abs/path", "image": "..."}` (both optional) | The box, 200 |
| `GET /api/boxes` | | All boxes, 200 |
| `GET /api/boxes/{name}` | | The box, 200 |
| `DELETE /api/boxes/{name}` | | 204 |
| `POST /api/boxes/{name}/exec` | `{"command": "...", "timeout_seconds": 60}` | `{"output", "exit_code", "timed_out"}`, 200 |
| `GET /healthz` | | `{"status": "ok"}`, 200 |

Box names must match `^[a-z0-9][a-z0-9-]{0,62}$`. A command runs in bash in
the box's workspace. `timeout_seconds` defaults to 60 and is capped at 600.
Output longer than 30000 bytes keeps its first and last 15000 bytes.

Errors: 400 for invalid input, 404 for an unknown box, 503 when the driver
cannot be reached.

Create a box and run a command in it:

    curl -s -X PUT http://127.0.0.1:8087/api/boxes/demo \
      -H 'Content-Type: application/json' \
      -d '{"workspace": "'"$HOME"'/work/demo"}'

    curl -s -X POST http://127.0.0.1:8087/api/boxes/demo/exec \
      -H 'Content-Type: application/json' \
      -d '{"command": "uname -a"}'

## Local driver

`SANDBOX_DRIVER=local` runs commands directly on the host with `os/exec`. It is
meant for tests and for machines without Docker. It isolates nothing: a command
sees the host's files, network and user, and boxes are kept in memory and lost
when the process exits.

## Live test

`make live` runs `internal/live`, which starts the Docker driver against the
daemon from `dev.json` and is skipped unless `SANDBOX_LIVE=1` is set (the
target sets it). It needs the Docker setup above.

    make live

Unit tests run with `make test`.
