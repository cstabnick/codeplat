package sandboxclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSandbox records the PUT and exec requests and answers with canned
// responses. putStatus, when non-zero, is the status returned for the PUT.
type fakeSandbox struct {
	t         *testing.T
	putStatus int
	putPath   string
	putBody   map[string]any
	execPath  string
	execBody  map[string]any
}

func (f *fakeSandbox) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			f.t.Errorf("request body is not JSON: %v: %s", err, b)
		}
	}
	switch {
	case r.Method == http.MethodPut:
		f.putPath, f.putBody = r.URL.Path, m
		if f.putStatus != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.putStatus)
			io.WriteString(w, `{"error":"docker unavailable"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"name":"x","image":"img","workspace":"/x","container_id":"c","state":"running","created_at":"now"}`)
	case r.Method == http.MethodPost:
		f.execPath, f.execBody = r.URL.Path, m
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"output":"hello","exit_code":3,"timed_out":true}`)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func TestRunCreatesBoxThenExecs(t *testing.T) {
	f := &fakeSandbox{t: t}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	c := New(srv.URL+"/", nil)
	out, code, timedOut, err := c.Run(context.Background(), "/x/0a1b2c", "ls -la", 2500*time.Millisecond)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if out != "hello" || code != 3 || !timedOut {
		t.Fatalf("Run = %q, %d, %v; want \"hello\", 3, true", out, code, timedOut)
	}
	if f.putPath != "/api/boxes/0a1b2c" {
		t.Errorf("PUT path = %q", f.putPath)
	}
	if f.putBody["workspace"] != "/x/0a1b2c" || len(f.putBody) != 1 {
		t.Errorf("PUT body = %v", f.putBody)
	}
	if f.execPath != "/api/boxes/0a1b2c/exec" {
		t.Errorf("exec path = %q", f.execPath)
	}
	if f.execBody["command"] != "ls -la" || f.execBody["timeout_seconds"] != float64(2) {
		t.Errorf("exec body = %v", f.execBody)
	}
}

func TestRunTimeoutAtLeastOneSecond(t *testing.T) {
	f := &fakeSandbox{t: t}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	if _, _, _, err := New(srv.URL, nil).Run(context.Background(), "/x/a", "true", 10*time.Millisecond); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.execBody["timeout_seconds"] != float64(1) {
		t.Errorf("timeout_seconds = %v; want 1", f.execBody["timeout_seconds"])
	}
}

func TestRunPutStatusError(t *testing.T) {
	f := &fakeSandbox{t: t, putStatus: http.StatusServiceUnavailable}
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	defer srv.Close()

	_, _, _, err := New(srv.URL, nil).Run(context.Background(), "/x/a", "true", time.Second)
	if err == nil {
		t.Fatal("Run succeeded; want status error")
	}
	want := "sandbox returned 503 Service Unavailable: "
	if !strings.HasPrefix(err.Error(), want) || !strings.Contains(err.Error(), "docker unavailable") {
		t.Errorf("error = %q; want prefix %q and the body", err, want)
	}
	if f.execPath != "" {
		t.Errorf("exec was called after PUT failed: %s", f.execPath)
	}
}

func TestRunStatusBodyLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, strings.Repeat("e", 2000))
	}))
	defer srv.Close()

	_, _, _, err := New(srv.URL, nil).Run(context.Background(), "/x/a", "true", time.Second)
	if err == nil {
		t.Fatal("Run succeeded; want error")
	}
	if got := len(err.Error()) - len("sandbox returned 500 Internal Server Error: "); got != 512 {
		t.Errorf("body in error has %d bytes; want 512", got)
	}
}

func TestRunUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, _, _, err := New(url, nil).Run(context.Background(), "/x/a", "true", time.Second)
	if err == nil || !strings.HasPrefix(err.Error(), "sandbox unreachable: ") {
		t.Fatalf("error = %v; want sandbox unreachable", err)
	}
}

func TestRunContextEnded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, _, err := New(srv.URL, nil).Run(ctx, "/x/a", "true", time.Second)
	if err != context.Canceled {
		t.Fatalf("error = %v; want context.Canceled", err)
	}
}

func TestBoxName(t *testing.T) {
	cases := map[string]string{
		"/x/0a1b2c":      "0a1b2c",
		"/x/My Project!": "my-project-",
		"/":              "workspace",
		"":               "workspace",
		"/x/-_-abc":      "abc",
		"/x/ÄB":          "b",
	}
	for in, want := range cases {
		if got := BoxName(in); got != want {
			t.Errorf("BoxName(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestBoxNameCutTo63(t *testing.T) {
	got := BoxName("/x/" + strings.Repeat("a", 100))
	if len(got) != 63 {
		t.Errorf("len(BoxName) = %d; want 63", len(got))
	}
}
