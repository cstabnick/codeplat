package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryEmbeddedConfigLoads(t *testing.T) {
	entries, err := files.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		env := strings.TrimSuffix(e.Name(), ".json")
		cfg, err := Load(env)
		if err != nil {
			t.Errorf("Load(%q): %v", env, err)
			continue
		}
		if cfg.Env != env {
			t.Errorf("%s declares env %q", e.Name(), cfg.Env)
		}
	}
}

func TestParseRejectsBadConfig(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":    `{"env":"x","store":{"driver":"memory"},"http":{"addr":":1"},"web":{"addr":":2"},"extra":1}`,
		"unknown driver":   `{"env":"x","store":{"driver":"redis"},"http":{"addr":":1"},"web":{"addr":":2"}}`,
		"sqlite no path":   `{"env":"x","store":{"driver":"sqlite"},"http":{"addr":":1"},"web":{"addr":":2"}}`,
		"missing addr":     `{"env":"x","store":{"driver":"memory"},"web":{"addr":":2"}}`,
		"missing web addr": `{"env":"x","store":{"driver":"memory"},"http":{"addr":":1"}}`,
	} {
		if _, err := parse(name, []byte(body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
	valid := `{"env":"x","store":{"driver":"memory"},"http":{"addr":":1"},"web":{"addr":":2"}}`
	if _, err := parse("valid", []byte(valid)); err != nil {
		t.Errorf("valid config rejected: %v", err)
	}
	if _, err := Load("nope"); err == nil {
		t.Error("Load(nope): want error")
	}
}

func TestFromEnvironmentResolvesStorePath(t *testing.T) {
	data := t.TempDir()
	t.Setenv("CODEPLAT_CONFIG", "")
	t.Setenv("CODEPLAT_ENV", "dev")
	t.Setenv("CODEPLAT_DATA_DIR", data)
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(data, "codeplat.dev.db"); cfg.Store.Path != want {
		t.Errorf("explicit data dir: path = %q, want %q", cfg.Store.Path, want)
	}

	xdg := t.TempDir()
	t.Setenv("CODEPLAT_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", xdg)
	if cfg, err = FromEnvironment(); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "codeplat", "codeplat.dev.db"); cfg.Store.Path != want {
		t.Errorf("XDG data dir: path = %q, want %q", cfg.Store.Path, want)
	}

	t.Setenv("CODEPLAT_ENV", "test")
	if cfg, err = FromEnvironment(); err != nil || cfg.Store.Path != "" {
		t.Errorf("memory store: path = %q, err = %v; want empty", cfg.Store.Path, err)
	}
}
