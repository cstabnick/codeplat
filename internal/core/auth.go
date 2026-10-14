package core

import (
	"context"
	"errors"
)

// ErrUnauthenticated is returned by an Authenticator for a missing or unknown token.
var ErrUnauthenticated = errors.New("unauthenticated")

// Principal is the caller a transport authenticated. Name is the assignee or
// comment author that the caller's own writes use. A Principal in a context
// always has a non-empty Name.
type Principal struct {
	Name string
}

type principalKey struct{}

// WithPrincipal returns a copy of ctx that carries p.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal carried by ctx, if any.
func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// Authenticator maps the credential a request presented to a principal. It is
// a port: transports call it, and internal/auth implements it.
type Authenticator interface {
	// Authenticate returns the principal for token. token is empty when the
	// request presented no credential. It returns an error wrapping
	// ErrUnauthenticated when the caller is not admitted.
	Authenticate(token string) (Principal, error)
}
