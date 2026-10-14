// Package ollama is an HTTP client for one Ollama server. It implements
// llm.Backend and llm.ModelManager.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"llmrouter/internal/llm"
)

// Client talks to one Ollama server.
type Client struct {
	name    string
	baseURL string
	hc      *http.Client
}

// New returns a client for the Ollama server at baseURL, such as
// "http://127.0.0.1:11434". A trailing slash is ignored. hc nil means
// a new http.Client with no timeout; callers bound calls with ctx.
func New(name, baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{name: name, baseURL: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Name returns the backend's name in the config.
func (c *Client) Name() string { return c.name }

// Kind returns llm.KindOllama.
func (c *Client) Kind() string { return llm.KindOllama }

// Health returns the server's version.
func (c *Client) Health(ctx context.Context) (string, error) {
	var out struct {
		Version string `json:"version"`
	}
	if err := c.json(ctx, http.MethodGet, "/api/version", nil, &out); err != nil {
		return "", err
	}
	return out.Version, nil
}

// ListModels returns the models the server has installed. It never returns
// nil on success.
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	var out struct {
		Models []struct {
			Name       string    `json:"name"`
			Size       int64     `json:"size"`
			ModifiedAt time.Time `json:"modified_at"`
		} `json:"models"`
	}
	if err := c.json(ctx, http.MethodGet, "/api/tags", nil, &out); err != nil {
		return nil, err
	}
	models := make([]llm.Model, 0, len(out.Models))
	for _, m := range out.Models {
		models = append(models, llm.Model{Name: m.Name, Size: m.Size, ModifiedAt: m.ModifiedAt})
	}
	return models, nil
}

// LoadedModels returns the models the server holds in memory. It never
// returns nil on success.
func (c *Client) LoadedModels(ctx context.Context) ([]llm.LoadedModel, error) {
	var out struct {
		Models []struct {
			Name      string    `json:"name"`
			SizeVRAM  int64     `json:"size_vram"`
			ExpiresAt time.Time `json:"expires_at"`
		} `json:"models"`
	}
	if err := c.json(ctx, http.MethodGet, "/api/ps", nil, &out); err != nil {
		return nil, err
	}
	loaded := make([]llm.LoadedModel, 0, len(out.Models))
	for _, m := range out.Models {
		loaded = append(loaded, llm.LoadedModel{Name: m.Name, SizeVRAM: m.SizeVRAM, ExpiresAt: m.ExpiresAt})
	}
	return loaded, nil
}

// PullModel downloads a model and returns once it is installed. An empty
// name is an error wrapping llm.ErrInvalid.
func (c *Client) PullModel(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("%w: model name is empty", llm.ErrInvalid)
	}
	in := map[string]any{"model": name, "stream": false}
	return c.json(ctx, http.MethodPost, "/api/pull", in, nil)
}

// DeleteModel removes a model. An unknown model is an error wrapping
// llm.ErrNotFound.
func (c *Client) DeleteModel(ctx context.Context, name string) error {
	in := map[string]string{"model": name}
	resp, err := c.send(ctx, http.MethodDelete, "/api/delete", in)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("%w: %v", llm.ErrNotFound, statusError(http.MethodDelete, "/api/delete", resp))
	default:
		return statusError(http.MethodDelete, "/api/delete", resp)
	}
}

// Classify asks model to pick one of labels for prompt, with system as the
// system message, and returns the route it answered. The answer is not
// checked against labels; the caller does that.
func (c *Client) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	type format struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	in := struct {
		Model     string         `json:"model"`
		Stream    bool           `json:"stream"`
		KeepAlive string         `json:"keep_alive"`
		Options   map[string]any `json:"options"`
		Format    format         `json:"format"`
		Messages  []llm.Message  `json:"messages"`
	}{
		Model:     model,
		Stream:    false,
		KeepAlive: "30m",
		Options:   map[string]any{"temperature": 0},
		Format: format{
			Type: "object",
			Properties: map[string]any{
				"route": map[string]any{"type": "string", "enum": labels},
			},
			Required: []string{"route"},
		},
		Messages: []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
	}
	var out struct {
		Message llm.Message `json:"message"`
	}
	if err := c.json(ctx, http.MethodPost, "/api/chat", in, &out); err != nil {
		return "", err
	}
	var answer struct {
		Route string `json:"route"`
	}
	if err := json.Unmarshal([]byte(out.Message.Content), &answer); err != nil {
		return "", fmt.Errorf("ollama classify with %s: answer %q is not {\"route\": string}: %w", model, out.Message.Content, err)
	}
	return answer.Route, nil
}

// ChatCompletions sends body to /v1/chat/completions and returns the
// response unread, whatever its status. The caller closes the response Body.
func (c *Client) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	if body == nil {
		body = []byte{}
	}
	return c.send(ctx, http.MethodPost, "/v1/chat/completions", body)
}

// json sends in as a JSON body (nil for none), requires status 200, and
// decodes the response into out (nil to discard it).
func (c *Client) json(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("ollama %s %s: encode request: %w", method, path, err)
		}
		body = b
	}
	resp, err := c.send(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return statusError(method, path, resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("ollama %s %s: decode response: %w", method, path, err)
	}
	return nil
}

// send makes one request. A non-nil body is sent as JSON.
func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case []byte:
		rdr = bytes.NewReader(b)
	default:
		enc, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("ollama %s %s: encode request: %w", method, path, err)
		}
		rdr = bytes.NewReader(enc)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("ollama %s %s: %w", method, path, err)
	}
	if rdr != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", llm.ErrUnavailable, err)
	}
	return resp, nil
}

// statusError reads resp's body, closes it, and describes the failure.
func statusError(method, path string, resp *http.Response) error {
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	msg := strings.TrimSpace(string(data))
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(data, &e) == nil && e.Error != "" {
		msg = e.Error
	}
	if msg == "" {
		return fmt.Errorf("ollama %s %s: %s", method, path, resp.Status)
	}
	return fmt.Errorf("ollama %s %s: %s: %s", method, path, resp.Status, msg)
}
