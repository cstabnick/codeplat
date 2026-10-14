package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bookmarks/internal/bookmark"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL, nil)
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestListWithItems(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/bookmarks" {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, `[{"id":1,"title":"Go","url":"https://go.dev","created_at":"2026-01-01T00:00:00Z"},{"id":2,"title":"Docs","url":"https://pkg.go.dev","created_at":"2026-01-02T00:00:00Z"}]`)
	})

	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ID != 1 || got[0].Title != "Go" || got[0].URL != "https://go.dev" {
		t.Errorf("got[0] = %+v", got[0])
	}
	if got[1].ID != 2 || got[1].Title != "Docs" {
		t.Errorf("got[1] = %+v", got[1])
	}
	if got[0].CreatedAt.Year() != 2026 {
		t.Errorf("created_at not decoded: %v", got[0].CreatedAt)
	}
}

func TestListEmptyIsNonNil(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, `[]`)
	})

	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got == nil {
		t.Fatal("List returned nil, want empty non-nil slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestAddSuccess(t *testing.T) {
	var gotBody map[string]string
	var gotCT, gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		writeJSON(w, http.StatusCreated, `{"id":7,"title":"Go","url":"https://go.dev","created_at":"2026-01-01T00:00:00Z"}`)
	})

	b, err := c.Add(context.Background(), "Go", "https://go.dev")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/bookmarks" {
		t.Errorf("request = %s %s, want POST /api/bookmarks", gotMethod, gotPath)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", gotCT)
	}
	if gotBody["title"] != "Go" || gotBody["url"] != "https://go.dev" || len(gotBody) != 2 {
		t.Errorf("request body = %v, want {title, url}", gotBody)
	}
	if b.ID != 7 || b.Title != "Go" || b.URL != "https://go.dev" {
		t.Errorf("returned bookmark = %+v", b)
	}
}

func TestAddInvalid(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusBadRequest, `{"error":"invalid bookmark: title is required"}`)
	})

	_, err := c.Add(context.Background(), "", "https://go.dev")
	if err == nil {
		t.Fatal("Add: want error, got nil")
	}
	if !errors.Is(err, bookmark.ErrInvalid) {
		t.Errorf("errors.Is(err, ErrInvalid) = false; err = %v", err)
	}
	const want = "invalid bookmark: title is required"
	if err.Error() != want {
		t.Errorf("error text = %q, want %q", err.Error(), want)
	}
}

func TestDeleteSuccess(t *testing.T) {
	var gotMethod, gotPath string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	})

	if err := c.Delete(context.Background(), 3); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/api/bookmarks/3" {
		t.Errorf("request = %s %s, want DELETE /api/bookmarks/3", gotMethod, gotPath)
	}
}

func TestDeleteNotFound(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, `{"error":"bookmark not found"}`)
	})

	err := c.Delete(context.Background(), 99)
	if !errors.Is(err, bookmark.ErrNotFound) {
		t.Errorf("errors.Is(err, ErrNotFound) = false; err = %v", err)
	}
}

func TestUnexpectedStatus(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusInternalServerError, `{"error":"boom"}`)
	})
	const want = "bookmarks api: unexpected status 500"

	if _, err := c.List(context.Background()); err == nil || err.Error() != want {
		t.Errorf("List error = %v, want %q", err, want)
	}
	if _, err := c.Add(context.Background(), "t", "https://x.example"); err == nil || err.Error() != want {
		t.Errorf("Add error = %v, want %q", err, want)
	}
	if err := c.Delete(context.Background(), 1); err == nil || err.Error() != want {
		t.Errorf("Delete error = %v, want %q", err, want)
	}
}

func TestTrailingSlashInBaseURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		writeJSON(w, http.StatusOK, `[]`)
	}))
	t.Cleanup(srv.Close)

	c := New(srv.URL+"/", nil)
	if _, err := c.List(context.Background()); err != nil {
		t.Fatalf("List: %v", err)
	}
	if gotPath != "/api/bookmarks" || strings.Contains(gotPath, "//") {
		t.Errorf("path = %q, want /api/bookmarks", gotPath)
	}
}
