// Command decide evaluates decision models from the command line.
package main

import (
	"context"
	"fmt"
	"os"

	"decide/internal/app"
	"decide/internal/cli"
	"decide/internal/config"
)

func main() {
	cfg, err := config.FromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "decide: %v\n", err)
		os.Exit(1)
	}
	svc, err := app.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "decide: %v\n", err)
		os.Exit(1)
	}
	os.Exit(cli.Run(context.Background(), svc, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
