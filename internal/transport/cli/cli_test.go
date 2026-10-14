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
		{[]string{"issue", "update", "PLAT-1", "-status", "done"}, 0, "Assignee    sam", ""},
		{[]string{"issue", "update", "PLAT-1", "-assignee", ""}, 0, "Status      done", ""},
		{[]string{"issue", "list", "-status", "done"}, 0, "PLAT-1  refinement  done", ""},
		{[]string{"issue", "create", "-project", "plat", "-title", "Second", "-stage", "implementation"}, 0, "created issue PLAT-2", ""},
		{[]string{"issue", "create", "-project", "plat", "-title", "x", "-stage", "review"}, 1, "", "stage"},
		{[]string{"issue", "list", "-stage", "implementation", "-status", "todo"}, 0, "PLAT-2  implementation  todo", ""},
		{[]string{"issue", "claim", "PLAT-2"}, 1, "", "assignee is required"},
		{[]string{"issue", "claim", "plat-2", "-assignee", "w1"}, 0, "w1 claimed PLAT-2", ""},
		{[]string{"issue", "claim", "PLAT-2", "-assignee", "w2"}, 1, "", "is in_progress, not todo"},
		{[]string{"issue", "update", "PLAT-2", "-stage", "testing", "-status", "todo", "-assignee", ""}, 0, "Stage       testing", ""},
		{[]string{"issue", "list", "-assignee", "sam"}, 0, "ID", ""},
		{[]string{"issue", "show", "PLAT-1"}, 0, "Title       First", ""},
		{[]string{"issue", "show", "PLAT-9"}, 1, "", "not found"},
		{[]string{"issue", "update", "PLAT-1", "-status", "closed"}, 1, "", "invalid input"},
		{[]string{"issue", "create", "-project", "plat", "-title", "Third", "-assignee", "sam"}, 0, "created issue PLAT-3", ""},
		{[]string{"issue", "block", "PLAT-3", "-author", "sam", "-reason", "no spec"}, 0, "blocked PLAT-3", ""},
		{[]string{"issue", "show", "PLAT-3"}, 0, "Status      blocked", ""},
		{[]string{"issue", "block", "PLAT-3", "-author", "sam"}, 1, "", "invalid input"},
		{[]string{"issue", "block"}, 2, "", "usage:"},
		{[]string{"issue", "comments", "PLAT-1"}, 0, "", ""},
		{[]string{"issue", "comment", "plat-1", "-body", "on it", "-author", "sam"}, 0, "added comment #1 to PLAT-1", ""},
		{[]string{"issue", "comment", "PLAT-1", "-body", "no name"}, 0, "added comment #2 to PLAT-1", ""},
		{[]string{"issue", "comment", "PLAT-1"}, 1, "", "comment body is required"},
		{[]string{"issue", "comment", "PLAT-9", "-body", "x"}, 1, "", "not found"},
		{[]string{"issue", "comments", "PLAT-1"}, 0, "#1  sam  2026-01-01 00:00:00Z\non it\n\n#2  (no author)", ""},
		{[]string{"issue", "comments", "PLAT-9"}, 1, "", "not found"},
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

func TestCLIEpicsAndReady(t *testing.T) {
	svc := apptest.New(t).Service
	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), svc, args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	if code, _, errOut := run("project", "create", "plat", "Platform"); code != 0 {
		t.Fatalf("project create: exit %d: %s", code, errOut)
	}
	if code, _, errOut := run("issue", "create", "-project", "plat", "-title", "First"); code != 0 {
		t.Fatalf("issue create First: exit %d: %s", code, errOut)
	}

	// A dependent issue with an epic is created and shown with both fields.
	if code, out, errOut := run("issue", "create", "-project", "plat", "-title", "Second", "-epic", "2", "-depends-on", "plat-1"); code != 0 || !strings.Contains(out, "created issue PLAT-2") {
		t.Fatalf("issue create Second: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
	if code, out, errOut := run("issue", "show", "PLAT-2"); code != 0 || !strings.Contains(out, "Epic        2") || !strings.Contains(out, "Depends on  PLAT-1") {
		t.Fatalf("issue show PLAT-2: exit %d, stdout %q, stderr %q", code, out, errOut)
	}

	// An unknown dependency is rejected by the Service.
	if code, _, errOut := run("issue", "create", "-project", "plat", "-title", "Third", "-depends-on", "PLAT-99"); code != 1 || !strings.Contains(errOut, "does not exist") {
		t.Fatalf("issue create Third: exit %d, stderr %q, want 1 with does not exist", code, errOut)
	}

	// PLAT-2 waits for PLAT-1, which is still todo, so only PLAT-1 is ready.
	code, out, errOut := run("issue", "list", "-ready")
	if code != 0 {
		t.Fatalf("issue list -ready: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "PLAT-1") || strings.Contains(out, "PLAT-2") {
		t.Fatalf("issue list -ready: stdout %q, want PLAT-1 and not PLAT-2", out)
	}

	// Clearing the dependencies with an empty value.
	if code, _, errOut := run("issue", "update", "PLAT-2", "-depends-on", ""); code != 0 {
		t.Fatalf("issue update PLAT-2 -depends-on \"\": exit %d, stderr %q", code, errOut)
	}
	if code, out, errOut := run("issue", "show", "PLAT-2"); code != 0 || !strings.Contains(out, "Depends on  -") {
		t.Fatalf("issue show PLAT-2 after clearing: exit %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestCLIPaging(t *testing.T) {
	svc := apptest.New(t).Service
	run := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := cli.Run(context.Background(), svc, args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	if code, _, errOut := run("project", "create", "plat", "Platform"); code != 0 {
		t.Fatalf("project create: exit %d: %s", code, errOut)
	}
	for _, title := range []string{"First", "Second", "Third"} {
		if code, _, errOut := run("issue", "create", "-project", "plat", "-title", title); code != 0 {
			t.Fatalf("issue create %s: exit %d: %s", title, code, errOut)
		}
	}
	for _, body := range []string{"one", "two", "three"} {
		if code, _, errOut := run("issue", "comment", "PLAT-1", "-body", body, "-author", "sam"); code != 0 {
			t.Fatalf("comment %s: exit %d: %s", body, code, errOut)
		}
	}

	// Issues: page of one, skipping the first, must be PLAT-2 only.
	code, out, errOut := run("issue", "list", "-limit", "1", "-offset", "1")
	if code != 0 {
		t.Fatalf("issue list -limit 1 -offset 1: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "PLAT-2") || strings.Contains(out, "PLAT-1") || strings.Contains(out, "PLAT-3") {
		t.Fatalf("issue list -limit 1 -offset 1: stdout %q, want only PLAT-2", out)
	}

	// Limit above the maximum is a validation error.
	code, _, errOut = run("issue", "list", "-limit", "500")
	if code != 1 || !strings.Contains(errOut, "invalid input") {
		t.Fatalf("issue list -limit 500: exit %d, stderr %q, want 1 with invalid input", code, errOut)
	}

	// Comments: page of one, skipping two, must be #3 only.
	code, out, errOut = run("issue", "comments", "PLAT-1", "-limit", "1", "-offset", "2")
	if code != 0 {
		t.Fatalf("issue comments PLAT-1 -limit 1 -offset 2: exit %d, stderr %q", code, errOut)
	}
	if !strings.Contains(out, "#3") || strings.Contains(out, "#1") {
		t.Fatalf("issue comments PLAT-1 -limit 1 -offset 2: stdout %q, want only #3", out)
	}

	// Missing ID is a usage error.
	if code, _, _ := run("issue", "comments"); code != 2 {
		t.Fatalf("issue comments with no ID: exit %d, want 2", code)
	}

	// Non-integer limit is a flag error.
	if code, _, _ := run("issue", "comments", "PLAT-1", "-limit", "abc"); code != 2 {
		t.Fatalf("issue comments PLAT-1 -limit abc: exit %d, want 2", code)
	}
}
