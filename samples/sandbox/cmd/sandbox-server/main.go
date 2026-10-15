// Command sandbox-server serves the sandbox API over HTTP.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sandbox/internal/app"
	"sandbox/internal/config"
	"sandbox/internal/httpapi"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	cfg, err := config.FromEnvironment()
	if err != nil {
		return err
	}
	svc, err := app.New(cfg)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    cfg.Addr,
		Handler: httpapi.NewHandler(svc),
		// No write timeout: exec can run for up to 10 minutes.
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errs := make(chan error, 1)
	go func() { errs <- srv.ListenAndServe() }()
	log.Printf("sandbox listening on http://%s (driver %s, root %s)", cfg.Addr, cfg.Driver, cfg.Root)

	select {
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
