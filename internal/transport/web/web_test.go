package web_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeplat/internal/apptest"
	"codeplat/internal/core"
	"codeplat/internal/transport/web"
)

func get(t *testing.T, h http.Handler, path string) (int, string, http.Header) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String(), rec.Header()
}

func TestWeb(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(a.Service, a.Log)

	if code, body, _ := get(t, h, "/"); code != http.StatusOK || !strings.Contains(body, "No projects yet") {
		t.Fatalf("empty home = %d\n%s", code, body)
	}

	for _, p := range []core.NewProject{{Key: "AAA", Name: "Alpha"}, {Key: "BBB", Name: "Beta"}} {
		if _, err := a.Service.CreateProject(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for _, in := range []core.NewIssue{
		{ProjectKey: "AAA", Title: "Waiting for refinement"},
		{ProjectKey: "AAA", Title: "Being built", Stage: core.StageImplementation},
		{ProjectKey: "AAA", Title: "<script>alert(1)</script>", Stage: core.StageTesting, Description: "Line one\n<b>not bold</b>"},
		{ProjectKey: "BBB", Title: "Other project's issue"},
	} {
		if _, err := a.Service.CreateIssue(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Service.ClaimIssue(ctx, "AAA-2", "worker-7"); err != nil {
		t.Fatal(err)
	}
	done := core.StatusDone
	if _, err := a.Service.UpdateIssue(ctx, "AAA-3", core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.AddComment(ctx, core.NewComment{IssueID: "AAA-3", Author: "test-1", Body: "all checks pass"}); err != nil {
		t.Fatal(err)
	}

	code, _, header := get(t, h, "/")
	if code != http.StatusSeeOther || header.Get("Location") != "/projects/AAA" {
		t.Fatalf("home = %d to %q; want redirect to the first project", code, header.Get("Location"))
	}

	code, body, _ := get(t, h, "/projects/aaa")
	if code != http.StatusOK {
		t.Fatalf("project page = %d", code)
	}
	for _, want := range []string{
		`<a class="tab active" href="/projects/AAA">Alpha`, // the selected tab
		`<a class="tab" href="/projects/BBB">Beta`,         // the other tab
		`href="/issues/AAA-1"`, "Waiting for refinement",
		"worker-7", `class="badge in_progress">in progress<`,
		"&lt;script&gt;alert(1)&lt;/script&gt;",
		`hx-get="/projects/AAA"`, `hx-trigger="every 3s"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("project page is missing %q", want)
		}
	}
	for _, reject := range []string{"<script>alert(1)</script>", "Other project's issue", "Other project&#39;s issue"} {
		if strings.Contains(body, reject) {
			t.Errorf("project page should not contain %q", reject)
		}
	}
	// Columns appear in pipeline order, and a done issue sits under Done
	// whatever stage it finished in.
	positions := []int{
		strings.Index(body, "Refinement"), strings.Index(body, "AAA-1"),
		strings.Index(body, "Implementation"), strings.Index(body, "AAA-2"),
		strings.Index(body, "Testing"), strings.Index(body, "Done"), strings.Index(body, "AAA-3"),
	}
	for n := 1; n < len(positions); n++ {
		if positions[n-1] < 0 || positions[n] < positions[n-1] {
			t.Fatalf("board is out of order: %v", positions)
		}
	}

	code, body, _ = get(t, h, "/issues/aaa-3")
	if code != http.StatusOK {
		t.Fatalf("issue page = %d", code)
	}
	for _, want := range []string{
		`<a class="tab active" href="/projects/AAA">`, `href="/projects/AAA">&larr; Back to board`,
		"Line one\n&lt;b&gt;not bold&lt;/b&gt;", "Comments (1)", "test-1", "all checks pass",
		`class="badge">testing<`, `class="badge done">done<`, `hx-get="/issues/AAA-3"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue page is missing %q", want)
		}
	}

	if code, body, _ := get(t, h, "/projects/NOPE"); code != http.StatusNotFound || !strings.Contains(body, "no project NOPE") || !strings.Contains(body, "Alpha") {
		t.Errorf("missing project = %d; want a 404 page that still shows the tabs", code)
	}
	if code, body, _ := get(t, h, "/issues/AAA-99"); code != http.StatusNotFound || !strings.Contains(body, "no issue AAA-99") {
		t.Errorf("missing issue = %d", code)
	}
}
