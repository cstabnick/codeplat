package config

import (
	"testing"
)

func TestLoadEmbeddedEnvironments(t *testing.T) {
	dev, err := Load("dev")
	if err != nil {
		t.Fatalf("Load(dev): %v", err)
	}
	if want := (Config{Addr: "127.0.0.1:8085", ModelsDir: "models"}); dev != want {
		t.Errorf("Load(dev) = %+v, want %+v", dev, want)
	}

	test, err := Load("test")
	if err != nil {
		t.Fatalf("Load(test): %v", err)
	}
	if want := (Config{Addr: "127.0.0.1:0", ModelsDir: ""}); test != want {
		t.Errorf("Load(test) = %+v, want %+v", test, want)
	}
}

func TestLoadUnknownNameFails(t *testing.T) {
	if _, err := Load("nope"); err == nil {
		t.Fatal("Load(nope) succeeded, want an error")
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	_, err := parse("bad.json", []byte(`{"addr": "127.0.0.1:1", "models_dir": "m", "extra": true}`))
	if err == nil {
		t.Fatal("parse with an unknown field succeeded, want an error")
	}
}

func TestParseRejectsEmptyAddr(t *testing.T) {
	if _, err := parse("bad.json", []byte(`{"addr": "", "models_dir": "m"}`)); err == nil {
		t.Fatal("parse with an empty addr succeeded, want an error")
	}
}

func TestParseRejectsMalformedJSON(t *testing.T) {
	if _, err := parse("bad.json", []byte(`{"addr": `)); err == nil {
		t.Fatal("parse of malformed JSON succeeded, want an error")
	}
}

// clearDecideEnv unsets the variables FromEnvironment reads for this test.
// t.Setenv restores the previous values when the test ends.
func clearDecideEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DECIDE_ENV", "")
	t.Setenv("DECIDE_ADDR", "")
	t.Setenv("DECIDE_MODELS", "")
}

func TestFromEnvironmentDefaultsToDev(t *testing.T) {
	clearDecideEnv(t)
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if want := (Config{Addr: "127.0.0.1:8085", ModelsDir: "models"}); cfg != want {
		t.Errorf("FromEnvironment = %+v, want %+v", cfg, want)
	}
}

func TestFromEnvironmentHonoursDecideEnv(t *testing.T) {
	clearDecideEnv(t)
	t.Setenv("DECIDE_ENV", "test")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if want := (Config{Addr: "127.0.0.1:0", ModelsDir: ""}); cfg != want {
		t.Errorf("FromEnvironment = %+v, want %+v", cfg, want)
	}
}

func TestFromEnvironmentUnknownDecideEnvFails(t *testing.T) {
	clearDecideEnv(t)
	t.Setenv("DECIDE_ENV", "nope")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("FromEnvironment with an unknown DECIDE_ENV succeeded, want an error")
	}
}

func TestFromEnvironmentHonoursDecideAddr(t *testing.T) {
	clearDecideEnv(t)
	t.Setenv("DECIDE_ADDR", "0.0.0.0:9000")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "0.0.0.0:9000" {
		t.Errorf("Addr = %q, want %q", cfg.Addr, "0.0.0.0:9000")
	}
	if cfg.ModelsDir != "models" {
		t.Errorf("ModelsDir = %q, want %q (unchanged)", cfg.ModelsDir, "models")
	}
}

func TestFromEnvironmentHonoursDecideModels(t *testing.T) {
	clearDecideEnv(t)
	t.Setenv("DECIDE_MODELS", "/srv/models")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.ModelsDir != "/srv/models" {
		t.Errorf("ModelsDir = %q, want %q", cfg.ModelsDir, "/srv/models")
	}
	if cfg.Addr != "127.0.0.1:8085" {
		t.Errorf("Addr = %q, want %q (unchanged)", cfg.Addr, "127.0.0.1:8085")
	}
}
