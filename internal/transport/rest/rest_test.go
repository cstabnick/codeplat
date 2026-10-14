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

// send serves one request with "Authorization: Bearer token" when token is not
// "", and returns the recorder without decoding the body.
func send(h http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	h.ServeHTTP(rec, req)
	return rec
}

func TestREST(t *testing.T) {
	a := apptest.New(t)
	h := rest.NewHandler(a.Service, a.Auth, a.Log)

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

	if code := do(t, h, "POST", "/issues/PLAT-1/block", `{"author":"sam","reason":""}`, nil); code != http.StatusBadRequest {
		t.Fatalf("block with empty reason = %d; want 400", code)
	}
	if code := do(t, h, "POST", "/issues/PLAT-9/block", `{"author":"sam","reason":"no spec"}`, nil); code != http.StatusNotFound {
		t.Fatalf("block missing issue = %d; want 404", code)
	}
	if code := do(t, h, "POST", "/issues/PLAT-1/block", `{"author":"sam","reason":"no spec"}`, &issue); code != http.StatusOK || issue.Status != core.StatusBlocked || issue.Assignee != "sam" {
		t.Fatalf("block = %d, %+v; want 200, blocked, assignee sam", code, issue)
	}
	if code := do(t, h, "GET", "/issues?status=blocked", "", &issues); code != http.StatusOK || len(issues) != 1 || issues[0].ID != "PLAT-1" {
		t.Fatalf("list blocked = %d, %+v; want PLAT-1 only", code, issues)
	}
}

func TestRESTPaging(t *testing.T) {
	a := apptest.New(t)
	h := rest.NewHandler(a.Service, a.Auth, a.Log)

	if code := do(t, h, "POST", "/projects", `{"key":"plat","name":"Platform"}`, nil); code != http.StatusCreated {
		t.Fatalf("create project = %d", code)
	}
	for _, title := range []string{"One", "Two", "Three"} {
		if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"`+title+`"}`, nil); code != http.StatusCreated {
			t.Fatalf("create issue %s = %d", title, code)
		}
	}
	for _, body := range []string{"c1", "c2", "c3"} {
		if code := do(t, h, "POST", "/issues/PLAT-1/comments", `{"body":"`+body+`"}`, nil); code != http.StatusCreated {
			t.Fatalf("add comment %s = %d", body, code)
		}
	}

	var issues []core.Issue
	if code := do(t, h, "GET", "/issues?limit=2&offset=1", "", &issues); code != http.StatusOK || len(issues) != 2 || issues[0].ID != "PLAT-2" || issues[1].ID != "PLAT-3" {
		t.Fatalf("page limit=2 offset=1 = %d, %+v; want PLAT-2 and PLAT-3", code, issues)
	}
	issues = nil
	if code := do(t, h, "GET", "/issues", "", &issues); code != http.StatusOK || len(issues) != 3 {
		t.Fatalf("unpaged list = %d, %+v; want 3 issues", code, issues)
	}
	if code := do(t, h, "GET", "/issues?limit=201", "", nil); code != http.StatusBadRequest {
		t.Fatalf("limit=201 = %d; want 400", code)
	}
	if code := do(t, h, "GET", "/issues?offset=-1", "", nil); code != http.StatusBadRequest {
		t.Fatalf("offset=-1 = %d; want 400", code)
	}
	var badLimit map[string]string
	if code := do(t, h, "GET", "/issues?limit=abc", "", &badLimit); code != http.StatusBadRequest || badLimit["error"] != "invalid input: limit must be an integer" {
		t.Fatalf("limit=abc = %d, %+v; want 400 with invalid input: limit must be an integer", code, badLimit)
	}

	var comments []core.Comment
	if code := do(t, h, "GET", "/issues/PLAT-1/comments?limit=1&offset=1", "", &comments); code != http.StatusOK || len(comments) != 1 || comments[0].Body != "c2" {
		t.Fatalf("comment page = %d, %+v; want only comment 2", code, comments)
	}
	if code := do(t, h, "GET", "/issues/PLAT-1/comments?limit=500", "", nil); code != http.StatusBadRequest {
		t.Fatalf("comments limit=500 = %d; want 400", code)
	}
}

func TestRESTEpicsAndReady(t *testing.T) {
	a := apptest.New(t)
	h := rest.NewHandler(a.Service, a.Auth, a.Log)

	if code := do(t, h, "POST", "/projects", `{"key":"plat","name":"Platform"}`, nil); code != http.StatusCreated {
		t.Fatalf("create project = %d", code)
	}
	var base core.Issue
	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"Base"}`, &base); code != http.StatusCreated || base.ID != "PLAT-1" {
		t.Fatalf("create base = %d, %+v; want 201 and PLAT-1", code, base)
	}

	var second core.Issue
	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"Second","epic":2,"depends_on":["PLAT-1"]}`, &second); code != http.StatusCreated {
		t.Fatalf("create with epic and depends_on = %d, %+v; want 201", code, second)
	}
	if second.ID != "PLAT-2" || second.Epic != 2 || len(second.DependsOn) != 1 || second.DependsOn[0] != "PLAT-1" {
		t.Fatalf("created issue = %+v; want PLAT-2, epic 2, depends_on [PLAT-1]", second)
	}

	if code := do(t, h, "POST", "/projects/PLAT/issues", `{"title":"x","depends_on":["PLAT-9"]}`, nil); code != http.StatusBadRequest {
		t.Fatalf("missing dependency = %d; want 400", code)
	}

	if code := do(t, h, "PATCH", "/issues/PLAT-1", `{"depends_on":["PLAT-2"]}`, nil); code != http.StatusBadRequest {
		t.Fatalf("dependency cycle = %d; want 400", code)
	}

	var ready []core.Issue
	if code := do(t, h, "GET", "/issues?project=PLAT&ready=true", "", &ready); code != http.StatusOK || len(ready) != 1 || ready[0].ID != "PLAT-1" {
		t.Fatalf("ready list = %d, %+v; want PLAT-1 only", code, ready)
	}
	var all []core.Issue
	if code := do(t, h, "GET", "/issues?project=PLAT", "", &all); code != http.StatusOK || len(all) != 2 {
		t.Fatalf("project list = %d, %+v; want both issues", code, all)
	}
}

func TestRESTParentsAndEpics(t *testing.T) {
	a := apptest.New(t)
	h := rest.NewHandler(a.Service, a.Auth, a.Log)

	if code := do(t, h, "POST", "/projects", `{"key":"plat","name":"Platform"}`, nil); code != http.StatusCreated {
		t.Fatalf("create project = %d", code)
	}
	if code := do(t, h, "POST", "/projects", `{"key":"other","name":"Other"}`, nil); code != http.StatusCreated {
		t.Fatalf("create other project = %d", code)
	}
	if code := do(t, h, "POST", "/projects/plat/issues", `{"title":"Base"}`, nil); code != http.StatusCreated {
		t.Fatalf("create base = %d", code)
	}

	var part core.Issue
	if code := do(t, h, "POST", "/projects/plat/issues", `{"title":"Part","parent":"plat-1"}`, &part); code != http.StatusCreated || part.Parent != "PLAT-1" {
		t.Fatalf("create with parent = %d, %+v; want 201 and parent PLAT-1", code, part)
	}
	if code := do(t, h, "POST", "/projects/plat/issues", `{"title":"Orphan","parent":"PLAT-9"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("create with missing parent = %d; want 400", code)
	}

	var children []core.Issue
	if code := do(t, h, "GET", "/issues?parent=PLAT-1", "", &children); code != http.StatusOK || len(children) != 1 || children[0].ID != "PLAT-2" {
		t.Fatalf("children of PLAT-1 = %d, %+v; want PLAT-2 only", code, children)
	}

	if code := do(t, h, "PATCH", "/issues/PLAT-2", `{"parent":""}`, &part); code != http.StatusOK || part.Parent != "" {
		t.Fatalf("clear parent = %d, %+v; want 200 and empty parent", code, part)
	}

	var epic core.Epic
	if code := do(t, h, "PUT", "/projects/plat/epics", `{"parent":"PLAT-1","number":1,"name":"Core"}`, &epic); code != http.StatusOK ||
		epic.ProjectKey != "PLAT" || epic.Parent != "PLAT-1" || epic.Number != 1 || epic.Name != "Core" {
		t.Fatalf("set epic = %d, %+v; want 200 with PLAT, PLAT-1, 1, Core", code, epic)
	}
	if code := do(t, h, "PUT", "/projects/plat/epics", `{"parent":"PLAT-1","number":0,"name":"Zero"}`, nil); code != http.StatusBadRequest {
		t.Fatalf("epic number 0 = %d; want 400", code)
	}
	if code := do(t, h, "PUT", "/projects/nope/epics", `{"number":1,"name":"Core"}`, nil); code != http.StatusNotFound {
		t.Fatalf("epic in missing project = %d; want 404", code)
	}

	var epics []core.Epic
	if code := do(t, h, "GET", "/projects/plat/epics", "", &epics); code != http.StatusOK || len(epics) != 1 || epics[0].Name != "Core" {
		t.Fatalf("list epics = %d, %+v; want the Core epic only", code, epics)
	}
	rec := send(h, "", "GET", "/projects/other/epics", "")
	if rec.Code != http.StatusOK || rec.Body.String() != "[]\n" {
		t.Fatalf("list epics of other = %d, %q; want 200 and []", rec.Code, rec.Body.String())
	}
}

func TestRESTRequiresToken(t *testing.T) {
	a := apptest.NewWithAuth(t)
	h := rest.NewHandler(a.Service, a.Auth, a.Log)

	if rec := send(h, "", "GET", "/healthz", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz without token = %d; want 200", rec.Code)
	}

	for _, token := range []string{"", "wrong-token"} {
		rec := send(h, token, "GET", "/projects", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET /projects with token %q = %d; want 401", token, rec.Code)
		}
		if token == "" {
			if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="codeplat"` {
				t.Fatalf("WWW-Authenticate = %q", got)
			}
			if rec.Body.String() != "{\"error\":\"unauthorized\"}\n" {
				t.Fatalf("401 body = %q", rec.Body.String())
			}
		}
	}

	alice := apptest.Tokens["alice"]
	if rec := send(h, alice, "POST", "/projects", `{"key":"plat","name":"Platform"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create project as alice = %d, %s; want 201", rec.Code, rec.Body.String())
	}
	rec := send(h, alice, "POST", "/projects/PLAT/issues", `{"title":"First"}`)
	var issue core.Issue
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &issue) != nil || issue.Assignee != "alice" {
		t.Fatalf("create issue as alice = %d, %s; want 201 with assignee alice", rec.Code, rec.Body.String())
	}
	if rec := send(h, alice, "POST", "/issues/PLAT-1/comments", `{"author":"bob","body":"x"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("comment as bob with alice token = %d; want 400", rec.Code)
	}
	rec = send(h, alice, "POST", "/issues/PLAT-1/comments", `{"author":"","body":"x"}`)
	var comment core.Comment
	if rec.Code != http.StatusCreated || json.Unmarshal(rec.Body.Bytes(), &comment) != nil || comment.Author != "alice" {
		t.Fatalf("comment with empty author = %d, %s; want 201 with author alice", rec.Code, rec.Body.String())
	}

	bob := apptest.Tokens["bob"]
	rec = send(h, bob, "POST", "/issues/PLAT-1/claim", `{"assignee":""}`)
	issue = core.Issue{}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &issue) != nil || issue.Assignee != "bob" {
		t.Fatalf("claim as bob = %d, %s; want 200 with assignee bob", rec.Code, rec.Body.String())
	}
}
