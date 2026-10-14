package ui_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"bookmarks/internal/bookmark"
	"bookmarks/internal/ui"
)

// fakeService is an in-memory bookmark.Service. If failErr is set, every call
// returns it.
type fakeService struct {
	items   []bookmark.Bookmark
	nextID  int
	failErr error
}

func newFake(items ...bookmark.Bookmark) *fakeService {
	f := &fakeService{nextID: 1}
	for _, b := range items {
		f.items = append(f.items, b)
		if b.ID >= f.nextID {
			f.nextID = b.ID + 1
		}
	}
	return f
}

func (f *fakeService) List(ctx context.Context) ([]bookmark.Bookmark, error) {
	if f.failErr != nil {
		return nil, f.failErr
	}
	out := make([]bookmark.Bookmark, len(f.items))
	copy(out, f.items)
	return out, nil
}

func (f *fakeService) Add(ctx context.Context, title, u string) (bookmark.Bookmark, error) {
	if f.failErr != nil {
		return bookmark.Bookmark{}, f.failErr
	}
	if title == "" {
		return bookmark.Bookmark{}, fmt.Errorf("%w: title is required", bookmark.ErrInvalid)
	}
	if !strings.HasPrefix(u, "http") {
		return bookmark.Bookmark{}, fmt.Errorf("%w: url must start with http", bookmark.ErrInvalid)
	}
	b := bookmark.Bookmark{ID: f.nextID, Title: title, URL: u, CreatedAt: time.Unix(0, 0).UTC()}
	f.nextID++
	f.items = append(f.items, b)
	return b, nil
}

func (f *fakeService) Delete(ctx context.Context, id int) error {
	if f.failErr != nil {
		return f.failErr
	}
	for i, b := range f.items {
		if b.ID == id {
			f.items = append(f.items[:i], f.items[i+1:]...)
			return nil
		}
	}
	return bookmark.ErrNotFound
}

func do(t *testing.T, h http.Handler, req *http.Request) (int, http.Header, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, res.Header, string(body)
}

func postForm(path string, form url.Values) *http.Request {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func assertHTML(t *testing.T, h http.Header) {
	t.Helper()
	if got := h.Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", got)
	}
}

func TestGetPageIsFullDocument(t *testing.T) {
	svc := newFake(bookmark.Bookmark{ID: 1, Title: "Go Home", URL: "https://go.dev"})
	status, hdr, body := do(t, ui.NewHandler(svc), httptest.NewRequest(http.MethodGet, "/", nil))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	assertHTML(t, hdr)
	for _, want := range []string{
		"<!doctype html>",
		"<title>Bookmarks</title>",
		`<script src="https://unpkg.com/htmx.org@2.0.4"></script>`,
		`<section id="app"`,
		"Go Home",
		"https://go.dev",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q\n%s", want, body)
		}
	}
}

func TestGetUnknownPathIs404(t *testing.T) {
	status, _, _ := do(t, ui.NewHandler(newFake()), httptest.NewRequest(http.MethodGet, "/nope", nil))
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

func TestPostSuccessReturnsFragment(t *testing.T) {
	svc := newFake()
	form := url.Values{"title": {"Go"}, "url": {"https://go.dev"}}
	status, hdr, body := do(t, ui.NewHandler(svc), postForm("/bookmarks", form))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	assertHTML(t, hdr)
	if !strings.HasPrefix(body, `<section id="app"`) {
		t.Errorf("body does not start with <section id=\"app\"; got %.80q", body)
	}
	if strings.Contains(body, "<html") {
		t.Errorf("fragment contains <html")
	}
	if !strings.Contains(body, "Go") || !strings.Contains(body, "https://go.dev") {
		t.Errorf("fragment does not list the new bookmark\n%s", body)
	}
	if !strings.Contains(body, `name="title" value=""`) || !strings.Contains(body, `name="url" value=""`) {
		t.Errorf("inputs are not empty after success\n%s", body)
	}
	if len(svc.items) != 1 {
		t.Errorf("service has %d items, want 1", len(svc.items))
	}
}

func TestPostInvalidReturnsErrorAndKeepsValues(t *testing.T) {
	svc := newFake()
	form := url.Values{"title": {"Keep me"}, "url": {"not-a-url"}}
	status, hdr, body := do(t, ui.NewHandler(svc), postForm("/bookmarks", form))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a validation error", status)
	}
	assertHTML(t, hdr)
	if !strings.HasPrefix(body, `<section id="app"`) {
		t.Errorf("body does not start with <section id=\"app\"")
	}
	if !strings.Contains(body, "invalid bookmark: url must start with http") {
		t.Errorf("error text missing\n%s", body)
	}
	if !strings.Contains(body, `value="Keep me"`) || !strings.Contains(body, `value="not-a-url"`) {
		t.Errorf("submitted values not kept\n%s", body)
	}
	if len(svc.items) != 0 {
		t.Errorf("invalid bookmark was stored")
	}
}

func TestDeleteReturnsFragmentWithoutBookmark(t *testing.T) {
	svc := newFake(
		bookmark.Bookmark{ID: 1, Title: "First", URL: "https://one.example"},
		bookmark.Bookmark{ID: 2, Title: "Second", URL: "https://two.example"},
	)
	status, hdr, body := do(t, ui.NewHandler(svc), httptest.NewRequest(http.MethodDelete, "/bookmarks/1", nil))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	assertHTML(t, hdr)
	if !strings.HasPrefix(body, `<section id="app"`) {
		t.Errorf("body does not start with <section id=\"app\"")
	}
	if strings.Contains(body, "First") {
		t.Errorf("deleted bookmark still listed\n%s", body)
	}
	if !strings.Contains(body, "Second") {
		t.Errorf("remaining bookmark missing\n%s", body)
	}
}

func TestDeleteMissingIDStillReturnsFragment(t *testing.T) {
	svc := newFake(bookmark.Bookmark{ID: 1, Title: "Only", URL: "https://only.example"})
	status, _, body := do(t, ui.NewHandler(svc), httptest.NewRequest(http.MethodDelete, "/bookmarks/99", nil))

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if !strings.HasPrefix(body, `<section id="app"`) || !strings.Contains(body, "Only") {
		t.Errorf("fragment with fresh list expected\n%s", body)
	}
}

func TestDeleteNonIntegerIDIs400(t *testing.T) {
	status, _, _ := do(t, ui.NewHandler(newFake()), httptest.NewRequest(http.MethodDelete, "/bookmarks/abc", nil))
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", status)
	}
}

func TestServiceFailureIs502(t *testing.T) {
	boom := errors.New("connection refused")
	h := ui.NewHandler(&fakeService{nextID: 1, failErr: boom})

	cases := map[string]*http.Request{
		"page":   httptest.NewRequest(http.MethodGet, "/", nil),
		"add":    postForm("/bookmarks", url.Values{"title": {"a"}, "url": {"http://a"}}),
		"delete": httptest.NewRequest(http.MethodDelete, "/bookmarks/1", nil),
	}
	for name, req := range cases {
		t.Run(name, func(t *testing.T) {
			status, _, body := do(t, h, req)
			if status != http.StatusBadGateway {
				t.Fatalf("status = %d, want 502", status)
			}
			if !strings.Contains(body, "connection refused") {
				t.Errorf("body %q does not carry the failure", body)
			}
		})
	}
}

func TestTitleIsEscaped(t *testing.T) {
	const evil = "<script>alert(1)</script>"
	svc := newFake(bookmark.Bookmark{ID: 1, Title: evil, URL: "https://x.example"})
	h := ui.NewHandler(svc)

	_, _, page := do(t, h, httptest.NewRequest(http.MethodGet, "/", nil))
	_, _, frag := do(t, h, postForm("/bookmarks", url.Values{"title": {evil}, "url": {"bad"}}))

	for name, body := range map[string]string{"page": page, "fragment": frag} {
		if strings.Contains(body, "<script>alert(1)") {
			t.Errorf("%s contains an unescaped script tag", name)
		}
		if !strings.Contains(body, "&lt;script&gt;") {
			t.Errorf("%s does not contain the escaped title", name)
		}
	}
}

func TestURLIsEscapedInInputValue(t *testing.T) {
	const evil = `"><script>alert(1)</script>`
	svc := newFake()
	_, _, body := do(t, ui.NewHandler(svc), postForm("/bookmarks", url.Values{"title": {""}, "url": {evil}}))
	if strings.Contains(body, "<script>alert(1)") {
		t.Errorf("submitted URL echoed unescaped\n%s", body)
	}
}
