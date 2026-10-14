package rest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"codeplat/internal/apptest"
	"codeplat/internal/core"
	"codeplat/internal/transport/rest"
)

func do(t *testing.T, h http.Handler, method, path, body string, out any) int {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	if out != nil {
		if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
			t.Fatalf("%s %s: decode %q: %v", method, path, rec.Body.String(), err)
		}
	}
	return rec.Code
}

func TestREST(t *testing.T) {
	a := apptest.New(t)
	h := rest.NewHandler(a.Service, a.Log)

	if code := do(t, h, "GET", "/healthz", "", nil); code != http.StatusOK {
		t.Fatalf("healthz = %d", code)
	}

	var projects []core.Project
	if code := do(t, h, "GET", "/projects", "", &projects); code != http.StatusOK || projects == nil {
		t.Fatalf("empty list = %d, %v; want 200 and []", code, projects)
	}

	var project core.Project
	if code := do(t, h, "POST", "/projects", `{"key":"plat","name":"Platform"}`, &project); code != http.StatusCreated || project.Key != "PLAT" {
		t.Fatalf("create project = %d, %+v", code, project)
	}
	if code := do(t, h, "POST", "/projects", `{"key":"plat","name":"Platform"}`, nil); code != http.StatusConflict {
		t.Fatalf("duplicate project = %d; want 409", code)
	}
	if code := do(t, h, "POST", "/projects", `{"key":"plat","nope":1}`, nil); code != http.StatusBadRequest {
		t.Fatalf("unknown field = %d; want 400", code)
	}
	if code := do(t, h, "POST", "/projects", `{"key":"!","name":"x"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("invalid key = %d; want 400", code)
	}

	var issue core.Issue
	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"First","assignee":"sam"}`, &issue); code != http.StatusCreated || issue.ID != "PLAT-1" {
		t.Fatalf("create issue = %d, %+v", code, issue)
	}
	if code := do(t, h, "POST", "/projects/NOPE/issues", `{"title":"x"}`, nil); code != http.StatusNotFound {
		t.Fatalf("issue in missing project = %d; want 404", code)
	}

	if code := do(t, h, "PATCH", "/issues/PLAT-1", `{"status":"in_progress"}`, &issue); code != http.StatusOK || issue.Status != core.StatusInProgress || issue.Assignee != "sam" {
		t.Fatalf("patch = %d, %+v", code, issue)
	}
	if code := do(t, h, "PATCH", "/issues/PLAT-1", `{"status":"closed"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d; want 400", code)
	}

	if code := do(t, h, "GET", "/issues/PLAT-1", "", &issue); code != http.StatusOK || issue.Status != core.StatusInProgress {
		t.Fatalf("get = %d, %+v", code, issue)
	}
	if code := do(t, h, "GET", "/issues/PLAT-9", "", nil); code != http.StatusNotFound {
		t.Fatalf("get missing = %d; want 404", code)
	}

	var staged core.Issue
	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"Second","stage":"testing"}`, &staged); code != http.StatusCreated || staged.Stage != core.StageTesting {
		t.Fatalf("create with stage = %d, %+v", code, staged)
	}
	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"x","stage":"review"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("bad stage = %d; want 400", code)
	}
	var queue []core.Issue
	if code := do(t, h, "GET", "/issues?stage=testing&status=todo", "", &queue); code != http.StatusOK || len(queue) != 1 || queue[0].ID != "PLAT-2" {
		t.Fatalf("stage queue = %d, %+v", code, queue)
	}
	if code := do(t, h, "POST", "/issues/PLAT-2/claim", `{"assignee":"tester"}`, &staged); code != http.StatusOK || staged.Status != core.StatusInProgress || staged.Assignee != "tester" {
		t.Fatalf("claim = %d, %+v", code, staged)
	}
	if code := do(t, h, "POST", "/issues/PLAT-2/claim", `{"assignee":"other"}`, nil); code != http.StatusConflict {
		t.Fatalf("second claim = %d; want 409", code)
	}
	if code := do(t, h, "PATCH", "/issues/PLAT-2", `{"stage":"implementation"}`, &staged); code != http.StatusOK || staged.Stage != core.StageImplementation {
		t.Fatalf("patch stage = %d, %+v", code, staged)
	}

	var comment core.Comment
	if code := do(t, h, "POST", "/issues/plat-1/comments", `{"author":"sam","body":"on it"}`, &comment); code != http.StatusCreated || comment.Number != 1 || comment.IssueID != "PLAT-1" {
		t.Fatalf("add comment = %d, %+v", code, comment)
	}
	if code := do(t, h, "POST", "/issues/PLAT-1/comments", `{"body":""}`, nil); code != http.StatusBadRequest {
		t.Fatalf("empty comment = %d; want 400", code)
	}
	if code := do(t, h, "POST", "/issues/PLAT-9/comments", `{"body":"x"}`, nil); code != http.StatusNotFound {
		t.Fatalf("comment on missing issue = %d; want 404", code)
	}
	var comments []core.Comment
	if code := do(t, h, "GET", "/issues/PLAT-1/comments", "", &comments); code != http.StatusOK || len(comments) != 1 || comments[0].Body != "on it" {
		t.Fatalf("list comments = %d, %+v", code, comments)
	}
	if code := do(t, h, "GET", "/issues/PLAT-9/comments", "", nil); code != http.StatusNotFound {
		t.Fatalf("comments of missing issue = %d; want 404", code)
	}

	var issues []core.Issue
	if code := do(t, h, "GET", "/issues?project=PLAT&status=in_progress&assignee=sam", "", &issues); code != http.StatusOK || len(issues) != 1 {
		t.Fatalf("filtered list = %d, %+v", code, issues)
	}
	if code := do(t, h, "GET", "/issues?status=done", "", &issues); code != http.StatusOK || len(issues) != 0 {
		t.Fatalf("list done = %d, %+v", code, issues)
	}
}
