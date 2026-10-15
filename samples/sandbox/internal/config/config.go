// Package config loads the per-environment configuration of the sandbox. It
// describes which driver to use; it never constructs one. Only internal/app
// chooses an implementation from this config.
package config

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed *.json
var files embed.FS

const (
	DriverDocker = "docker"
	DriverLocal  = "local"

	NetworkBridge = "bridge"
	NetworkNone   = "none"
)

type Config struct {
	Addr       string  `json:"addr"`
	Driver     string  `json:"driver"`
	DockerHost string  `json:"docker_host,omitempty"` // required for docker
	Root       string  `json:"root"`                  // workspaces root; "" is filled by FromEnvironment
	Image      string  `json:"image"`
	MemoryMB   int     `json:"memory_mb"`
	CPUs       float64 `json:"cpus"`
	Network    string  `json:"network"`        // "bridge" | "none"
	User       string  `json:"user,omitempty"` // "" is filled by FromEnvironment
}

// Load returns the embedded config named name, read from name.json. Unknown
// fields are an error, and the result is validated.
func Load(name string) (Config, error) {
	data, err := files.ReadFile(name + ".json")
	if err != nil {
		return Config{}, fmt.Errorf("config: unknown environment %q", name)
	}
	return parse(name+".json", data)
}

func parse(file string, data []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", file, err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", file, err)
	}
	return cfg, nil
}

// FromEnvironment loads SANDBOX_ENV (default "dev"), then applies the
// environment overrides. An empty variable changes nothing. An empty Root
// becomes $XDG_DATA_HOME/sandbox/workspaces (or the home directory fallback),
// and an empty User becomes "uid:gid". The result is validated.
func FromEnvironment() (Config, error) {
	name := os.Getenv("SANDBOX_ENV")
	if name == "" {
		name = "dev"
	}
	cfg, err := Load(name)
	if err != nil {
		return Config{}, err
	}
	override(&cfg.Addr, "SANDBOX_ADDR")
	override(&cfg.Driver, "SANDBOX_DRIVER")
	override(&cfg.DockerHost, "DOCKER_HOST")
	override(&cfg.Root, "SANDBOX_ROOT")
	override(&cfg.Image, "SANDBOX_IMAGE")
	if cfg.Root == "" {
		root, err := defaultRoot()
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		cfg.Root = root
	}
	if cfg.User == "" {
		cfg.User = fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid())
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

func override(field *string, key string) {
	if v := os.Getenv(key); v != "" {
		*field = v
	}
}

// defaultRoot is $XDG_DATA_HOME/sandbox/workspaces when XDG_DATA_HOME is an
// absolute path, else ~/.local/share/sandbox/workspaces.
func defaultRoot() (string, error) {
	if dir := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return filepath.Join(dir, "sandbox", "workspaces"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "share", "sandbox", "workspaces"), nil
}

// Validate reports the first problem with the config.
func (c Config) Validate() error {
	if c.Addr == "" {
		return fmt.Errorf("addr is required")
	}
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}
	switch c.Driver {
	case DriverDocker:
		if c.DockerHost == "" {
			return fmt.Errorf("docker_host is required for driver %q", DriverDocker)
		}
	case DriverLocal:
	default:
		return fmt.Errorf("driver must be %q or %q, got %q", DriverDocker, DriverLocal, c.Driver)
	}
	switch c.Network {
	case NetworkBridge, NetworkNone:
	default:
		return fmt.Errorf("network must be %q or %q, got %q", NetworkBridge, NetworkNone, c.Network)
	}
	if c.MemoryMB < 0 {
		return fmt.Errorf("memory_mb cannot be negative")
	}
	if c.CPUs < 0 {
		return fmt.Errorf("cpus cannot be negative")
	}
	if c.Root != "" && !filepath.IsAbs(c.Root) {
		return fmt.Errorf("root must be absolute, got %q", c.Root)
	}
	return nil
}
