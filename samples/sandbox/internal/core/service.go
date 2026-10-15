package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeoutSeconds = 60
	maxTimeoutSeconds     = 600
	// killGraceSeconds is how long timeout(1) waits after SIGTERM before SIGKILL.
	killGraceSeconds = 5
	// timeoutSlackSeconds is added to the command's limit for the context
	// that wraps the driver call, so timeout(1) fires first.
	timeoutSlackSeconds = 15
	// timedOutExitCode is the status timeout(1) returns when it killed the command.
	timedOutExitCode = 124
	// keepEach is how many bytes of output are kept from the start and from the end.
	keepEach         = 15000
	truncationMarker = "\n[... output truncated ...]\n"
)

// Service applies the sandbox's rules on top of a Driver.
type Service struct {
	driver Driver
	opts   Options
}

// NewService returns a Service that uses d for every runtime operation.
func NewService(d Driver, opts Options) *Service {
	return &Service{driver: d, opts: opts}
}

// Ensure creates the named box if needed and makes sure it is running.
// An empty workspace means Root/name; an empty image means the default image.
func (s *Service) Ensure(ctx context.Context, name, workspace, image string) (Box, error) {
	if err := checkName(name); err != nil {
		return Box{}, err
	}
	if workspace == "" {
		workspace = filepath.Join(s.opts.Root, name)
	}
	if !filepath.IsAbs(workspace) {
		return Box{}, fmt.Errorf("%w: workspace %q is not an absolute path", ErrInvalid, workspace)
	}
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return Box{}, fmt.Errorf("create workspace: %w", err)
	}
	ws, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return Box{}, fmt.Errorf("resolve workspace: %w", err)
	}
	root, err := filepath.EvalSymlinks(s.opts.Root)
	if err != nil {
		return Box{}, fmt.Errorf("resolve root: %w", err)
	}
	if !strictlyInside(root, ws) {
		return Box{}, fmt.Errorf("%w: workspace %q is not inside %q", ErrInvalid, ws, root)
	}
	if image == "" {
		image = s.opts.Image
	}
	existing, err := s.driver.Get(ctx, name)
	switch {
	case err == nil:
		if existing.Workspace != ws {
			return Box{}, fmt.Errorf("%w: box %q already uses workspace %q", ErrInvalid, name, existing.Workspace)
		}
	case errors.Is(err, ErrNotFound):
	default:
		return Box{}, err
	}
	return s.driver.Ensure(ctx, s.spec(name, ws, image))
}

// Box returns the named box.
func (s *Service) Box(ctx context.Context, name string) (Box, error) {
	if err := checkName(name); err != nil {
		return Box{}, err
	}
	return s.driver.Get(ctx, name)
}

// Boxes returns every box. The result is never nil.
func (s *Service) Boxes(ctx context.Context) ([]Box, error) {
	boxes, err := s.driver.List(ctx)
	if err != nil {
		return nil, err
	}
	if boxes == nil {
		boxes = []Box{}
	}
	return boxes, nil
}

// Remove deletes the named box.
func (s *Service) Remove(ctx context.Context, name string) error {
	if err := checkName(name); err != nil {
		return err
	}
	return s.driver.Remove(ctx, name)
}

// Exec runs a bash command in the box's workspace. A stopped box is started
// first. timeoutSeconds 0 means 60 and values above 600 are capped at 600.
// Output keeps its first and last 15000 bytes when it is longer than 30000.
func (s *Service) Exec(ctx context.Context, name, command string, timeoutSeconds int) (ExecResult, error) {
	if err := checkName(name); err != nil {
		return ExecResult{}, err
	}
	if strings.TrimSpace(command) == "" {
		return ExecResult{}, fmt.Errorf("%w: command is empty", ErrInvalid)
	}
	limit, err := timeoutFor(timeoutSeconds)
	if err != nil {
		return ExecResult{}, err
	}
	box, err := s.driver.Get(ctx, name)
	if err != nil {
		return ExecResult{}, err
	}
	if box.State != "running" {
		box, err = s.driver.Ensure(ctx, s.spec(name, box.Workspace, box.Image))
		if err != nil {
			return ExecResult{}, err
		}
	}

	argv := []string{"timeout", "-k", strconv.Itoa(killGraceSeconds), strconv.Itoa(limit), "bash", "-c", command}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(limit+timeoutSlackSeconds)*time.Second)
	defer cancel()

	out := &capture{}
	code, err := s.driver.Exec(runCtx, name, argv, box.Workspace, out)
	if err != nil {
		return ExecResult{}, err
	}
	return ExecResult{
		Output:   out.String(),
		ExitCode: code,
		TimedOut: code == timedOutExitCode,
	}, nil
}

func (s *Service) spec(name, workspace, image string) BoxSpec {
	return BoxSpec{
		Name:      name,
		Image:     image,
		Workspace: workspace,
		MemoryMB:  s.opts.MemoryMB,
		CPUs:      s.opts.CPUs,
		Network:   s.opts.Network,
		User:      s.opts.User,
	}
}

func checkName(name string) error {
	if !NamePattern.MatchString(name) {
		return fmt.Errorf("%w: box name %q must match %s", ErrInvalid, name, NamePattern)
	}
	return nil
}

func timeoutFor(seconds int) (int, error) {
	switch {
	case seconds < 0:
		return 0, fmt.Errorf("%w: timeout_seconds must not be negative", ErrInvalid)
	case seconds == 0:
		return defaultTimeoutSeconds, nil
	case seconds > maxTimeoutSeconds:
		return maxTimeoutSeconds, nil
	}
	return seconds, nil
}

// strictlyInside reports whether path is a proper descendant of root.
// Both arguments must already have their symlinks resolved.
func strictlyInside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// capture keeps the first and the last keepEach bytes written to it and
// counts the rest, so a command's output uses bounded memory.
type capture struct {
	mu    sync.Mutex
	total int
	head  []byte
	tail  []byte
}

func (c *capture) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	written := len(p)
	c.total += written
	if room := keepEach - len(c.head); room > 0 {
		n := min(room, len(p))
		c.head = append(c.head, p[:n]...)
		p = p[n:]
	}
	if len(p) > 0 {
		c.tail = append(c.tail, p...)
		if len(c.tail) > 2*keepEach {
			c.tail = append([]byte(nil), c.tail[len(c.tail)-keepEach:]...)
		}
	}
	return written, nil
}

// String returns the captured output, with the middle removed when it was
// longer than the two kept parts.
func (c *capture) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.total <= 2*keepEach {
		return string(c.head) + string(c.tail)
	}
	tail := c.tail
	if len(tail) > keepEach {
		tail = tail[len(tail)-keepEach:]
	}
	return string(c.head) + truncationMarker + string(tail)
}

var _ io.Writer = (*capture)(nil)
