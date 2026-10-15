package config

import (
	"strings"
	"testing"
)

func TestLoadEmbedded(t *testing.T) {
	dev, err := Load("dev")
	if err != nil {
		t.Fatalf("Load(dev): %v", err)
	}
	if dev.Addr != "127.0.0.1:8086" || dev.Store.Kind != StoreSQLite || dev.Store.Path != "chess.db" {
		t.Errorf("dev store/addr = %+v / %q", dev.Store, dev.Addr)
	}
	if dev.Opponent.Kind != OpponentRouter || dev.Opponent.URL != "http://127.0.0.1:8083" || dev.Opponent.Model != "reason" {
		t.Errorf("dev opponent = %+v", dev.Opponent)
	}
	if dev.Decider.Kind != DeciderHTTP || dev.Decider.URL != "http://127.0.0.1:8085" {
		t.Errorf("dev decider = %+v", dev.Decider)
	}

	test, err := Load("test")
	if err != nil {
		t.Fatalf("Load(test): %v", err)
	}
	if test.Addr != "127.0.0.1:0" || test.Seed != 1 || test.Store.Kind != StoreMemory {
		t.Errorf("test = %+v", test)
	}
	if test.Opponent.Kind != OpponentRandom || test.Opponent.Seed != 1 {
		t.Errorf("test opponent = %+v", test.Opponent)
	}
	if test.Decider.Kind != DeciderFixed {
		t.Errorf("test decider = %+v", test.Decider)
	}
}

func TestLoadUnknownName(t *testing.T) {
	_, err := Load("nope")
	if err == nil || !strings.Contains(err.Error(), `unknown environment "nope"`) {
		t.Fatalf("Load(nope) error = %v", err)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	data := []byte(`{"addr": "x", "store": {"kind": "memory"}, "opponent": {"kind": "random"}, "decider": {"kind": "fixed"}, "bogus": 1}`)
	_, err := parse("bad.json", data)
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("parse error = %v, want unknown field", err)
	}
}

func TestParseRejectsInvalidConfig(t *testing.T) {
	_, err := parse("bad.json", []byte(`{"addr": "", "store": {"kind": "memory"}, "opponent": {"kind": "random"}, "decider": {"kind": "fixed"}}`))
	if err == nil || !strings.Contains(err.Error(), "addr is required") {
		t.Fatalf("parse error = %v, want addr required", err)
	}
}

func valid() Config {
	return Config{
		Addr:     "127.0.0.1:1",
		Store:    Store{Kind: StoreMemory},
		Opponent: Opponent{Kind: OpponentRandom},
		Decider:  Decider{Kind: DeciderFixed},
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string // "" means valid
	}{
		{"valid", func(*Config) {}, ""},
		{"empty addr", func(c *Config) { c.Addr = "" }, "addr is required"},
		{"unknown store", func(c *Config) { c.Store.Kind = "redis" }, "store.kind must be"},
		{"sqlite without path", func(c *Config) { c.Store = Store{Kind: StoreSQLite} }, "store.path is required for sqlite"},
		{"sqlite with path", func(c *Config) { c.Store = Store{Kind: StoreSQLite, Path: "a.db"} }, ""},
		{"unknown opponent", func(c *Config) { c.Opponent.Kind = "human" }, "opponent.kind must be"},
		{"router without url", func(c *Config) { c.Opponent = Opponent{Kind: OpponentRouter} }, "opponent.url is required for router"},
		{"router with url", func(c *Config) { c.Opponent = Opponent{Kind: OpponentRouter, URL: "http://x"} }, ""},
		{"unknown decider", func(c *Config) { c.Decider.Kind = "human" }, "decider.kind must be"},
		{"http without url", func(c *Config) { c.Decider = Decider{Kind: DeciderHTTP} }, "decider.url is required for http"},
		{"http with url", func(c *Config) { c.Decider = Decider{Kind: DeciderHTTP, URL: "http://x"} }, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := valid()
			tc.mutate(&c)
			err := c.Validate()
			if tc.want == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestFromEnvironmentDefaultsToDev(t *testing.T) {
	t.Setenv("CHESS_ENV", "")
	t.Setenv("CHESS_ADDR", "")
	t.Setenv("CHESS_DB", "")
	t.Setenv("CHESS_ROUTER_URL", "")
	t.Setenv("CHESS_DECIDE_URL", "")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "127.0.0.1:8086" {
		t.Errorf("Addr = %q, want dev value", cfg.Addr)
	}
}

func TestFromEnvironmentSelectsEnv(t *testing.T) {
	t.Setenv("CHESS_ENV", "test")
	t.Setenv("CHESS_ADDR", "")
	t.Setenv("CHESS_DB", "")
	t.Setenv("CHESS_ROUTER_URL", "")
	t.Setenv("CHESS_DECIDE_URL", "")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Store.Kind != StoreMemory {
		t.Errorf("Store.Kind = %q, want memory", cfg.Store.Kind)
	}
}

func TestFromEnvironmentUnknownEnv(t *testing.T) {
	t.Setenv("CHESS_ENV", "nope")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("FromEnvironment with unknown CHESS_ENV: want error")
	}
}

func TestFromEnvironmentOverrides(t *testing.T) {
	t.Setenv("CHESS_ENV", "test")
	t.Setenv("CHESS_ADDR", "127.0.0.1:9999")
	t.Setenv("CHESS_DB", "other.db")
	t.Setenv("CHESS_ROUTER_URL", "http://router")
	t.Setenv("CHESS_DECIDE_URL", "http://decide")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9999" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.Store.Path != "other.db" {
		t.Errorf("Store.Path = %q", cfg.Store.Path)
	}
	if cfg.Opponent.URL != "http://router" {
		t.Errorf("Opponent.URL = %q", cfg.Opponent.URL)
	}
	if cfg.Decider.URL != "http://decide" {
		t.Errorf("Decider.URL = %q", cfg.Decider.URL)
	}
}
