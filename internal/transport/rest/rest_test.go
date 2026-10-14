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

	var issues []core.Issue
	if code := do(t, h, "GET", "/issues?project=PLAT&status=in_progress&assignee=sam", "", &issues); code != http.StatusOK || len(issues) != 1 {
		t.Fatalf("filtered list = %d, %+v", code, issues)
	}
	if code := do(t, h, "GET", "/issues?status=done", "", &issues); code != http.StatusOK || len(issues) != 0 {
		t.Fatalf("list done = %d, %+v", code, issues)
	}
}
