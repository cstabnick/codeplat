// Package oaiclient is an HTTP client for an OpenAI-compatible server, such
// as llama.cpp's llama-server. It implements llm.Backend. It cannot install
// models, so it does not implement llm.ModelManager.
package oaiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"llmrouter/internal/llm"
)

// Client talks to one OpenAI-compatible server.
type Client struct {
	name    string
	baseURL string
	hc      *http.Client
}

// New returns a client for the server at baseURL, such as
// "http://127.0.0.1:8085". A trailing slash is ignored. hc nil means a
// new http.Client with no timeout; callers bound calls with ctx.
func New(name, baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{name: name, baseURL: strings.TrimSuffix(baseURL, "/"), hc: hc}
}

// Name returns the backend's name in the config.
func (c *Client) Name() string { return c.name }

// Kind returns llm.KindOpenAI.
func (c *Client) Kind() string { return llm.KindOpenAI }

// Health checks GET /v1/models. It returns no version, since the server
// does not report one.
func (c *Client) Health(ctx context.Context) (string, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/models", nil, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// llama-server answers 503 while it is still loading a model.
		return "", fmt.Errorf("%w: GET /v1/models: %s", llm.ErrUnavailable, resp.Status)
	}
	return "", nil
}

type modelList struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListModels returns the models the server reports at GET /v1/models.
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	resp, err := c.do(ctx, http.MethodGet, "/v1/models", nil, "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := statusError(resp, "GET", "/v1/models"); err != nil {
		return nil, err
	}
	var list modelList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("openai GET /v1/models: decode: %w", err)
	}
	models := make([]llm.Model, 0, len(list.Data))
	for _, m := range list.Data {
		models = append(models, llm.Model{Name: m.ID})
	}
	return models, nil
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type routeAnswer struct {
	Route string `json:"route"`
}

// Classify asks model to pick one of labels for prompt. It constrains the
// answer with a JSON schema, so the server returns {"route": LABEL}.
func (c *Client) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	body, err := json.Marshal(map[string]any{
		"model":       model,
		"stream":      false,
		"temperature": 0,
		"max_tokens":  64,
		"response_format": map[string]any{
			"type": "json_schema",
			"json_schema": map[string]any{
				"name":   "route",
				"strict": true,
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"route": map[string]any{"type": "string", "enum": labels},
					},
					"required": []string{"route"},
				},
			},
		},
		"messages": []llm.Message{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return "", fmt.Errorf("openai POST /v1/chat/completions: encode: %w", err)
	}
	resp, err := c.do(ctx, http.MethodPost, "/v1/chat/completions", body, "application/json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := statusError(resp, "POST", "/v1/chat/completions"); err != nil {
		return "", err
	}
	var cr chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return "", fmt.Errorf("openai POST /v1/chat/completions: decode: %w", err)
	}
	if len(cr.Choices) == 0 {
		return "", errors.New("openai POST /v1/chat/completions: no choices in response")
	}
	content := cr.Choices[0].Message.Content
	var answer routeAnswer
	if err := json.Unmarshal([]byte(content), &answer); err != nil {
		return "", fmt.Errorf("openai POST /v1/chat/completions: content is not a route %q: %w", content, err)
	}
	if answer.Route == "" {
		return "", fmt.Errorf("openai POST /v1/chat/completions: content has no route: %q", content)
	}
	return answer.Route, nil
}

// ChatCompletions posts body to /v1/chat/completions and returns the
// response unread, whatever its status. The caller closes the body.
func (c *Client) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, "/v1/chat/completions", body, "application/json")
}

// do sends one request. A failure to reach the server wraps
// llm.ErrUnavailable. A nil body sends no request body.
func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rd)
	if err != nil {
		return nil, fmt.Errorf("openai %s %s: %w", method, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", llm.ErrUnavailable, err)
	}
	return resp, nil
}

// statusError returns nil for a 200 response. Otherwise it reads a prefix
// of the body into the error. The caller still closes the body.
func statusError(resp *http.Response, method, path string) error {
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	return fmt.Errorf("openai %s %s: %s: %s", method, path, resp.Status, buf)
}
