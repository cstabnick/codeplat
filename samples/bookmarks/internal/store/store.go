package store

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"bookmarks/internal/bookmark"
)

// Store is an in-memory bookmark.Service. It is safe for concurrent use.
type Store struct {
	mu     sync.Mutex
	nextID int
	items  []bookmark.Bookmark
}

var _ bookmark.Service = (*Store)(nil)

// New returns an empty Store. The first bookmark added gets ID 1.
func New() *Store {
	return &Store{nextID: 1}
}

// Add validates and stores a new bookmark.
func (s *Store) Add(ctx context.Context, title, url_ string) (bookmark.Bookmark, error) {
	title = strings.TrimSpace(title)
	url_ = strings.TrimSpace(url_)

	if title == "" {
		return bookmark.Bookmark{}, fmt.Errorf("%w: title is required", bookmark.ErrInvalid)
	}
	if utf8.RuneCountInString(title) > 200 {
		return bookmark.Bookmark{}, fmt.Errorf("%w: title must be at most 200 characters", bookmark.ErrInvalid)
	}
	u, err := url.Parse(url_)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return bookmark.Bookmark{}, fmt.Errorf("%w: url must be an http or https URL", bookmark.ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.nextID == 0 {
		s.nextID = 1
	}
	b := bookmark.Bookmark{
		ID:        s.nextID,
		Title:     title,
		URL:       url_,
		CreatedAt: time.Now().UTC(),
	}
	s.nextID++
	s.items = append(s.items, b)
	return b, nil
}

// List returns all bookmarks ordered by ID ascending. It never returns nil.
func (s *Store) List(ctx context.Context) ([]bookmark.Bookmark, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]bookmark.Bookmark, len(s.items))
	copy(out, s.items)
	return out, nil
}

// Delete removes the bookmark with the given ID. Its ID is not reused.
func (s *Store) Delete(ctx context.Context, id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, b := range s.items {
		if b.ID == id {
			s.items = append(s.items[:i], s.items[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("bookmark %d: %w", id, bookmark.ErrNotFound)
}
