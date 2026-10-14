package web_test

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	h := web.NewHandler(t.Context(), a.Service, a.Log)

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
		`hx-get="/projects/AAA"`, `hx-trigger="sse:changed, every 60s"`,
		`sse-connect="/events?topic=project.AAA"`,
		// the cards are boosted links; they must not inherit the board's hx-select
		`hx-swap="outerHTML" hx-disinherit="*">`,
		`<script src="https://unpkg.com/htmx-ext-sse@2.2.2/sse.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("project page is missing %q", want)
		}
	}
	if strings.Contains(body, "every 3s") {
		t.Error("project page still polls every 3s")
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
		`sse-connect="/events?topic=issue.AAA-3"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("issue page is missing %q", want)
		}
	}
	if strings.Contains(body, "every 3s") {
		t.Error("issue page still polls every 3s")
	}

	if code, body, _ := get(t, h, "/projects/NOPE"); code != http.StatusNotFound || !strings.Contains(body, "no project NOPE") || !strings.Contains(body, "Alpha") {
		t.Errorf("missing project = %d; want a 404 page that still shows the tabs", code)
	}
	if code, body, _ := get(t, h, "/issues/AAA-99"); code != http.StatusNotFound || !strings.Contains(body, "no issue AAA-99") {
		t.Errorf("missing issue = %d", code)
	}

	if _, err := a.Service.BlockIssue(ctx, "AAA-1", "sam", "no spec"); err != nil {
		t.Fatal(err)
	}
	if code, body, _ := get(t, h, "/projects/aaa"); code != http.StatusOK || !strings.Contains(body, "badge blocked") || !strings.Contains(body, "AAA-1") {
		t.Errorf("project page with a blocked issue = %d; want the blocked badge and AAA-1", code)
	}
	if code, body, _ := get(t, h, "/issues/aaa-1"); code != http.StatusOK || !strings.Contains(body, "badge blocked") {
		t.Errorf("blocked issue page = %d; want the blocked badge", code)
	}
}

func TestWebPaging(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "AAA", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 201; n++ {
		if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "AAA", Title: fmt.Sprintf("issue %d", n)}); err != nil {
			t.Fatal(err)
		}
	}
	for n := 1; n <= 201; n++ {
		if _, err := a.Service.AddComment(ctx, core.NewComment{IssueID: "AAA-1", Author: "test", Body: fmt.Sprintf("comment %d", n)}); err != nil {
			t.Fatal(err)
		}
	}

	code, body, _ := get(t, h, "/projects/AAA")
	if code != http.StatusOK {
		t.Fatalf("project page = %d", code)
	}
	for _, want := range []string{"AAA-201", "AAA-1"} {
		if !strings.Contains(body, `href="/issues/`+want+`"`) {
			t.Errorf("project page is missing issue %s", want)
		}
	}

	code, body, _ = get(t, h, "/issues/AAA-1")
	if code != http.StatusOK {
		t.Fatalf("issue page = %d", code)
	}
	for _, want := range []string{"comment 201", "comment 1", "Comments (201)"} {
		if !strings.Contains(body, want) {
			t.Errorf("issue page is missing %q", want)
		}
	}
}

func TestEventsWithoutTopicIsBadRequest(t *testing.T) {
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)
	if code, _, _ := get(t, h, "/events"); code != http.StatusBadRequest {
		t.Fatalf("GET /events without topic = %d; want 400", code)
	}
}

func TestEventsDeliversNewIssueToProjectTopic(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "AAA", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(web.NewHandler(t.Context(), a.Service, a.Log))
	defer server.Close()

	reqCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, "GET", server.URL+"/events?topic=project.AAA", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q; want text/event-stream", ct)
	}

	// Read lines in the background so each wait can time out.
	lines := make(chan string, 64)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	waitFor := func(want string) {
		t.Helper()
		timeout := time.After(2 * time.Second)
		for {
			select {
			case line, open := <-lines:
				if !open {
					t.Fatalf("stream closed before %q", want)
				}
				if line == want {
					return
				}
			case <-timeout:
				t.Fatalf("timed out waiting for %q", want)
			}
		}
	}

	waitFor(": connected")
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "AAA", Title: "Pushed live"}); err != nil {
		t.Fatal(err)
	}
	waitFor("event: changed")
}

func TestIssuePageShowsBlockers(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "BLK", Name: "Blockers"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "BLK", Title: "Dependency"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "BLK", Title: "Dependent", Epic: 2, DependsOn: []string{"BLK-1"}}); err != nil {
		t.Fatal(err)
	}

	code, body, _ := get(t, h, "/issues/BLK-2")
	if code != http.StatusOK {
		t.Fatalf("issue page = %d", code)
	}
	if !strings.Contains(body, "Epic 2") {
		t.Errorf("issue page is missing %q", "Epic 2")
	}
	blockedAt := strings.Index(body, "Blocked by")
	if blockedAt < 0 {
		t.Fatalf("issue page is missing %q while BLK-1 is open", "Blocked by")
	}
	if !strings.Contains(body[blockedAt:], `href="/issues/BLK-1"`) {
		t.Errorf("issue page does not link BLK-1 under %q", "Blocked by")
	}

	done := core.StatusDone
	if _, err := a.Service.UpdateIssue(ctx, "BLK-1", core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	code, body, _ = get(t, h, "/issues/BLK-2")
	if code != http.StatusOK {
		t.Fatalf("issue page after BLK-1 is done = %d", code)
	}
	if strings.Contains(body, "Blocked by") {
		t.Error("issue page still shows Blocked by after BLK-1 is done")
	}
}
