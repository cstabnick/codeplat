// Package admin serves llmrouter's JSON admin API under /api/.
package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"llmrouter/internal/llm"
)

const defaultDecisions = 50

// NewHandler serves the admin API over the fleet and router.
func NewHandler(f llm.Fleet, r llm.Router) http.Handler {
	h := &handler{fleet: f, router: r}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/backends", h.backends)
	mux.HandleFunc("GET /api/backends/{backend}/models", h.listModels)
	mux.HandleFunc("POST /api/backends/{backend}/models/pull", h.pullModel)
	mux.HandleFunc("DELETE /api/backends/{backend}/models/{name...}", h.deleteModel)
	mux.HandleFunc("GET /api/routes", h.routes)
	mux.HandleFunc("GET /api/decisions", h.decisions)
	mux.HandleFunc("POST /api/route", h.route)
	return mux
}

type handler struct {
	fleet  llm.Fleet
	router llm.Router
}

func (h *handler) backends(w http.ResponseWriter, r *http.Request) {
	statuses := []llm.BackendStatus{}
	for _, b := range h.fleet.Backends() {
		statuses = append(statuses, llm.CheckBackend(r.Context(), b, h.fleet.Managed(b.Name())))
	}
	writeJSON(w, http.StatusOK, statuses)
}

func (h *handler) listModels(w http.ResponseWriter, r *http.Request) {
	b, err := h.backend(r)
	if err != nil {
		writeError(w, err)
		return
	}
	models, err := b.ListModels(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if models == nil {
		models = []llm.Model{}
	}
	writeJSON(w, http.StatusOK, models)
}

type modelBody struct {
	Name string `json:"name"`
}

func (h *handler) pullModel(w http.ResponseWriter, r *http.Request) {
	m, err := h.manager(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var body modelBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Name == "" {
		writeError(w, fmt.Errorf("model name is required: %w", llm.ErrInvalid))
		return
	}
	if err := m.PullModel(r.Context(), body.Name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "success"})
}

func (h *handler) deleteModel(w http.ResponseWriter, r *http.Request) {
	m, err := h.manager(r)
	if err != nil {
		writeError(w, err)
		return
	}
	name := r.PathValue("name")
	if name == "" {
		writeError(w, fmt.Errorf("model name is required: %w", llm.ErrInvalid))
		return
	}
	if err := m.DeleteModel(r.Context(), name); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) routes(w http.ResponseWriter, _ *http.Request) {
	routes := h.router.Routes()
	if routes == nil {
		routes = []llm.Route{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default": h.router.DefaultRoute(),
		"routes":  routes,
	})
}

func (h *handler) decisions(w http.ResponseWriter, r *http.Request) {
	n := defaultDecisions
	if s := r.URL.Query().Get("n"); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil {
			writeError(w, fmt.Errorf("n must be an integer: %w", llm.ErrInvalid))
			return
		}
		n = v
	}
	decisions := h.router.Recent(n)
	if decisions == nil {
		decisions = []llm.Decision{}
	}
	writeJSON(w, http.StatusOK, decisions)
}

type routeBody struct {
	Prompt string `json:"prompt"`
}

func (h *handler) route(w http.ResponseWriter, r *http.Request) {
	var body routeBody
	if err := decodeBody(r, &body); err != nil {
		writeError(w, err)
		return
	}
	if body.Prompt == "" {
		writeError(w, fmt.Errorf("prompt is required: %w", llm.ErrInvalid))
		return
	}
	d := h.router.Route(r.Context(), []llm.Message{{Role: "user", Content: body.Prompt}})
	writeJSON(w, http.StatusOK, d)
}

// backend returns the backend named in the path, or an error wrapping
// llm.ErrNotFound.
func (h *handler) backend(r *http.Request) (llm.Backend, error) {
	name := r.PathValue("backend")
	b, ok := h.fleet.Backend(name)
	if !ok {
		return nil, fmt.Errorf("backend %q: %w", name, llm.ErrNotFound)
	}
	return b, nil
}

// manager returns the backend named in the path if it is a
// llm.ModelManager. An unknown backend is checked first.
func (h *handler) manager(r *http.Request) (llm.ModelManager, error) {
	b, err := h.backend(r)
	if err != nil {
		return nil, err
	}
	m, ok := b.(llm.ModelManager)
	if !ok {
		return nil, fmt.Errorf("backend %q cannot manage models: %w", b.Name(), llm.ErrUnsupported)
	}
	return m, nil
}

// decodeBody decodes a JSON request body into v, rejecting unknown fields.
// Any failure wraps llm.ErrInvalid.
func decodeBody(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %v: %w", err, llm.ErrInvalid)
	}
	return nil
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, llm.ErrInvalid), errors.Is(err, llm.ErrUnsupported):
		status = http.StatusBadRequest
	case errors.Is(err, llm.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, llm.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
