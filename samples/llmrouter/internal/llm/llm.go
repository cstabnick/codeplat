// Package llm holds the types and interfaces shared by every part of
// llmrouter. It imports only the standard library.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// Errors returned by llmrouter's parts. Callers test for them with errors.Is.
var (
	ErrNotFound    = errors.New("not found")
	ErrInvalid     = errors.New("invalid input")
	ErrUnavailable = errors.New("backend unavailable")
	ErrUnsupported = errors.New("not supported by this backend")
)

// AutoModel is the model name a client sends to let the router choose.
const AutoModel = "auto"

// Backend kinds.
const (
	KindOllama = "ollama" // an Ollama server
	KindOpenAI = "openai" // any OpenAI-compatible server, such as llama-server
)

// Message is one chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Route sends prompts of one intent to one model on one backend. The
// intent model sees Name and Description when it classifies a prompt.
//
// Options, when set, is a JSON object of top-level request fields that the
// gateway sets on every chat-completions request sent to this route. Each
// field replaces the client's field with the same name. Absent or null
// means no options. The config loader validates it; "model" is not allowed.
type Route struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Backend     string          `json:"backend"`
	Model       string          `json:"model"`
	Options     json.RawMessage `json:"options,omitempty"`
}

// Decision records how one conversation was routed. Fallback is true when
// the default route was used because classification failed, and Error then
// says why.
type Decision struct {
	At       time.Time     `json:"at"`
	Prompt   string        `json:"prompt"`
	Route    string        `json:"route"`
	Backend  string        `json:"backend"`
	Model    string        `json:"model"`
	Fallback bool          `json:"fallback"`
	Error    string        `json:"error,omitempty"`
	Took     time.Duration `json:"took"`
}

// Model is a model a backend can serve.
type Model struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

// LoadedModel is a model a backend holds in memory.
type LoadedModel struct {
	Name      string    `json:"name"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Backend is a server that runs models.
type Backend interface {
	// Name is the backend's name in the config, such as "ollama".
	Name() string
	// Kind is KindOllama or KindOpenAI.
	Kind() string
	// Health returns nil when the backend answers, with the server's
	// version if it reports one. An error wraps ErrUnavailable.
	Health(ctx context.Context) (version string, err error)
	// ListModels returns the models the backend can serve.
	ListModels(ctx context.Context) ([]Model, error)
	// Classify asks model to pick one of labels for prompt, with system as
	// the system message, and returns the label it answered.
	Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error)
	// ChatCompletions sends body, an OpenAI chat-completions request, to
	// the backend's /v1/chat/completions and returns the response unread,
	// whatever its status. The caller closes the response Body. A request
	// that cannot reach the backend returns an error wrapping ErrUnavailable.
	ChatCompletions(ctx context.Context, body []byte) (*http.Response, error)
}

// ModelManager is implemented by backends that can install and remove
// models, such as Ollama.
type ModelManager interface {
	LoadedModels(ctx context.Context) ([]LoadedModel, error)
	// PullModel downloads a model and returns once it is installed.
	PullModel(ctx context.Context, name string) error
	// DeleteModel removes a model. An unknown model is an error wrapping ErrNotFound.
	DeleteModel(ctx context.Context, name string) error
}

// BackendStatus describes one backend.
type BackendStatus struct {
	Name      string        `json:"name"`
	Kind      string        `json:"kind"`
	Reachable bool          `json:"reachable"`
	Version   string        `json:"version"`
	Managed   bool          `json:"managed"`
	Loaded    []LoadedModel `json:"loaded"`
	Error     string        `json:"error,omitempty"`
}

// CheckBackend reports on b. managed says whether llmrouter started b's
// process. Loaded is filled only for a reachable ModelManager, and is never nil.
func CheckBackend(ctx context.Context, b Backend, managed bool) BackendStatus {
	s := BackendStatus{Name: b.Name(), Kind: b.Kind(), Managed: managed, Loaded: []LoadedModel{}}
	v, err := b.Health(ctx)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	s.Reachable, s.Version = true, v
	if m, ok := b.(ModelManager); ok {
		loaded, err := m.LoadedModels(ctx)
		if err != nil {
			s.Error = err.Error()
			return s
		}
		if loaded != nil {
			s.Loaded = loaded
		}
	}
	return s
}

// Router chooses a route for each conversation.
type Router interface {
	// Route classifies the conversation and records the decision. It never
	// fails: when classification fails it returns the default route with
	// Fallback set.
	Route(ctx context.Context, messages []Message) Decision
	// Routes returns the configured routes in config order.
	Routes() []Route
	// DefaultRoute returns the name of the default route.
	DefaultRoute() string
	// Recent returns up to n of the latest decisions, newest first. n <= 0
	// returns every decision kept.
	Recent(n int) []Decision
}

// Fleet is the configured set of backends.
type Fleet interface {
	// Backends returns every backend in config order.
	Backends() []Backend
	// Backend returns the backend with this name.
	Backend(name string) (Backend, bool)
	// Managed reports whether llmrouter started this backend's process and
	// it is still running.
	Managed(name string) bool
}

// NewFleet returns a Fleet over backends, whose names must be distinct.
// managed may be nil, which means no backend is managed.
func NewFleet(backends []Backend, managed func(name string) bool) Fleet {
	return fleet{backends: append([]Backend(nil), backends...), managed: managed}
}

type fleet struct {
	backends []Backend
	managed  func(string) bool
}

func (f fleet) Backends() []Backend { return append([]Backend(nil), f.backends...) }

func (f fleet) Backend(name string) (Backend, bool) {
	for _, b := range f.backends {
		if b.Name() == name {
			return b, true
		}
	}
	return nil, false
}

func (f fleet) Managed(name string) bool { return f.managed != nil && f.managed(name) }
