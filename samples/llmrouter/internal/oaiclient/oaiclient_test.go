package oaiclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"llmrouter/internal/llm"
)

var _ llm.Backend = (*Client)(nil)

// newServer starts an httptest server whose handler checks the method and
// path, and then calls h. It returns a client for the server.
func newServer(t *testing.T, method, path string, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			t.Errorf("method = %s, want %s", r.Method, method)
		}
		if r.URL.Path != path {
			t.Errorf("path = %s, want %s", r.URL.Path, path)
		}
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return New("test", srv.URL+"/", nil)
}

func TestNewTrimsTrailingSlashAndKind(t *testing.T) {
	c := New("bonsai", "http://127.0.0.1:8085/", nil)
	if c.Name() != "bonsai" {
		t.Errorf("Name() = %q, want bonsai", c.Name())
	}
	if c.Kind() != llm.KindOpenAI {
		t.Errorf("Kind() = %q, want %q", c.Kind(), llm.KindOpenAI)
	}
	if c.baseURL != "http://127.0.0.1:8085" {
		t.Errorf("baseURL = %q, want no trailing slash", c.baseURL)
	}
}

func TestHealthOK(t *testing.T) {
	c := newServer(t, http.MethodGet, "/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	v, err := c.Health(context.Background())
	if err != nil {
		t.Fatalf("Health: %v", err)
	}
	if v != "" {
		t.Errorf("version = %q, want empty", v)
	}
}

func TestHealthLoadingWrapsUnavailable(t *testing.T) {
	c := newServer(t, http.MethodGet, "/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	_, err := c.Health(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Health error = %v, want wrapping ErrUnavailable", err)
	}
}

func TestHealthClosedServerWrapsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New("test", url, nil)
	_, err := c.Health(context.Background())
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("Health error = %v, want wrapping ErrUnavailable", err)
	}
}

func TestListModelsMapsIDs(t *testing.T) {
	c := newServer(t, http.MethodGet, "/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"object":"list","data":[{"id":"Ternary-Bonsai-2-27B-PQ2_0.gguf","object":"model","created":1792017950,"owned_by":"llamacpp"},{"id":"other.gguf","object":"model","created":1,"owned_by":"llamacpp"}]}`)
	})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].Name != "Ternary-Bonsai-2-27B-PQ2_0.gguf" || models[1].Name != "other.gguf" {
		t.Errorf("names = %q, %q", models[0].Name, models[1].Name)
	}
}

func TestListModelsEmptyIsNotNil(t *testing.T) {
	c := newServer(t, http.MethodGet, "/v1/models", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"object":"list","data":[]}`)
	})
	models, err := c.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if models == nil {
		t.Error("models is nil, want non-nil empty slice")
	}
}

func TestClassifySendsRequestAndReturnsLabel(t *testing.T) {
	labels := []string{"code", "chat", "math"}
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if body["model"] != "bonsai" {
			t.Errorf("model = %v, want bonsai", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream = %v, want false", body["stream"])
		}
		if body["temperature"] != float64(0) {
			t.Errorf("temperature = %v, want 0", body["temperature"])
		}
		if body["max_tokens"] != float64(64) {
			t.Errorf("max_tokens = %v, want 64", body["max_tokens"])
		}
		rf, _ := body["response_format"].(map[string]any)
		if rf["type"] != "json_schema" {
			t.Errorf("response_format.type = %v, want json_schema", rf["type"])
		}
		js, _ := rf["json_schema"].(map[string]any)
		if js["name"] != "route" || js["strict"] != true {
			t.Errorf("json_schema name/strict = %v/%v", js["name"], js["strict"])
		}
		schema, _ := js["schema"].(map[string]any)
		props, _ := schema["properties"].(map[string]any)
		route, _ := props["route"].(map[string]any)
		enum, _ := route["enum"].([]any)
		if len(enum) != len(labels) {
			t.Errorf("enum = %v, want %v", enum, labels)
			return
		}
		for i, l := range labels {
			if enum[i] != l {
				t.Errorf("enum[%d] = %v, want %q", i, enum[i], l)
			}
		}
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 2 {
			t.Fatalf("messages = %v, want 2", msgs)
		}
		sys, _ := msgs[0].(map[string]any)
		usr, _ := msgs[1].(map[string]any)
		if sys["role"] != "system" || sys["content"] != "classify it" {
			t.Errorf("system message = %v", sys)
		}
		if usr["role"] != "user" || usr["content"] != "write a func" {
			t.Errorf("user message = %v", usr)
		}
		io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"route\":\"code\"}"},"finish_reason":"stop"}]}`)
	})
	got, err := c.Classify(context.Background(), "bonsai", "classify it", "write a func", labels)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != "code" {
		t.Errorf("Classify = %q, want code", got)
	}
}

func TestClassifyNoChoicesErrors(t *testing.T) {
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[]}`)
	})
	_, err := c.Classify(context.Background(), "m", "s", "p", []string{"a"})
	if err == nil {
		t.Fatal("Classify with no choices returned nil error")
	}
}

func TestClassifyNonJSONContentErrorsWithContent(t *testing.T) {
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"choices":[{"index":0,"message":{"role":"assistant","content":"just words"},"finish_reason":"stop"}]}`)
	})
	_, err := c.Classify(context.Background(), "m", "s", "p", []string{"a"})
	if err == nil {
		t.Fatal("Classify with non-JSON content returned nil error")
	}
	if !strings.Contains(err.Error(), "just words") {
		t.Errorf("error %q does not include the content", err)
	}
}

func TestClassifyBadRequestErrorsWithBody(t *testing.T) {
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "grammar is invalid")
	})
	_, err := c.Classify(context.Background(), "m", "s", "p", []string{"a"})
	if err == nil {
		t.Fatal("Classify with 400 returned nil error")
	}
	if !strings.Contains(err.Error(), "grammar is invalid") {
		t.Errorf("error %q does not include the body", err)
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error %q does not include the status", err)
	}
}

func TestChatCompletionsPassesBodyThrough(t *testing.T) {
	in := `{"model":"auto","messages":[{"role":"user","content":"hi"}]}`
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", ct)
		}
		got, _ := io.ReadAll(r.Body)
		if string(got) != in {
			t.Errorf("body = %s, want %s", got, in)
		}
		io.WriteString(w, `{"ok":true}`)
	})
	resp, err := c.ChatCompletions(context.Background(), []byte(in))
	if err != nil {
		t.Fatalf("ChatCompletions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestChatCompletionsReturnsNon200Unread(t *testing.T) {
	c := newServer(t, http.MethodPost, "/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		io.WriteString(w, "bad input")
	})
	resp, err := c.ChatCompletions(context.Background(), []byte(`{}`))
	if err != nil {
		t.Fatalf("ChatCompletions returned error for 400: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(got) != "bad input" {
		t.Errorf("body = %q, want it unread and intact", got)
	}
}

func TestChatCompletionsClosedServerWrapsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()
	c := New("test", url, nil)
	_, err := c.ChatCompletions(context.Background(), []byte(`{}`))
	if !errors.Is(err, llm.ErrUnavailable) {
		t.Fatalf("ChatCompletions error = %v, want wrapping ErrUnavailable", err)
	}
}
