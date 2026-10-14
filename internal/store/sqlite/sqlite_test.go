package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"codeplat/internal/core"
	"codeplat/internal/store/sqlite"
	"codeplat/internal/store/storetest"
)

func TestContract(t *testing.T) {
	storetest.Run(t, func(t *testing.T) core.Store {
		s, err := sqlite.Open(filepath.Join(t.TempDir(), "new-dir", "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { s.Close() })
		return s
	})
}

// A database written before schema versioning has the first tables at
// user_version 0. Opening it must upgrade it and keep its rows.
func TestOpenUpgradesUnversionedDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE projects (key TEXT PRIMARY KEY, name TEXT NOT NULL, next_number INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL);
		CREATE TABLE issues (id TEXT PRIMARY KEY, project_key TEXT NOT NULL REFERENCES projects(key), number INTEGER NOT NULL,
			title TEXT NOT NULL, description TEXT NOT NULL, status TEXT NOT NULL, assignee TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO projects VALUES ('OLD', 'Old', 2, '2026-01-01T00:00:00Z');
		INSERT INTO issues VALUES ('OLD-1', 'OLD', 1, 'kept', '', 'todo', '', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()

	ctx := context.Background()
	for range 2 { // the second Open checks that migrating is idempotent
		s, err := sqlite.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		got, err := s.GetIssue(ctx, "OLD-1")
		if err != nil || got.Title != "kept" || got.Stage != core.StageRefinement {
			t.Fatalf("GetIssue after upgrade = %+v, %v", got, err)
		}
		if _, err := s.ListComments(ctx, "OLD-1"); err != nil {
			t.Fatalf("ListComments after upgrade: %v", err)
		}
		s.Close()
	}
}
