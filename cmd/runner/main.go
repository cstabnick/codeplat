// Command codeplat-runner starts a worker for each todo issue in the tracker's stage queues.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"codeplat/config"
	"codeplat/internal/app"
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

	r, err := a.NewRunner()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := r.Run(ctx); err != nil {
		a.Log.Error("runner stopped", "err", err)
		return 1
	}
	return 0
}
