// Package config describes the per-environment configuration of decide. It
// only describes: it never constructs objects. internal/app turns a Config
// into them.
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

// Config is the configuration of one environment.
type Config struct {
	Addr      string `json:"addr"`
	ModelsDir string `json:"models_dir"`
}

// Load decodes the embedded <name>.json with DisallowUnknownFields. An unknown
// name or an empty Addr is an error.
func Load(name string) (Config, error) {
	data, err := files.ReadFile(name + ".json")
	if err != nil {
		return Config{}, fmt.Errorf("config: unknown environment %q", name)
	}
	return parse(name+".json", data)
}

// FromEnvironment loads DECIDE_ENV (default "dev"), then DECIDE_ADDR and
// DECIDE_MODELS replace Addr and ModelsDir when non-empty.
func FromEnvironment() (Config, error) {
	env := os.Getenv("DECIDE_ENV")
	if env == "" {
		env = "dev"
	}
	cfg, err := Load(env)
	if err != nil {
		return Config{}, err
	}
	if addr := os.Getenv("DECIDE_ADDR"); addr != "" {
		cfg.Addr = addr
	}
	if models := os.Getenv("DECIDE_MODELS"); models != "" {
		cfg.ModelsDir = models
	}
	if cfg.Addr == "" {
		return Config{}, fmt.Errorf("config: addr is required")
	}
	return cfg, nil
}

// parse is the decoding Load uses for embedded files.
func parse(name string, data []byte) (Config, error) {
	var cfg Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("config: %s: %w", name, err)
	}
	if cfg.Addr == "" {
		return Config{}, fmt.Errorf("config: %s: addr is required", name)
	}
	return cfg, nil
}
