// Package app builds the llmchat server from configuration: the store, the
// router client, the engine, the API and the page, joined on one mux.
package app

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"llmchat/internal/agent"
	"llmchat/internal/api"
	"llmchat/internal/chat"
	"llmchat/internal/config"
	"llmchat/internal/routerclient"
	"llmchat/internal/sqlitestore"
	"llmchat/internal/tools/edit"
	"llmchat/internal/tools/fs"
	"llmchat/internal/tools/search"
	"llmchat/internal/tools/shell"
	"llmchat/internal/web"
)

// SystemPrompt is sent first in every request. The engine adds the workspace
// directory after it when the conversation has one.
const SystemPrompt = "You are a coding assistant. You can read, search and edit files and run shell commands in the workspace directory using the provided tools. Look at files before changing them, make small focused edits, and explain briefly what you did. Tool calls that change files or run commands are shown to the person for approval first."

// App is a built server. Handler serves the API and the page.
type App struct {
	Handler http.Handler

	store     *sqlitestore.Store
	debugFile *os.File // the raw stream log; nil when off
}

// New opens the database (creating its directory with os.MkdirAll,
// mode 0o755, first) and builds everything. When cfg.DebugStreamPath is set
// it opens that file for appending (created with mode 0o644) as the raw
// stream log. It starts nothing.
func New(cfg config.Config) (*App, error) {
	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("app: create database directory: %w", err)
	}
	store, err := sqlitestore.Open(cfg.DBPath, nil)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}

	var debugFile *os.File
	var debugOut io.Writer
	if cfg.DebugStreamPath != "" {
		f, err := os.OpenFile(cfg.DebugStreamPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			store.Close()
			return nil, fmt.Errorf("app: open debug stream log: %w", err)
		}
		debugFile = f
		debugOut = f
	}

	routerClient := routerclient.New(cfg.RouterURL, nil).WithDebugLog(debugOut)
	approvals := api.NewApprovals()
	tools := []chat.Tool{
		fs.ReadFile(), fs.ListDir(), search.Grep(),
		edit.WriteFile(), edit.EditFile(), shell.Bash(),
	}
	engine := agent.New(store, routerClient, tools, approvals, agent.Options{
		System:   SystemPrompt,
		MaxSteps: cfg.MaxSteps,
	})

	logger := log.New(os.Stderr, "llmchat: ", log.LstdFlags)
	apiStore := defaultWorkspaceStore{Store: store, workspace: cfg.Workspace}
	apiHandler := api.NewHandler(apiStore, engine, approvals, routerClient.Routes, logger)
	webHandler := web.NewHandler()

	mux := http.NewServeMux()
	mux.Handle("/api/", apiHandler)
	mux.Handle("/healthz", apiHandler)
	mux.Handle("/", webHandler)

	return &App{Handler: mux, store: store, debugFile: debugFile}, nil
}

// defaultWorkspaceStore gives new conversations without a workspace the
// configured default. The engine uses the unwrapped store.
type defaultWorkspaceStore struct {
	chat.Store
	workspace string
}

func (s defaultWorkspaceStore) CreateConversation(ctx context.Context, c chat.Conversation) (chat.Conversation, error) {
	if c.Workspace == "" {
		c.Workspace = s.workspace
	}
	return s.Store.CreateConversation(ctx, c)
}

// Close closes the store and the debug stream log, if any.
func (a *App) Close() error {
	err := a.store.Close()
	if a.debugFile != nil {
		if cerr := a.debugFile.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
