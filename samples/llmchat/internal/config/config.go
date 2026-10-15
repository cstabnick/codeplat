// Package config reads llmchat's settings from the environment. It is the only
// place that does.
package config

import (
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
}

// defaultMaxSteps is used when CHAT_MAX_STEPS is unset or not a positive integer.
const defaultMaxSteps = 40

// FromEnvironment reads CHAT_ADDR (default "127.0.0.1:8084"), CHAT_DB
// (default: $XDG_DATA_HOME/llmchat/llmchat.db, else
// ~/.local/share/llmchat/llmchat.db), CHAT_ROUTER_URL (default
// "http://127.0.0.1:8083"), CHAT_MAX_STEPS (a positive integer, default 40)
// CHAT_WORKSPACE (default empty) and CHAT_DEBUG_STREAM (default empty, which
// turns the raw stream log off). An empty variable counts as unset.
func FromEnvironment() (Config, error) {
	return Config{
		Addr:            envOr("CHAT_ADDR", "127.0.0.1:8084"),
		DBPath:          envOr("CHAT_DB", defaultDBPath()),
		RouterURL:       envOr("CHAT_ROUTER_URL", "http://127.0.0.1:8083"),
		MaxSteps:        maxStepsOr(envOr("CHAT_MAX_STEPS", ""), defaultMaxSteps),
		Workspace:       envOr("CHAT_WORKSPACE", ""),
		DebugStreamPath: envOr("CHAT_DEBUG_STREAM", ""),
	}, nil
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
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "llmchat", "llmchat.db")
}
