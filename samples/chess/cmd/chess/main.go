// Command chess serves the chess REST API and the chess page.
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

	"chess/internal/api"
	"chess/internal/app"
	"chess/internal/config"
	"chess/internal/web"
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
	svc, closeFn, err := app.New(cfg)
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", api.NewHandler(svc))
	mux.Handle("/healthz", api.NewHandler(svc))
	mux.Handle("/", web.NewHandler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second, // no write timeout: opponent moves are slow
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("chess listening on http://%s", cfg.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			_ = closeFn()
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}
	return closeFn()
}
