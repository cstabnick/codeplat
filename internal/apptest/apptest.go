// Package apptest is the test entrypoint: it builds the app through the
// same composition root as the binaries, using the test environment's config.
package apptest

import (
	"testing"

	"codeplat/config"
	"codeplat/internal/app"
)

// New returns a fresh, isolated app that is closed when the test ends.
func New(t testing.TB) *app.App {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	a, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
