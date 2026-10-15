// Package httpapi serves the sandbox service over HTTP. It decodes requests,
// calls core.Service and maps its errors to status codes. It validates
// nothing else: names, workspaces, timeouts and commands are the service's
// rules.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"sandbox/internal/core"
)

// maxBodyBytes limits every request body.
const maxBodyBytes = 1 << 20

// NewHandler returns the HTTP handler for the sandbox API, backed by svc.
func NewHandler(svc *core.Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /api/boxes/{name}", h.ensure)
	mux.HandleFunc("GET /api/boxes", h.list)
	mux.HandleFunc("GET /api/boxes/{name}", h.get)
	mux.HandleFunc("DELETE /api/boxes/{name}", h.remove)
	mux.HandleFunc("POST /api/boxes/{name}/exec", h.exec)
	mux.HandleFunc("GET /healthz", health)
	return mux
}

type handler struct {
	svc *core.Service
}

type ensureRequest struct {
	Workspace string `json:"workspace"`
	Image     string `json:"image"`
}

type execRequest struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

func (h *handler) ensure(w http.ResponseWriter, r *http.Request) {
	var req ensureRequest
	if !decodeBody(w, r, &req) {
		return
	}
	box, err := h.svc.Ensure(r.Context(), r.PathValue("name"), req.Workspace, req.Image)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, box)
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	boxes, err := h.svc.Boxes(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, boxes)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	box, err := h.svc.Box(r.Context(), r.PathValue("name"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, box)
}

func (h *handler) remove(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Remove(r.Context(), r.PathValue("name")); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) exec(w http.ResponseWriter, r *http.Request) {
	var req execRequest
	if !decodeBody(w, r, &req) {
		return
	}
	res, err := h.svc.Exec(r.Context(), r.PathValue("name"), req.Command, req.TimeoutSeconds)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// decodeBody reads a JSON body into v. An empty body leaves v unchanged. It
// writes a 400 and returns false when the body cannot be read or decoded.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	err := json.NewDecoder(body).Decode(v)
	if err == nil || errors.Is(err, io.EOF) {
		return true
	}
	writeJSON(w, http.StatusBadRequest, errorBody{Error: "invalid request body: " + err.Error()})
	return false
}

// writeError maps a service error to its status code and JSON error body.
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, core.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, core.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, core.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, errorBody{Error: err.Error()})
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
