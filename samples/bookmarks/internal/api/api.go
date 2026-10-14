// Package api exposes a bookmark.Service as a JSON HTTP API.
package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"bookmarks/internal/bookmark"
)

// maxBodyBytes limits the size of a request body.
const maxBodyBytes = 1 << 20

type handler struct {
	svc bookmark.Service
}

// NewHandler returns an http.Handler that serves the bookmark API:
//
//	GET    /api/bookmarks       list bookmarks
//	POST   /api/bookmarks       add a bookmark from {"title":"...","url":"..."}
//	DELETE /api/bookmarks/{id}  delete a bookmark
func NewHandler(svc bookmark.Service) http.Handler {
	h := &handler{svc: svc}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/bookmarks", h.list)
	mux.HandleFunc("POST /api/bookmarks", h.add)
	mux.HandleFunc("DELETE /api/bookmarks/{id}", h.delete)
	return mux
}

type addRequest struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type errorBody struct {
	Error string `json:"error"`
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	bookmarks, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if bookmarks == nil {
		bookmarks = []bookmark.Bookmark{}
	}
	writeJSON(w, http.StatusOK, bookmarks)
}

func (h *handler) add(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	b, err := h.svc.Add(r.Context(), req.Title, req.URL)
	if err != nil {
		if errors.Is(err, bookmark.ErrInvalid) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.svc.Delete(r.Context(), id); err != nil {
		if errors.Is(err, bookmark.ErrNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}
