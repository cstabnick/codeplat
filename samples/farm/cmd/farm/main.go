// Command farm serves the Gossip Acres page.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"farm/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	addr := flag.String("addr", defaultAddr(), "listen address")
	flag.Parse()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           web.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Printf("farm: serving http://%s", *addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}

// defaultAddr is FARM_ADDR if set, else 127.0.0.1:8088.
func defaultAddr() string {
	if addr := os.Getenv("FARM_ADDR"); addr != "" {
		return addr
	}
	return "127.0.0.1:8088"
}
