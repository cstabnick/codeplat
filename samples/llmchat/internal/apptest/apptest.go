// Package apptest builds llmchat's app for tests, through app.New, so tests
// exercise the production wiring with a fake router and their own database.
package apptest

import (
	"testing"

	"llmchat/internal/app"
	"llmchat/internal/config"
)

// New builds the app against routerURL with the database at dbPath and closes
// it when the test ends. A test may close the app itself to simulate a
// restart; the second close is ignored.
func New(t *testing.T, routerURL, dbPath string) *app.App {
	t.Helper()
	return NewWith(t, config.Config{
		Addr:      "127.0.0.1:0",
		DBPath:    dbPath,
		RouterURL: routerURL,
	})
}

// NewWith builds the app from cfg, as given, and closes it when the test ends.
// Tests use it to set fields New does not, such as Exec, SandboxURL,
// ProjectsDir and Workspace.
func NewWith(t *testing.T, cfg config.Config) *app.App {
	t.Helper()
	a, err := app.New(cfg)
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
