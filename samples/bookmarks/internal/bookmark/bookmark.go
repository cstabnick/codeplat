package bookmark

import (
	"context"
	"errors"
	"time"
)

// Errors returned by a Service. Callers test for them with errors.Is.
var (
	ErrInvalid  = errors.New("invalid bookmark")
	ErrNotFound = errors.New("bookmark not found")
)

type Bookmark struct {
	ID        int       `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

// Service stores bookmarks. The in-memory store and the HTTP client both
// implement it; the API handler and the UI handler both consume it.
type Service interface {
	// List returns all bookmarks ordered by ID ascending. It never returns nil.
	List(ctx context.Context) ([]Bookmark, error)
	// Add returns an error wrapping ErrInvalid if title or url is not acceptable.
	Add(ctx context.Context, title, url string) (Bookmark, error)
	// Delete returns an error wrapping ErrNotFound if no bookmark has that ID.
	Delete(ctx context.Context, id int) error
}
