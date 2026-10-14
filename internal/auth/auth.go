// Package auth implements core.Authenticator.
package auth

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	"codeplat/internal/core"
)

// Open admits every request and identifies no one, so a write keeps the
// free-text assignee and author it would have without authentication. The dev
// and test environments use it.
type Open struct{}

// Authenticate ignores the token and returns the zero Principal and no error.
func (Open) Authenticate(string) (core.Principal, error) {
	return core.Principal{}, nil
}

// Static admits a request whose token equals one of a fixed table of tokens.
// Each entry names one principal.
type Static struct {
	entries []entry // unexported; sorted by name
}

type entry struct {
	name  string
	token string
}

// NewStatic returns an authenticator for tokens, which maps each principal
// name to its token. It returns a plain error, which does not wrap
// core.ErrUnauthenticated, with a message that names the problem, if a name is
// empty or contains whitespace, a token is empty, or two names have the same
// token. An empty map is valid and admits nothing.
func NewStatic(tokens map[string]string) (*Static, error) {
	entries := make([]entry, 0, len(tokens))
	seen := make(map[string]string, len(tokens))
	for _, name := range slices.Sorted(maps.Keys(tokens)) {
		token := tokens[name]
		if name == "" {
			return nil, errors.New("auth: principal name is empty")
		}
		if strings.ContainsFunc(name, unicode.IsSpace) {
			return nil, fmt.Errorf("auth: principal name %q contains whitespace", name)
		}
		if token == "" {
			return nil, fmt.Errorf("auth: token for principal %q is empty", name)
		}
		if other, ok := seen[token]; ok {
			return nil, fmt.Errorf("auth: principals %q and %q have the same token", other, name)
		}
		seen[token] = name
		entries = append(entries, entry{name: name, token: token})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return strings.Compare(a.name, b.name)
	})
	return &Static{entries: entries}, nil
}

// Authenticate returns the principal whose token equals token. An empty token,
// or one that matches no entry, returns the zero Principal and an error
// wrapping core.ErrUnauthenticated.
func (s *Static) Authenticate(token string) (core.Principal, error) {
	if token == "" {
		return core.Principal{}, fmt.Errorf("auth: no token: %w", core.ErrUnauthenticated)
	}
	found := false
	var principal core.Principal
	for _, e := range s.entries {
		if subtle.ConstantTimeCompare([]byte(e.token), []byte(token)) == 1 {
			found = true
			principal = core.Principal{Name: e.name}
		}
	}
	if !found {
		return core.Principal{}, fmt.Errorf("auth: unknown token: %w", core.ErrUnauthenticated)
	}
	return principal, nil
}
