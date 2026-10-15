// Package api serves the chess game over REST. It decodes requests, calls the
// Service and maps its errors to status codes; it validates nothing itself.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"chess/internal/core"
)

// maxBody limits a request body.
const maxBody = 1 << 20

// NewHandler returns the REST API. It registers the full paths, so it can be
// mounted at "/api/" and "/healthz" without stripping the prefix.
func NewHandler(svc *core.Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/games", h.list)
	mux.HandleFunc("POST /api/games", h.create)
	mux.HandleFunc("GET /api/games/{id}", h.get)
	mux.HandleFunc("POST /api/games/{id}/moves", h.play)
	mux.HandleFunc("POST /api/games/{id}/resign", h.resign)
	mux.HandleFunc("GET /healthz", health)
	return mux
}

type handler struct {
	svc *core.Service
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	games, err := h.svc.Games(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, games)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Human      core.Color `json:"human"`
		Difficulty string     `json:"difficulty"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	g, err := h.svc.NewGame(r.Context(), in.Human, in.Difficulty)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (h *handler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := gameID(w, r)
	if !ok {
		return
	}
	g, err := h.svc.Game(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *handler) play(w http.ResponseWriter, r *http.Request) {
	id, ok := gameID(w, r)
	if !ok {
		return
	}
	var in struct {
		Move string `json:"move"`
	}
	if !decodeBody(w, r, &in) {
		return
	}
	g, err := h.svc.Play(r.Context(), id, in.Move)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (h *handler) resign(w http.ResponseWriter, r *http.Request) {
	id, ok := gameID(w, r)
	if !ok {
		return
	}
	g, err := h.svc.Resign(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// gameID reads the {id} path value. It writes a 400 and returns false if it is not an integer.
func gameID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "game id must be an integer"})
		return 0, false
	}
	return id, true
}

// decodeBody decodes the JSON request body into v. It writes a 400 and returns false on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, errorBody{Error: "bad request body: " + err.Error()})
		return false
	}
	return true
}

type errorBody struct {
	Error string `json:"error"`
}

// writeError maps a Service error to its status code.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorBody{Error: err.Error()})
	case errors.Is(err, core.ErrInvalid), errors.Is(err, core.ErrIllegal):
		writeJSON(w, http.StatusBadRequest, errorBody{Error: err.Error()})
	case errors.Is(err, core.ErrConflict):
		writeJSON(w, http.StatusConflict, errorBody{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorBody{Error: "internal error"})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
