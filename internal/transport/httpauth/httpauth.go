// Package httpauth admits the HTTP requests whose credential a
// core.Authenticator accepts. The REST and web transports share it.
package httpauth

import (
	"encoding/json"
	"net/http"
	"strings"

	"codeplat/internal/core"
)

// Require returns a handler that authenticates each request, then calls next.
//
// The credential is the remainder of an "Authorization: Bearer " header. If
// there is no such header, it is the password of HTTP Basic credentials, and
// the username is ignored. Basic credentials let browsers prompt for the token
// and resend it on every request, including EventSource requests, which
// cannot set headers.
//
// The Authenticator is called with the credential, or with "" when none was
// sent. Any error from it refuses the request. A refused request gets status
// 401, the header `WWW-Authenticate: Basic realm="codeplat"`, the header
// `Content-Type: application/json`, the body `{"error":"unauthorized"}` and a
// newline, and next is not called.
//
// An admitted request whose principal has a non-empty Name goes to next with
// that principal in its context, set by core.WithPrincipal. A principal with
// an empty Name, which is what Open returns, leaves the context unchanged.
func Require(auth core.Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := credential(r)
		p, err := auth.Authenticate(token)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="codeplat"`)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
			return
		}
		if p.Name != "" {
			r = r.WithContext(core.WithPrincipal(r.Context(), p))
		}
		next.ServeHTTP(w, r)
	})
}

// credential returns the bearer token, else the Basic password, else "".
func credential(r *http.Request) string {
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return strings.TrimSpace(token)
	}
	if _, password, ok := r.BasicAuth(); ok {
		return password
	}
	return ""
}
