package httpauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"codeplat/internal/core"
	"codeplat/internal/transport/httpauth"
)

// fakeAuth accepts only the token "good", as principal alice.
type fakeAuth struct{}

func (fakeAuth) Authenticate(token string) (core.Principal, error) {
	if token == "good" {
		return core.Principal{Name: "alice"}, nil
	}
	return core.Principal{}, core.ErrUnauthenticated
}

// openAuth admits every request with the zero principal.
type openAuth struct{}

func (openAuth) Authenticate(string) (core.Principal, error) {
	return core.Principal{}, nil
}

// recorder is a next handler that records whether it ran and what principal
// its request carried.
type recorder struct {
	called    bool
	principal core.Principal
	found     bool
}

func (rec *recorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec.called = true
	rec.principal, rec.found = core.PrincipalFrom(r.Context())
	w.WriteHeader(http.StatusOK)
}

func serve(t *testing.T, auth core.Authenticator, next http.Handler, header func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/issues", nil)
	if header != nil {
		header(req)
	}
	w := httptest.NewRecorder()
	httpauth.Require(auth, next).ServeHTTP(w, req)
	return w
}

func TestNoCredentialIsRefused(t *testing.T) {
	rec := &recorder{}
	w := serve(t, fakeAuth{}, rec, nil)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if got, want := w.Header().Get("WWW-Authenticate"), `Basic realm="codeplat"`; got != want {
		t.Errorf("WWW-Authenticate = %q, want %q", got, want)
	}
	if got, want := w.Header().Get("Content-Type"), "application/json"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if got, want := w.Body.String(), "{\"error\":\"unauthorized\"}\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if rec.called {
		t.Error("next was called for a refused request")
	}
}

func TestBearerGoodIsAdmitted(t *testing.T) {
	rec := &recorder{}
	w := serve(t, fakeAuth{}, rec, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer good")
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !rec.called {
		t.Fatal("next was not called for an admitted request")
	}
	if !rec.found || rec.principal.Name != "alice" {
		t.Errorf("principal = %+v (found %v), want Name alice", rec.principal, rec.found)
	}
}

func TestBearerBadIsRefused(t *testing.T) {
	rec := &recorder{}
	w := serve(t, fakeAuth{}, rec, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer bad")
	})

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if rec.called {
		t.Error("next was called for a refused request")
	}
}

func TestBasicGoodPasswordIsAdmitted(t *testing.T) {
	rec := &recorder{}
	w := serve(t, fakeAuth{}, rec, func(r *http.Request) {
		r.SetBasicAuth("anyone", "good")
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !rec.called {
		t.Fatal("next was not called for an admitted Basic request")
	}
	if !rec.found || rec.principal.Name != "alice" {
		t.Errorf("principal = %+v (found %v), want Name alice", rec.principal, rec.found)
	}
}

func TestZeroPrincipalLeavesContextUnchanged(t *testing.T) {
	rec := &recorder{}
	w := serve(t, openAuth{}, rec, nil)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !rec.called {
		t.Fatal("next was not called for an admitted request")
	}
	if rec.found {
		t.Errorf("PrincipalFrom reported a principal %+v for a zero principal", rec.principal)
	}
}
