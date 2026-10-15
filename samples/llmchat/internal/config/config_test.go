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
	stream := filepath.Join(t.TempDir(), "stream.jsonl")
	t.Setenv("CHAT_DEBUG_STREAM", stream)
	t.Setenv("CHAT_EXEC", "local")
	t.Setenv("CHAT_SANDBOX_URL", "http://sandbox:9087")
	t.Setenv("CHAT_PROJECTS_DIR", "/work/projects")

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	want := Config{Addr: "0.0.0.0:9000", DBPath: "/var/lib/x.db", RouterURL: "http://router:8083", MaxSteps: 7, Workspace: "/work/project", DebugStreamPath: stream, Exec: "local", SandboxURL: "http://sandbox:9087", ProjectsDir: "/work/projects"}
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
	t.Setenv("CHAT_EXEC", "")
	t.Setenv("CHAT_SANDBOX_URL", "")
	t.Setenv("CHAT_PROJECTS_DIR", "")
	t.Setenv("XDG_DATA_HOME", dataHome)

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	want := Config{
		Addr:        "127.0.0.1:8084",
		DBPath:      filepath.Join(dataHome, "llmchat", "llmchat.db"),
		RouterURL:   "http://127.0.0.1:8083",
		MaxSteps:    40,
		Exec:        "local",
		SandboxURL:  "http://127.0.0.1:8087",
		ProjectsDir: "",
	}
	if got != want {
		t.Errorf("FromEnvironment() = %+v, want %+v", got, want)
	}
}

func TestConfigSandboxProjectsDefault(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("CHAT_EXEC", "sandbox")
	t.Setenv("CHAT_PROJECTS_DIR", "")
	t.Setenv("XDG_DATA_HOME", dataHome)

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if got.Exec != "sandbox" {
		t.Errorf("Exec = %q, want sandbox", got.Exec)
	}
	if got.SandboxURL != "http://127.0.0.1:8087" {
		t.Errorf("SandboxURL = %q, want http://127.0.0.1:8087", got.SandboxURL)
	}
	want := filepath.Join(dataHome, "sandbox", "workspaces")
	if got.ProjectsDir != want {
		t.Errorf("ProjectsDir = %q, want %q", got.ProjectsDir, want)
	}
}

func TestConfigSandboxProjectsExplicit(t *testing.T) {
	t.Setenv("CHAT_EXEC", "sandbox")
	t.Setenv("CHAT_PROJECTS_DIR", "/srv/projects")

	got, err := FromEnvironment()
	if err != nil {
		t.Fatalf("FromEnvironment: %v", err)
	}
	if got.Exec != "sandbox" || got.ProjectsDir != "/srv/projects" {
		t.Errorf("Exec, ProjectsDir = %q, %q; want sandbox, /srv/projects", got.Exec, got.ProjectsDir)
	}
}

func TestConfigExecInvalidIsError(t *testing.T) {
	t.Setenv("CHAT_EXEC", "docker")

	if _, err := FromEnvironment(); err == nil {
		t.Fatal("FromEnvironment with CHAT_EXEC=docker: want error, got nil")
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
