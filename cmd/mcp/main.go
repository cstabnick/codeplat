// Command codeplat-mcp is the MCP entrypoint. It speaks MCP over stdio.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codeplat/config"
	"codeplat/internal/app"
	"codeplat/internal/transport/mcp"
)

func main() {
	os.Exit(run())
}

func run() int {
	cfg, err := config.FromEnvironment()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	a, err := app.New(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer a.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a.Log.Info("mcp serving on stdio", "env", cfg.Env)
	if err := mcp.NewServer(a.Service, a.Log).Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
		a.Log.Error("mcp server stopped", "err", err)
		return 1
	}
	return 0
}
