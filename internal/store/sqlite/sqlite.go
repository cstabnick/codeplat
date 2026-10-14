// Package sqlite is a core.Store backed by a SQLite database file.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
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
	`CREATE TABLE events (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		type        TEXT NOT NULL,
		project_key TEXT NOT NULL,
		issue_id    TEXT NOT NULL,
		at          TEXT NOT NULL
	);`,
	`ALTER TABLE issues ADD COLUMN epic INTEGER NOT NULL DEFAULT 0;`,
	`ALTER TABLE issues ADD COLUMN depends_on TEXT NOT NULL DEFAULT '[]';`,
	`ALTER TABLE issues ADD COLUMN parent TEXT NOT NULL DEFAULT '';`,
	`CREATE TABLE epics (
		project_key TEXT NOT NULL REFERENCES projects(key),
		parent      TEXT NOT NULL,
		number      INTEGER NOT NULL,
		name        TEXT NOT NULL,
		PRIMARY KEY (project_key, parent, number)
	);`,
}

const issueColumns = `id, project_key, number, title, description, status, assignee, created_at, updated_at, stage, epic, depends_on, parent`

// encodeDependsOn returns the JSON array text for ids; nil is "[]".
func encodeDependsOn(ids []string) (string, error) {
	if ids == nil {
		ids = []string{}
	}
	b, err := json.Marshal(ids)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

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

// Close folds the write-ahead log into the database file before closing, so
// the file alone holds everything written so far and can be copied or
// committed. The checkpoint is passive: it never waits for another process,
// and whatever it could not fold stays in the log for the next close.
func (s *Store) Close() error {
	_, _ = s.db.Exec(`PRAGMA wal_checkpoint(PASSIVE)`)
	return s.db.Close()
}

func (s *Store) CreateProject(ctx context.Context, p core.Project) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO projects (key, name, created_at) VALUES (?, ?, ?) ON CONFLICT (key) DO NOTHING`,
		p.Key, p.Name, formatTime(p.CreatedAt))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("project %s already exists: %w", p.Key, core.ErrConflict)
	}
	if err := recordEvent(ctx, tx, core.EventProjectCreated, p.Key, "", p.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
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
	if i.DependsOn == nil {
		i.DependsOn = []string{}
	}
	dependsOn, err := encodeDependsOn(i.DependsOn)
	if err != nil {
		return core.Issue{}, err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO issues (`+issueColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		i.ID, i.ProjectKey, i.Number, i.Title, i.Description, i.Status, i.Assignee,
		formatTime(i.CreatedAt), formatTime(i.UpdatedAt), i.Stage, i.Epic, dependsOn, i.Parent)
	if err != nil {
		return core.Issue{}, err
	}
	if err := recordEvent(ctx, tx, core.EventIssueCreated, i.ProjectKey, i.ID, i.CreatedAt); err != nil {
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
	if f.Parent != "" {
		where = append(where, "parent = ?")
		args = append(args, f.Parent)
	}
	query := `SELECT ` + issueColumns + ` FROM issues`
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, " AND ")
	}
	query += ` ORDER BY project_key, number`
	if f.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, f.Limit)
	} else if f.Offset > 0 {
		// SQLite accepts OFFSET only after LIMIT, so "no limit" is LIMIT -1.
		query += ` LIMIT -1`
	}
	if f.Offset > 0 {
		query += ` OFFSET ?`
		args = append(args, f.Offset)
	}

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
	if i.DependsOn == nil {
		i.DependsOn = []string{}
	}
	dependsOn, err := encodeDependsOn(i.DependsOn)
	if err != nil {
		return core.Issue{}, err
	}
	_, err = tx.ExecContext(ctx,
		`UPDATE issues SET title = ?, description = ?, stage = ?, status = ?, assignee = ?, updated_at = ?, epic = ?, depends_on = ?, parent = ? WHERE id = ?`,
		i.Title, i.Description, i.Stage, i.Status, i.Assignee, formatTime(i.UpdatedAt), i.Epic, dependsOn, i.Parent, id)
	if err != nil {
		return core.Issue{}, err
	}
	if err := recordEvent(ctx, tx, core.EventIssueUpdated, i.ProjectKey, id, i.UpdatedAt); err != nil {
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

	var projectKey string
	err = tx.QueryRowContext(ctx, `SELECT project_key FROM issues WHERE id = ?`, c.IssueID).Scan(&projectKey)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Comment{}, fmt.Errorf("issue %s: %w", c.IssueID, core.ErrNotFound)
	}
	if err != nil {
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
	if err := recordEvent(ctx, tx, core.EventCommentAdded, projectKey, c.IssueID, c.CreatedAt); err != nil {
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

func (s *Store) EventsSince(ctx context.Context, after int64) ([]core.Event, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, project_key, issue_id, at FROM events WHERE id > ? ORDER BY id`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Event{}
	for rows.Next() {
		var e core.Event
		var at string
		if err := rows.Scan(&e.ID, &e.Type, &e.ProjectKey, &e.IssueID, &at); err != nil {
			return nil, err
		}
		if e.At, err = parseTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) LastEventID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM events`).Scan(&id)
	return id, err
}

func (s *Store) SetEpic(ctx context.Context, e core.Epic, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var one int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE key = ?`, e.ProjectKey).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("project %s: %w", e.ProjectKey, core.ErrNotFound)
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO epics (project_key, parent, number, name) VALUES (?, ?, ?, ?) ON CONFLICT (project_key, parent, number) DO UPDATE SET name = excluded.name`,
		e.ProjectKey, e.Parent, e.Number, e.Name)
	if err != nil {
		return err
	}
	if err := recordEvent(ctx, tx, core.EventEpicSet, e.ProjectKey, e.Parent, at); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListEpics(ctx context.Context, projectKey string) ([]core.Epic, error) {
	query := `SELECT project_key, parent, number, name FROM epics`
	var args []any
	if projectKey != "" {
		query += ` WHERE project_key = ?`
		args = append(args, projectKey)
	}
	query += ` ORDER BY project_key, parent, number`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []core.Epic{}
	for rows.Next() {
		var e core.Epic
		if err := rows.Scan(&e.ProjectKey, &e.Parent, &e.Number, &e.Name); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// recordEvent appends an event inside the caller's transaction, so the event
// commits if and only if the change does.
func recordEvent(ctx context.Context, tx *sql.Tx, t core.EventType, projectKey, issueID string, at time.Time) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO events (type, project_key, issue_id, at) VALUES (?, ?, ?, ?)`,
		t, projectKey, issueID, formatTime(at))
	return err
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
	var created, updated, dependsOn string
	err := row.Scan(&i.ID, &i.ProjectKey, &i.Number, &i.Title, &i.Description, &i.Status, &i.Assignee, &created, &updated, &i.Stage, &i.Epic, &dependsOn, &i.Parent)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Issue{}, fmt.Errorf("issue %s: %w", id, core.ErrNotFound)
	}
	if err != nil {
		return core.Issue{}, err
	}
	if err := json.Unmarshal([]byte(dependsOn), &i.DependsOn); err != nil {
		return core.Issue{}, fmt.Errorf("issue %s: depends_on: %w", i.ID, err)
	}
	if i.DependsOn == nil {
		i.DependsOn = []string{}
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
