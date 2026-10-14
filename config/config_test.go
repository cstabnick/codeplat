package config

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunnerSection(t *testing.T) {
	base := `"env":"x","store":{"driver":"memory"},"http":{"addr":":1"},"web":{"addr":":2"}`
	valid := `{` + base + `,"runner":{"command":["claude"],"poll_interval":"5s","cooldown":"60s","max_workers":2}}`
	cfg, err := parse("valid runner", []byte(valid))
	if err != nil {
		t.Fatalf("valid runner section rejected: %v", err)
	}
	if got := time.Duration(cfg.Runner.PollInterval); got != 5*time.Second {
		t.Errorf("PollInterval = %v, want 5s", got)
	}

	for name, body := range map[string]string{
		"numeric poll_interval":  `{` + base + `,"runner":{"poll_interval":5}}`,
		"bad poll_interval":      `{` + base + `,"runner":{"poll_interval":"soon"}}`,
		"numeric cooldown":       `{` + base + `,"runner":{"cooldown":60}}`,
		"negative max_workers":   `{` + base + `,"runner":{"max_workers":-1}}`,
		"negative poll_interval": `{` + base + `,"runner":{"poll_interval":"-5s"}}`,
	} {
		if _, err := parse(name, []byte(body)); err == nil {
			t.Errorf("%s: want error", name)
		}
	}

	noRunner := `{` + base + `}`
	if _, err := parse("no runner", []byte(noRunner)); err != nil {
		t.Errorf("config without runner section rejected: %v", err)
	}
}

func TestDevRunnerConfig(t *testing.T) {
	cfg, err := Load("dev")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"claude", "--agent", "haiku-worker", "-p"}
	if strings.Join(cfg.Runner.Command, " ") != strings.Join(want, " ") {
		t.Errorf("Runner.Command = %q, want %q", cfg.Runner.Command, want)
	}
	if got := time.Duration(cfg.Runner.PollInterval); got != 5*time.Second {
		t.Errorf("Runner.PollInterval = %v, want 5s", got)
	}
}

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

func TestFromEnvironmentReadsTokens(t *testing.T) {
	t.Setenv("CODEPLAT_CONFIG", "")
	t.Setenv("CODEPLAT_DATA_DIR", t.TempDir())

	t.Setenv("CODEPLAT_ENV", "prod")
	t.Setenv("CODEPLAT_TOKENS", `{"alice":"a-token"}`)
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Auth.Open {
		t.Error("prod: Auth.Open = true, want false")
	}
	if got := cfg.Auth.Tokens["alice"]; got != "a-token" {
		t.Errorf("prod: Auth.Tokens[alice] = %q, want %q", got, "a-token")
	}

	t.Setenv("CODEPLAT_ENV", "dev")
	if cfg, err = FromEnvironment(); err != nil {
		t.Fatal(err)
	}
	if !cfg.Auth.Open {
		t.Error("dev: Auth.Open = false, want true")
	}

	t.Setenv("CODEPLAT_TOKENS", "not json")
	if _, err := FromEnvironment(); err == nil {
		t.Error("invalid CODEPLAT_TOKENS: want error")
	}
}
