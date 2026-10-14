// Package router chooses a route for each conversation by asking a small
// intent model which route fits the last user message.
package router

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"llmrouter/internal/llm"
)

// Classifier is what the router needs from a backend.
type Classifier interface {
	Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error)
}

// Options configures a Router.
type Options struct {
	// IntentBackend and IntentModel name the model that classifies prompts.
	IntentBackend string
	IntentModel   string
	Routes        []llm.Route
	// Default is the name of the route used when classification fails.
	Default string
	// Timeout bounds one classification. Zero or negative means 15 seconds.
	Timeout time.Duration
	// History is how many decisions Recent keeps. Zero or negative means 100.
	History int
	// Now returns the current time. Nil means time.Now.
	Now func() time.Time
}

const (
	defaultTimeout = 15 * time.Second
	defaultHistory = 100
	maxPromptRunes = 200
)

// Router implements llm.Router.
type Router struct {
	classifiers   map[string]Classifier
	intentBackend string
	intentModel   string
	routes        []llm.Route
	byName        map[string]llm.Route
	defaultName   string
	timeout       time.Duration
	historyCap    int
	now           func() time.Time

	mu      sync.Mutex
	history []llm.Decision
}

var _ llm.Router = (*Router)(nil)

// New returns a router. classifiers maps each backend name to its
// backend. It returns an error wrapping llm.ErrInvalid when
// IntentModel is empty, IntentBackend is not in classifiers, Routes is
// empty, a route's Name, Backend or Model is empty, two routes share a
// Name, a route's Backend is not in classifiers, or Default is not a
// route's Name.
func New(classifiers map[string]Classifier, opts Options) (*Router, error) {
	if opts.IntentModel == "" {
		return nil, fmt.Errorf("router: intent model is empty: %w", llm.ErrInvalid)
	}
	if _, ok := classifiers[opts.IntentBackend]; !ok {
		return nil, fmt.Errorf("router: intent backend %q is not configured: %w", opts.IntentBackend, llm.ErrInvalid)
	}
	if len(opts.Routes) == 0 {
		return nil, fmt.Errorf("router: no routes: %w", llm.ErrInvalid)
	}
	byName := make(map[string]llm.Route, len(opts.Routes))
	for _, r := range opts.Routes {
		if r.Name == "" {
			return nil, fmt.Errorf("router: a route has an empty name: %w", llm.ErrInvalid)
		}
		if r.Backend == "" {
			return nil, fmt.Errorf("router: route %q has an empty backend: %w", r.Name, llm.ErrInvalid)
		}
		if r.Model == "" {
			return nil, fmt.Errorf("router: route %q has an empty model: %w", r.Name, llm.ErrInvalid)
		}
		if _, dup := byName[r.Name]; dup {
			return nil, fmt.Errorf("router: route %q is defined twice: %w", r.Name, llm.ErrInvalid)
		}
		if _, ok := classifiers[r.Backend]; !ok {
			return nil, fmt.Errorf("router: route %q uses backend %q, which is not configured: %w", r.Name, r.Backend, llm.ErrInvalid)
		}
		byName[r.Name] = r
	}
	if _, ok := byName[opts.Default]; !ok {
		return nil, fmt.Errorf("router: default %q is not a route: %w", opts.Default, llm.ErrInvalid)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	historyCap := opts.History
	if historyCap <= 0 {
		historyCap = defaultHistory
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Router{
		classifiers:   classifiers,
		intentBackend: opts.IntentBackend,
		intentModel:   opts.IntentModel,
		routes:        append([]llm.Route(nil), opts.Routes...),
		byName:        byName,
		defaultName:   opts.Default,
		timeout:       timeout,
		historyCap:    historyCap,
		now:           now,
	}, nil
}

// SystemPrompt is the system message the intent model gets.
func SystemPrompt(routes []llm.Route) string {
	var b strings.Builder
	b.WriteString("You route chat messages. Pick the one route that fits the user's message best.\n\n")
	b.WriteString("Routes:\n")
	for _, r := range routes {
		fmt.Fprintf(&b, "- %s: %s\n", r.Name, r.Description)
	}
	b.WriteString("\nAnswer with JSON: {\"route\": \"<name>\"}")
	return b.String()
}

// Route classifies the conversation and records the decision. It never
// fails: when classification fails it returns the default route with
// Fallback set.
func (r *Router) Route(ctx context.Context, messages []llm.Message) llm.Decision {
	began := r.now()
	prompt := lastUserMessage(messages)
	d := llm.Decision{At: began, Prompt: truncateRunes(prompt, maxPromptRunes)}

	switch {
	case !hasUserMessage(messages):
		r.fill(&d, r.byName[r.defaultName])
		d.Fallback = true
		d.Error = "no user message"
	case len(r.routes) == 1:
		r.fill(&d, r.routes[0])
	default:
		chosen, err := r.classify(ctx, prompt)
		switch {
		case err != nil:
			r.fill(&d, r.byName[r.defaultName])
			d.Fallback = true
			d.Error = err.Error()
		default:
			if rt, ok := r.match(chosen); ok {
				r.fill(&d, rt)
			} else {
				r.fill(&d, r.byName[r.defaultName])
				d.Fallback = true
				d.Error = fmt.Sprintf("unknown route \"%s\"", chosen)
			}
		}
	}

	d.Took = r.now().Sub(began)
	r.record(d)
	return d
}

// Routes returns a copy of the configured routes in config order.
func (r *Router) Routes() []llm.Route {
	return append([]llm.Route(nil), r.routes...)
}

// DefaultRoute returns the name of the default route.
func (r *Router) DefaultRoute() string {
	return r.defaultName
}

// Recent returns up to n of the latest decisions, newest first. n <= 0
// returns every decision kept. It never returns nil.
func (r *Router) Recent(n int) []llm.Decision {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n <= 0 || n > len(r.history) {
		n = len(r.history)
	}
	out := make([]llm.Decision, 0, n)
	for i := len(r.history) - 1; i >= 0 && len(out) < n; i-- {
		out = append(out, r.history[i])
	}
	return out
}

func (r *Router) classify(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	names := make([]string, 0, len(r.routes))
	for _, rt := range r.routes {
		names = append(names, rt.Name)
	}
	return r.classifiers[r.intentBackend].Classify(ctx, r.intentModel, SystemPrompt(r.routes), prompt, names)
}

// match finds the route named by answer, ignoring case and surrounding space.
func (r *Router) match(answer string) (llm.Route, bool) {
	want := strings.TrimSpace(answer)
	for _, rt := range r.routes {
		if strings.EqualFold(rt.Name, want) {
			return rt, true
		}
	}
	return llm.Route{}, false
}

func (r *Router) fill(d *llm.Decision, rt llm.Route) {
	d.Route = rt.Name
	d.Backend = rt.Backend
	d.Model = rt.Model
}

func (r *Router) record(d llm.Decision) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append(r.history, d)
	if over := len(r.history) - r.historyCap; over > 0 {
		r.history = append([]llm.Decision(nil), r.history[over:]...)
	}
}

func hasUserMessage(messages []llm.Message) bool {
	for _, m := range messages {
		if m.Role == "user" {
			return true
		}
	}
	return false
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
