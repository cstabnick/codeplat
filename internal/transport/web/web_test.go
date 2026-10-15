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

func TestBoardGroupsByGoalAndEpic(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "GRP", Name: "Grouped"}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []core.NewIssue{
		{ProjectKey: "GRP", Title: "Goal one"},
		{ProjectKey: "GRP", Title: "Part in epic one", Parent: "GRP-1", Epic: 1},
		{ProjectKey: "GRP", Title: "Part in epic two", Parent: "GRP-1", Epic: 2},
		{ProjectKey: "GRP", Title: "Loose issue"},
	} {
		if _, err := a.Service.CreateIssue(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Service.SetEpic(ctx, core.Epic{ProjectKey: "GRP", Parent: "GRP-1", Number: 1, Name: "Core"}); err != nil {
		t.Fatal(err)
	}
	done := core.StatusDone
	if _, err := a.Service.UpdateIssue(ctx, "GRP-2", core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}

	code, body, _ := get(t, h, "/projects/GRP")
	if code != http.StatusOK {
		t.Fatalf("project page = %d", code)
	}
	for _, want := range []string{
		`<a href="/issues/GRP-1">GRP-1 · Goal one</a>`,
		"1/2 done",
		"Epic 1 · Core",
		"Epic 2",
		"Other issues",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("project page is missing %q", want)
		}
	}
	if n := strings.Count(body, `href="/issues/GRP-1"`); n != 1 {
		t.Errorf("GRP-1 links on the page = %d; want 1, in the lane heading only", n)
	}
	epicOne := strings.Index(body, "Epic 1 · Core")
	epicTwo := strings.Index(body, "Epic 2")
	other := strings.Index(body, "Other issues")
	loose := strings.Index(body, `href="/issues/GRP-4"`)
	if !(epicOne >= 0 && epicOne < epicTwo && epicTwo < other && other < loose) {
		t.Errorf("order is wrong: Epic 1 at %d, Epic 2 at %d, Other issues at %d, GRP-4 at %d", epicOne, epicTwo, other, loose)
	}
	if !strings.Contains(body, `<details class="lane" open>`) {
		t.Error("the lane of an unfinished goal is not open")
	}

	if _, err := a.Service.UpdateIssue(ctx, "GRP-3", core.IssuePatch{Status: &done}); err != nil {
		t.Fatal(err)
	}
	code, body, _ = get(t, h, "/projects/GRP")
	if code != http.StatusOK {
		t.Fatalf("project page after all parts are done = %d", code)
	}
	if !strings.Contains(body, "2/2 done") {
		t.Error("project page is missing 2/2 done after all parts are done")
	}
	if n := strings.Count(body, `<details class="lane" open>`); n != 1 {
		t.Errorf("open lanes = %d; want 1 (Other issues only) once the goal is done", n)
	}
	if !strings.Contains(body, `<details class="lane">`) {
		t.Error("the lane of the finished goal is not closed")
	}

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "NOG", Name: "No goals"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "NOG", Title: "Plain issue"}); err != nil {
		t.Fatal(err)
	}
	code, body, _ = get(t, h, "/projects/NOG")
	if code != http.StatusOK {
		t.Fatalf("project without goals = %d", code)
	}
	if strings.Contains(body, `<details class="lane"`) {
		t.Error("a project without goals has a lane")
	}
	if strings.Contains(body, "Other issues") {
		t.Error("a project without goals shows Other issues")
	}
	if !strings.Contains(body, `href="/issues/NOG-1"`) {
		t.Error("a project without goals is missing its card")
	}
}

func TestBoardEmptyProjectKeepsColumns(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "EMP", Name: "Empty"}); err != nil {
		t.Fatal(err)
	}
	code, body, _ := get(t, h, "/projects/EMP")
	if code != http.StatusOK {
		t.Fatalf("empty project page = %d", code)
	}
	for _, want := range []string{"<h2>Refinement", "<h2>Implementation", "<h2>Testing", "<h2>Done"} {
		if !strings.Contains(body, want) {
			t.Errorf("empty project page is missing column %q", want)
		}
	}
}

func TestIssuePageShowsGoalAndParts(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "PRT", Name: "Prototype"}); err != nil {
		t.Fatal(err)
	}
	for _, in := range []core.NewIssue{
		{ProjectKey: "PRT", Title: "Goal"},
		{ProjectKey: "PRT", Title: "First part", Parent: "PRT-1", Epic: 1},
		{ProjectKey: "PRT", Title: "Second part", Parent: "PRT-1", Epic: 2},
		{ProjectKey: "PRT", Title: "Unrelated"},
	} {
		if _, err := a.Service.CreateIssue(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Service.SetEpic(ctx, core.Epic{ProjectKey: "PRT", Parent: "PRT-1", Number: 1, Name: "Core"}); err != nil {
		t.Fatal(err)
	}

	code, body, _ := get(t, h, "/issues/PRT-2")
	if code != http.StatusOK {
		t.Fatalf("part page = %d", code)
	}
	partOf := strings.Index(body, "Part of")
	if partOf < 0 {
		t.Fatal("part page is missing Part of")
	}
	if !strings.Contains(body[partOf:], `href="/issues/PRT-1"`) {
		t.Errorf("part page does not link PRT-1 under Part of")
	}
	if strings.Contains(body, ">Parts<") {
		t.Error("part page should not list parts")
	}

	code, body, _ = get(t, h, "/issues/PRT-1")
	if code != http.StatusOK {
		t.Fatalf("goal page = %d", code)
	}
	if strings.Contains(body, "Part of") {
		t.Error("goal page should not say Part of")
	}
	// The blockers line also links the parts, so only look after the heading.
	heading := strings.Index(body, ">Parts<")
	if heading < 0 {
		t.Fatal("goal page is missing Parts")
	}
	rest := body[heading:]
	positions := []int{
		0, strings.Index(rest, "Epic 1 · Core"),
		strings.Index(rest, `href="/issues/PRT-2"`), strings.Index(rest, "Epic 2"),
		strings.Index(rest, `href="/issues/PRT-3"`),
	}
	for n := 1; n < len(positions); n++ {
		if positions[n-1] < 0 || positions[n] < positions[n-1] {
			t.Fatalf("goal page parts are out of order: %v", positions)
		}
	}

	code, body, _ = get(t, h, "/issues/PRT-4")
	if code != http.StatusOK {
		t.Fatalf("unrelated page = %d", code)
	}
	if strings.Contains(body, "Part of") || strings.Contains(body, ">Parts<") {
		t.Error("issue with no parent and no parts should show neither Part of nor Parts")
	}
}

func TestProjectsIndex(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	code, body, _ := get(t, h, "/projects")
	if code != http.StatusOK {
		t.Fatalf("empty /projects = %d; want 200", code)
	}
	if !strings.Contains(body, "<title>Projects · codeplat</title>") {
		t.Errorf("empty /projects is missing the Projects title")
	}

	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "AAA", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: "BBB", Name: "Beta"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "AAA", Title: "Listed"}); err != nil {
		t.Fatal(err)
	}

	code, body, _ = get(t, h, "/projects")
	if code != http.StatusOK {
		t.Fatalf("/projects = %d; want 200", code)
	}
	if !strings.Contains(body, "<title>Projects · codeplat</title>") {
		t.Errorf("/projects is missing the Projects title")
	}
	for _, want := range []string{`href="/projects/AAA"`, `href="/projects/BBB"`} {
		if !strings.Contains(body, want) {
			t.Errorf("/projects is missing %q", want)
		}
	}
}

func TestPagesRenderWithSwitcher(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	for _, key := range []string{"AAA", "BBB", "CCC", "DDD", "EEE", "FFF", "GGG"} {
		if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: key, Name: "Project " + key}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "AAA", Title: "First"}); err != nil {
		t.Fatal(err)
	}

	// The project page renders, with the selected project in the tabs.
	code, body, _ := get(t, h, "/projects/GGG")
	if code != http.StatusOK {
		t.Fatalf("project page = %d; want 200", code)
	}
	if !strings.Contains(body, `<a class="tab active" href="/projects/GGG">`) {
		t.Error("project page does not show GGG as the active tab")
	}

	// The issue page renders with its project's tab active.
	code, body, _ = get(t, h, "/issues/AAA-1")
	if code != http.StatusOK {
		t.Fatalf("issue page = %d; want 200", code)
	}
	if !strings.Contains(body, `<a class="tab active" href="/projects/AAA">`) {
		t.Error("issue page does not show AAA as the active tab")
	}

	// Both not-found pages still render, with the switcher.
	code, body, _ = get(t, h, "/projects/NOPE")
	if code != http.StatusNotFound || !strings.Contains(body, "no project NOPE") || !strings.Contains(body, `href="/projects/AAA"`) {
		t.Errorf("missing project = %d; want a 404 page with the switcher", code)
	}
	code, body, _ = get(t, h, "/issues/ZZZ-9")
	if code != http.StatusNotFound || !strings.Contains(body, "no issue ZZZ-9") || !strings.Contains(body, `href="/projects/AAA"`) {
		t.Errorf("missing issue = %d; want a 404 page with the switcher", code)
	}
}

func TestSwitcherWithManyProjects(t *testing.T) {
	ctx := context.Background()
	a := apptest.New(t)
	h := web.NewHandler(t.Context(), a.Service, a.Log)

	keys := []string{"AAA", "BBB", "CCC", "DDD", "EEE", "FFF", "GGG", "HTM"}
	for _, key := range keys {
		name := "Project " + key
		if key == "HTM" {
			name = "<b>Bad</b> name"
		}
		if _, err := a.Service.CreateProject(ctx, core.NewProject{Key: key, Name: name}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.Service.CreateIssue(ctx, core.NewIssue{ProjectKey: "AAA", Title: "Listed"}); err != nil {
		t.Fatal(err)
	}

	code, body, _ := get(t, h, "/projects/AAA")
	if code != http.StatusOK {
		t.Fatalf("project page = %d; want 200", code)
	}
	if n := strings.Count(body, `<a class="tab`); n != 6 {
		t.Errorf("header shows %d tabs; want 6", n)
	}
	if !strings.Contains(body, `<a class="tab active" href="/projects/AAA">`) {
		t.Error("selected project AAA is not an active tab")
	}
	if n := strings.Count(body, `class="project-link`); n != len(keys) {
		t.Errorf("switcher lists %d projects; want %d", n, len(keys))
	}
	if n := strings.Count(body, `<script>`); n != 1 {
		t.Errorf("project page has %d inline scripts; want 1", n)
	}
	if strings.Contains(body, "<b>Bad</b>") || !strings.Contains(body, "&lt;b&gt;Bad") {
		t.Error("project name is not escaped in the page")
	}

	code, body, _ = get(t, h, "/projects")
	if code != http.StatusOK {
		t.Fatalf("/projects = %d; want 200", code)
	}
	if n := strings.Count(body, `<tr data-filter=`); n != len(keys) {
		t.Errorf("/projects lists %d rows; want %d", n, len(keys))
	}
	if !strings.Contains(body, `<td>1</td>`) {
		t.Error("/projects does not show the open count of AAA")
	}
	if strings.Contains(body, "<b>Bad</b>") || !strings.Contains(body, "&lt;b&gt;Bad") {
		t.Error("project name is not escaped on /projects")
	}
	if n := strings.Count(body, `<script>`); n != 1 {
		t.Errorf("/projects has %d inline scripts; want 1", n)
	}
}
