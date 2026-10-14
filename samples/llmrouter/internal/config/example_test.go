package config

import (
	"path/filepath"
	"reflect"
	"testing"
)

// TestExamples checks that every example config loads, and that
// examples/ollama.json is the same as Default().
func TestExamples(t *testing.T) {
	clearAddrEnv(t)
	paths, err := filepath.Glob("../../examples/*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) < 2 {
		t.Fatalf("found %d example configs, want at least 2", len(paths))
	}
	for _, path := range paths {
		c, err := Load(path)
		if err != nil {
			t.Errorf("Load(%s) error: %v", path, err)
			continue
		}
		if filepath.Base(path) == "ollama.json" && !reflect.DeepEqual(c, Default()) {
			t.Errorf("Load(%s) = %+v, want Default()", path, c)
		}
	}
}
