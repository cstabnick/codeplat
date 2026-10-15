// Package sandboxclient talks to a sandbox server over HTTP. It runs commands
// in a workspace's box, creating the box first when needed.
package sandboxclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Client talks to a sandbox server.
type Client struct {
	base string
	hc   *http.Client
}

// New returns a client for the sandbox server at baseURL, such as
// "http://127.0.0.1:8087". A trailing slash is ignored. hc nil means a new
// http.Client with no timeout; callers bound calls with ctx.
func New(baseURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{}
	}
	return &Client{base: strings.TrimRight(baseURL, "/"), hc: hc}
}

// Run executes command in the box for workspace, creating the box first. It
// returns the trimmed output, the exit code and whether the command timed out.
// It satisfies shell.Runner.
func (c *Client) Run(ctx context.Context, workspace, command string, timeout time.Duration) (output string, exitCode int, timedOut bool, err error) {
	name := BoxName(workspace)
	if err := c.putBox(ctx, name, workspace); err != nil {
		return "", 0, false, err
	}
	secs := int(timeout / time.Second)
	if secs < 1 {
		secs = 1
	}
	var out struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
		TimedOut bool   `json:"timed_out"`
	}
	req := map[string]any{"command": command, "timeout_seconds": secs}
	if err := c.post(ctx, "/api/boxes/"+name+"/exec", req, &out); err != nil {
		return "", 0, false, err
	}
	return out.Output, out.ExitCode, out.TimedOut, nil
}

// putBox creates or starts the named box with workspace mounted.
func (c *Client) putBox(ctx context.Context, name, workspace string) error {
	return c.send(ctx, http.MethodPut, "/api/boxes/"+name, map[string]any{"workspace": workspace}, nil)
}

// post sends a JSON body and decodes the JSON response into out.
func (c *Client) post(ctx context.Context, path string, in, out any) error {
	return c.send(ctx, http.MethodPost, path, in, out)
}

// send encodes in as JSON (when non-nil), sends it, and decodes a 2xx
// response into out (when non-nil).
func (c *Client) send(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.do(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return statusError(resp)
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode sandbox response: %w", err)
	}
	return nil
}

// do sends req. A failure reports the context's error when the context
// ended, and otherwise says the sandbox is unreachable.
func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	resp, err := c.hc.Do(req)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, fmt.Errorf("sandbox unreachable: %w", err)
	}
	return resp, nil
}

// statusError reads the start of a non-2xx body into an error.
func statusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("sandbox returned %s: %s", resp.Status, body)
}

// BoxName derives the box name from the workspace: the lower-case base name
// with every character outside [a-z0-9-] replaced by "-", leading "-"
// trimmed, and cut to 63 characters. An empty result becomes "workspace".
func BoxName(workspace string) string {
	base := strings.ToLower(filepath.Base(workspace))
	var b strings.Builder
	for _, r := range base {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := strings.TrimLeft(b.String(), "-")
	if len(name) > 63 {
		name = name[:63]
	}
	if name == "" {
		return "workspace"
	}
	return name
}
