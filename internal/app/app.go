// Package app is the composition root. It is the only package that reads
// config to decide which implementations to construct.
package app

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"codeplat/config"
	"codeplat/internal/core"
	"codeplat/internal/store/memory"
	"codeplat/internal/store/sqlite"
)

// App is the wired object graph that every entrypoint starts from.
type App struct {
	Config  config.Config
	Log     *slog.Logger
	Service *core.Service

	closers []io.Closer
}

func New(cfg config.Config) (*App, error) {
	a := &App{Config: cfg}

	// Logs go to stderr so stdout stays free for CLI output and MCP frames.
	a.Log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.Log.Level}))

	store, err := a.newStore(cfg.Store)
	if err != nil {
		return nil, err
	}

	a.Service = core.NewService(store, newClock(cfg.Clock))
	return a, nil
}

// Close releases everything New opened.
func (a *App) Close() error {
	var first error
	for _, c := range a.closers {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

func (a *App) newStore(cfg config.Store) (core.Store, error) {
	switch cfg.Driver {
	case config.DriverMemory:
		return memory.New(), nil
	case config.DriverSQLite:
		s, err := sqlite.Open(cfg.Path)
		if err != nil {
			return nil, err
		}
		a.closers = append(a.closers, s)
		return s, nil
	default:
		return nil, fmt.Errorf("app: unknown store driver %q", cfg.Driver)
	}
}

func newClock(cfg config.Clock) func() time.Time {
	if cfg.Fixed != nil {
		fixed := *cfg.Fixed
		return func() time.Time { return fixed }
	}
	return time.Now
}
