package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"decide/internal/apptest"
)

// runCLI calls Run with a fresh service and returns the exit code and output.
func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	svc := apptest.New(t)
	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), svc, args, strings.NewReader(stdin), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestEvalPrintsIndentedResult(t *testing.T) {
	code, stdout, stderr := runCLI(t, `{"country":"US","weight":3}`, "eval", "example-shipping")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
	}
	if !strings.HasSuffix(stdout, "\n") {
		t.Errorf("stdout %q does not end with a newline", stdout)
	}
	if !strings.Contains(stdout, "\n  \"cost\": 5") {
		t.Errorf("stdout = %q, want indented output with two spaces and cost 5", stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v", err)
	}
	if got := out["cost"]; got != float64(5) {
		t.Errorf("cost = %#v, want 5", got)
	}
}

func TestEvalEmptyStdinIsEmptyObject(t *testing.T) {
	// Empty stdin must be decoded as {}, so it behaves exactly like "{}".
	// example-shipping cannot decide an empty request (country and weight are
	// missing), so the expected result is the engine's error with exit 1, not
	// a JSON decoding error.
	emptyCode, emptyOut, emptyErr := runCLI(t, "", "eval", "example-shipping")
	objCode, objOut, objErr := runCLI(t, "{}", "eval", "example-shipping")
	if emptyCode != objCode || emptyOut != objOut || emptyErr != objErr {
		t.Fatalf("empty stdin: exit %d stdout %q stderr %q; {}: exit %d stdout %q stderr %q",
			emptyCode, emptyOut, emptyErr, objCode, objOut, objErr)
	}
	if strings.Contains(emptyErr, "input:") {
		t.Errorf("stderr = %q, want no JSON decoding error", emptyErr)
	}
}

func TestModelsListsExample(t *testing.T) {
	code, stdout, stderr := runCLI(t, "", "models")
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr = %q", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if !slices.Contains(lines, "example-shipping") {
		t.Errorf("models = %q, want it to include example-shipping", lines)
	}
}

func TestUnknownModelExitsOne(t *testing.T) {
	code, stdout, stderr := runCLI(t, "", "eval", "no-such-model")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.HasPrefix(stderr, "decide: ") {
		t.Errorf("stderr = %q, want a decide: prefix", stderr)
	}
}

func TestInvalidKeyExitsOne(t *testing.T) {
	code, _, stderr := runCLI(t, "", "eval", "Bad Key")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, "decide: ") {
		t.Errorf("stderr = %q, want a decide: prefix", stderr)
	}
}

func TestInvalidJSONInputExitsOne(t *testing.T) {
	code, _, stderr := runCLI(t, `{"country":`, "eval", "example-shipping")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if !strings.HasPrefix(stderr, "decide: ") {
		t.Errorf("stderr = %q, want a decide: prefix", stderr)
	}
}

func TestNoArgsExitsTwo(t *testing.T) {
	code, stdout, stderr := runCLI(t, "")
	if code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want nothing", stdout)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q, want usage", stderr)
	}
}

func TestUnknownSubcommandExitsTwo(t *testing.T) {
	if code, _, _ := runCLI(t, "", "frobnicate"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}

func TestEvalWithoutKeyExitsTwo(t *testing.T) {
	if code, _, _ := runCLI(t, "", "eval"); code != 2 {
		t.Errorf("exit = %d, want 2", code)
	}
}
