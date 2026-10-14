// Package ui serves a one-page HTMX interface for bookmarks.
package ui

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"html/template"
	"net/http"
	"strconv"

	"bookmarks/internal/bookmark"
)

//go:embed page.html
var pageFS embed.FS

var tmpl = template.Must(template.ParseFS(pageFS, "page.html"))

// view is the data the templates render.
type view struct {
	Title     string
	URL       string
	Error     string
	Bookmarks []bookmark.Bookmark
}

type handler struct {
	svc bookmark.Service
	mux *http.ServeMux
}

// NewHandler returns an http.Handler that serves the bookmark UI on top of svc.
func NewHandler(svc bookmark.Service) http.Handler {
	h := &handler{svc: svc, mux: http.NewServeMux()}
	h.mux.HandleFunc("GET /{$}", h.page)
	h.mux.HandleFunc("POST /bookmarks", h.add)
	h.mux.HandleFunc("DELETE /bookmarks/{id}", h.delete)
	return h.mux
}

func (h *handler) page(w http.ResponseWriter, r *http.Request) {
	v, err := h.load(r.Context(), view{})
	if err != nil {
		serviceError(w, err)
		return
	}
	h.render(w, "page", v)
}

func (h *handler) add(w http.ResponseWriter, r *http.Request) {
	title := r.PostFormValue("title")
	url := r.PostFormValue("url")

	v := view{}
	_, err := h.svc.Add(r.Context(), title, url)
	switch {
	case err == nil:
	case errors.Is(err, bookmark.ErrInvalid):
		v.Title = title
		v.URL = url
		v.Error = err.Error()
	default:
		serviceError(w, err)
		return
	}

	v, err = h.load(r.Context(), v)
	if err != nil {
		serviceError(w, err)
		return
	}
	h.render(w, "app", v)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid bookmark id", http.StatusBadRequest)
		return
	}

	err = h.svc.Delete(r.Context(), id)
	if err != nil && !errors.Is(err, bookmark.ErrNotFound) {
		serviceError(w, err)
		return
	}

	v, err := h.load(r.Context(), view{})
	if err != nil {
		serviceError(w, err)
		return
	}
	h.render(w, "app", v)
}

// load fills in the bookmark list on v, keeping the other fields as given.
func (h *handler) load(ctx context.Context, v view) (view, error) {
	list, err := h.svc.List(ctx)
	if err != nil {
		return v, err
	}
	v.Bookmarks = list
	return v, nil
}

// render executes the named template into a buffer first, so that a failure
// produces a clean error response rather than a partial page.
func (h *handler) render(w http.ResponseWriter, name string, v view) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, v); err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// serviceError reports a failure to reach or use the service.
func serviceError(w http.ResponseWriter, err error) {
	http.Error(w, "bookmark service unavailable: "+err.Error(), http.StatusBadGateway)
}
