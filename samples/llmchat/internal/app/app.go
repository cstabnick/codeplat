// Package app builds the llmchat server from configuration: the store, the
// router client, the engine, the API and the page, joined on one mux.
package app

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"llmchat/internal/agent"
	"llmchat/internal/api"
	"llmchat/internal/routerclient"
	"llmchat/internal/sqlitestore"
	"llmchat/internal/web"
)

// Config holds the settings the server runs with.
type Config struct {
	Addr      string // listen address
	DBPath    string // SQLite file
	RouterURL string // the llmrouter base URL
}

// ConfigFromEnv reads CHAT_ADDR (default "127.0.0.1:8084"), CHAT_DB
// (default: $XDG_DATA_HOME/llmchat/llmchat.db, else
// ~/.local/share/llmchat/llmchat.db) and CHAT_ROUTER_URL (default
// "http://127.0.0.1:8083"). An empty variable counts as unset.
func ConfigFromEnv() Config {
	return Config{
		Addr:      envOr("CHAT_ADDR", "127.0.0.1:8084"),
		DBPath:    envOr("CHAT_DB", defaultDBPath()),
		RouterURL: envOr("CHAT_ROUTER_URL", "http://127.0.0.1:8083"),
	}
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

// App is a built server. Handler serves the API and the page.
type App struct {
	Handler http.Handler

	store *sqlitestore.Store
}

// New opens the database (creating its directory with os.MkdirAll,
// mode 0o755, first) and builds everything. It starts nothing.
func New(cfg Config) (*App, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("app: create database directory: %w", err)
	}
	store, err := sqlitestore.Open(cfg.DBPath, nil)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	routerClient := routerclient.New(cfg.RouterURL, nil)
	approvals := api.NewApprovals()
	engine := agent.New(store, routerClient, nil, approvals, agent.Options{})

	apiHandler := api.NewHandler(store, engine, approvals, routerClient.Routes)
	webHandler := web.NewHandler()

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/healthz", apiHandler)
	mux.Handle("/", webHandler)

	return &App{Handler: mux, store: store}, nil
}

// Close closes the store.
func (a *App) Close() error {
	return a.store.Close()
}
