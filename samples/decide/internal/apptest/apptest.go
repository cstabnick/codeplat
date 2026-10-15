// Package apptest builds the decide service for tests from the test config.
package apptest

import (
	"os"
	"path/filepath"
	"testing"

	"decide/internal/app"
	"decide/internal/config"
	"decide/internal/core"
)

// New builds the app from config.Load("test") with ModelsDir set to ModelsDir(t);
// t.Fatal on error.
func New(t testing.TB) *core.Service {
	t.Helper()
	cfg, err := config.Load("test")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ModelsDir = ModelsDir(t)
	svc, err := app.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// ModelsDir is <module root>/models, where the module root is the nearest
// ancestor of os.Getwd() containing go.mod.
func ModelsDir(t testing.TB) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "models")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("apptest: no go.mod in or above %s", wd)
		}
		dir = parent
	}
}
