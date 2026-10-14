package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, method, path string) *http.Response {
	t.Helper()
	srv := httptest.NewServer(NewHandler())
	t.Cleanup(srv.Close)
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func body(t *testing.T, resp *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestIndexPage(t *testing.T) {
	resp := get(t, "GET", "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET / Content-Type = %q, want text/html", ct)
	}
	page := body(t, resp)
	for _, want := range []string{
		`id="sidebar"`,
		`id="conversations"`,
		`id="log"`,
		`id="composer"`,
		`id="prompt"`,
		`id="model"`,
		`id="new-chat"`,
		`id="delete"`,
		`src="/app.js"`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("page does not contain %s", want)
		}
	}
}

func TestAppScript(t *testing.T) {
	resp := get(t, "GET", "/app.js")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /app.js status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/javascript") && !strings.HasPrefix(ct, "application/javascript") {
		t.Fatalf("GET /app.js Content-Type = %q, want a JavaScript type", ct)
	}
	script := body(t, resp)
	for _, want := range []string{
		"/api/conversations",
		"/api/routes",
		"/messages",
		"renderMarkdown",
		"getReader",
		"textContent",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("app.js does not contain %q", want)
		}
	}
	for _, banned := range []string{"eval(", "document.write"} {
		if strings.Contains(script, banned) {
			t.Errorf("app.js contains %q", banned)
		}
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	resp := get(t, "GET", "/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /nope status = %d, want 404", resp.StatusCode)
	}
}

func TestPostToRootIsRejected(t *testing.T) {
	resp := get(t, "POST", "/")
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("POST / status = %d, want 405 or 404", resp.StatusCode)
	}
}
