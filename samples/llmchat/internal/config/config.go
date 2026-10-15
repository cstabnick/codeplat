// Package config reads llmchat's settings from the environment. It is the only
// place that does.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

// Config holds the settings the server runs with.
type Config struct {
	Addr      string // listen address
	DBPath    string // SQLite file
	RouterURL string // the llmrouter base URL
	MaxSteps  int    // model calls allowed in one turn
	Workspace string // default workspace for new conversations; empty means none
	// DebugStreamPath is a file that receives one JSON line per router
	// completion (the raw stream). Empty means off.
	DebugStreamPath string
	Exec            string // "local" or "sandbox": where bash runs
	SandboxURL      string // sandbox server base URL
	// ProjectsDir, when set, gives each new conversation without a workspace
	// the directory <ProjectsDir>/<id>. Empty means none.
	ProjectsDir string
}

// defaultMaxSteps is used when CHAT_MAX_STEPS is unset or not a positive integer.
const defaultMaxSteps = 40

// FromEnvironment reads CHAT_ADDR (default "127.0.0.1:8084"), CHAT_DB
// (default: $XDG_DATA_HOME/llmchat/llmchat.db, else
// ~/.local/share/llmchat/llmchat.db), CHAT_ROUTER_URL (default
// "http://127.0.0.1:8083"), CHAT_MAX_STEPS (a positive integer, default 40),
// CHAT_WORKSPACE (default empty), CHAT_DEBUG_STREAM (default empty, which
// turns the raw stream log off), CHAT_EXEC ("local" or "sandbox", default
// "local"; any other value is an error), CHAT_SANDBOX_URL (default
// "http://127.0.0.1:8087") and CHAT_PROJECTS_DIR (default empty, which means
// none; with CHAT_EXEC=sandbox the default is $XDG_DATA_HOME/sandbox/workspaces,
// else ~/.local/share/sandbox/workspaces, the sandbox server's default root).
// An empty variable counts as unset.
func FromEnvironment() (Config, error) {
	exec := envOr("CHAT_EXEC", "local")
	if exec != "local" && exec != "sandbox" {
		return Config{}, fmt.Errorf("CHAT_EXEC=%q: want \"local\" or \"sandbox\"", exec)
	}
	projectsDir := defaultProjectsDir(exec)
	return Config{
		Addr:            envOr("CHAT_ADDR", "127.0.0.1:8084"),
		DBPath:          envOr("CHAT_DB", defaultDBPath()),
		RouterURL:       envOr("CHAT_ROUTER_URL", "http://127.0.0.1:8083"),
		MaxSteps:        maxStepsOr(envOr("CHAT_MAX_STEPS", ""), defaultMaxSteps),
		Workspace:       envOr("CHAT_WORKSPACE", ""),
		DebugStreamPath: envOr("CHAT_DEBUG_STREAM", ""),
		Exec:            exec,
		SandboxURL:      envOr("CHAT_SANDBOX_URL", "http://127.0.0.1:8087"),
		ProjectsDir:     envOr("CHAT_PROJECTS_DIR", projectsDir),
	}, nil
}

// defaultProjectsDir is the projects directory used when CHAT_PROJECTS_DIR is
// unset: the sandbox workspace root in sandbox mode, and none otherwise.
func defaultProjectsDir(exec string) string {
	if exec != "sandbox" {
		return ""
	}
	return filepath.Join(dataHome(), "sandbox", "workspaces")
}

// maxStepsOr parses s as a positive integer, or returns def if it is not one.
func maxStepsOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func defaultDBPath() string {
	return filepath.Join(dataHome(), "llmchat", "llmchat.db")
}

// dataHome is $XDG_DATA_HOME, else ~/.local/share.
func dataHome() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".local", "share")
}
