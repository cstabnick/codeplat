// Package app is the composition root. It is the only package that reads
// config to decide which implementations to construct.
package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"codeplat/config"
	"codeplat/internal/auth"
	"codeplat/internal/core"
	"codeplat/internal/process"
	"codeplat/internal/runner"
	"codeplat/internal/store/memory"
	"codeplat/internal/store/sqlite"
)

// App is the wired object graph that every entrypoint starts from.
type App struct {
	Config  config.Config
	Log     *slog.Logger
	Service *core.Service
	Auth    core.Authenticator

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

	if cfg.Auth.Open {
		a.Auth = auth.Open{}
	} else {
		s, err := auth.NewStatic(cfg.Auth.Tokens)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("app: auth: %w", err)
		}
		if len(cfg.Auth.Tokens) == 0 {
			a.Log.Warn("auth is closed but CODEPLAT_TOKENS is empty: every REST and web request will be refused")
		}
		a.Auth = s
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

// NewRunner builds the worker runner from the runner section of the config.
// It returns an error if runner.command is empty. The only launcher is
// process.Start, bound to runner.dir.
func (a *App) NewRunner() (*runner.Runner, error) {
	if len(a.Config.Runner.Command) == 0 {
		return nil, fmt.Errorf("app: runner.command is required")
	}
	launch := func(_ context.Context, argv []string) (func() error, error) {
		return process.Start(a.Config.Runner.Dir, argv)
	}
	opts := runner.Options{
		Command:      a.Config.Runner.Command,
		PollInterval: time.Duration(a.Config.Runner.PollInterval),
		Cooldown:     time.Duration(a.Config.Runner.Cooldown),
		MaxWorkers:   a.Config.Runner.MaxWorkers,
	}
	return runner.New(a.Service, launch, opts, a.Log), nil
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
