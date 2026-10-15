// Command codeplat-nav helps agents find their way around the Go code in the
// current directory. "codeplat-nav mcp" serves it as MCP tools over stdio;
// the other subcommands print the same answers for a shell.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"codeplat/internal/codenav"
	"codeplat/internal/transport/mcp"
)

const usage = `usage:
  codeplat-nav map [DIR]                      packages and exported API under DIR
  codeplat-nav find [-dir DIR] QUERY          declarations whose name contains QUERY
  codeplat-nav refs [-dir DIR] [-in PATH] SYMBOL   every use of SYMBOL
  codeplat-nav mcp                            serve these as MCP tools on stdio
DIR is relative to the current directory, which is the root.`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	nav, err := codenav.New(".", codenav.FindGopls())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	dir := fs.String("dir", "", "directory whose module to search")
	in := fs.String("in", "", "part of the declaring file's path")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	var out string
	switch args[0] {
	case "map":
		out, err = nav.Map(fs.Arg(0))
	case "find":
		var syms []codenav.Symbol
		if syms, err = nav.Find(ctx, *dir, fs.Arg(0)); err == nil {
			out = codenav.FormatSymbols(syms)
		}
	case "refs":
		var locs []codenav.Location
		if locs, err = nav.References(ctx, *dir, fs.Arg(0), *in); err == nil {
			out = codenav.FormatLocations(locs)
		}
	case "mcp":
		log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
		if err := mcp.NewCodenavServer(nav, log).Serve(ctx, os.Stdin, os.Stdout); err != nil && ctx.Err() == nil {
			log.Error("mcp server stopped", "err", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Print(out)
	return 0
}
