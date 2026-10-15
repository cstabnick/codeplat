// Package httpdecider implements core.Decider by asking the decide server,
// a JDM decision engine, over HTTP.
package httpdecider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"chess/internal/core"
)

// maxErrorBody is how many bytes of a non-200 response body are kept in the error.
const maxErrorBody = 512

// Client asks the decide server for decisions.
type Client struct {
	base string
	hc   *http.Client
}

var _ core.Decider = (*Client)(nil)

// New returns a Client for the decide server at baseURL, such as
// "http://127.0.0.1:8085". A trailing slash is ignored. A nil hc means a
// client with a 10 second timeout.
func New(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Decide evaluates the decision model named key with input and returns its
// result. A null result is returned as an empty map.
func (c *Client) Decide(ctx context.Context, key string, input map[string]any) (map[string]any, error) {
	body, err := json.Marshal(map[string]any{"context": input})
	if err != nil {
		return nil, fmt.Errorf("decide: encode request: %w", err)
	}

	endpoint := c.base + "/api/evaluate/" + url.PathEscape(key)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decide: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("decide unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		return nil, fmt.Errorf("decide returned %d: %s", resp.StatusCode, msg)
	}

	var out struct {
		Result map[string]any `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decide: decode response: %w", err)
	}
	if out.Result == nil {
		return map[string]any{}, nil
	}
	return out.Result, nil
}
