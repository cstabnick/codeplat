package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"bookmarks/internal/bookmark"
)

// fakeService is an in-test bookmark.Service. Each method records its call
// and returns the configured result.
type fakeService struct {
	listResult []bookmark.Bookmark
	listErr    error

	addArgs   [2]string
	addResult bookmark.Bookmark
	addErr    error
	addCalled bool

	deleteID     int
	deleteErr    error
	deleteCalled bool
}

func (f *fakeService) List(ctx context.Context) ([]bookmark.Bookmark, error) {
	return f.listResult, f.listErr
}

func (f *fakeService) Add(ctx context.Context, title, url string) (bookmark.Bookmark, error) {
	f.addCalled = true
	f.addArgs = [2]string{title, url}
	return f.addResult, f.addErr
}

func (f *fakeService) Delete(ctx context.Context, id int) error {
	f.deleteCalled = true
	f.deleteID = id
	return f.deleteErr
}

var sampleTime = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func sampleBookmark() bookmark.Bookmark {
	return bookmark.Bookmark{ID: 1, Title: "Go", URL: "https://go.dev", CreatedAt: sampleTime}
}

// do sends a request to the handler and returns the recorded response.
func do(t *testing.T, svc bookmark.Service, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
	}
	rec := httptest.NewRecorder()
	NewHandler(svc).ServeHTTP(rec, req)
	return rec
}

func assertJSONContentType(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func assertErrorBody(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantMsg string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d (body %q)", rec.Code, wantStatus, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body %q is not an error object: %v", rec.Body.String(), err)
	}
	if got.Error != wantMsg {
		t.Errorf("error = %q, want %q", got.Error, wantMsg)
	}
}

// --- GET /api/bookmarks ---

func TestListReturnsBookmarks(t *testing.T) {
	svc := &fakeService{listResult: []bookmark.Bookmark{sampleBookmark()}}
	rec := do(t, svc, http.MethodGet, "/api/bookmarks", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	assertJSONContentType(t, rec)
	var got []bookmark.Bookmark
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != 1 || got[0].Title != "Go" || got[0].URL != "https://go.dev" {
		t.Errorf("got %+v, want one bookmark Go", got)
	}
}

func TestListWireFormat(t *testing.T) {
	svc := &fakeService{listResult: []bookmark.Bookmark{sampleBookmark()}}
	rec := do(t, svc, http.MethodGet, "/api/bookmarks", "")

	var raw []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(raw) != 1 {
		t.Fatalf("got %d items, want 1", len(raw))
	}
	for _, key := range []string{"id", "title", "url", "created_at"} {
		if _, ok := raw[0][key]; !ok {
			t.Errorf("missing JSON field %q in %s", key, rec.Body.String())
		}
	}
}

func TestListEmptyEncodesAsArray(t *testing.T) {
	cases := map[string][]bookmark.Bookmark{
		"nil slice":   nil,
		"empty slice": {},
	}
	for name, list := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeService{listResult: list}
			rec := do(t, svc, http.MethodGet, "/api/bookmarks", "")
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", rec.Code)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
				t.Errorf("body = %q, want []", got)
			}
		})
	}
}

func TestListServiceErrorIs500(t *testing.T) {
	svc := &fakeService{listErr: errors.New("disk on fire")}
	rec := do(t, svc, http.MethodGet, "/api/bookmarks", "")
	assertErrorBody(t, rec, http.StatusInternalServerError, "internal error")
}

// --- POST /api/bookmarks ---

func TestAddCreatesBookmark(t *testing.T) {
	svc := &fakeService{addResult: sampleBookmark()}
	rec := do(t, svc, http.MethodPost, "/api/bookmarks", `{"title":"Go","url":"https://go.dev"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
	assertJSONContentType(t, rec)
	if !svc.addCalled {
		t.Fatal("Add was not called")
	}
	if svc.addArgs != [2]string{"Go", "https://go.dev"} {
		t.Errorf("Add args = %v, want [Go https://go.dev]", svc.addArgs)
	}
	var got bookmark.Bookmark
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != 1 || got.Title != "Go" || got.URL != "https://go.dev" || !got.CreatedAt.Equal(sampleTime) {
		t.Errorf("body bookmark = %+v, want the bookmark returned by Add", got)
	}
}

func TestAddInvalidJSONBodies(t *testing.T) {
	cases := map[string]string{
		"malformed":     `{"title":`,
		"empty body":    ``,
		"wrong type":    `{"title":5,"url":"https://go.dev"}`,
		"not an object": `"just a string"`,
		"array body":    `[]`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			svc := &fakeService{}
			rec := do(t, svc, http.MethodPost, "/api/bookmarks", body)
			assertErrorBody(t, rec, http.StatusBadRequest, "invalid JSON body")
			if svc.addCalled {
				t.Error("Add was called for an undecodable body")
			}
		})
	}
}

func TestAddBodyOverOneMegabyteIsInvalidJSON(t *testing.T) {
	big := `{"title":"` + strings.Repeat("a", 1<<20+1) + `","url":"https://go.dev"}`
	svc := &fakeService{}
	rec := do(t, svc, http.MethodPost, "/api/bookmarks", big)
	assertErrorBody(t, rec, http.StatusBadRequest, "invalid JSON body")
	if svc.addCalled {
		t.Error("Add was called for an oversize body")
	}
}

func TestAddUnknownFieldsAccepted(t *testing.T) {
	svc := &fakeService{addResult: sampleBookmark()}
	rec := do(t, svc, http.MethodPost, "/api/bookmarks", `{"title":"Go","url":"https://go.dev","extra":1}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %q)", rec.Code, rec.Body.String())
	}
}

func TestAddInvalidBookmarkIs400WithServiceMessage(t *testing.T) {
	svc := &fakeService{addErr: fmt.Errorf("title is empty: %w", bookmark.ErrInvalid)}
	rec := do(t, svc, http.MethodPost, "/api/bookmarks", `{"title":"","url":"https://go.dev"}`)
	assertErrorBody(t, rec, http.StatusBadRequest, "title is empty: invalid bookmark")
}

func TestAddOtherServiceErrorIs500(t *testing.T) {
	svc := &fakeService{addErr: errors.New("db down")}
	rec := do(t, svc, http.MethodPost, "/api/bookmarks", `{"title":"Go","url":"https://go.dev"}`)
	assertErrorBody(t, rec, http.StatusInternalServerError, "internal error")
}

// --- DELETE /api/bookmarks/{id} ---

func TestDeleteReturns204WithNoBody(t *testing.T) {
	svc := &fakeService{}
	rec := do(t, svc, http.MethodDelete, "/api/bookmarks/7", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body = %q, want empty", rec.Body.String())
	}
	if !svc.deleteCalled || svc.deleteID != 7 {
		t.Errorf("Delete called=%v id=%d, want called with 7", svc.deleteCalled, svc.deleteID)
	}
}

func TestDeleteNonIntegerIDIs400(t *testing.T) {
	for _, id := range []string{"abc", "1.5", "x1"} {
		t.Run(id, func(t *testing.T) {
			svc := &fakeService{}
			rec := do(t, svc, http.MethodDelete, "/api/bookmarks/"+id, "")
			assertErrorBody(t, rec, http.StatusBadRequest, "invalid id")
			if svc.deleteCalled {
				t.Error("Delete was called for a non-integer id")
			}
		})
	}
}

func TestDeleteNotFoundIs404WithServiceMessage(t *testing.T) {
	svc := &fakeService{deleteErr: fmt.Errorf("id 9: %w", bookmark.ErrNotFound)}
	rec := do(t, svc, http.MethodDelete, "/api/bookmarks/9", "")
	assertErrorBody(t, rec, http.StatusNotFound, "id 9: bookmark not found")
}

func TestDeleteOtherServiceErrorIs500(t *testing.T) {
	svc := &fakeService{deleteErr: errors.New("db down")}
	rec := do(t, svc, http.MethodDelete, "/api/bookmarks/9", "")
	assertErrorBody(t, rec, http.StatusInternalServerError, "internal error")
}
