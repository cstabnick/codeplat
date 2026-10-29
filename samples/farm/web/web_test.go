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
	Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func checkNoCache(t *testing.T, path string, rec *httptest.ResponseRecorder) {
	t.Helper()
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Errorf("GET %s: Cache-Control = %q, want %q", path, got, "no-cache")
	}
}

func TestIndex(t *testing.T) {
	rec := get(t, "/")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /: status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /: Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), `<canvas id="farm"`) {
		t.Errorf("GET /: body does not contain %q", `<canvas id="farm"`)
	}
	checkNoCache(t, "/", rec)
}

func TestScript(t *testing.T) {
	rec := get(t, "/js/state.js")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /js/state.js: status = %d, want 200", rec.Code)
	}
	const want = "text/javascript; charset=utf-8"
	if ct := rec.Header().Get("Content-Type"); ct != want {
		t.Errorf("GET /js/state.js: Content-Type = %q, want %q", ct, want)
	}
	checkNoCache(t, "/js/state.js", rec)
}

func TestMissingScriptIs404(t *testing.T) {
	rec := get(t, "/js/nope.js")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /js/nope.js: status = %d, want 404", rec.Code)
	}
	checkNoCache(t, "/js/nope.js", rec)
}

func TestDirectoryIs404(t *testing.T) {
	rec := get(t, "/js/")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /js/: status = %d, want 404", rec.Code)
	}
	checkNoCache(t, "/js/", rec)
}

func TestUnknownPathIs404(t *testing.T) {
	rec := get(t, "/somewhere/else")
	if rec.Code != http.StatusNotFound {
		t.Errorf("GET /somewhere/else: status = %d, want 404", rec.Code)
	}
	checkNoCache(t, "/somewhere/else", rec)
}
