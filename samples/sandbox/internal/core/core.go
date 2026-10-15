// Package core holds the sandbox's box types, the Driver port that a
// container runtime implements, and the Service, which owns every rule about
// names, workspaces, timeouts and output size. Dependencies are passed to
// NewService; nothing here is global.
package core

import (
	"context"
	"errors"
	"io"
	"regexp"
	"time"
)

var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid")
	ErrUnavailable = errors.New("driver unavailable") // drivers wrap connection failures in it
)

// NamePattern is the rule every box name must follow.
var NamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// BoxSpec describes the box a driver should create or start.
type BoxSpec struct {
	Name      string
	Image     string
	Workspace string  // absolute host path, mounted at the same path in the box
	MemoryMB  int     // 0 = no limit
	CPUs      float64 // 0 = no limit
	Network   string  // "bridge" or "none"
	User      string  // "uid:gid", "" = image default
}

// Box is a sandbox box as the driver reports it.
type Box struct {
	Name        string    `json:"name"`
	Image       string    `json:"image"`
	Workspace   string    `json:"workspace"`
	ContainerID string    `json:"container_id"`
	State       string    `json:"state"` // "running", "exited", "created", ...
	CreatedAt   time.Time `json:"created_at"`
}

// Driver is the port a container runtime implements.
type Driver interface {
	// Ensure creates the box if missing and starts it if not running; it returns the box as it now is.
	Ensure(ctx context.Context, spec BoxSpec) (Box, error)
	// Get returns ErrNotFound when the box does not exist.
	Get(ctx context.Context, name string) (Box, error)
	List(ctx context.Context) ([]Box, error)
	// Remove returns ErrNotFound when the box does not exist.
	Remove(ctx context.Context, name string) error
	// Exec runs argv in the box with workdir as its directory, writing stdout and stderr to out.
	Exec(ctx context.Context, name string, argv []string, workdir string, out io.Writer) (exitCode int, err error)
}

// Options are the service's defaults and limits.
type Options struct {
	Root     string // absolute; workspaces must be inside it
	Image    string // default image
	MemoryMB int
	CPUs     float64
	Network  string
	User     string
}

// ExecResult is the outcome of one command run in a box.
type ExecResult struct {
	Output   string `json:"output"`
	ExitCode int    `json:"exit_code"`
	TimedOut bool   `json:"timed_out"`
}
