package e2e_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"bookmarks/internal/api"
	"bookmarks/internal/client"
	"bookmarks/internal/store"
	"bookmarks/internal/ui"
)

// do sends a request and returns the status code and body.
func do(t *testing.T, req *http.Request) (int, string) {
	t.Helper()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s %s: %v", req.Method, req.URL, err)
	}
	return resp.StatusCode, string(body)
}

func get(t *testing.T, u string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		t.Fatal(err)
	}
	return do(t, req)
}

func postForm(t *testing.T, u string, form url.Values) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return do(t, req)
}

func TestBookmarkChain(t *testing.T) {
	apiServer := httptest.NewServer(api.NewHandler(store.New()))
	t.Cleanup(apiServer.Close)
	uiServer := httptest.NewServer(ui.NewHandler(client.New(apiServer.URL, nil)))
	t.Cleanup(uiServer.Close)

	// 1. The page loads.
	status, body := get(t, uiServer.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("step 1: GET / status = %d; want 200", status)
	}
	if !strings.Contains(body, `id="app"`) {
		t.Fatalf("step 1: GET / body lacks id=\"app\":\n%s", body)
	}

	// 2. A valid bookmark is added and shown.
	status, body = postForm(t, uiServer.URL+"/bookmarks", url.Values{
		"title": {"Go"},
		"url":   {"https://go.dev"},
	})
	if status != http.StatusOK {
		t.Fatalf("step 2: POST /bookmarks status = %d; want 200", status)
	}
	if !strings.Contains(body, "https://go.dev") {
		t.Fatalf("step 2: POST /bookmarks body lacks https://go.dev:\n%s", body)
	}

	// 3. An invalid bookmark is rejected with the message from the store.
	status, body = postForm(t, uiServer.URL+"/bookmarks", url.Values{
		"title": {"Bad"},
		"url":   {"not-a-url"},
	})
	if status != http.StatusOK {
		t.Fatalf("step 3: POST /bookmarks status = %d; want 200", status)
	}
	const wantErr = "url must be an http or https URL"
	if !strings.Contains(body, wantErr) {
		t.Fatalf("step 3: POST /bookmarks body lacks %q:\n%s", wantErr, body)
	}

	// 4. The API lists exactly the one valid bookmark.
	status, body = get(t, apiServer.URL+"/api/bookmarks")
	if status != http.StatusOK {
		t.Fatalf("step 4: GET /api/bookmarks status = %d; want 200", status)
	}
	var list []struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatalf("step 4: decode /api/bookmarks: %v\nbody: %s", err, body)
	}
	if len(list) != 1 {
		t.Fatalf("step 4: got %d bookmarks; want 1\nbody: %s", len(list), body)
	}
	if list[0].Title != "Go" {
		t.Fatalf("step 4: title = %q; want %q", list[0].Title, "Go")
	}
	id := list[0].ID

	// 5. Deleting the bookmark through the UI removes it from the response.
	req, err := http.NewRequest(http.MethodDelete, uiServer.URL+fmt.Sprintf("/bookmarks/%d", id), nil)
	if err != nil {
		t.Fatal(err)
	}
	status, body = do(t, req)
	if status != http.StatusOK {
		t.Fatalf("step 5: DELETE /bookmarks/%d status = %d; want 200", id, status)
	}
	if strings.Contains(body, "https://go.dev") {
		t.Fatalf("step 5: DELETE response still contains https://go.dev:\n%s", body)
	}

	// 6. The page no longer shows the bookmark.
	status, body = get(t, uiServer.URL+"/")
	if status != http.StatusOK {
		t.Fatalf("step 6: GET / status = %d; want 200", status)
	}
	if strings.Contains(body, "https://go.dev") {
		t.Fatalf("step 6: GET / still contains https://go.dev:\n%s", body)
	}
}
