// Package web is a read-only browser UI over core.Service: one tab per
// project, a board of that project's issues by stage, and a page per issue.
package web

import (
	"context"
	_ "embed"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"codeplat/internal/bus"
	"codeplat/internal/core"
	"codeplat/internal/transport/web/sse"
)

// feedInterval is how often the web server looks for new events.
const feedInterval = 500 * time.Millisecond

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

	IssueEpic      int
	IssueDepends   []string
	IssueBlockedBy []string
}

type column struct {
	Name   string
	Issues []core.Issue
}

type handler struct {
	svc *core.Service
	log *slog.Logger
}

// NewHandler serves the web UI. It starts a feed that publishes new events to
// the browsers listening on /events until ctx is done.
func NewHandler(ctx context.Context, svc *core.Service, log *slog.Logger) http.Handler {
	h := &handler{svc: svc, log: log}
	b := bus.New()
	after, err := svc.LastEventID(ctx)
	if err != nil {
		log.Error("reading last event ID", "err", err)
		after = 0
	}
	go feed(ctx, svc, b, after, feedInterval, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.home)
	mux.HandleFunc("GET /projects/{key}", h.project)
	mux.HandleFunc("GET /issues/{id}", h.issue)
	mux.Handle("GET /events", sse.Handler(b))
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
	issues, err := all(func(p core.Page) ([]core.Issue, error) {
		return h.svc.ListIssues(r.Context(), core.IssueFilter{ProjectKey: key, Page: p})
	})
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
	comments, err := all(func(p core.Page) ([]core.Comment, error) {
		return h.svc.ListCommentsPage(r.Context(), issue.ID, p)
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	others, err := all(func(p core.Page) ([]core.Issue, error) {
		return h.svc.ListIssues(r.Context(), core.IssueFilter{Page: p})
	})
	if err != nil {
		h.fail(w, err)
		return
	}
	blockers := core.BlockedBy(issue, others)
	h.render(w, http.StatusOK, page{
		Title:          issue.ID + " " + issue.Title,
		Projects:       projects,
		Active:         issue.ProjectKey,
		Issue:          &issue,
		Comments:       comments,
		IssueEpic:      issue.Epic,
		IssueDepends:   issue.DependsOn,
		IssueBlockedBy: blockers,
	})
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

// all reads every page of a list. fetch is called with pages of
// core.MaxPageLimit items at increasing offsets. It stops at the first page
// that is shorter than the limit, and returns everything it read.
func all[T any](fetch func(core.Page) ([]T, error)) ([]T, error) {
	var out []T
	for offset := 0; ; offset += core.MaxPageLimit {
		items, err := fetch(core.Page{Limit: core.MaxPageLimit, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
		if len(items) < core.MaxPageLimit {
			return out, nil
		}
	}
}
