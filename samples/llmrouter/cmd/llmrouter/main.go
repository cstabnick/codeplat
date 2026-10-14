// Command llmrouter serves an OpenAI-compatible API that routes each chat
// to the model that fits it best.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"llmrouter/internal/app"
	"llmrouter/internal/config"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", os.Getenv("LLMROUTER_CONFIG"), "path to a JSON config; empty uses the built-in default")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}
	a, err := app.New(cfg, log.Default())
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a.Start(ctx)

	srv := &http.Server{Addr: cfg.Addr, Handler: a.Handler, ReadHeaderTimeout: 10 * time.Second}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("llmrouter listening on http://%s", cfg.Addr)

	select {
	case err := <-errc:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			if cerr := a.Close(); cerr != nil {
				log.Printf("close: %v", cerr)
			}
			return err
		}
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	return a.Close()
}
