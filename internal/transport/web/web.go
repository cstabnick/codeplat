// Package web is a read-only browser UI over core.Service: one tab per
// project, a board of that project's issues by stage, and a page per issue.
package web

import (
	_ "embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"codeplat/internal/core"
)

//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("page").Funcs(template.FuncMap{
	"label": func(s any) string {
		switch v := s.(type) {
		case core.Stage:
			return strings.ReplaceAll(string(v), "_", " ")
		case core.Status:
			return strings.ReplaceAll(string(v), "_", " ")
		}
		return ""
	},
	"ts": func(t time.Time) string { return t.Format("2006-01-02 15:04 MST") },
}).Parse(pageHTML))

// page is everything the template renders. Navigation is plain links that
// htmx boosts, and live regions poll their own URL and pick themselves out
// of the response, so the server only ever renders whole pages.
type page struct {
	Title    string
	Projects []core.Project
	Active   string // key of the selected project tab
	Message  string
	Board    []column
	Issue    *core.Issue
	Comments []core.Comment
}

type column struct {
	Name   string
	Issues []core.Issue
}

type handler struct {
	svc *core.Service
	log *slog.Logger
}

func NewHandler(svc *core.Service, log *slog.Logger) http.Handler {
	h := &handler{svc: svc, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /projects/{key}", h.project)
	mux.HandleFunc("GET /issues/{id}", h.issue)
	return mux
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	if len(projects) > 0 {
		http.Redirect(w, r, "/projects/"+projects[0].Key, http.StatusSeeOther)
		return
	}
	h.render(w, http.StatusOK, page{Title: "No projects", Message: "No projects yet. Create one with the CLI, the REST API or the MCP tools."})
}

func (h *handler) project(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	key := strings.ToUpper(r.PathValue("key"))
	var project *core.Project
	for i := range projects {
		if projects[i].Key == key {
			project = &projects[i]
		}
	}
	if project == nil {
		h.render(w, http.StatusNotFound, page{Title: "Not found", Projects: projects, Message: "There is no project " + key + "."})
		return
	}
	issues, err := h.svc.ListIssues(r.Context(), core.IssueFilter{ProjectKey: key})
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, http.StatusOK, page{Title: project.Name, Projects: projects, Active: key, Board: board(issues)})
}

func (h *handler) issue(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	issue, err := h.svc.GetIssue(r.Context(), r.PathValue("id"))
	if errors.Is(err, core.ErrNotFound) {
		h.render(w, http.StatusNotFound, page{Title: "Not found", Projects: projects, Message: "There is no issue " + strings.ToUpper(r.PathValue("id")) + "."})
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	comments, err := h.svc.ListComments(r.Context(), issue.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.render(w, http.StatusOK, page{Title: issue.ID + " " + issue.Title, Projects: projects, Active: issue.ProjectKey, Issue: &issue, Comments: comments})
}

// board sorts issues into one column per stage plus a final column for
// everything that is done, whichever stage it finished in.
func board(issues []core.Issue) []column {
	cols := []column{{Name: "Refinement"}, {Name: "Implementation"}, {Name: "Testing"}, {Name: "Done"}}
	index := map[core.Stage]int{core.StageRefinement: 0, core.StageImplementation: 1, core.StageTesting: 2}
	for _, i := range issues {
		n := index[i.Stage]
		if i.Status == core.StatusDone {
			n = 3
		}
		cols[n].Issues = append(cols[n].Issues, i)
	}
	return cols
}

func (h *handler) render(w http.ResponseWriter, status int, p page) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pageTemplate.Execute(w, p); err != nil {
		h.log.Error("render page", "err", err)
	}
}

func (h *handler) fail(w http.ResponseWriter, err error) {
	h.log.Error("request failed", "err", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
