// Package api serves llmchat's JSON and server-sent-events API.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"llmchat/internal/chat"
)

// Runner runs one turn; internal/agent.Engine satisfies it.
type Runner interface {
	Run(ctx context.Context, conversationID, text string, emit func(chat.Event)) error
}

// Approvals holds the tool calls waiting for the person. It implements chat.Approver.
type Approvals struct {
	mu      sync.Mutex
	waiting map[string]chan bool
}

var _ chat.Approver = (*Approvals)(nil)

// NewApprovals returns an Approvals with nothing waiting.
func NewApprovals() *Approvals {
	return &Approvals{waiting: make(map[string]chan bool)}
}

func approvalKey(conversationID, callID string) string {
	return conversationID + "/" + callID
}

// Approve registers r and blocks until Resolve answers it (return the
// answer) or ctx ends (return false and ctx.Err()). The key is r.ConversationID + "/" + r.CallID.
func (a *Approvals) Approve(ctx context.Context, r chat.ApprovalRequest) (bool, error) {
	key := approvalKey(r.ConversationID, r.CallID)
	ch := make(chan bool, 1)

	a.mu.Lock()
	if _, ok := a.waiting[key]; ok {
		a.mu.Unlock()
		return false, fmt.Errorf("approval for %s is already waiting: %w", key, chat.ErrInvalid)
	}
	a.waiting[key] = ch
	a.mu.Unlock()

	select {
	case approve := <-ch:
		return approve, nil
	case <-ctx.Done():
		a.approvalCleanup(key, ch)
		return false, ctx.Err()
	}
}

// approvalCleanup removes key from the waiting map only if it still holds ch,
// this call's own channel. If Resolve already answered this call and a new
// Approve registered the same key, the new waiter's entry is left alone.
func (a *Approvals) approvalCleanup(key string, ch chan bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cur, ok := a.waiting[key]; ok && cur == ch {
		delete(a.waiting, key)
	}
}

// Resolve answers a waiting request. It returns an error wrapping chat.ErrNotFound if none is waiting.
func (a *Approvals) Resolve(conversationID, callID string, approve bool) error {
	key := approvalKey(conversationID, callID)

	a.mu.Lock()
	ch, ok := a.waiting[key]
	if ok {
		delete(a.waiting, key)
	}
	a.mu.Unlock()

	if !ok {
		return fmt.Errorf("no approval waiting for %s: %w", key, chat.ErrNotFound)
	}
	ch <- approve // buffered, and only this call sends on it
	return nil
}

// NewHandler serves the API. routes lists the router's route names; logger
// receives the errors the API cannot return to a client.
func NewHandler(store chat.Store, runner Runner, approvals *Approvals, routes func(ctx context.Context) ([]string, error), logger *log.Logger) http.Handler {
	h := &handler{
		store:     store,
		runner:    runner,
		approvals: approvals,
		routes:    routes,
		log:       logger,
		turns:     make(map[string]*turn),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /api/routes", h.listRoutes)
	mux.HandleFunc("GET /api/conversations", h.listConversations)
	mux.HandleFunc("POST /api/conversations", h.createConversation)
	mux.HandleFunc("GET /api/conversations/{id}", h.getConversation)
	mux.HandleFunc("PATCH /api/conversations/{id}", h.updateConversation)
	mux.HandleFunc("DELETE /api/conversations/{id}", h.deleteConversation)
	mux.HandleFunc("POST /api/conversations/{id}/messages", h.sendMessage)
	mux.HandleFunc("POST /api/conversations/{id}/approvals/{call}", h.answerApproval)
	mux.HandleFunc("POST /api/conversations/{id}/stop", h.stopTurn)
	return mux
}

// turn is one running turn. cancel ends the turn's context; stopped records
// that the person pressed Stop, as opposed to the client leaving.
type turn struct {
	cancel  context.CancelFunc
	stopped atomic.Bool
}

type handler struct {
	store     chat.Store
	runner    Runner
	approvals *Approvals
	routes    func(ctx context.Context) ([]string, error)
	log       *log.Logger

	mu    sync.Mutex
	turns map[string]*turn // running turns, keyed by conversation ID
}

const maxBody = 1 << 20

// decode reads a JSON body into v, rejecting unknown fields and bodies over
// 1 MiB. It returns io.EOF for an empty body.
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// badBody answers a body that could not be decoded.
func (h *handler) badBody(w http.ResponseWriter, err error) {
	h.writeError(w, fmt.Errorf("bad request body: %v: %w", err, chat.ErrInvalid))
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Printf("api: write response: %v", err)
	}
}

// writeError maps err to a status code and writes it as {"error": "..."}.
func (h *handler) writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	msg := err.Error()
	switch {
	case errors.Is(err, chat.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, chat.ErrNotFound):
		status = http.StatusNotFound
	default:
		h.log.Printf("api: %v", err)
		msg = "internal error"
	}
	h.writeJSON(w, status, map[string]string{"error": msg})
}

func (h *handler) healthz(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *handler) listRoutes(w http.ResponseWriter, r *http.Request) {
	names, err := h.routes(r.Context())
	if err != nil {
		h.log.Printf("api: list routes: %v", err)
		h.writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if names == nil {
		names = []string{}
	}
	h.writeJSON(w, http.StatusOK, names)
}

func (h *handler) listConversations(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListConversations(r.Context())
	if err != nil {
		h.writeError(w, err)
		return
	}
	if list == nil {
		list = []chat.Conversation{}
	}
	h.writeJSON(w, http.StatusOK, list)
}

func (h *handler) createConversation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Title     string `json:"title"`
		Model     string `json:"model"`
		Workspace string `json:"workspace"`
	}
	// An empty body means every field takes its default.
	if err := decode(w, r, &body); err != nil && !errors.Is(err, io.EOF) {
		h.badBody(w, err)
		return
	}
	c, err := h.store.CreateConversation(r.Context(), chat.Conversation{
		Title:     body.Title,
		Model:     body.Model,
		Workspace: body.Workspace,
	})
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusCreated, c)
}

func (h *handler) getConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := h.store.GetConversation(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	msgs, err := h.store.ListMessages(r.Context(), id)
	if err != nil {
		h.writeError(w, err)
		return
	}
	if msgs == nil {
		msgs = []chat.Message{}
	}
	h.writeJSON(w, http.StatusOK, struct {
		Conversation chat.Conversation `json:"conversation"`
		Messages     []chat.Message    `json:"messages"`
	}{c, msgs})
}

func (h *handler) updateConversation(w http.ResponseWriter, r *http.Request) {
	var p chat.ConversationPatch
	if err := decode(w, r, &p); err != nil {
		h.badBody(w, err)
		return
	}
	c, err := h.store.UpdateConversation(r.Context(), r.PathValue("id"), p)
	if err != nil {
		h.writeError(w, err)
		return
	}
	h.writeJSON(w, http.StatusOK, c)
}

func (h *handler) deleteConversation(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteConversation(r.Context(), r.PathValue("id")); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// startTurn claims the conversation for a turn and registers its cancel. It
// returns nil when a turn is already running for it.
func (h *handler) startTurn(id string, cancel context.CancelFunc) *turn {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, busy := h.turns[id]; busy {
		return nil
	}
	t := &turn{cancel: cancel}
	h.turns[id] = t
	return t
}

func (h *handler) endTurn(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.turns, id)
}

// stopTurn ends the running turn of a conversation by cancelling its context.
// The turn then emits a stopped event and done on its open stream. It returns
// ErrNotFound when no turn is running.
func (h *handler) stopTurn(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mu.Lock()
	t, ok := h.turns[id]
	h.mu.Unlock()
	if !ok {
		h.writeError(w, fmt.Errorf("no turn running in conversation %s: %w", id, chat.ErrNotFound))
		return
	}
	t.stopped.Store(true)
	t.cancel()
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) sendMessage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(w, r, &body); err != nil {
		h.badBody(w, err)
		return
	}
	if _, err := h.store.GetConversation(r.Context(), id); err != nil {
		h.writeError(w, err)
		return
	}
	text := strings.TrimSpace(body.Text)
	if text == "" {
		h.writeError(w, fmt.Errorf("text is empty: %w", chat.ErrInvalid))
		return
	}
	// The turn runs under its own context, cancelled by stopTurn, by the
	// client leaving (the request context ends), or by returning here.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	t := h.startTurn(id, cancel)
	if t == nil {
		h.writeJSON(w, http.StatusConflict, map[string]string{"error": "a turn is already running for this conversation"})
		return
	}
	defer h.endTurn(id)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)

	// The runner may emit from more than one goroutine. Writes are serialized,
	// and once the turn has returned, late events are dropped rather than
	// written to a finished response.
	var (
		mu     sync.Mutex
		closed bool
	)
	emit := func(ev chat.Event) {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return
		}
		data, err := json.Marshal(ev)
		if err != nil {
			h.log.Printf("api: marshal event: %v", err)
			return
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return // the client went away; the request context ends the turn
		}
		if err := rc.Flush(); err != nil {
			h.log.Printf("api: flush event: %v", err)
		}
	}

	err := h.runner.Run(ctx, id, text, emit)
	switch {
	case err != nil && t.stopped.Load() && ctx.Err() != nil:
		// The person pressed Stop. The error is the cancellation, not a failure.
		emit(chat.Event{Type: chat.EventStopped})
		emit(chat.Event{Type: chat.EventDone})
	case err != nil:
		if r.Context().Err() != nil {
			h.log.Printf("api: client left turn in conversation %s: %v", id, err)
		} else {
			h.log.Printf("api: turn in conversation %s: %v", id, err)
		}
		emit(chat.Event{Type: chat.EventError, Error: err.Error()})
		emit(chat.Event{Type: chat.EventDone})
	}

	mu.Lock()
	closed = true
	mu.Unlock()
}

func (h *handler) answerApproval(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Approve bool `json:"approve"`
	}
	if err := decode(w, r, &body); err != nil {
		h.badBody(w, err)
		return
	}
	if err := h.approvals.Resolve(r.PathValue("id"), r.PathValue("call"), body.Approve); err != nil {
		h.writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
