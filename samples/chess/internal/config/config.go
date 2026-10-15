// Package config loads the per-environment configuration. It says which
// implementation to use; it never constructs one. Only internal/app does that.
package config

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
)

//go:embed *.json
var files embed.FS

const (
	StoreMemory = "memory"
	StoreSQLite = "sqlite"

	OpponentRouter = "router"
	OpponentRandom = "random"

	DeciderHTTP  = "http"
	DeciderFixed = "fixed"
)

// Config is one environment's configuration.
type Config struct {
	Addr     string   `json:"addr"`
	Seed     int64    `json:"seed"` // the service's random picks; 0 = time-based
	Store    Store    `json:"store"`
	Opponent Opponent `json:"opponent"`
	Decider  Decider  `json:"decider"`
}

// Store picks the game store.
type Store struct {
	Kind string `json:"kind"`           // "memory" | "sqlite"
	Path string `json:"path,omitempty"` // required for sqlite
}

// Opponent picks the move source for the computer side.
type Opponent struct {
	Kind  string `json:"kind"`            // "router" | "random"
	URL   string `json:"url,omitempty"`   // required for router
	Model string `json:"model,omitempty"` // router model when the policy names no route; "" = "auto"
	Seed  int64  `json:"seed,omitempty"`  // random
}

// Decider picks how the service decides a game's outcome.
type Decider struct {
	Kind string `json:"kind"`          // "http" | "fixed"
	URL  string `json:"url,omitempty"` // required for http
}

// Load returns the embedded configuration named name (dev, test, ...).
func Load(name string) (Config, error) {
	data, err := files.ReadFile(name + ".json")
	if err != nil {
		return Config{}, fmt.Errorf("config: unknown environment %q", name)
	}
	return parse(name+".json", data)
}

// parse decodes data strictly, rejecting unknown fields, then validates it.
func parse(name string, data []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", name, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", name, err)
	}
	return cfg, nil
}

// FromEnvironment loads CHESS_ENV (default "dev"), then lets CHESS_ADDR,
// CHESS_DB, CHESS_ROUTER_URL and CHESS_DECIDE_URL replace their field when
// set and non-empty. The result is validated.
func FromEnvironment() (Config, error) {
	env := os.Getenv("CHESS_ENV")
	if env == "" {
		env = "dev"
	}
	cfg, err := Load(env)
	if err != nil {
		return Config{}, err
	}
	if v := os.Getenv("CHESS_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("CHESS_DB"); v != "" {
		cfg.Store.Path = v
	}
	if v := os.Getenv("CHESS_ROUTER_URL"); v != "" {
		cfg.Opponent.URL = v
	}
	if v := os.Getenv("CHESS_DECIDE_URL"); v != "" {
		cfg.Decider.URL = v
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: environment: %w", err)
	}
	return cfg, nil
}

// Validate reports the first problem with the configuration.
func (c Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("addr is required")
	}
	switch c.Store.Kind {
	case StoreMemory:
	case StoreSQLite:
		if c.Store.Path == "" {
			return fmt.Errorf("store.path is required for sqlite")
		}
	default:
		return fmt.Errorf("store.kind must be %q or %q, got %q", StoreMemory, StoreSQLite, c.Store.Kind)
	}
	switch c.Opponent.Kind {
	case OpponentRouter:
		if c.Opponent.URL == "" {
			return fmt.Errorf("opponent.url is required for router")
		}
	case OpponentRandom:
	default:
		return fmt.Errorf("opponent.kind must be %q or %q, got %q", OpponentRouter, OpponentRandom, c.Opponent.Kind)
	}
	switch c.Decider.Kind {
	case DeciderHTTP:
		if c.Decider.URL == "" {
			return fmt.Errorf("decider.url is required for http")
		}
	case DeciderFixed:
	default:
		return fmt.Errorf("decider.kind must be %q or %q, got %q", DeciderHTTP, DeciderFixed, c.Decider.Kind)
	}
	return nil
}
