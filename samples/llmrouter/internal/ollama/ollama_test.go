package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"llmrouter/internal/llm"
)

var _ llm.Backend = (*Client)(nil)
var _ llm.ModelManager = (*Client)(nil)

// fake serves one route, checking method and path, and counts requests.
type fake struct {
	t      *testing.T
	method string
	path   string
	status int
	resp   string
	check  func(t *testing.T, body []byte)
	hits   atomic.Int32
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.hits.Add(1)
	if r.Method != f.method {
		f.t.Errorf("method = %s, want %s", r.Method, f.method)
	}
	if r.URL.Path != f.path {
		f.t.Errorf("path = %s, want %s", r.URL.Path, f.path)
	}
	body, _ := io.ReadAll(r.Body)
	if f.check != nil {
		f.check(f.t, body)
	}
	status := f.status
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, f.resp)
}

func newFake(t *testing.T, f *fake) (*Client, *fake) {
	t.Helper()
	f.t = t
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New("ollama", srv.URL+"/", srv.Client()), f
}

func jsonEq(t *testing.T, got []byte, want string) {
	t.Helper()
	var g, w any
	if err := json.Unmarshal(got, &g); err != nil {
		t.Errorf("request body is not JSON: %v: %s", err, got)
		return
	}
	if err := json.Unmarshal([]byte(want), &w); err != nil {
		t.Fatalf("bad want JSON: %v", err)
	}
	if !reflect.DeepEqual(g, w) {
		t.Errorf("request body = %s, want %s", got, want)
	}
}

func TestNewTrimsTrailingSlashAndKind(t *testing.T) {
	c := New("gpu", "http://127.0.0.1:11434/", nil)
	if c.Name() != "gpu" {
		t.Errorf("Name = %q, want gpu", c.Name())
	}
	if c.Kind() != llm.KindOllama {
		t.Errorf("Kind = %q, want %q", c.Kind(), llm.KindOllama)
	}
	if c.baseURL != "http://127.0.0.1:11434" {
		t.Errorf("baseURL = %q, want no trailing slash", c.baseURL)
	}
	if c.hc == nil {
		t.Error("hc is nil for nil client")
	}
}

func TestHealth(t *testing.T) {
	c, _ := newFake(t, &fake{method: "GET", path: "/api/version", resp: `{"version":"0.32.1"}`})
	v, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if v != "0.32.1" {
		t.Errorf("version = %q, want 0.32.1", v)
	}
}

func TestListModels(t *testing.T) {
	resp := `{"models":[{"name":"llama3:8b","size":4661224676,"modified_at":"2026-08-23T16:59:52.16993965-04:00","digest":"abc","details":{"family":"llama"}}]}`
	c, _ := newFake(t, &fake{method: "GET", path: "/api/tags", resp: resp})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("got %d models, want 1", len(models))
	}
	m := models[0]
	if m.Name != "llama3:8b" || m.Size != 4661224676 {
		t.Errorf("model = %+v", m)
	}
	if !m.ModifiedAt.Equal(parseTime(t, "2026-08-23T16:59:52.16993965-04:00")) {
		t.Errorf("ModifiedAt = %v", m.ModifiedAt)
	}
}

func TestListModelsEmptyIsNotNil(t *testing.T) {
	c, _ := newFake(t, &fake{method: "GET", path: "/api/tags", resp: `{"models":[]}`})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if models == nil {
		t.Error("ListModels returned nil on success")
	}
}

func TestListModelsServerError(t *testing.T) {
	c, _ := newFake(t, &fake{method: "GET", path: "/api/tags", status: 500, resp: `{"error":"disk full"}`})
	_, err := c.ListModels(context.Background())
	if err == nil {
		t.Fatal("ListModels succeeded on 500")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error %q does not contain server message", err)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error %q does not contain status", err)
	}
}

func TestLoadedModels(t *testing.T) {
	resp := `{"models":[{"name":"llama3:8b","size_vram":5000000000,"expires_at":"2026-10-14T23:00:00Z"}]}`
	c, _ := newFake(t, &fake{method: "GET", path: "/api/ps", resp: resp})
	loaded, err := c.LoadedModels(context.Background())
	if err != nil {
		t.Fatalf("LoadedModels: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Name != "llama3:8b" || loaded[0].SizeVRAM != 5000000000 {
		t.Fatalf("loaded = %+v", loaded)
	}
	if !loaded[0].ExpiresAt.Equal(parseTime(t, "2026-10-14T23:00:00Z")) {
		t.Errorf("ExpiresAt = %v", loaded[0].ExpiresAt)
	}
}

func TestLoadedModelsEmptyIsNotNil(t *testing.T) {
	c, _ := newFake(t, &fake{method: "GET", path: "/api/ps", resp: `{"models":[]}`})
	loaded, err := c.LoadedModels(context.Background())
	if err != nil {
		t.Fatalf("LoadedModels: %v", err)
	}
	if loaded == nil {
		t.Error("LoadedModels returned nil on success")
	}
}

func TestPullModel(t *testing.T) {
	c, f := newFake(t, &fake{
		method: "POST", path: "/api/pull", resp: `{"status":"success"}`,
		check: func(t *testing.T, body []byte) { jsonEq(t, body, `{"model":"llama3:8b","stream":false}`) },
	})
	if err := c.PullModel(context.Background(), "llama3:8b"); err != nil {
		t.Fatalf("PullModel: %v", err)
	}
	if f.hits.Load() != 1 {
		t.Errorf("hits = %d, want 1", f.hits.Load())
	}
}

func TestPullModelEmptyNameSendsNothing(t *testing.T) {
	c, f := newFake(t, &fake{method: "POST", path: "/api/pull"})
	err := c.PullModel(context.Background(), "")
	if !errors.Is(err, llm.ErrInvalid) {
		t.Fatalf("PullModel(\"\") = %v, want ErrInvalid", err)
	}
	if f.hits.Load() != 0 {
		t.Errorf("empty name sent %d requests, want 0", f.hits.Load())
	}
}

func TestDeleteModel(t *testing.T) {
	c, _ := newFake(t, &fake{
		method: "DELETE", path: "/api/delete", resp: "",
		check: func(t *testing.T, body []byte) { jsonEq(t, body, `{"model":"llama3:8b"}`) },
	})
	if err := c.DeleteModel(context.Background(), "llama3:8b"); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
}

func TestDeleteModelNotFound(t *testing.T) {
	c, _ := newFake(t, &fake{method: "DELETE", path: "/api/delete", status: 404, resp: `{"error":"model not found"}`})
	err := c.DeleteModel(context.Background(), "nope")
	if !errors.Is(err, llm.ErrNotFound) {
		t.Fatalf("DeleteModel 404 = %v, want ErrNotFound", err)
	}
}

func TestClassify(t *testing.T) {
	labels := []string{"code", "chat"}
	want := `{
		"model": "router:1b", "stream": false, "keep_alive": "30m",
		"options": {"temperature": 0},
		"format": {"type": "object",
		           "properties": {"route": {"type": "string", "enum": ["code", "chat"]}},
		           "required": ["route"]},
		"messages": [{"role": "system", "content": "pick a route"},
		             {"role": "user", "content": "fix my loop"}]
	}`
	c, _ := newFake(t, &fake{
		method: "POST", path: "/api/chat",
		resp:  `{"message":{"role":"assistant","content":"{\"route\": \"code\"}"},"done":true}`,
		check: func(t *testing.T, body []byte) { jsonEq(t, body, want) },
	})
	got, err := c.Classify(context.Background(), "router:1b", "pick a route", "fix my loop", labels)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != "code" {
		t.Errorf("Classify = %q, want code", got)
	}
}

func TestClassifyNotJSON(t *testing.T) {
	c, _ := newFake(t, &fake{
		method: "POST", path: "/api/chat",
		resp: `{"message":{"role":"assistant","content":"not json"},"done":true}`,
	})
	_, err := c.Classify(context.Background(), "m", "s", "p", []string{"code"})
	if err == nil {
		t.Fatal("Classify accepted non-JSON content")
	}
	if !strings.Contains(err.Error(), "not json") {
		t.Errorf("error %q does not include the content", err)
	}
}

func TestChatCompletionsPassesBodyThrough(t *testing.T) {
	body := `{"model":"llama","messages":[{"role":"user","content":"hi"}]}`
	c, _ := newFake(t, &fake{
		method: "POST", path: "/v1/chat/completions", resp: `{"id":"x"}`,
		check: func(t *testing.T, got []byte) {
			if string(got) != body {
				t.Errorf("body = %s, want %s", got, body)
			}
		},
	})
	resp, err := c.ChatCompletions(context.Background(), []byte(body))
	if err != nil {
		t.Fatalf("ChatCompletions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("response Content-Type = %q", ct)
	}
}

func TestChatCompletionsReturnsNon200Unread(t *testing.T) {
	c, _ := newFake(t, &fake{method: "POST", path: "/v1/chat/completions", status: 404, resp: `{"error":"no such model"}`})
	resp, err := c.ChatCompletions(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("ChatCompletions returned error on 404: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
	data, _ := io.ReadAll(resp.Body)
	if string(data) != `{"error":"no such model"}` {
		t.Errorf("body = %s, want it unread", data)
	}
}

func TestHealthUnreachableWrapsErrUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New("ollama", url, nil)
	_, err := c.Health(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Health on closed server = %v, want ErrUnavailable", err)
	}
}

func parseTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return tm
}
