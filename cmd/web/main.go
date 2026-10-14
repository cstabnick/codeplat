// Command codeplat-web is the web UI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codeplat/config"
	"codeplat/internal/app"
	"codeplat/internal/transport/httpauth"
	"codeplat/internal/transport/httprun"
	"codeplat/internal/transport/web"
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

	if err := httprun.Serve(ctx, "web", cfg.Web.Addr, httpauth.Require(a.Auth, web.NewHandler(ctx, a.Service, a.Log)), a.Log); err != nil {
		a.Log.Error("web server stopped", "err", err)
		return 1
	}
	return 0
}
