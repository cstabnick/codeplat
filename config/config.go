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
	Env    string `json:"env"`
	Log    Log    `json:"log"`
	Store  Store  `json:"store"`
	HTTP   HTTP   `json:"http"`
	Web    HTTP   `json:"web"`
	Clock  Clock  `json:"clock"`
	Auth   Auth   `json:"auth"`
	Runner Runner `json:"runner,omitempty"`
}

// Duration is a time.Duration that is read from a JSON string such as "5s".
type Duration time.Duration

// UnmarshalJSON accepts only a JSON string that time.ParseDuration accepts.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("duration must be a JSON string such as \"5s\": %w", err)
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = Duration(v)
	return nil
}

// Runner configures the worker runner. The section is optional. A zero
// value means the runner's default (see runner.Options).
type Runner struct {
	Dir          string   `json:"dir,omitempty"`           // working directory of workers; empty means the runner's own
	Command      []string `json:"command,omitempty"`       // worker command; the runner appends the message
	PollInterval Duration `json:"poll_interval,omitempty"` // e.g. "5s"
	Cooldown     Duration `json:"cooldown,omitempty"`      // e.g. "60s"
	MaxWorkers   int      `json:"max_workers,omitempty"`
}

// Auth says whether requests must present a token. Tokens are not read from
// config files.
type Auth struct {
	// Open admits every request without a token. False, the zero value, requires
	// a token, so an environment is closed unless its config says otherwise.
	Open bool `json:"open"`
	// Tokens maps each principal name to its token. FromEnvironment fills it
	// from CODEPLAT_TOKENS, a JSON object of name to token.
	Tokens map[string]string `json:"-"`
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
	if raw := os.Getenv("CODEPLAT_TOKENS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &cfg.Auth.Tokens); err != nil {
			return Config{}, fmt.Errorf("config: CODEPLAT_TOKENS: %w", err)
		}
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

// dataDir is CODEPLAT_DATA_DIR, else the data directory of the checkout the
// binary was built in (see checkoutDataDir), else $XDG_DATA_HOME/codeplat,
// else ~/.local/share/codeplat.
func dataDir() (string, error) {
	if dir := os.Getenv("CODEPLAT_DATA_DIR"); dir != "" {
		return filepath.Abs(dir)
	}
	if exe, err := os.Executable(); err == nil {
		if dir, ok := checkoutDataDir(exe); ok {
			return dir, nil
		}
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

// checkoutDataDir returns ROOT/data when exe is ROOT/bin/NAME and ROOT/data
// is a directory. A checkout opts in by having a data directory, so its
// database lives beside the code and can be committed with it; a binary
// installed anywhere else is unaffected.
func checkoutDataDir(exe string) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	bin := filepath.Dir(exe)
	if filepath.Base(bin) != "bin" {
		return "", false
	}
	dir := filepath.Join(filepath.Dir(bin), "data")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
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
	if c.Runner.MaxWorkers < 0 {
		return fmt.Errorf("runner.max_workers cannot be negative")
	}
	if c.Runner.PollInterval < 0 || c.Runner.Cooldown < 0 {
		return fmt.Errorf("runner durations cannot be negative")
	}
	return nil
}
