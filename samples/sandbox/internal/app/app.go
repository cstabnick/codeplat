// Package app is the only place that chooses a driver implementation from
// config. It turns a config into a ready core.Service.
package app

import (
	"fmt"
	"os"
	"path/filepath"

	"sandbox/internal/config"
	"sandbox/internal/core"
	"sandbox/internal/docker"
	"sandbox/internal/localdriver"
)

// New builds the sandbox service for cfg. cfg.Root must be a non-empty
// absolute path; it is created if missing. The driver is chosen from
// cfg.Driver: "docker" uses cfg.DockerHost, "local" is in-memory with os/exec.
func New(cfg config.Config) (*core.Service, error) {
	if cfg.Root == "" {
		return nil, fmt.Errorf("app: root is required")
	}
	if !filepath.IsAbs(cfg.Root) {
		return nil, fmt.Errorf("app: root must be absolute, got %q", cfg.Root)
	}
	if err := os.MkdirAll(cfg.Root, 0o755); err != nil {
		return nil, fmt.Errorf("app: create root: %w", err)
	}

	var driver core.Driver
	switch cfg.Driver {
	case config.DriverDocker:
		d, err := docker.New(cfg.DockerHost, nil)
		if err != nil {
			return nil, fmt.Errorf("app: docker driver: %w", err)
		}
		driver = d
	case config.DriverLocal:
		driver = localdriver.New()
	default:
		return nil, fmt.Errorf("app: unknown driver %q", cfg.Driver)
	}

	return core.NewService(driver, core.Options{
		Root:     cfg.Root,
		Image:    cfg.Image,
		MemoryMB: cfg.MemoryMB,
		CPUs:     cfg.CPUs,
		Network:  cfg.Network,
		User:     cfg.User,
	}), nil
}
