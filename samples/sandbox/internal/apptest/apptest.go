// Package apptest builds the sandbox app from the test config for tests.
package apptest

import (
	"testing"

	"sandbox/internal/app"
	"sandbox/internal/config"
	"sandbox/internal/core"
)

// New builds the app from config.Load("test") with Root set to a temporary
// directory that is removed when the test ends. It fails the test on error.
func New(t testing.TB) *core.Service {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Root = t.TempDir()
	svc, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}
