// Package apptest builds an app for tests. Tests use it instead of calling
// app.New directly, so every test app is validated and closed the same way.
package apptest

import (
	"io"
	"log"
	"testing"

	"llmrouter/internal/app"
	"llmrouter/internal/config"
)

// New returns an app built from a base config with Addr "127.0.0.1:0" and
// no backends. edit, when not nil, sets the test's backends and routes on
// the config before it is validated. New fails the test if the config is
// invalid or the app cannot be built. It registers t.Cleanup to Close the
// app. It does not Start the app or serve HTTP; the test does both.
func New(t *testing.T, edit func(*config.Config)) *app.App {
	t.Helper()
	cfg := config.Config{Addr: "127.0.0.1:0"}
	if edit != nil {
		edit(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config is invalid: %v", err)
	}
	a, err := app.New(cfg, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return a
}
