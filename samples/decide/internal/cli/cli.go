// Package cli is the command-line transport of decide. It decodes its own
// input, calls core.Service and maps errors to exit codes. It validates nothing
// that core does not.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"decide/internal/core"
)

const usage = `usage:
  decide models
  decide eval KEY < input.json
`

// Run returns the exit code.
func Run(ctx context.Context, svc *core.Service, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "models":
		keys, err := svc.Models(ctx)
		if err != nil {
			return fail(stderr, err)
		}
		for _, k := range keys {
			fmt.Fprintln(stdout, k)
		}
		return 0
	case "eval":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		input, err := readInput(stdin)
		if err != nil {
			return fail(stderr, err)
		}
		out, err := svc.Evaluate(ctx, args[1], input)
		if err != nil {
			return fail(stderr, err)
		}
		data, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fail(stderr, err)
		}
		stdout.Write(append(data, '\n'))
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

// readInput decodes one JSON object from r. Empty input is an empty object.
func readInput(r io.Reader) (map[string]any, error) {
	if r == nil {
		return map[string]any{}, nil
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read input: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var input map[string]any
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("input: %w", err)
	}
	return input, nil
}

// fail prints err to stderr and returns exit code 1.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintf(stderr, "decide: %v\n", err)
	return 1
}
