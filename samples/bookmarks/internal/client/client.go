// Package client implements bookmark.Service by calling the bookmarks JSON API over HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"bookmarks/internal/bookmark"
)

// Client is a bookmark.Service backed by a remote JSON API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

var _ bookmark.Service = (*Client)(nil)

// New returns a Client for the API at baseURL. A nil httpClient means http.DefaultClient.
func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

type apiError struct {
	Error string `json:"error"`
}

// List returns all bookmarks from the API. It never returns nil.
func (c *Client) List(ctx context.Context) ([]bookmark.Bookmark, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/bookmarks", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, unexpectedStatus(resp)
	}
	var items []bookmark.Bookmark
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("bookmarks api: decode list: %w", err)
	}
	if items == nil {
		items = []bookmark.Bookmark{}
	}
	return items, nil
}

// Add creates a bookmark through the API.
func (c *Client) Add(ctx context.Context, title, url string) (bookmark.Bookmark, error) {
	body, err := json.Marshal(struct {
		Title string `json:"title"`
		URL   string `json:"url"`
	}{Title: title, URL: url})
	if err != nil {
		return bookmark.Bookmark{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/bookmarks", bytes.NewReader(body))
	if err != nil {
		return bookmark.Bookmark{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return bookmark.Bookmark{}, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		var b bookmark.Bookmark
		if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
			return bookmark.Bookmark{}, fmt.Errorf("bookmarks api: decode bookmark: %w", err)
		}
		return b, nil
	case http.StatusBadRequest:
		msg, ok := readErrorMessage(resp.Body)
		if !ok {
			return bookmark.Bookmark{}, unexpectedStatus(resp)
		}
		msg = strings.TrimPrefix(msg, "invalid bookmark: ")
		return bookmark.Bookmark{}, fmt.Errorf("%w: %s", bookmark.ErrInvalid, msg)
	default:
		return bookmark.Bookmark{}, unexpectedStatus(resp)
	}
}

// Delete removes a bookmark through the API.
func (c *Client) Delete(ctx context.Context, id int) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/bookmarks/"+strconv.Itoa(id), nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("%w: %d", bookmark.ErrNotFound, id)
	default:
		return unexpectedStatus(resp)
	}
}

// readErrorMessage returns the "error" field of a JSON error body.
// It reports false if the body does not decode or has no message.
func readErrorMessage(r io.Reader) (string, bool) {
	var e apiError
	if err := json.NewDecoder(r).Decode(&e); err != nil || e.Error == "" {
		return "", false
	}
	return e.Error, true
}

func unexpectedStatus(resp *http.Response) error {
	return fmt.Errorf("bookmarks api: unexpected status %d", resp.StatusCode)
}
