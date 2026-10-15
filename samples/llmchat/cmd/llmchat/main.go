// Command llmchat runs the chat server.
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

	"llmchat/internal/app"
	"llmchat/internal/config"
)

func main() {
	cfg, err := config.FromEnvironment()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
	a, err := app.New(cfg)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           a.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() {
		log.Printf("llmchat listening on http://%s (router %s, db %s)", cfg.Addr, cfg.RouterURL, cfg.DBPath)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Print(err)
			a.Close()
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Print(err)
		}
	}
	if err := a.Close(); err != nil {
		log.Print(err)
	}
}
