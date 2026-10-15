// Package gateway serves the OpenAI-compatible API that apps call. It picks
// a backend and model for each request and streams the backend's response
// back unchanged.
package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"

	"llmrouter/internal/llm"
)

// maxBodyBytes limits the size of a chat-completions request body.
const maxBodyBytes = 8 << 20

// copyBufBytes is the size of the buffer used to copy streamed responses.
const copyBufBytes = 32 << 10

// NewHandler serves the OpenAI-compatible API over the fleet's
// backends, choosing models with r. It logs to logger, which must not be nil.
func NewHandler(f llm.Fleet, r llm.Router, logger *log.Logger) http.Handler {
	h := &handler{fleet: f, router: r, logger: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/chat/completions", h.chatCompletions)
	mux.HandleFunc("GET /v1/models", h.listModels)
	return mux
}

type handler struct {
	fleet  llm.Fleet
	router llm.Router
	logger *log.Logger
}

// chatCompletions forwards one chat-completions request to the chosen backend.
func (h *handler) chatCompletions(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	raw, err := io.ReadAll(http.MaxBytesReader(w, req.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read request body: "+err.Error())
		return
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil || body == nil {
		writeError(w, http.StatusBadRequest, "request body must be a JSON object")
		return
	}

	var model string
	if m, ok := body["model"]; ok {
		if err := json.Unmarshal(m, &model); err != nil {
			writeError(w, http.StatusBadRequest, `"model" must be a string`)
			return
		}
	}
	messages, err := parseMessages(body["messages"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var routeName, backendName, chosenModel string
	var options json.RawMessage // the route's options; nil when no route applies
	switch {
	case model == "" || model == llm.AutoModel:
		d := h.router.Route(ctx, messages)
		routeName, backendName, chosenModel = d.Route, d.Backend, d.Model
		if rt, ok := findRoute(h.router.Routes(), d.Route); ok {
			options = rt.Options
		}
	default:
		if rt, ok := findRoute(h.router.Routes(), model); ok {
			routeName, backendName, chosenModel = rt.Name, rt.Backend, rt.Model
			options = rt.Options
		} else if prefix, rest, ok := strings.Cut(model, "/"); ok && h.knownBackend(prefix) {
			backendName, chosenModel = prefix, rest
		} else {
			rt, ok := findRoute(h.router.Routes(), h.router.DefaultRoute())
			if !ok {
				writeError(w, http.StatusInternalServerError, "default route "+h.router.DefaultRoute()+" is not configured")
				return
			}
			backendName, chosenModel = rt.Backend, model
			options = rt.Options
		}
	}

	backend, ok := h.fleet.Backend(backendName)
	if !ok {
		writeError(w, http.StatusInternalServerError, "backend "+backendName+" is not in the fleet")
		return
	}

	body["model"], err = json.Marshal(chosenModel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode model: "+err.Error())
		return
	}
	if err := applyOptions(body, options); err != nil {
		writeError(w, http.StatusInternalServerError, "route options: "+err.Error())
		return
	}
	out, err := json.Marshal(body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode request: "+err.Error())
		return
	}

	resp, err := backend.ChatCompletions(ctx, out)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, llm.ErrUnavailable) {
			status = http.StatusServiceUnavailable
		}
		h.logger.Printf("gateway: backend %s: %v", backendName, err)
		writeError(w, status, err.Error())
		return
	}
	if resp == nil {
		h.logger.Printf("gateway: backend %s returned no response", backendName)
		writeError(w, http.StatusBadGateway, "backend "+backendName+" returned no response")
		return
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}

	for _, name := range []string{"Content-Type", "Cache-Control"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	w.Header().Set("X-Llmrouter-Backend", backendName)
	w.Header().Set("X-Llmrouter-Model", chosenModel)
	if routeName != "" {
		w.Header().Set("X-Llmrouter-Route", routeName)
	}
	w.WriteHeader(resp.StatusCode)
	if resp.Body != nil {
		h.copyBody(w, resp.Body)
	}
}

// copyBody copies the backend's body to w, flushing after each write so
// streamed responses reach the client as they arrive.
func (h *handler) copyBody(w http.ResponseWriter, body io.Reader) {
	rc := http.NewResponseController(w)
	buf := make([]byte, copyBufBytes)
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				h.logger.Printf("gateway: write response: %v", werr)
				return
			}
			_ = rc.Flush()
		}
		if rerr == io.EOF {
			return
		}
		if rerr != nil {
			h.logger.Printf("gateway: read backend response: %v", rerr)
			return
		}
	}
}

// knownBackend reports whether the fleet has a backend with this name.
func (h *handler) knownBackend(name string) bool {
	_, ok := h.fleet.Backend(name)
	return ok
}

// modelEntry is one entry of the /v1/models list.
type modelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// listModels serves the models a client may name: auto, each route, and
// every model each backend serves.
func (h *handler) listModels(w http.ResponseWriter, req *http.Request) {
	data := []modelEntry{{ID: llm.AutoModel, Object: "model", OwnedBy: "llmrouter"}}
	for _, rt := range h.router.Routes() {
		data = append(data, modelEntry{ID: rt.Name, Object: "model", OwnedBy: "llmrouter"})
	}
	for _, b := range h.fleet.Backends() {
		models, err := b.ListModels(req.Context())
		if err != nil {
			h.logger.Printf("gateway: list models of backend %s: %v", b.Name(), err)
			continue
		}
		for _, m := range models {
			data = append(data, modelEntry{ID: b.Name() + "/" + m.Name, Object: "model", OwnedBy: b.Name()})
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(struct {
		Object string       `json:"object"`
		Data   []modelEntry `json:"data"`
	}{Object: "list", Data: data})
}

// parseMessages converts the raw "messages" field into llm messages.
func parseMessages(raw json.RawMessage) ([]llm.Message, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errors.New(`"messages" must be an array`)
	}
	var items []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(trimmed, &items); err != nil {
		return nil, errors.New(`"messages" must be an array of {role, content} objects`)
	}
	messages := make([]llm.Message, 0, len(items))
	for _, it := range items {
		text, err := contentText(it.Content)
		if err != nil {
			return nil, err
		}
		messages = append(messages, llm.Message{Role: it.Role, Content: text})
	}
	return messages, nil
}

// contentText returns the text of a message's content, which may be a
// string, null, or an array of parts. The text parts are joined with "\n".
func contentText(raw json.RawMessage) (string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}
	switch trimmed[0] {
	case '"':
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return "", errors.New(`"content" must be a string`)
		}
		return s, nil
	case '[':
		var parts []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(trimmed, &parts); err != nil {
			return "", errors.New(`"content" parts must be objects with a string "text"`)
		}
		var texts []string
		for _, p := range parts {
			if p.Type == "text" {
				texts = append(texts, p.Text)
			}
		}
		return strings.Join(texts, "\n"), nil
	default:
		return "", errors.New(`"content" must be a string, null or an array`)
	}
}

// applyOptions sets each top-level field of options on body, replacing the
// client's field with the same name and leaving the other fields alone. The
// merge is shallow: a replaced object's sibling keys are dropped. Absent or
// null options change nothing. The config loader guarantees options is an
// object without "model".
func applyOptions(body map[string]json.RawMessage, options json.RawMessage) error {
	if len(options) == 0 {
		return nil
	}
	var opts map[string]json.RawMessage
	if err := json.Unmarshal(options, &opts); err != nil {
		return err
	}
	for k, v := range opts {
		body[k] = v
	}
	return nil
}

// findRoute returns the route with this name.
func findRoute(routes []llm.Route, name string) (llm.Route, bool) {
	for _, rt := range routes {
		if rt.Name == name {
			return rt, true
		}
	}
	return llm.Route{}, false
}

// errorBody is the OpenAI error envelope.
type errorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// writeError writes an error in OpenAI's format.
func writeError(w http.ResponseWriter, status int, msg string) {
	typ := "server_error"
	if status == http.StatusBadRequest {
		typ = "invalid_request_error"
	}
	var body errorBody
	body.Error.Message = msg
	body.Error.Type = typ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
