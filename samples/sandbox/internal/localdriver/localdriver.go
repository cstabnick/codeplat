// Package localdriver is a core.Driver that runs commands directly on the
// host with os/exec, for tests and for machines without Docker.
//
// It isolates nothing. A command sees the host's files, network and user,
// the box's workspace is only a label, and the workdir passed to Exec is
// just the directory the command starts in. Boxes are kept in memory and
// are lost when the process exits.
package localdriver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"sync"
	"syscall"
	"time"

	"sandbox/internal/core"
)

// Driver keeps boxes in memory and runs commands on the host.
type Driver struct {
	mu    sync.Mutex
	boxes map[string]core.Box
}

var _ core.Driver = (*Driver)(nil)

// New returns a driver that keeps boxes in memory and runs Exec with os/exec.
func New() *Driver {
	return &Driver{boxes: make(map[string]core.Box)}
}

// Ensure stores a box for spec if none has that name and returns the stored box.
// A box that already exists is returned unchanged.
func (d *Driver) Ensure(ctx context.Context, spec core.BoxSpec) (core.Box, error) {
	if err := ctx.Err(); err != nil {
		return core.Box{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if box, ok := d.boxes[spec.Name]; ok {
		return box, nil
	}
	box := core.Box{
		Name:        spec.Name,
		Image:       spec.Image,
		Workspace:   spec.Workspace,
		ContainerID: "local-" + spec.Name,
		State:       "running",
		CreatedAt:   time.Now(),
	}
	d.boxes[spec.Name] = box
	return box, nil
}

// Get returns the named box, or an error wrapping core.ErrNotFound.
func (d *Driver) Get(ctx context.Context, name string) (core.Box, error) {
	if err := ctx.Err(); err != nil {
		return core.Box{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	box, ok := d.boxes[name]
	if !ok {
		return core.Box{}, fmt.Errorf("%w: box %q", core.ErrNotFound, name)
	}
	return box, nil
}

// List returns every box, sorted by name.
func (d *Driver) List(ctx context.Context) ([]core.Box, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	boxes := make([]core.Box, 0, len(d.boxes))
	for _, box := range d.boxes {
		boxes = append(boxes, box)
	}
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].Name < boxes[j].Name })
	return boxes, nil
}

// Remove deletes the named box, or returns an error wrapping core.ErrNotFound.
func (d *Driver) Remove(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, ok := d.boxes[name]; !ok {
		return fmt.Errorf("%w: box %q", core.ErrNotFound, name)
	}
	delete(d.boxes, name)
	return nil
}

// Exec runs argv with workdir as its directory on the host, writing stdout and
// stderr to out. The command runs in its own process group, and the group is
// killed when ctx ends. It returns the command's exit code, or -1 when the
// command was killed by a signal, in which case err is nil. A cancelled
// context returns an error and -1.
func (d *Driver) Exec(ctx context.Context, name string, argv []string, workdir string, out io.Writer) (int, error) {
	if _, err := d.Get(ctx, name); err != nil {
		return -1, err
	}
	if len(argv) == 0 {
		return -1, fmt.Errorf("%w: argv is empty", core.ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return -1, fmt.Errorf("command cancelled: %w", err)
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = workdir
	cmd.Stdout = out
	cmd.Stderr = out
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start %s: %w", argv[0], err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			return 0, nil
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode(), nil
		}
		return -1, fmt.Errorf("run %s: %w", argv[0], err)
	case <-ctx.Done():
		// Kill the whole group so that children do not outlive the command
		// or keep its output open.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-done
		return -1, fmt.Errorf("command cancelled: %w", ctx.Err())
	}
}
