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

// migrations run in order, once each; PRAGMA user_version counts how many
// have been applied. Append new entries; never edit or reorder old ones.
var migrations = []string{
	// IF NOT EXISTS because databases created before versioning already
	// have these tables at user_version 0.
	`CREATE TABLE IF NOT EXISTS projects (
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
	);`,
	`CREATE TABLE comments (
		issue_id   TEXT NOT NULL REFERENCES issues(id),
		number     INTEGER NOT NULL,
		author     TEXT NOT NULL,
		body       TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (issue_id, number)
	);`,
	`ALTER TABLE issues ADD COLUMN stage TEXT NOT NULL DEFAULT 'refinement';`,
}

const issueColumns = `id, project_key, number, title, description, status, assignee, created_at, updated_at, stage`

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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	for {
		done, err := migrateNext(db)
		if err != nil || done {
			return err
		}
	}
}

// migrateNext applies one pending migration. It reads the version inside
// the transaction so that processes starting together do not both apply it.
func migrateNext(db *sql.DB) (done bool, err error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return false, err
	}
	if version > len(migrations) {
		return false, fmt.Errorf("schema version %d is newer than this binary supports (%d)", version, len(migrations))
	}
	if version == len(migrations) {
		return true, nil
	}
	if _, err := tx.Exec(migrations[version]); err != nil {
		return false, fmt.Errorf("migration %d: %w", version+1, err)
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
		return false, err
	}
	return false, tx.Commit()
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
		return fmt.Errorf("project %s already exists: %w", p.Key, core.ErrConflict)
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
		`INSERT INTO issues (`+issueColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.ProjectKey, i.Number, i.Title, i.Description, i.Status, i.Assignee,
		formatTime(i.CreatedAt), formatTime(i.UpdatedAt), i.Stage)
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
	if f.Stage != "" {
		where = append(where, "stage = ?")
		args = append(args, f.Stage)
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
		`UPDATE issues SET title = ?, description = ?, stage = ?, status = ?, assignee = ?, updated_at = ? WHERE id = ?`,
		i.Title, i.Description, i.Stage, i.Status, i.Assignee, formatTime(i.UpdatedAt), id)
	if err != nil {
		return core.Issue{}, err
	}
	return i, tx.Commit()
}

func (s *Store) AddComment(ctx context.Context, c core.Comment) (core.Comment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Comment{}, err
	}
	defer tx.Rollback()

	if err := issueExists(ctx, tx, c.IssueID); err != nil {
		return core.Comment{}, err
	}
	err = tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM comments WHERE issue_id = ?`, c.IssueID).Scan(&c.Number)
	if err != nil {
		return core.Comment{}, err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO comments (issue_id, number, author, body, created_at) VALUES (?, ?, ?, ?, ?)`,
		c.IssueID, c.Number, c.Author, c.Body, formatTime(c.CreatedAt))
	if err != nil {
		return core.Comment{}, err
	}
	return c, tx.Commit()
}

func (s *Store) ListComments(ctx context.Context, issueID string) ([]core.Comment, error) {
	if err := issueExists(ctx, s.db, issueID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT issue_id, number, author, body, created_at FROM comments WHERE issue_id = ? ORDER BY number`, issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Comment{}
	for rows.Next() {
		var c core.Comment
		var created string
		if err := rows.Scan(&c.IssueID, &c.Number, &c.Author, &c.Body, &created); err != nil {
			return nil, err
		}
		if c.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func issueExists(ctx context.Context, q queryer, id string) error {
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM issues WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("issue %s: %w", id, core.ErrNotFound)
	}
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanIssue(id string, row scanner) (core.Issue, error) {
	var i core.Issue
	var created, updated string
	err := row.Scan(&i.ID, &i.ProjectKey, &i.Number, &i.Title, &i.Description, &i.Status, &i.Assignee, &created, &updated, &i.Stage)
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
