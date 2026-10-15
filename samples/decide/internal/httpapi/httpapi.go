// Package httpapi serves the decide service over HTTP as JSON. It decodes its
// input and maps errors to status codes; business rules stay in core.Service.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"decide/internal/core"
)

// maxBody is the largest request body accepted, in bytes.
const maxBody = 1 << 20

// NewHandler returns the HTTP handler for svc.
func NewHandler(svc *core.Service) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/evaluate/{key}", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Context map[string]any `json:"context"`
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			writeError(w, http.StatusBadRequest, "request body is not readable or exceeds 1 MiB")
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
		input := req.Context
		if input == nil {
			input = map[string]any{}
		}
		out, err := svc.Evaluate(r.Context(), r.PathValue("key"), input)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"result": out})
	})

	mux.HandleFunc("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		keys, err := svc.Models(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"models": keys})
	})

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})

	return mux
}

// writeServiceError maps a core error to its status code.
func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, core.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
