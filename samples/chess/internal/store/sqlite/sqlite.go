// Package sqlite is a core.Store backed by a SQLite database file.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"

	"chess/internal/core"
)

// Store keeps games in a SQLite database file.
type Store struct {
	db *sql.DB
}

// Open opens (creating it if needed) the database at path and creates the
// games table if it does not exist.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlite: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS games (id INTEGER PRIMARY KEY AUTOINCREMENT, updated_ns INTEGER NOT NULL, data TEXT NOT NULL)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: open %s: create games: %w", path, err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts g and returns it with ID set from the id column.
func (s *Store) Create(ctx context.Context, g core.Game) (core.Game, error) {
	data, err := json.Marshal(g)
	if err != nil {
		return core.Game{}, fmt.Errorf("sqlite: create: encode game: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO games (updated_ns, data) VALUES (?, ?)`, g.UpdatedAt.UnixNano(), string(data))
	if err != nil {
		return core.Game{}, fmt.Errorf("sqlite: create: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return core.Game{}, fmt.Errorf("sqlite: create: %w", err)
	}
	g.ID = id
	return g, nil
}

// Get returns the game with that ID; no row wraps core.ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (core.Game, error) {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT data FROM games WHERE id = ?`, id).Scan(&data)
	if err == sql.ErrNoRows {
		return core.Game{}, fmt.Errorf("sqlite: game %d: %w", id, core.ErrNotFound)
	}
	if err != nil {
		return core.Game{}, fmt.Errorf("sqlite: get: %w", err)
	}
	return decode(id, data)
}

// List returns every game ordered by updated_ns DESC, id DESC. Never nil.
func (s *Store) List(ctx context.Context) ([]core.Game, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, data FROM games ORDER BY updated_ns DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("sqlite: list: %w", err)
	}
	defer rows.Close()
	out := []core.Game{}
	for rows.Next() {
		var id int64
		var data string
		if err := rows.Scan(&id, &data); err != nil {
			return nil, fmt.Errorf("sqlite: list: %w", err)
		}
		g, err := decode(id, data)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite: list: %w", err)
	}
	return out, nil
}

// Save replaces the row with g.ID. updated_ns is g.UpdatedAt.UnixNano().
// No affected row wraps core.ErrNotFound.
func (s *Store) Save(ctx context.Context, g core.Game) error {
	data, err := json.Marshal(g)
	if err != nil {
		return fmt.Errorf("sqlite: save: encode game: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE games SET updated_ns = ?, data = ? WHERE id = ?`, g.UpdatedAt.UnixNano(), string(data), g.ID)
	if err != nil {
		return fmt.Errorf("sqlite: save: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("sqlite: game %d: %w", g.ID, core.ErrNotFound)
	}
	return nil
}

// decode reads a stored game and sets its ID from the id column.
func decode(id int64, data string) (core.Game, error) {
	var g core.Game
	if err := json.Unmarshal([]byte(data), &g); err != nil {
		return core.Game{}, fmt.Errorf("sqlite: decode game %d: %w", id, err)
	}
	g.ID = id
	return g, nil
}

var _ core.Store = (*Store)(nil)
