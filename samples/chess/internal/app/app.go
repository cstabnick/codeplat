// Package app is the only place that chooses an implementation from config.
// It turns a config.Config into the real core.Service.
package app

import (
	"fmt"
	"math/rand"
	"sync"
	"time"

	"chess/internal/board"
	"chess/internal/config"
	"chess/internal/core"
	"chess/internal/decider/httpdecider"
	"chess/internal/opponent/random"
	"chess/internal/opponent/router"
	"chess/internal/store/memory"
	"chess/internal/store/sqlite"
)

// New builds the service described by cfg. The returned close function releases
// the store; it is a no-op for the memory store. An unknown kind is an error.
func New(cfg config.Config) (svc *core.Service, close func() error, err error) {
	seed := cfg.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	var mover core.Mover
	switch cfg.Opponent.Kind {
	case config.OpponentRouter:
		mover = router.New(cfg.Opponent.URL, cfg.Opponent.Model, nil)
	case config.OpponentRandom:
		mover = random.New(cfg.Opponent.Seed)
	default:
		return nil, nil, fmt.Errorf("app: unknown opponent kind %q", cfg.Opponent.Kind)
	}

	var decider core.Decider
	switch cfg.Decider.Kind {
	case config.DeciderHTTP:
		decider = httpdecider.New(cfg.Decider.URL, nil)
	case config.DeciderFixed:
		decider = core.FixedDecider{}
	default:
		return nil, nil, fmt.Errorf("app: unknown decider kind %q", cfg.Decider.Kind)
	}

	var store core.Store
	closeStore := func() error { return nil }
	switch cfg.Store.Kind {
	case config.StoreMemory:
		store = memory.New()
	case config.StoreSQLite:
		s, err := sqlite.Open(cfg.Store.Path)
		if err != nil {
			return nil, nil, fmt.Errorf("app: open sqlite store: %w", err)
		}
		store = s
		closeStore = s.Close
	default:
		return nil, nil, fmt.Errorf("app: unknown store kind %q", cfg.Store.Kind)
	}

	rng := rand.New(rand.NewSource(seed))
	var mu sync.Mutex
	intn := func(n int) int {
		mu.Lock()
		defer mu.Unlock()
		return rng.Intn(n)
	}

	svc = core.NewService(store, board.New(), mover, decider, core.Options{
		Now:  time.Now,
		Intn: intn,
	})
	return svc, closeStore, nil
}
