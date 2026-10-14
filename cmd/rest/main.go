// Command codeplat-rest is the REST entrypoint.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeplat/config"
	"codeplat/internal/app"
	"codeplat/internal/transport/rest"
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

	srv := &http.Server{
		Addr:              cfg.HTTP.Addr,
		Handler:           rest.NewHandler(a.Service, a.Log),
		ReadHeaderTimeout: 10 * time.Second,
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()
	a.Log.Info("rest listening", "addr", cfg.HTTP.Addr, "env", cfg.Env)

	select {
	case err := <-failed:
		if !errors.Is(err, http.ErrServerClosed) {
			a.Log.Error("rest server stopped", "err", err)
			return 1
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			a.Log.Error("rest shutdown", "err", err)
			return 1
		}
	}
	return 0
}
