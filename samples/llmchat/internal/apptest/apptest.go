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
	a, err := app.New(config.Config{
		Addr:      "127.0.0.1:0",
		DBPath:    dbPath,
		RouterURL: routerURL,
	})
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}
