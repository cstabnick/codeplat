package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func get(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	NewHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestPageServesHTML(t *testing.T) {
	rec := get(t, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET / Content-Type = %q, want text/html", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="board"`, "app.js"} {
		if !strings.Contains(body, want) {
			t.Errorf("GET / body does not contain %q", want)
		}
	}
}

func TestScriptUsesGamesAPI(t *testing.T) {
	rec := get(t, "/app.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /app.js status = %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("GET /app.js Content-Type = %q, want text/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/games") {
		t.Errorf("GET /app.js body does not contain %q", "/api/games")
	}
}

func TestUnknownPathIsNotFound(t *testing.T) {
	rec := get(t, "/missing")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /missing status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
