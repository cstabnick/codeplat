package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"sandbox/internal/core"
)

// fakeEngine serves the routes on a unix socket and returns a Driver for it.
// Requests no route matches fail the test.
func fakeEngine(t *testing.T, routes func(mux *http.ServeMux)) *Driver {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "d.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	routes(mux)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
		http.NotFound(w, r)
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	d, err := New("unix://"+sock, nil)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"message": msg})
}

func decodeBody(t *testing.T, r *http.Request, v any) {
	t.Helper()
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		t.Errorf("decode request body of %s %s: %v", r.Method, r.URL.Path, err)
	}
}

// frame builds one multiplexed output frame.
func frame(stream byte, payload string) []byte {
	h := make([]byte, 8)
	h[0] = stream
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	return append(h, payload...)
}

// fakeDocker is the container state behind the Ensure tests.
type fakeDocker struct {
	mu        sync.Mutex
	exists    bool
	running   bool
	config    map[string]any // Image and Labels of the container
	created   map[string]any // decoded body of the last create
	imageGone bool           // create answers 404 until an image pull has run
	pullErr   string         // if set, the pull stream reports this error
	pulled    []string       // "fromImage:tag" of each pull
	starts    int
}

func ensureFake(t *testing.T, f *fakeDocker) *Driver {
	return fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /containers/{name}/json", func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if !f.exists {
				writeErr(w, http.StatusNotFound, "No such container: "+r.PathValue("name"))
				return
			}
			state := "exited"
			if f.running {
				state = "running"
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"Id":      "abc123",
				"Name":    "/" + r.PathValue("name"),
				"Created": "2026-10-14T10:00:00.123456789Z",
				"State":   map[string]any{"Status": state},
				"Config":  map[string]any{"Image": f.config["Image"], "Labels": f.config["Labels"]},
			})
		})
		mux.HandleFunc("POST /containers/create", func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if got := r.URL.Query().Get("name"); got != "sandbox-web" {
				t.Errorf("create name = %q, want sandbox-web", got)
			}
			if f.imageGone && len(f.pulled) == 0 {
				writeErr(w, http.StatusNotFound, "No such image: alpine:3.20")
				return
			}
			var body map[string]any
			decodeBody(t, r, &body)
			f.created = body
			f.config = body
			f.exists = true
			f.running = false
			writeJSON(w, http.StatusCreated, map[string]any{"Id": "abc123", "Warnings": []string{}})
		})
		mux.HandleFunc("POST /images/create", func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			q := r.URL.Query()
			f.pulled = append(f.pulled, q.Get("fromImage")+":"+q.Get("tag"))
			w.Header().Set("Content-Type", "application/json")
			if f.pullErr != "" {
				fmt.Fprintf(w, `{"status":"Pulling"}`+"\n"+`{"error":%q}`+"\n", f.pullErr)
				return
			}
			fmt.Fprint(w, `{"status":"Pulling from library/alpine"}`+"\n"+`{"status":"Downloaded"}`+"\n")
		})
		mux.HandleFunc("POST /containers/{id}/start", func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			defer f.mu.Unlock()
			if r.PathValue("id") != "abc123" || !f.exists {
				writeErr(w, http.StatusNotFound, "No such container")
				return
			}
			f.running = true
			f.starts++
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

func webSpec() core.BoxSpec {
	return core.BoxSpec{
		Name:      "web",
		Image:     "alpine:3.20",
		Workspace: "/ws",
		MemoryMB:  512,
		CPUs:      1.5,
		Network:   "none",
		User:      "1000:1000",
	}
}

func TestEnsureCreatesMissingBox(t *testing.T) {
	f := &fakeDocker{}
	d := ensureFake(t, f)

	box, err := d.Ensure(context.Background(), webSpec())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	c := f.created
	if c == nil {
		t.Fatal("no create request was sent")
	}
	checks := map[string]any{
		"Image":      "alpine:3.20",
		"WorkingDir": "/ws",
		"User":       "1000:1000",
	}
	for k, want := range checks {
		if c[k] != want {
			t.Errorf("create body %s = %v, want %v", k, c[k], want)
		}
	}
	if want := []any{"sleep", "infinity"}; !reflect.DeepEqual(c["Cmd"], want) {
		t.Errorf("Cmd = %v, want %v", c["Cmd"], want)
	}
	labels, _ := c["Labels"].(map[string]any)
	wantLabels := map[string]any{
		"codeplat.sandbox":           "1",
		"codeplat.sandbox.name":      "web",
		"codeplat.sandbox.workspace": "/ws",
	}
	if !reflect.DeepEqual(labels, wantLabels) {
		t.Errorf("Labels = %v, want %v", labels, wantLabels)
	}
	hc, _ := c["HostConfig"].(map[string]any)
	if got := hc["Binds"]; !reflect.DeepEqual(got, []any{"/ws:/ws"}) {
		t.Errorf("Binds = %v, want [/ws:/ws]", got)
	}
	if got := hc["Memory"]; got != float64(512<<20) {
		t.Errorf("Memory = %v, want %d", got, 512<<20)
	}
	if got := hc["NanoCpus"]; got != float64(1500000000) {
		t.Errorf("NanoCpus = %v, want 1500000000", got)
	}
	if got := hc["NetworkMode"]; got != "none" {
		t.Errorf("NetworkMode = %v, want none", got)
	}

	if f.starts != 1 {
		t.Errorf("starts = %d, want 1", f.starts)
	}
	if box.Name != "web" || box.State != "running" || box.Workspace != "/ws" ||
		box.Image != "alpine:3.20" || box.ContainerID != "abc123" {
		t.Errorf("box = %+v", box)
	}
	wantTime := time.Date(2026, 10, 14, 10, 0, 0, 123456789, time.UTC)
	if !box.CreatedAt.Equal(wantTime) {
		t.Errorf("CreatedAt = %v, want %v", box.CreatedAt, wantTime)
	}
}

func TestEnsurePullsMissingImage(t *testing.T) {
	f := &fakeDocker{imageGone: true}
	d := ensureFake(t, f)

	box, err := d.Ensure(context.Background(), webSpec())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if want := []string{"alpine:3.20"}; !reflect.DeepEqual(f.pulled, want) {
		t.Errorf("pulls = %v, want %v", f.pulled, want)
	}
	if f.created == nil || box.State != "running" {
		t.Errorf("box after pull = %+v, created = %v", box, f.created != nil)
	}
}

func TestEnsureFailsWhenPullReportsError(t *testing.T) {
	f := &fakeDocker{imageGone: true, pullErr: "pull access denied"}
	d := ensureFake(t, f)

	_, err := d.Ensure(context.Background(), webSpec())
	if err == nil || !strings.Contains(err.Error(), "pull access denied") {
		t.Fatalf("Ensure err = %v, want the pull error", err)
	}
	if f.created != nil {
		t.Error("container was created after a failed pull")
	}
}

func TestEnsureStartsExistingStoppedBox(t *testing.T) {
	f := &fakeDocker{
		exists: true,
		config: map[string]any{
			"Image":  "alpine:3.20",
			"Labels": map[string]any{"codeplat.sandbox.workspace": "/ws"},
		},
	}
	d := ensureFake(t, f)

	box, err := d.Ensure(context.Background(), webSpec())
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if f.created != nil {
		t.Error("existing box was created again")
	}
	if f.starts != 1 {
		t.Errorf("starts = %d, want 1", f.starts)
	}
	if box.State != "running" || box.Workspace != "/ws" {
		t.Errorf("box = %+v", box)
	}
}

func TestGetNotFound(t *testing.T) {
	d := ensureFake(t, &fakeDocker{})
	_, err := d.Get(context.Background(), "web")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get err = %v, want ErrNotFound", err)
	}
}

func TestGetOtherErrorCarriesStatusAndMessage(t *testing.T) {
	d := fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /containers/{name}/json", func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusInternalServerError, "boom")
		})
	})
	_, err := d.Get(context.Background(), "web")
	if err == nil || errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get err = %v, want a non-NotFound error", err)
	}
	if !strings.Contains(err.Error(), "500") || !strings.Contains(err.Error(), "boom") {
		t.Errorf("Get err = %q, want status 500 and message boom", err)
	}
}

func TestList(t *testing.T) {
	d := fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("GET /containers/json", func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			if q.Get("all") != "1" {
				t.Errorf("all = %q, want 1", q.Get("all"))
			}
			var filters map[string][]string
			if err := json.Unmarshal([]byte(q.Get("filters")), &filters); err != nil {
				t.Errorf("filters %q: %v", q.Get("filters"), err)
			}
			if want := []string{"codeplat.sandbox=1"}; !reflect.DeepEqual(filters["label"], want) {
				t.Errorf("label filter = %v, want %v", filters["label"], want)
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"Id": "abc123", "Names": []string{"/sandbox-web"}, "Image": "alpine:3.20",
					"State": "running", "Created": 1781431200,
					"Labels": map[string]string{"codeplat.sandbox": "1", "codeplat.sandbox.workspace": "/ws"},
				},
				{
					"Id": "def456", "Names": []string{"/sandbox-api"}, "Image": "busybox",
					"State": "exited", "Created": 1781431300,
					"Labels": map[string]string{"codeplat.sandbox": "1", "codeplat.sandbox.workspace": "/srv"},
				},
			})
		})
	})

	boxes, err := d.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(boxes) != 2 {
		t.Fatalf("len = %d, want 2", len(boxes))
	}
	web, api := boxes[0], boxes[1]
	if web.Name != "web" || web.State != "running" || web.Workspace != "/ws" ||
		web.Image != "alpine:3.20" || web.ContainerID != "abc123" {
		t.Errorf("web = %+v", web)
	}
	if want := time.Unix(1781431200, 0).UTC(); !web.CreatedAt.Equal(want) {
		t.Errorf("web.CreatedAt = %v, want %v", web.CreatedAt, want)
	}
	if api.Name != "api" || api.State != "exited" || api.Workspace != "/srv" {
		t.Errorf("api = %+v", api)
	}
}

func TestRemove(t *testing.T) {
	d := fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("DELETE /containers/{name}", func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("force"); got != "1" {
				t.Errorf("force = %q, want 1", got)
			}
			if r.PathValue("name") == "sandbox-gone" {
				writeErr(w, http.StatusNotFound, "No such container")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
	ctx := context.Background()
	if err := d.Remove(ctx, "web"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := d.Remove(ctx, "gone"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Remove missing err = %v, want ErrNotFound", err)
	}
}

func TestExecDemultiplexesFramesAndReturnsExitCode(t *testing.T) {
	d := fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /containers/{name}/exec", func(w http.ResponseWriter, r *http.Request) {
			if r.PathValue("name") != "sandbox-web" {
				t.Errorf("exec container = %q", r.PathValue("name"))
			}
			var body map[string]any
			decodeBody(t, r, &body)
			if body["AttachStdout"] != true || body["AttachStderr"] != true {
				t.Errorf("attach flags = %v", body)
			}
			if want := []any{"sh", "-c", "echo hi"}; !reflect.DeepEqual(body["Cmd"], want) {
				t.Errorf("Cmd = %v, want %v", body["Cmd"], want)
			}
			if body["WorkingDir"] != "/ws" {
				t.Errorf("WorkingDir = %v, want /ws", body["WorkingDir"])
			}
			writeJSON(w, http.StatusCreated, map[string]string{"Id": "e1"})
		})
		mux.HandleFunc("POST /exec/{id}/start", func(w http.ResponseWriter, r *http.Request) {
			if r.PathValue("id") != "e1" {
				t.Errorf("exec id = %q", r.PathValue("id"))
			}
			var body map[string]any
			decodeBody(t, r, &body)
			if body["Detach"] != false || body["Tty"] != false {
				t.Errorf("start body = %v", body)
			}
			w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
			_, _ = w.Write(frame(1, "hello "))
			_, _ = w.Write(frame(2, "world\n"))
		})
		mux.HandleFunc("GET /exec/{id}/json", func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, http.StatusOK, map[string]any{"ID": "e1", "ExitCode": 3})
		})
	})

	var out bytes.Buffer
	code, err := d.Exec(context.Background(), "web", []string{"sh", "-c", "echo hi"}, "/ws", &out)
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
	if got := out.String(); got != "hello world\n" {
		t.Errorf("output = %q, want %q", got, "hello world\n")
	}
}

func TestExecMissingBox(t *testing.T) {
	d := fakeEngine(t, func(mux *http.ServeMux) {
		mux.HandleFunc("POST /containers/{name}/exec", func(w http.ResponseWriter, r *http.Request) {
			writeErr(w, http.StatusNotFound, "No such container")
		})
	})
	code, err := d.Exec(context.Background(), "web", []string{"true"}, "/", io.Discard)
	if !errors.Is(err, core.ErrNotFound) || code != -1 {
		t.Fatalf("Exec = (%d, %v), want (-1, ErrNotFound)", code, err)
	}
}

func TestUnreachableSocketIsUnavailable(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "missing.sock")
	d, err := New("unix://"+sock, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.Get(context.Background(), "web")
	if !errors.Is(err, core.ErrUnavailable) {
		t.Fatalf("Get err = %v, want ErrUnavailable", err)
	}
}

func TestNewOverTCP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "No such container")
	}))
	t.Cleanup(srv.Close)
	d, err := New("tcp://"+strings.TrimPrefix(srv.URL, "http://"), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := d.Get(context.Background(), "web"); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Get err = %v, want ErrNotFound", err)
	}
}

func TestNewRejectsBadHosts(t *testing.T) {
	for _, host := range []string{"ftp://x", "", "unix://", "tcp://"} {
		if _, err := New(host, nil); err == nil {
			t.Errorf("New(%q) succeeded, want error", host)
		}
	}
}

func TestSplitImage(t *testing.T) {
	cases := []struct{ in, repo, tag string }{
		{"alpine", "alpine", "latest"},
		{"alpine:3.20", "alpine", "3.20"},
		{"registry.local:5000/team/app", "registry.local:5000/team/app", "latest"},
		{"registry.local:5000/team/app:v2", "registry.local:5000/team/app", "v2"},
	}
	for _, c := range cases {
		repo, tag := splitImage(c.in)
		if repo != c.repo || tag != c.tag {
			t.Errorf("splitImage(%q) = (%q, %q), want (%q, %q)", c.in, repo, tag, c.repo, c.tag)
		}
	}
}

func TestCopyFramesRejectsTruncatedPayload(t *testing.T) {
	var out bytes.Buffer
	err := copyFrames(bytes.NewReader(frame(1, "hello")[:10]), &out)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("copyFrames err = %v, want ErrUnexpectedEOF", err)
	}
}
