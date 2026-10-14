// Command codeplat is the CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"

	"codeplat/config"
	"codeplat/internal/app"
	"codeplat/internal/transport/cli"
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

	return cli.Run(context.Background(), a.Service, os.Args[1:], os.Stdout, os.Stderr)
}
