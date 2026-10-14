// Package ui serves a one-page HTMX interface for llmrouter.
package ui

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"llmrouter/internal/llm"
)

//go:embed page.html
var pageFS embed.FS

var tmpl = template.Must(template.New("page.html").Funcs(template.FuncMap{
	"clock": func(t time.Time) string { return t.Format("15:04:05") },
}).ParseFS(pageFS, "page.html"))

// view is the data the template renders.
type view struct {
	Backends    []llm.BackendStatus
	Routes      []llm.Route
	Default     string
	Models      []backendModels
	ModelsNote  string
	ModelsError string
	Prompt      string
	Try         *llm.Decision
	TryError    string
	Decisions   []llm.Decision
}

// backendModels is the model list of one backend.
type backendModels struct {
	Name    string
	Manager bool
	Models  []llm.Model
	Error   string
}

type handler struct {
	fleet  llm.Fleet
	router llm.Router
}

// NewHandler serves the web page over the fleet and router.
func NewHandler(f llm.Fleet, r llm.Router) http.Handler {
	h := &handler{fleet: f, router: r}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", h.page)
	mux.HandleFunc("POST /try", h.try)
	mux.HandleFunc("POST /models/pull", h.pull)
	mux.HandleFunc("POST /models/delete", h.delete)
	return mux
}

func (h *handler) page(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, view{})
}

func (h *handler) try(w http.ResponseWriter, r *http.Request) {
	prompt := r.PostFormValue("prompt")
	v := view{Prompt: prompt}
	if strings.TrimSpace(prompt) == "" {
		v.TryError = "Enter a prompt."
	} else {
		d := h.router.Route(r.Context(), []llm.Message{{Role: "user", Content: prompt}})
		v.Try = &d
	}
	h.render(w, r, v)
}

func (h *handler) pull(w http.ResponseWriter, r *http.Request) {
	name := r.PostFormValue("name")
	v := view{}
	m, err := h.manager(r.PostFormValue("backend"))
	if err == nil {
		err = m.PullModel(r.Context(), name)
	}
	if err != nil {
		v.ModelsError = err.Error()
	} else {
		v.ModelsNote = "Pulled " + name + "."
	}
	h.render(w, r, v)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	name := r.PostFormValue("name")
	v := view{}
	m, err := h.manager(r.PostFormValue("backend"))
	if err == nil {
		err = m.DeleteModel(r.Context(), name)
	}
	if err != nil {
		v.ModelsError = err.Error()
	} else {
		v.ModelsNote = "Deleted " + name + "."
	}
	h.render(w, r, v)
}

// manager returns the backend called name as a ModelManager.
func (h *handler) manager(name string) (llm.ModelManager, error) {
	b, ok := h.fleet.Backend(name)
	if !ok {
		return nil, fmt.Errorf("unknown backend %q", name)
	}
	m, ok := b.(llm.ModelManager)
	if !ok {
		return nil, fmt.Errorf("backend %q does not manage models", name)
	}
	return m, nil
}

// render fills in the fleet and router data on v, keeping the fields
// already set, and writes the whole page.
func (h *handler) render(w http.ResponseWriter, r *http.Request, v view) {
	ctx := r.Context()
	for _, b := range h.fleet.Backends() {
		v.Backends = append(v.Backends, llm.CheckBackend(ctx, b, h.fleet.Managed(b.Name())))
		v.Models = append(v.Models, backendModelsOf(ctx, b))
	}
	v.Routes = h.router.Routes()
	v.Default = h.router.DefaultRoute()
	v.Decisions = h.router.Recent(20)

	// Render into a buffer first, so that a failure produces a clean error
	// response rather than a partial page.
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, v); err != nil {
		http.Error(w, "render failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(buf.Bytes())
}

// backendModelsOf lists the models b can serve, or the error saying why not.
func backendModelsOf(ctx context.Context, b llm.Backend) backendModels {
	_, manager := b.(llm.ModelManager)
	bm := backendModels{Name: b.Name(), Manager: manager}
	models, err := b.ListModels(ctx)
	if err != nil {
		bm.Error = err.Error()
	}
	bm.Models = models
	return bm
}
