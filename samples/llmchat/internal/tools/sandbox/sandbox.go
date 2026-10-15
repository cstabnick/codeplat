// Package sandbox turns paths the model gives into real paths inside a
// conversation's workspace, refusing anything that would escape it.
package sandbox

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"llmchat/internal/chat"
)

// Resolve returns the absolute, cleaned path that rel names inside
// workspace. rel may be relative (to workspace) or absolute. It returns
// an error wrapping chat.ErrInvalid when workspace is not an absolute
// path to an existing directory, or when the path, after cleaning and
// after resolving symbolic links, is not workspace itself or inside it.
// The path does not have to exist: for the part that does not exist
// yet, resolve symbolic links on the longest existing ancestor and
// append the rest. An empty rel means the workspace itself.
func Resolve(workspace, rel string) (string, error) {
	if !filepath.IsAbs(workspace) {
		return "", fmt.Errorf("%w: workspace %q is not an absolute path", chat.ErrInvalid, workspace)
	}
	ws, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		return "", fmt.Errorf("%w: workspace %q: %v", chat.ErrInvalid, workspace, err)
	}
	info, err := os.Stat(ws)
	if err != nil {
		return "", fmt.Errorf("%w: workspace %q: %v", chat.ErrInvalid, workspace, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: workspace %q is not a directory", chat.ErrInvalid, workspace)
	}
	if rel == "" {
		return ws, nil
	}

	var candidate string
	if filepath.IsAbs(rel) {
		candidate = filepath.Clean(rel)
	} else {
		candidate = filepath.Join(ws, rel)
	}

	resolved, err := realPath(candidate)
	if err != nil {
		return "", err
	}
	if !within(ws, resolved) {
		return "", fmt.Errorf("%w: path %q is outside the workspace", chat.ErrInvalid, rel)
	}
	return resolved, nil
}

// Rel returns path relative to workspace using forward slashes, "." for
// the workspace itself. path is a result of Resolve.
func Rel(workspace, path string) string {
	ws := filepath.Clean(workspace)
	if resolved, err := filepath.EvalSymlinks(workspace); err == nil {
		ws = resolved
	}
	r, err := filepath.Rel(ws, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(r)
}

// realPath resolves symbolic links on the longest existing ancestor of p
// and appends the non-existing remainder. A dangling symbolic link is
// refused, because writing through it could create a file somewhere the
// link's target points to.
func realPath(p string) (string, error) {
	cur := p
	rest := ""
	for {
		r, err := filepath.EvalSymlinks(cur)
		if err == nil {
			return filepath.Join(r, rest), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: cannot resolve path %q: %v", chat.ErrInvalid, p, err)
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", fmt.Errorf("%w: path %q goes through a dangling symbolic link", chat.ErrInvalid, p)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", fmt.Errorf("%w: cannot resolve path %q", chat.ErrInvalid, p)
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
	}
}

// within reports whether path is dir itself or lies inside it. Both
// arguments must be clean, symbolic-link-free absolute paths.
func within(dir, path string) bool {
	if path == dir {
		return true
	}
	prefix := dir
	if !strings.HasSuffix(prefix, string(filepath.Separator)) {
		prefix += string(filepath.Separator)
	}
	return strings.HasPrefix(path, prefix)
}
