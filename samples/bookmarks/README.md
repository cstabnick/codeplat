# Bookmarks

A small sample of two cooperating Go servers. The API server keeps
bookmarks in memory and serves JSON. The UI server serves an HTMX page and
reaches the bookmarks only by calling the API server over HTTP.

Data is lost when the API server stops.

## Run

Use two terminals, both in this directory.

    go run ./cmd/api      # terminal 1: API server
    go run ./cmd/ui       # terminal 2: UI server

Then open http://127.0.0.1:8082.

Or use the Makefile: `make run-api` and `make run-ui`.

## Configuration

| Variable            | Default                 | Purpose                          |
|---------------------|-------------------------|----------------------------------|
| `BOOKMARKS_API_ADDR`| `127.0.0.1:8081`        | API listen address               |
| `BOOKMARKS_UI_ADDR` | `127.0.0.1:8082`        | UI listen address                |
| `BOOKMARKS_API_URL` | `http://127.0.0.1:8081` | Where the UI finds the API       |

## API

- `GET /api/bookmarks` lists bookmarks.
- `POST /api/bookmarks` adds one. Body: `{"title":"...","url":"..."}`.
- `DELETE /api/bookmarks/{id}` removes one.

## Layout

- `internal/bookmark`: shared types and the Service interface
- `internal/store`: in-memory store
- `internal/api`: JSON handler for the API
- `internal/client`: HTTP client for the API
- `internal/ui`: HTMX handler for the UI
- `cmd/api`, `cmd/ui`: the two servers

## Test

    go test ./...
