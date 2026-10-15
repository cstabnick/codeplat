package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var sandboxVars = []string{
	"SANDBOX_ENV",
	"SANDBOX_ADDR",
	"SANDBOX_DRIVER",
	"DOCKER_HOST",
	"SANDBOX_ROOT",
	"SANDBOX_IMAGE",
}

// clearEnvironment makes every variable FromEnvironment reads empty, which it
// treats as unset, and points HOME at a temporary directory.
func clearEnvironment(t *testing.T) {
	t.Helper()
	for _, k := range sandboxVars {
		t.Setenv(k, "")
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", t.TempDir())
}

func TestLoadDev(t *testing.T) {
	got, err := Load("dev")
	if err != nil {
		t.Fatalf("Load(dev): %v", err)
	}
	want := Config{
		Addr:       "127.0.0.1:8087",
		Driver:     "docker",
		DockerHost: "unix:///var/run/docker.sock",
		Root:       "",
		Image:      "debian:bookworm-slim",
		MemoryMB:   2048,
		CPUs:       2,
		Network:    "bridge",
	}
	if got != want {
		t.Errorf("Load(dev) = %+v, want %+v", got, want)
	}
}

func TestLoadTest(t *testing.T) {
	got, err := Load("test")
	if err != nil {
		t.Fatalf("Load(test): %v", err)
	}
	want := Config{
		Addr:     "127.0.0.1:0",
		Driver:   "local",
		Root:     "",
		Image:    "local",
		MemoryMB: 0,
		CPUs:     0,
		Network:  "none",
	}
	if got != want {
		t.Errorf("Load(test) = %+v, want %+v", got, want)
	}
}

func TestLoadUnknownName(t *testing.T) {
	_, err := Load("nope")
	if err == nil || !strings.Contains(err.Error(), "unknown environment") {
		t.Fatalf("Load(nope) error = %v, want unknown environment", err)
	}
}

func TestParseUnknownField(t *testing.T) {
	data := []byte(`{"addr":"127.0.0.1:1","driver":"local","image":"local","network":"none","bogus":1}`)
	_, err := parse("x.json", data)
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("parse with unknown field error = %v, want it to name bogus", err)
	}
}

func TestParseValidatesResult(t *testing.T) {
	data := []byte(`{"addr":"127.0.0.1:1","driver":"local","image":"","network":"none"}`)
	_, err := parse("x.json", data)
	if err == nil || !strings.Contains(err.Error(), "image is required") {
		t.Fatalf("parse with empty image error = %v, want image is required", err)
	}
}

func TestValidate(t *testing.T) {
	base := Config{
		Addr:    "127.0.0.1:1",
		Driver:  "local",
		Image:   "img",
		Network: "none",
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{name: "valid local", mutate: func(c *Config) {}},
		{name: "valid docker", mutate: func(c *Config) {
			c.Driver = "docker"
			c.DockerHost = "unix:///var/run/docker.sock"
			c.Network = "bridge"
		}},
		{name: "valid absolute root", mutate: func(c *Config) {
			c.Root = filepath.Join(string(filepath.Separator), "work")
		}},
		{name: "empty addr", mutate: func(c *Config) { c.Addr = "" }, wantErr: "addr is required"},
		{name: "empty image", mutate: func(c *Config) { c.Image = "" }, wantErr: "image is required"},
		{name: "unknown driver", mutate: func(c *Config) { c.Driver = "podman" }, wantErr: "driver must be"},
		{name: "empty driver", mutate: func(c *Config) { c.Driver = "" }, wantErr: "driver must be"},
		{name: "docker without host", mutate: func(c *Config) {
			c.Driver = "docker"
			c.DockerHost = ""
		}, wantErr: "docker_host is required"},
		{name: "unknown network", mutate: func(c *Config) { c.Network = "host" }, wantErr: "network must be"},
		{name: "negative memory", mutate: func(c *Config) { c.MemoryMB = -1 }, wantErr: "memory_mb cannot be negative"},
		{name: "negative cpus", mutate: func(c *Config) { c.CPUs = -0.5 }, wantErr: "cpus cannot be negative"},
		{name: "relative root", mutate: func(c *Config) { c.Root = "relative/path" }, wantErr: "root must be absolute"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestFromEnvironmentDefaults(t *testing.T) {
	clearEnvironment(t)
	home := os.Getenv("HOME")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "127.0.0.1:8087" || cfg.Driver != "docker" || cfg.DockerHost != "unix:///var/run/docker.sock" {
		t.Errorf("dev values not kept: %+v", cfg)
	}
	if want := filepath.Join(home, ".local", "share", "sandbox", "workspaces"); cfg.Root != want {
		t.Errorf("Root = %q, want %q", cfg.Root, want)
	}
	if want := fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()); cfg.User != want {
		t.Errorf("User = %q, want %q", cfg.User, want)
	}
}

func TestFromEnvironmentXDGDefaultRoot(t *testing.T) {
	clearEnvironment(t)
	xdg := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdg)
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if want := filepath.Join(xdg, "sandbox", "workspaces"); cfg.Root != want {
		t.Errorf("Root = %q, want %q", cfg.Root, want)
	}
}

func TestFromEnvironmentRelativeXDGIgnored(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("XDG_DATA_HOME", "relative/data")
	home := os.Getenv("HOME")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if want := filepath.Join(home, ".local", "share", "sandbox", "workspaces"); cfg.Root != want {
		t.Errorf("Root = %q, want %q", cfg.Root, want)
	}
}

func TestFromEnvironmentSelectsEnvironment(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("SANDBOX_ENV", "test")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Driver != "local" || cfg.Network != "none" || cfg.Addr != "127.0.0.1:0" {
		t.Errorf("SANDBOX_ENV=test gave %+v", cfg)
	}
}

func TestFromEnvironmentUnknownEnvironment(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("SANDBOX_ENV", "nope")
	if _, err := FromEnvironment(); err == nil {
		t.Fatal("FromEnvironment with unknown SANDBOX_ENV: want error")
	}
}

func TestFromEnvironmentOverrides(t *testing.T) {
	clearEnvironment(t)
	root := t.TempDir()
	t.Setenv("SANDBOX_ADDR", "127.0.0.1:9000")
	t.Setenv("SANDBOX_DRIVER", "local")
	t.Setenv("DOCKER_HOST", "unix:///tmp/other.sock")
	t.Setenv("SANDBOX_ROOT", root)
	t.Setenv("SANDBOX_IMAGE", "alpine:3.20")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "127.0.0.1:9000" {
		t.Errorf("Addr = %q", cfg.Addr)
	}
	if cfg.Driver != "local" {
		t.Errorf("Driver = %q", cfg.Driver)
	}
	if cfg.DockerHost != "unix:///tmp/other.sock" {
		t.Errorf("DockerHost = %q", cfg.DockerHost)
	}
	if cfg.Root != root {
		t.Errorf("Root = %q, want %q", cfg.Root, root)
	}
	if cfg.Image != "alpine:3.20" {
		t.Errorf("Image = %q", cfg.Image)
	}
}

func TestFromEnvironmentEmptyOverrideChangesNothing(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("SANDBOX_ADDR", "")
	t.Setenv("SANDBOX_IMAGE", "")
	cfg, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if cfg.Addr != "127.0.0.1:8087" || cfg.Image != "debian:bookworm-slim" {
		t.Errorf("empty overrides changed values: %+v", cfg)
	}
}

func TestFromEnvironmentInvalidOverride(t *testing.T) {
	clearEnvironment(t)
	t.Setenv("SANDBOX_ROOT", "relative/root")
	_, err := FromEnvironment()
	if err == nil || !strings.Contains(err.Error(), "root must be absolute") {
		t.Fatalf("FromEnvironment with relative SANDBOX_ROOT error = %v", err)
	}
}
