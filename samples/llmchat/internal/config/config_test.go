package config

import (
	"path/filepath"
	"testing"
)

func TestConfigFromEnvSet(t *testing.T) {
	t.Setenv("CHAT_ADDR", "0.0.0.0:9000")
	t.Setenv("CHAT_DB", "/var/lib/x.db")
	t.Setenv("CHAT_ROUTER_URL", "http://router:8083")
	t.Setenv("CHAT_MAX_STEPS", "7")
	t.Setenv("CHAT_WORKSPACE", "/work/project")
	t.Setenv("CHAT_DEBUG_STREAM", "/tmp/stream.jsonl")

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	want := Config{Addr: "0.0.0.0:9000", DBPath: "/var/lib/x.db", RouterURL: "http://router:8083", MaxSteps: 7, Workspace: "/work/project", DebugStreamPath: "/tmp/stream.jsonl"}
	if got != want {
		t.Errorf("FromEnvironment() = %+v, want %+v", got, want)
	}
}

func TestConfigFromEnvDefaults(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("CHAT_ADDR", "")
	t.Setenv("CHAT_DB", "")
	t.Setenv("CHAT_ROUTER_URL", "")
	t.Setenv("CHAT_MAX_STEPS", "")
	t.Setenv("CHAT_WORKSPACE", "")
	t.Setenv("CHAT_DEBUG_STREAM", "")
	t.Setenv("XDG_DATA_HOME", dataHome)

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	want := Config{
		Addr:      "127.0.0.1:8084",
		DBPath:    filepath.Join(dataHome, "llmchat", "llmchat.db"),
		RouterURL: "http://127.0.0.1:8083",
		MaxSteps:  40,
	}
	if got != want {
		t.Errorf("FromEnvironment() = %+v, want %+v", got, want)
	}
}

func TestConfigMaxStepsInvalidFallsBack(t *testing.T) {
	for _, v := range []string{"", "abc", "0", "-3"} {
		t.Run("CHAT_MAX_STEPS="+v, func(t *testing.T) {
			t.Setenv("CHAT_MAX_STEPS", v)
			got, err := FromEnvironment()
			if err != nil {
				t.Fatalf("FromEnvironment: %v", err)
			}
			if got.MaxSteps != 40 {
				t.Errorf("MaxSteps = %d, want 40", got.MaxSteps)
			}
		})
	}
}
