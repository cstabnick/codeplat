// Package httprun runs an http.Handler until its context is cancelled. The
// HTTP entrypoints share it so they start and shut down the same way.
package httprun

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Serve listens on addr and blocks until ctx is cancelled, then shuts down
// gracefully. It returns nil on a clean shutdown.
func Serve(ctx context.Context, name, addr string, h http.Handler, log *slog.Logger) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.ListenAndServe() }()
	log.Info(name+" listening", "addr", addr)

	select {
	case err := <-failed:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
