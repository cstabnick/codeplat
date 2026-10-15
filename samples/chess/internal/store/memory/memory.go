// Package memory holds games in process memory. It is for tests and for runs that need no file.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"chess/internal/core"
)

// Store is an in-memory core.Store. It is safe for concurrent use.
type Store struct {
	mu    sync.Mutex
	next  int64
	games map[int64]core.Game
}

// New returns an empty Store.
func New() *Store {
	return &Store{games: map[int64]core.Game{}}
}

var _ core.Store = (*Store)(nil)

// Create stores a copy of g under the next ID (1, 2, 3, ...) and returns the stored copy.
func (s *Store) Create(ctx context.Context, g core.Game) (core.Game, error) {
	if err := ctx.Err(); err != nil {
		return core.Game{}, err
	}
	cp, err := clone(g)
	if err != nil {
		return core.Game{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	cp.ID = s.next
	s.games[cp.ID] = cp
	out, err := clone(cp)
	if err != nil {
		return core.Game{}, err
	}
	return out, nil
}

// Get returns a copy of the game with the given ID, or an error wrapping core.ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (core.Game, error) {
	if err := ctx.Err(); err != nil {
		return core.Game{}, err
	}
	s.mu.Lock()
	g, ok := s.games[id]
	s.mu.Unlock()
	if !ok {
		return core.Game{}, fmt.Errorf("game %d: %w", id, core.ErrNotFound)
	}
	return clone(g)
}

// List returns copies of all games, UpdatedAt newest first, then ID descending.
func (s *Store) List(ctx context.Context) ([]core.Game, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	all := make([]core.Game, 0, len(s.games))
	for _, g := range s.games {
		all = append(all, g)
	}
	s.mu.Unlock()
	sort.Slice(all, func(i, j int) bool {
		if !all[i].UpdatedAt.Equal(all[j].UpdatedAt) {
			return all[i].UpdatedAt.After(all[j].UpdatedAt)
		}
		return all[i].ID > all[j].ID
	})
	out := make([]core.Game, 0, len(all))
	for _, g := range all {
		cp, err := clone(g)
		if err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	return out, nil
}

// Save replaces the stored game with a copy of g, matched by ID. A missing ID wraps core.ErrNotFound.
func (s *Store) Save(ctx context.Context, g core.Game) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	cp, err := clone(g)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.games[cp.ID]; !ok {
		return fmt.Errorf("game %d: %w", cp.ID, core.ErrNotFound)
	}
	s.games[cp.ID] = cp
	return nil
}

// clone copies a game through JSON so that callers never share slices or Turn pointers with the store.
func clone(g core.Game) (core.Game, error) {
	data, err := json.Marshal(g)
	if err != nil {
		return core.Game{}, fmt.Errorf("copy game: %w", err)
	}
	var out core.Game
	if err := json.Unmarshal(data, &out); err != nil {
		return core.Game{}, fmt.Errorf("copy game: %w", err)
	}
	return out, nil
}
