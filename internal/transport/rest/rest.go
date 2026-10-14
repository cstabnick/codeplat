// Package rest exposes core.Service over HTTP and JSON.
package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"codeplat/internal/core"
	"codeplat/internal/transport/httpauth"
)

const maxBodyBytes = 1 << 20

type handler struct {
	svc *core.Service
	log *slog.Logger
}

func NewHandler(svc *core.Service, auth core.Authenticator, log *slog.Logger) http.Handler {
	h := &handler{svc: svc, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /projects", h.createProject)
	mux.HandleFunc("GET /projects", h.listProjects)
	mux.HandleFunc("POST /projects/{key}/issues", h.createIssue)
	mux.HandleFunc("PUT /projects/{key}/epics", h.setEpic)
	mux.HandleFunc("GET /projects/{key}/epics", h.listEpics)
	mux.HandleFunc("GET /issues", h.listIssues)
	mux.HandleFunc("GET /issues/{id}", h.getIssue)
	mux.HandleFunc("PATCH /issues/{id}", h.updateIssue)
	mux.HandleFunc("POST /issues/{id}/claim", h.claimIssue)
	mux.HandleFunc("POST /issues/{id}/block", h.blockIssue)
	mux.HandleFunc("POST /issues/{id}/comments", h.addComment)
	mux.HandleFunc("GET /issues/{id}/comments", h.listComments)

	top := http.NewServeMux()
	top.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		h.write(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	top.Handle("/", httpauth.Require(auth, mux))
	return top
}

func (h *handler) createProject(w http.ResponseWriter, r *http.Request) {
	var in core.NewProject
	if !h.decode(w, r, &in) {
		return
	}
	p, err := h.svc.CreateProject(r.Context(), in)
	h.respond(w, http.StatusCreated, p, err)
}

func (h *handler) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := h.svc.ListProjects(r.Context())
	h.respond(w, http.StatusOK, projects, err)
}

func (h *handler) createIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		Assignee    string     `json:"assignee"`
		Stage       core.Stage `json:"stage"`
		Parent      string     `json:"parent"`
		Epic        int        `json:"epic"`
		DependsOn   []string   `json:"depends_on"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	i, err := h.svc.CreateIssue(r.Context(), core.NewIssue{
		ProjectKey:  r.PathValue("key"),
		Title:       body.Title,
		Description: body.Description,
		Assignee:    body.Assignee,
		Stage:       body.Stage,
		Parent:      body.Parent,
		Epic:        body.Epic,
		DependsOn:   body.DependsOn,
	})
	h.respond(w, http.StatusCreated, i, err)
}

func (h *handler) setEpic(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Parent string `json:"parent"`
		Number int    `json:"number"`
		Name   string `json:"name"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	e, err := h.svc.SetEpic(r.Context(), core.Epic{
		ProjectKey: r.PathValue("key"),
		Parent:     body.Parent,
		Number:     body.Number,
		Name:       body.Name,
	})
	h.respond(w, http.StatusOK, e, err)
}

func (h *handler) listEpics(w http.ResponseWriter, r *http.Request) {
	epics, err := h.svc.ListEpics(r.Context(), r.PathValue("key"))
	h.respond(w, http.StatusOK, epics, err)
}

func (h *handler) listIssues(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		h.respond(w, http.StatusOK, nil, err)
		return
	}
	q := r.URL.Query()
	issues, err := h.svc.ListIssues(r.Context(), core.IssueFilter{
		ProjectKey: q.Get("project"),
		Stage:      core.Stage(q.Get("stage")),
		Status:     core.Status(q.Get("status")),
		Assignee:   q.Get("assignee"),
		Parent:     q.Get("parent"),
		Ready:      q.Get("ready") == "true",
		Page:       page,
	})
	h.respond(w, http.StatusOK, issues, err)
}

// pageFrom reads the limit and offset query values. An absent value leaves
// the field 0; a value that is not an integer is an ErrInvalid.
func pageFrom(r *http.Request) (core.Page, error) {
	var p core.Page
	q := r.URL.Query()
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return core.Page{}, fmt.Errorf("%w: limit must be an integer", core.ErrInvalid)
		}
		p.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return core.Page{}, fmt.Errorf("%w: offset must be an integer", core.ErrInvalid)
		}
		p.Offset = n
	}
	return p, nil
}

func (h *handler) getIssue(w http.ResponseWriter, r *http.Request) {
	i, err := h.svc.GetIssue(r.Context(), r.PathValue("id"))
	h.respond(w, http.StatusOK, i, err)
}

func (h *handler) updateIssue(w http.ResponseWriter, r *http.Request) {
	var patch core.IssuePatch
	if !h.decode(w, r, &patch) {
		return
	}
	i, err := h.svc.UpdateIssue(r.Context(), r.PathValue("id"), patch)
	h.respond(w, http.StatusOK, i, err)
}

func (h *handler) claimIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Assignee string `json:"assignee"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	i, err := h.svc.ClaimIssue(r.Context(), r.PathValue("id"), body.Assignee)
	h.respond(w, http.StatusOK, i, err)
}

func (h *handler) blockIssue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author string `json:"author"`
		Reason string `json:"reason"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	i, err := h.svc.BlockIssue(r.Context(), r.PathValue("id"), body.Author, body.Reason)
	h.respond(w, http.StatusOK, i, err)
}

func (h *handler) addComment(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Author string `json:"author"`
		Body   string `json:"body"`
	}
	if !h.decode(w, r, &body) {
		return
	}
	c, err := h.svc.AddComment(r.Context(), core.NewComment{
		IssueID: r.PathValue("id"),
		Author:  body.Author,
		Body:    body.Body,
	})
	h.respond(w, http.StatusCreated, c, err)
}

func (h *handler) listComments(w http.ResponseWriter, r *http.Request) {
	page, err := pageFrom(r)
	if err != nil {
		h.respond(w, http.StatusOK, nil, err)
		return
	}
	comments, err := h.svc.ListCommentsPage(r.Context(), r.PathValue("id"), page)
	h.respond(w, http.StatusOK, comments, err)
}

func (h *handler) decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		h.write(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func (h *handler) respond(w http.ResponseWriter, status int, body any, err error) {
	if err == nil {
		h.write(w, status, body)
		return
	}
	switch {
	case errors.Is(err, core.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, core.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, core.ErrConflict):
		status = http.StatusConflict
	default:
		h.log.Error("request failed", "err", err)
		h.write(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	h.write(w, status, map[string]string{"error": err.Error()})
}

func (h *handler) write(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		h.log.Error("write response", "err", err)
	}
}
