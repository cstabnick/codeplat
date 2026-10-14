// Package apptest is the test entrypoint: it builds the app through the
// same composition root as the binaries, using the test environment's config.
package apptest

import (
	"maps"
	"testing"

	"codeplat/config"
	"codeplat/internal/app"
)

// Tokens are the test tokens. A closed test app admits exactly these.
var Tokens = map[string]string{"alice": "alice-token", "bob": "bob-token"}

// New returns a fresh app in the test environment, open as config/test.json
// says, so requests need no token. It is closed when the test ends.
func New(t testing.TB) *app.App {
	t.Helper()
	return build(t, true)
}

// NewWithAuth returns a fresh app like New, but closed: requests need a
// token from Tokens. It is closed when the test ends.
func NewWithAuth(t testing.TB) *app.App {
	t.Helper()
	return build(t, false)
}

func build(t testing.TB, open bool) *app.App {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Auth.Open = open
	cfg.Auth.Tokens = maps.Clone(Tokens)
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
