package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeplat/internal/apptest"
	"codeplat/internal/transport/httpauth"
	"codeplat/internal/transport/web"
)

func TestWebRequiresToken(t *testing.T) {
	a := apptest.NewWithAuth(t)
	h := httpauth.Require(a.Auth, web.NewHandler(t.Context(), a.Service, a.Log))

	t.Run("no credential", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="codeplat"` {
			t.Fatalf("WWW-Authenticate = %q", got)
		}
	})

	t.Run("basic credentials", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("anyone", apptest.Tokens["bob"])
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !strings.Contains(rec.Body.String(), "No projects yet") {
			t.Fatalf("body does not show the home page:\n%s", rec.Body.String())
		}
	})

	t.Run("bearer token", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+apptest.Tokens["alice"])
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
	})

	t.Run("wrong basic password", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.SetBasicAuth("anyone", "not-a-token")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
