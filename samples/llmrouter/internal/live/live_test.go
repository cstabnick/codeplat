// Package live_test runs llmrouter against real model servers. Every test
// is skipped unless LLMROUTER_LIVE=1 is set; run them with `make live`.
package live_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"llmrouter/internal/app"
	"llmrouter/internal/config"
	"llmrouter/internal/llm"
)

// requireLive skips the test unless LLMROUTER_LIVE=1.
func requireLive(t *testing.T) {
	t.Helper()
	if os.Getenv("LLMROUTER_LIVE") != "1" {
		t.Skip("set LLMROUTER_LIVE=1 to run against real backends")
	}
}

// startApp builds and starts an app from cfg and serves it over HTTP.
func startApp(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()
	a, err := app.New(cfg, nil)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	<-a.Start(context.Background())
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return srv
}

// getJSON GETs url with a 30 second client timeout and decodes the body into v.
func getJSON(t *testing.T, url string, v any) {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", url, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("GET %s: decode: %v", url, err)
	}
}

// chat POSTs body to /v1/chat/completions on srv with the given client timeout.
func chat(t *testing.T, srv *httptest.Server, body map[string]any, timeout time.Duration) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Post(srv.URL+"/v1/chat/completions", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST /v1/chat/completions: %v", err)
	}
	return resp
}

// completionContent decodes the first choice's message content from a chat response.
func completionContent(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d: %s", resp.StatusCode, data)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode completion: %v: %s", err, data)
	}
	if len(out.Choices) == 0 {
		t.Fatalf("no choices: %s", data)
	}
	content := out.Choices[0].Message.Content
	if strings.TrimSpace(content) == "" {
		t.Fatalf("empty content: %s", data)
	}
	return content
}

func TestOllama(t *testing.T) {
	requireLive(t)
	base := os.Getenv("LLMROUTER_LIVE_OLLAMA")
	if base == "" {
		base = "http://127.0.0.1:11434"
	}
	model := os.Getenv("LLMROUTER_LIVE_MODEL")
	if model == "" {
		model = "llama3:8b"
	}
	cfg := config.Config{
		Addr: "127.0.0.1:0",
		Backends: []config.Backend{{
			Name: "ollama",
			Kind: llm.KindOllama,
			URL:  base,
		}},
		Intent: config.Intent{Backend: "ollama", Model: model, TimeoutSeconds: 120},
		Routes: []llm.Route{
			{Name: "code", Description: "Writing or debugging source code.", Backend: "ollama", Model: model},
			{Name: "general", Description: "Anything else.", Backend: "ollama", Model: model},
		},
		DefaultRoute: "general",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	srv := startApp(t, cfg)

	var models []llm.Model
	getJSON(t, srv.URL+"/api/backends/ollama/models", &models)
	found := false
	for _, m := range models {
		if m.Name == model {
			found = true
			break
		}
	}
	if !found {
		t.Skipf("Ollama at %s does not list model %q", base, model)
	}

	resp := chat(t, srv, map[string]any{
		"model":      "auto",
		"max_tokens": 32,
		"messages": []map[string]string{
			{"role": "user", "content": "Write a Go function that reverses a string."},
		},
	}, 3*time.Minute)
	route := resp.Header.Get("X-Llmrouter-Route")
	content := completionContent(t, resp)
	if route != "code" && route != "general" {
		t.Fatalf("X-Llmrouter-Route = %q, want code or general", route)
	}

	var decisions []llm.Decision
	getJSON(t, srv.URL+"/api/decisions?n=1", &decisions)
	if len(decisions) != 1 {
		t.Fatalf("decisions: got %d, want 1", len(decisions))
	}
	d := decisions[0]
	t.Logf("route=%s fallback=%v took=%s content=%q", route, d.Fallback, d.Took, content)
}

func TestOpenAI(t *testing.T) {
	requireLive(t)
	url := os.Getenv("LLMROUTER_LIVE_OPENAI_URL")
	if url == "" {
		t.Skip("set LLMROUTER_LIVE_OPENAI_URL to an OpenAI-compatible server to run this test")
	}
	model := os.Getenv("LLMROUTER_LIVE_OPENAI_MODEL")
	if model == "" {
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		getJSON(t, strings.TrimSuffix(url, "/")+"/v1/models", &list)
		if len(list.Data) == 0 {
			t.Fatalf("%s/v1/models lists no models", url)
		}
		model = list.Data[0].ID
	}
	cfg := config.Config{
		Addr: "127.0.0.1:0",
		Backends: []config.Backend{{
			Name: "remote",
			Kind: llm.KindOpenAI,
			URL:  url,
		}},
		Intent: config.Intent{Backend: "remote", Model: model},
		Routes: []llm.Route{
			{Name: "general", Description: "Anything else.", Backend: "remote", Model: model},
		},
		DefaultRoute: "general",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config: %v", err)
	}
	srv := startApp(t, cfg)

	resp := chat(t, srv, map[string]any{
		"model":      "remote/" + model,
		"max_tokens": 32,
		"messages": []map[string]string{
			{"role": "user", "content": "Say hello."},
		},
	}, 10*time.Minute)
	content := completionContent(t, resp)
	t.Logf("model=%s content=%q", model, content)
}
