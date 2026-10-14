// Package config loads the per-environment configuration that app.New wires from.
package config

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

//go:embed *.json
var files embed.FS

const (
	DriverMemory = "memory"
	DriverSQLite = "sqlite"
)

type Config struct {
	Env   string `json:"env"`
	Log   Log    `json:"log"`
	Store Store  `json:"store"`
	HTTP  HTTP   `json:"http"`
	Web   HTTP   `json:"web"`
	Clock Clock  `json:"clock"`
}

type Log struct {
	Level slog.Level `json:"level"`
}

type Store struct {
	Driver string `json:"driver"`
	Path   string `json:"path,omitempty"`
}

type HTTP struct {
	Addr string `json:"addr"`
}

// Clock pins time when Fixed is set; otherwise the system clock is used.
type Clock struct {
	Fixed *time.Time `json:"fixed,omitempty"`
}

// FromEnvironment loads the file named by CODEPLAT_CONFIG if set, otherwise
// the embedded config for CODEPLAT_ENV (default "dev"). A relative store
// path is resolved against the data directory, so every entrypoint opens
// the same database whatever its working directory.
func FromEnvironment() (Config, error) {
	cfg, err := fromEnvironment()
	if err != nil {
		return Config{}, err
	}
	if cfg.Store.Path != "" && !filepath.IsAbs(cfg.Store.Path) {
		dir, err := dataDir()
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		cfg.Store.Path = filepath.Join(dir, cfg.Store.Path)
	}
	return cfg, nil
}

func fromEnvironment() (Config, error) {
	if path := os.Getenv("CODEPLAT_CONFIG"); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		return parse(path, data)
	}
	env := os.Getenv("CODEPLAT_ENV")
	if env == "" {
		env = "dev"
	}
	return Load(env)
}

// dataDir is CODEPLAT_DATA_DIR, else $XDG_DATA_HOME/codeplat, else
// ~/.local/share/codeplat.
func dataDir() (string, error) {
	if dir := os.Getenv("CODEPLAT_DATA_DIR"); dir != "" {
		return filepath.Abs(dir)
	}
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "codeplat"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "codeplat"), nil
}

// Load returns the embedded config for the named environment.
func Load(env string) (Config, error) {
	data, err := files.ReadFile(env + ".json")
	if err != nil {
		return Config{}, fmt.Errorf("config: unknown environment %q", env)
	}
	return parse(env+".json", data)
}

func parse(name string, data []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", name, err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", name, err)
	}
	return cfg, nil
}

func (c Config) validate() error {
	if c.Env == "" {
		return fmt.Errorf("env is required")
	}
	switch c.Store.Driver {
	case DriverMemory:
	case DriverSQLite:
		if c.Store.Path == "" {
			return fmt.Errorf("store.path is required for sqlite")
		}
	default:
		return fmt.Errorf("store.driver must be %q or %q, got %q", DriverMemory, DriverSQLite, c.Store.Driver)
	}
	if c.HTTP.Addr == "" {
		return fmt.Errorf("http.addr is required")
	}
	if c.Web.Addr == "" {
		return fmt.Errorf("web.addr is required")
	}
	return nil
}
