// Package dirsource loads decision models from a directory of <key>.json files.
package dirsource

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"decide/internal/core"
	"decide/internal/jdm"
)

// Source loads models from one directory.
type Source struct {
	dir string
}

var _ core.ModelSource = (*Source)(nil)

// New returns a Source that reads models from dir.
func New(dir string) *Source {
	return &Source{dir: dir}
}

// Get reads <dir>/<key>.json on every call, so edits apply without a restart.
// A key not matching core.KeyPattern or a missing file returns an error wrapping
// core.ErrNotFound. A file that does not parse returns an error wrapping core.ErrInvalid.
func (s *Source) Get(ctx context.Context, key string) (*jdm.Graph, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !core.KeyPattern.MatchString(key) {
		return nil, fmt.Errorf("%w: model %q", core.ErrNotFound, key)
	}
	data, err := os.ReadFile(filepath.Join(s.dir, key+".json"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: model %q", core.ErrNotFound, key)
		}
		return nil, fmt.Errorf("read model %q: %w", key, err)
	}
	g, err := jdm.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%w: model %q: %v", core.ErrInvalid, key, err)
	}
	return g, nil
}

// List returns the keys of the *.json files directly in the directory, sorted.
// Names whose key does not match core.KeyPattern are skipped.
func (s *Source) List(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	keys := []string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		key := strings.TrimSuffix(name, ".json")
		if !core.KeyPattern.MatchString(key) {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}
