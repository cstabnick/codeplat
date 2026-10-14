// Package sqlite is a core.Store backed by a SQLite database file.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"codeplat/internal/core"
)

const schema = `
CREATE TABLE IF NOT EXISTS projects (
	key         TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	next_number INTEGER NOT NULL DEFAULT 1,
	created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS issues (
	id          TEXT PRIMARY KEY,
	project_key TEXT NOT NULL REFERENCES projects(key),
	number      INTEGER NOT NULL,
	title       TEXT NOT NULL,
	description TEXT NOT NULL,
	status      TEXT NOT NULL,
	assignee    TEXT NOT NULL,
	created_at  TEXT NOT NULL,
	updated_at  TEXT NOT NULL
);`

const issueColumns = `id, project_key, number, title, description, status, assignee, created_at, updated_at`

type Store struct {
	db *sql.DB
}

// Open opens the database at path, creating it, its directory and its
// schema if needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	// Immediate transactions take the write lock up front, so concurrent
	// writers queue on busy_timeout instead of failing mid-transaction.
	dsn := "file:" + path + "?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) CreateProject(ctx context.Context, p core.Project) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO projects (key, name, created_at) VALUES (?, ?, ?) ON CONFLICT (key) DO NOTHING`,
		p.Key, p.Name, formatTime(p.CreatedAt))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project %s: %w", p.Key, core.ErrConflict)
	}
	return nil
}

func (s *Store) ListProjects(ctx context.Context) ([]core.Project, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, name, created_at FROM projects ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Project{}
	for rows.Next() {
		var p core.Project
		var created string
		if err := rows.Scan(&p.Key, &p.Name, &created); err != nil {
			return nil, err
		}
		if p.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) CreateIssue(ctx context.Context, i core.Issue) (core.Issue, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Issue{}, err
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx,
		`UPDATE projects SET next_number = next_number + 1 WHERE key = ? RETURNING next_number - 1`,
		i.ProjectKey).Scan(&i.Number)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Issue{}, fmt.Errorf("project %s: %w", i.ProjectKey, core.ErrNotFound)
	}
	if err != nil {
		return core.Issue{}, err
	}
	i.ID = core.IssueID(i.ProjectKey, i.Number)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO issues (`+issueColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.ProjectKey, i.Number, i.Title, i.Description, i.Status, i.Assignee,
		formatTime(i.CreatedAt), formatTime(i.UpdatedAt))
	if err != nil {
		return core.Issue{}, err
	}
	return i, tx.Commit()
}

func (s *Store) GetIssue(ctx context.Context, id string) (core.Issue, error) {
	return scanIssue(id, s.db.QueryRowContext(ctx, `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
}

func (s *Store) ListIssues(ctx context.Context, f core.IssueFilter) ([]core.Issue, error) {
	var where []string
	var args []any
	if f.ProjectKey != "" {
		where = append(where, "project_key = ?")
		args = append(args, f.ProjectKey)
	}
	if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Assignee != "" {
		where = append(where, "assignee = ?")
		args = append(args, f.Assignee)
	}
	query := `SELECT ` + issueColumns + ` FROM issues`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY project_key, number`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Issue{}
	for rows.Next() {
		i, err := scanIssue("", rows)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Store) UpdateIssue(ctx context.Context, id string, fn func(*core.Issue) error) (core.Issue, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Issue{}, err
	}
	defer tx.Rollback()

	i, err := scanIssue(id, tx.QueryRowContext(ctx, `SELECT `+issueColumns+` FROM issues WHERE id = ?`, id))
	if err != nil {
		return core.Issue{}, err
	}
	if err := fn(&i); err != nil {
		return core.Issue{}, err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE issues SET title = ?, description = ?, status = ?, assignee = ?, updated_at = ? WHERE id = ?`,
		i.Title, i.Description, i.Status, i.Assignee, formatTime(i.UpdatedAt), id)
	if err != nil {
		return core.Issue{}, err
	}
	return i, tx.Commit()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanIssue(id string, row scanner) (core.Issue, error) {
	var i core.Issue
	var created, updated string
	err := row.Scan(&i.ID, &i.ProjectKey, &i.Number, &i.Title, &i.Description, &i.Status, &i.Assignee, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Issue{}, fmt.Errorf("issue %s: %w", id, core.ErrNotFound)
	}
	if err != nil {
		return core.Issue{}, err
	}
	if i.CreatedAt, err = parseTime(created); err != nil {
		return core.Issue{}, err
	}
	if i.UpdatedAt, err = parseTime(updated); err != nil {
		return core.Issue{}, err
	}
	return i, nil
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}
