// Package apptest builds the real service from the test configuration for tests.
package apptest

import (
	"testing"

	"chess/internal/app"
	"chess/internal/config"
	"chess/internal/core"
)

// New builds the service from the "test" configuration. The store is closed
// when the test ends.
func New(t testing.TB) *core.Service {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	svc, closeFn, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := closeFn(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return svc
}
