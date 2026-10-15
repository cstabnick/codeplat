// Package config loads llmrouter's configuration: its listen address, its
// backends, the intent model and the routes.
package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"

	"llmrouter/internal/llm"
)

// Backend is one model server.
type Backend struct {
	Name string `json:"name"`
	// Kind is llm.KindOllama or llm.KindOpenAI.
	Kind string `json:"kind"`
	URL  string `json:"url"`
	// Command starts the backend when it does not answer. Empty means
	// llmrouter never starts it.
	Command     []string `json:"command,omitempty"`
	Dir         string   `json:"dir,omitempty"`
	Env         []string `json:"env,omitempty"`
	StopCommand []string `json:"stop_command,omitempty"`
	// StartTimeoutSeconds bounds the wait after starting; 0 means the
	// supervisor's default.
	StartTimeoutSeconds int `json:"start_timeout_seconds,omitempty"`
	// PullMissing pulls the intent and route models this backend lacks
	// at startup. Only for kind ollama.
	PullMissing bool `json:"pull_missing,omitempty"`
}

// Intent names the model that classifies prompts.
type Intent struct {
	Backend string `json:"backend"`
	Model   string `json:"model"`
	// TimeoutSeconds bounds one classification; 0 means the router's default.
	TimeoutSeconds int `json:"timeout_seconds,omitempty"`
}

type Config struct {
	Addr         string      `json:"addr"`
	Backends     []Backend   `json:"backends"`
	Intent       Intent      `json:"intent"`
	DefaultRoute string      `json:"default_route"`
	Routes       []llm.Route `json:"routes"`
}

// defaultAddr is the listen address used when none is configured.
const defaultAddr = "127.0.0.1:8083"

// addrEnv names the environment variable that replaces Addr.
const addrEnv = "LLMROUTER_ADDR"

var backendNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Default is the config used when no file is given.
func Default() Config {
	return Config{
		Addr: defaultAddr,
		Backends: []Backend{{
			Name:        "ollama",
			Kind:        llm.KindOllama,
			URL:         "http://127.0.0.1:11434",
			Command:     []string{"ollama", "serve"},
			PullMissing: true,
		}},
		Intent:       Intent{Backend: "ollama", Model: "llama3.2:1b"},
		DefaultRoute: "general",
		Routes: []llm.Route{
			{
				Name:        "general",
				Description: "Conversation, questions, writing, and anything no other route fits.",
				Backend:     "ollama",
				Model:       "llama3:8b",
			},
			{
				Name:        "code",
				Description: "Writing, explaining, reviewing or debugging source code.",
				Backend:     "ollama",
				Model:       "qwen2.5-coder:1.5b",
			},
			{
				Name:        "quick",
				Description: "Greetings and short factual questions with a one-line answer.",
				Backend:     "ollama",
				Model:       "llama3.2:1b",
			},
		},
	}
}

// Load reads the JSON file at path, or uses Default() when path is "".
// An empty Addr becomes "127.0.0.1:8083". The environment variable
// LLMROUTER_ADDR, when set, replaces Addr; this applies whether or not a
// file is given. The result is validated.
func Load(path string) (Config, error) {
	var c Config
	if path == "" {
		c = Default()
	} else {
		f, err := os.Open(path)
		if err != nil {
			return Config{}, fmt.Errorf("config: %w", err)
		}
		defer f.Close()
		dec := json.NewDecoder(f)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&c); err != nil {
			return Config{}, fmt.Errorf("config: %s: %w", path, err)
		}
	}
	if c.Addr == "" {
		c.Addr = defaultAddr
	}
	if v := os.Getenv(addrEnv); v != "" {
		c.Addr = v
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Validate returns an error wrapping llm.ErrInvalid for the first
// problem it finds, naming the field, such as
// `routes[1].backend: unknown backend "x"`.
func (c Config) Validate() error {
	if c.Addr == "" {
		return invalid("addr: must not be empty")
	}
	if len(c.Backends) == 0 {
		return invalid("backends: at least one backend is required")
	}
	names := make(map[string]bool, len(c.Backends))
	for i, b := range c.Backends {
		field := fmt.Sprintf("backends[%d]", i)
		if !backendNamePattern.MatchString(b.Name) {
			return invalid("%s.name: %q must match %s", field, b.Name, backendNamePattern)
		}
		if names[b.Name] {
			return invalid("%s.name: duplicate backend %q", field, b.Name)
		}
		names[b.Name] = true
		if b.Kind != llm.KindOllama && b.Kind != llm.KindOpenAI {
			return invalid("%s.kind: unknown kind %q", field, b.Kind)
		}
		u, err := url.Parse(b.URL)
		if err != nil {
			return invalid("%s.url: %v", field, err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return invalid("%s.url: %q must be an http or https URL with a host", field, b.URL)
		}
		if b.PullMissing && b.Kind != llm.KindOllama {
			return invalid("%s.pull_missing: only kind %q can pull models", field, llm.KindOllama)
		}
		if b.StartTimeoutSeconds < 0 {
			return invalid("%s.start_timeout_seconds: must not be negative", field)
		}
	}
	if c.Intent.TimeoutSeconds < 0 {
		return invalid("intent.timeout_seconds: must not be negative")
	}
	if !names[c.Intent.Backend] {
		return invalid("intent.backend: unknown backend %q", c.Intent.Backend)
	}
	if c.Intent.Model == "" {
		return invalid("intent.model: must not be empty")
	}
	if len(c.Routes) == 0 {
		return invalid("routes: at least one route is required")
	}
	routes := make(map[string]bool, len(c.Routes))
	for i, r := range c.Routes {
		field := fmt.Sprintf("routes[%d]", i)
		if r.Name == "" {
			return invalid("%s.name: must not be empty", field)
		}
		if r.Name == llm.AutoModel {
			return invalid("%s.name: %q is reserved", field, llm.AutoModel)
		}
		if routes[r.Name] {
			return invalid("%s.name: duplicate route %q", field, r.Name)
		}
		routes[r.Name] = true
		if !names[r.Backend] {
			return invalid("%s.backend: unknown backend %q", field, r.Backend)
		}
		if r.Model == "" {
			return invalid("%s.model: must not be empty", field)
		}
		if err := checkOptions(field+".options", r.Options); err != nil {
			return err
		}
	}
	if !routes[c.DefaultRoute] {
		return invalid("default_route: unknown route %q", c.DefaultRoute)
	}
	return nil
}

// ModelsFor returns the models the given backend must have: the intent
// model if it is on that backend, then each route's model on it, in
// order and without repeats.
func (c Config) ModelsFor(backend string) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(m string) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	if c.Intent.Backend == backend {
		add(c.Intent.Model)
	}
	for _, r := range c.Routes {
		if r.Backend == backend {
			add(r.Model)
		}
	}
	return out
}

// checkOptions validates a route's options: absent or null is allowed, and
// anything else must be a JSON object without a "model" key.
func checkOptions(field string, raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	var opts map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &opts); err != nil || opts == nil {
		return invalid("%s: must be a JSON object", field)
	}
	if _, ok := opts["model"]; ok {
		return invalid("%s.model: not allowed, the gateway sets the model", field)
	}
	return nil
}

// invalid returns an error wrapping llm.ErrInvalid with the formatted message.
func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{llm.ErrInvalid}, args...)...)
}
