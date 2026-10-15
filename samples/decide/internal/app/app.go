// Package app is the only package that turns a config into concrete objects.
// Everything else receives its dependencies as constructor arguments.
package app

import (
	"fmt"
	"os"

	"decide/internal/config"
	"decide/internal/core"
	"decide/internal/dirsource"
	"decide/internal/engine"
	"decide/internal/zen"
)

// New returns an error if cfg.ModelsDir is empty or is not a directory.
// It builds core.NewService(dirsource.New(cfg.ModelsDir), engine.New(zen.Lang{})).
func New(cfg config.Config) (*core.Service, error) {
	if cfg.ModelsDir == "" {
		return nil, fmt.Errorf("app: models_dir is required")
	}
	info, err := os.Stat(cfg.ModelsDir)
	if err != nil {
		return nil, fmt.Errorf("app: models dir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("app: models dir %q is not a directory", cfg.ModelsDir)
	}
	return core.NewService(dirsource.New(cfg.ModelsDir), engine.New(zen.Lang{})), nil
}
