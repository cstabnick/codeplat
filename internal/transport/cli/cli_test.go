package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"codeplat/internal/apptest"
	"codeplat/internal/transport/cli"
)

func TestCLI(t *testing.T) {
	svc := apptest.New(t).Service

	steps := []struct {
		args       []string
		code       int
		wantStdout string
		wantStderr string
	}{
		{[]string{"project", "create", "plat", "Platform"}, 0, "created project PLAT", ""},
		{[]string{"project", "create", "plat", "Platform"}, 1, "", "already exists"},
		{[]string{"project", "list"}, 0, "PLAT  Platform", ""},
		{[]string{"issue", "create", "-project", "plat", "-title", "First", "-assignee", "sam"}, 0, "created issue PLAT-1", ""},
		{[]string{"issue", "create", "-project", "plat"}, 1, "", "title is required"},
		{[]string{"issue", "update", "PLAT-1", "-status", "done"}, 0, "Assignee  sam", ""},
		{[]string{"issue", "update", "PLAT-1", "-assignee", ""}, 0, "Status    done", ""},
		{[]string{"issue", "list", "-status", "done"}, 0, "PLAT-1  done", ""},
		{[]string{"issue", "list", "-assignee", "sam"}, 0, "ID", ""},
		{[]string{"issue", "show", "PLAT-1"}, 0, "Title     First", ""},
		{[]string{"issue", "show", "PLAT-9"}, 1, "", "not found"},
		{[]string{"issue", "update", "PLAT-1", "-status", "closed"}, 1, "", "invalid input"},
		{[]string{"issue", "list", "-bogus"}, 2, "", "usage:"},
		{[]string{"nope"}, 2, "", "usage:"},
	}
	for _, s := range steps {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), svc, s.args, &stdout, &stderr)
		if code != s.code {
			t.Fatalf("%v: exit %d, want %d\nstdout: %s\nstderr: %s", s.args, code, s.code, &stdout, &stderr)
		}
		if !strings.Contains(stdout.String(), s.wantStdout) {
			t.Fatalf("%v: stdout %q does not contain %q", s.args, &stdout, s.wantStdout)
		}
		if !strings.Contains(stderr.String(), s.wantStderr) {
			t.Fatalf("%v: stderr %q does not contain %q", s.args, &stderr, s.wantStderr)
		}
	}

	// Clearing the assignee must have taken effect: sam no longer has issues.
	var stdout, stderr bytes.Buffer
	cli.Run(context.Background(), svc, []string{"issue", "list", "-assignee", "sam"}, &stdout, &stderr)
	if strings.Contains(stdout.String(), "PLAT-1") {
		t.Fatalf("assignee was not cleared: %s", &stdout)
	}
}
