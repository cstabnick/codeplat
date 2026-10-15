package docker

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sandbox/internal/core"
)

// baseURL is the placeholder host for every request. The transport decides
// where the bytes really go, so the Engine never sees this name.
const baseURL = "http://docker"

// engine is a minimal client for the Docker Engine HTTP API.
type engine struct {
	hc *http.Client
}

// apiError is a non-2xx, non-3xx answer from the Engine.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string {
	return fmt.Sprintf("docker: HTTP %d: %s", e.Status, e.Message)
}

// isStatus reports whether err is an Engine answer with the given status.
func isStatus(err error, status int) bool {
	var ae *apiError
	return errors.As(err, &ae) && ae.Status == status
}

// call sends one request. A body, if non-nil, is encoded as JSON. Statuses
// of 400 and above become *apiError; the caller must close the returned body.
// Connection failures wrap core.ErrUnavailable.
func (e *engine) call(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("docker: encode %s %s: %w", method, path, err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, rd)
	if err != nil {
		return nil, fmt.Errorf("docker: build %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := e.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("docker engine unreachable: %w: %w", core.ErrUnavailable, err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		return nil, readAPIError(resp)
	}
	return resp, nil
}

// callJSON sends one request and decodes the JSON answer into out, which may
// be nil when the answer is not needed.
func (e *engine) callJSON(ctx context.Context, method, path string, body, out any) error {
	resp, err := e.call(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("docker: decode %s %s: %w", method, path, err)
	}
	return nil
}

// send sends one request and discards the answer.
func (e *engine) send(ctx context.Context, method, path string, body any) error {
	resp, err := e.call(ctx, method, path, body)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// readAPIError builds an *apiError from an error answer, preferring Docker's
// {"message"} field and falling back to the raw body.
func readAPIError(resp *http.Response) error {
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &payload)
	msg := payload.Message
	if msg == "" {
		msg = strings.TrimSpace(string(raw))
	}
	return &apiError{Status: resp.StatusCode, Message: msg}
}

// frameHeaderSize is the length of the header in front of each output frame.
const frameHeaderSize = 8

// copyFrames reads Engine multiplexed output: each frame is an 8-byte header
// (stream byte, three padding bytes, big-endian payload length) and a payload.
// Both streams go to out in arrival order.
func copyFrames(r io.Reader, out io.Writer) error {
	var hdr [frameHeaderSize]byte
	for {
		_, err := io.ReadFull(r, hdr[:])
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		n := int64(binary.BigEndian.Uint32(hdr[4:8]))
		if _, err := io.CopyN(out, r, n); err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
	}
}
