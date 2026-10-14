// Package sqlitestore is a chat.Store backed by a SQLite database file.
package sqlitestore

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"llmchat/internal/chat"
)

// migrations run in order, once each; PRAGMA user_version counts how many
// have been applied. Append new entries; never edit or reorder old ones.
var migrations = []string{
	`CREATE TABLE conversations (
		id         TEXT PRIMARY KEY,
		title      TEXT NOT NULL,
		model      TEXT NOT NULL,
		workspace  TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE messages (
		conversation_id TEXT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
		seq             INTEGER NOT NULL,
		role            TEXT NOT NULL,
		content         TEXT NOT NULL,
		reasoning       TEXT NOT NULL,
		tool_calls      TEXT NOT NULL,
		tool_call_id    TEXT NOT NULL,
		route           TEXT NOT NULL,
		backend         TEXT NOT NULL,
		model           TEXT NOT NULL,
		created_at      TEXT NOT NULL,
		PRIMARY KEY (conversation_id, seq)
	);`,
}

// Store keeps conversations in a SQLite database file.
type Store struct {
	db  *sql.DB
	now func() time.Time
}

// Open opens (creating it if needed) the database at path and applies
// the migrations. now returns the current time; nil means time.Now.
func Open(path string, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("sqlitestore: %w", err)
	}
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlitestore: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlitestore: open %s: %w", path, err)
	}
	return &Store{db: db, now: now}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("sqlitestore: read schema version: %w", err)
	}
	if version > len(migrations) {
		return fmt.Errorf("sqlitestore: schema version %d is newer than this binary supports (%d)", version, len(migrations))
	}
	for i := version; i < len(migrations); i++ {
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("sqlitestore: migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlitestore: migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			tx.Rollback()
			return fmt.Errorf("sqlitestore: migration %d: %w", i+1, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("sqlitestore: migration %d: %w", i+1, err)
		}
	}
	return nil
}

// timeLayout is fixed width (always nine fractional digits), so the stored
// text sorts in time order. time.RFC3339Nano trims trailing zeros and does not.
const timeLayout = "2006-01-02T15:04:05.000000000Z"

func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(timeLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("sqlitestore: parse time %q: %w", s, err)
	}
	return t, nil
}

func newID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("sqlitestore: new id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// CreateConversation stores c with a new ID, CreatedAt and UpdatedAt set, and returns it.
func (s *Store) CreateConversation(ctx context.Context, c chat.Conversation) (chat.Conversation, error) {
	id, err := newID()
	if err != nil {
		return chat.Conversation{}, err
	}
	if c.Model == "" {
		c.Model = "auto"
	}
	c.ID = id
	c.CreatedAt = s.now()
	c.UpdatedAt = c.CreatedAt
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO conversations (id, title, model, workspace, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Title, c.Model, c.Workspace, formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: create conversation: %w", err)
	}
	return c, nil
}

// GetConversation returns chat.ErrNotFound for an unknown id.
func (s *Store) GetConversation(ctx context.Context, id string) (chat.Conversation, error) {
	return getConversation(ctx, s.db, id)
}

// rowQueryer is satisfied by *sql.DB and *sql.Tx.
type rowQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func getConversation(ctx context.Context, q rowQueryer, id string) (chat.Conversation, error) {
	var c chat.Conversation
	var created, updated string
	err := q.QueryRowContext(ctx,
		`SELECT id, title, model, workspace, created_at, updated_at FROM conversations WHERE id = ?`, id).
		Scan(&c.ID, &c.Title, &c.Model, &c.Workspace, &created, &updated)
	if err == sql.ErrNoRows {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: conversation %s: %w", id, chat.ErrNotFound)
	}
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: get conversation: %w", err)
	}
	if c.CreatedAt, err = parseTime(created); err != nil {
		return chat.Conversation{}, err
	}
	if c.UpdatedAt, err = parseTime(updated); err != nil {
		return chat.Conversation{}, err
	}
	return c, nil
}

// ListConversations returns every conversation, most recently updated first. Never nil.
func (s *Store) ListConversations(ctx context.Context) ([]chat.Conversation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, title, model, workspace, created_at, updated_at FROM conversations ORDER BY updated_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("sqlitestore: list conversations: %w", err)
	}
	defer rows.Close()
	out := []chat.Conversation{}
	for rows.Next() {
		var c chat.Conversation
		var created, updated string
		if err := rows.Scan(&c.ID, &c.Title, &c.Model, &c.Workspace, &created, &updated); err != nil {
			return nil, fmt.Errorf("sqlitestore: list conversations: %w", err)
		}
		if c.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		if c.UpdatedAt, err = parseTime(updated); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlitestore: list conversations: %w", err)
	}
	return out, nil
}

// UpdateConversation applies the non-nil fields of p and sets UpdatedAt.
func (s *Store) UpdateConversation(ctx context.Context, id string, p chat.ConversationPatch) (chat.Conversation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: update conversation: %w", err)
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`UPDATE conversations SET title = COALESCE(?, title), model = COALESCE(?, model),
			workspace = COALESCE(?, workspace), updated_at = ? WHERE id = ?`,
		nullable(p.Title), nullable(p.Model), nullable(p.Workspace), formatTime(s.now()), id)
	if err != nil {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: update conversation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: conversation %s: %w", id, chat.ErrNotFound)
	}
	c, err := getConversation(ctx, tx, id)
	if err != nil {
		return chat.Conversation{}, err
	}
	if err := tx.Commit(); err != nil {
		return chat.Conversation{}, fmt.Errorf("sqlitestore: update conversation: %w", err)
	}
	return c, nil
}

func nullable(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

// DeleteConversation removes the conversation and its messages.
func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("sqlitestore: delete conversation: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("sqlitestore: conversation %s: %w", id, chat.ErrNotFound)
	}
	return nil
}

// AppendMessage stores m as the conversation's next message (Seq is the
// previous Seq plus 1; any Seq in m is ignored), sets CreatedAt, bumps
// the conversation's UpdatedAt, and returns the stored message.
func (s *Store) AppendMessage(ctx context.Context, conversationID string, m chat.Message) (chat.Message, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE id = ?`, conversationID).Scan(&exists)
	if err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}
	if exists == 0 {
		return chat.Message{}, fmt.Errorf("sqlitestore: conversation %s: %w", conversationID, chat.ErrNotFound)
	}

	var seq int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) + 1 FROM messages WHERE conversation_id = ?`, conversationID).Scan(&seq); err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}

	toolCalls := m.ToolCalls
	if toolCalls == nil {
		toolCalls = []chat.ToolCall{}
	}
	tcJSON, err := json.Marshal(toolCalls)
	if err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: encode tool calls: %w", err)
	}

	m.Seq = seq
	m.CreatedAt = s.now()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO messages (conversation_id, seq, role, content, reasoning, tool_calls, tool_call_id, route, backend, model, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		conversationID, m.Seq, m.Role, m.Content, m.Reasoning, string(tcJSON), m.ToolCallID,
		m.Route, m.Backend, m.Model, formatTime(m.CreatedAt))
	if err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE conversations SET updated_at = ? WHERE id = ?`, formatTime(m.CreatedAt), conversationID); err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return chat.Message{}, fmt.Errorf("sqlitestore: append message: %w", err)
	}
	m.ToolCalls = toolCalls
	if len(m.ToolCalls) == 0 {
		m.ToolCalls = nil
	}
	return m, nil
}

// ListMessages returns the conversation's messages oldest first. Never nil.
func (s *Store) ListMessages(ctx context.Context, conversationID string) ([]chat.Message, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE id = ?`, conversationID).Scan(&exists); err != nil {
		return nil, fmt.Errorf("sqlitestore: list messages: %w", err)
	}
	if exists == 0 {
		return nil, fmt.Errorf("sqlitestore: conversation %s: %w", conversationID, chat.ErrNotFound)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, role, content, reasoning, tool_calls, tool_call_id, route, backend, model, created_at
		 FROM messages WHERE conversation_id = ? ORDER BY seq`, conversationID)
	if err != nil {
		return nil, fmt.Errorf("sqlitestore: list messages: %w", err)
	}
	defer rows.Close()
	out := []chat.Message{}
	for rows.Next() {
		var m chat.Message
		var toolCalls, created string
		if err := rows.Scan(&m.Seq, &m.Role, &m.Content, &m.Reasoning, &toolCalls, &m.ToolCallID,
			&m.Route, &m.Backend, &m.Model, &created); err != nil {
			return nil, fmt.Errorf("sqlitestore: list messages: %w", err)
		}
		if err := json.Unmarshal([]byte(toolCalls), &m.ToolCalls); err != nil {
			return nil, fmt.Errorf("sqlitestore: list messages: decode tool calls: %w", err)
		}
		if len(m.ToolCalls) == 0 {
			m.ToolCalls = nil
		}
		if m.CreatedAt, err = parseTime(created); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlitestore: list messages: %w", err)
	}
	return out, nil
}

var _ chat.Store = (*Store)(nil)
