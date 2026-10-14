package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"llmrouter/internal/llm"
)

// clearAddrEnv makes sure LLMROUTER_ADDR is unset for the duration of the test.
func clearAddrEnv(t *testing.T) {
	t.Helper()
	t.Setenv("LLMROUTER_ADDR", "")
}

// writeFile writes content to a new file in a temp dir and returns its path.
func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDefaultValidates(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatalf("Default().Validate() = %v, want nil", err)
	}
}

func TestDefaultValues(t *testing.T) {
	c := Default()
	if c.Addr != "127.0.0.1:8083" {
		t.Errorf("Addr = %q, want %q", c.Addr, "127.0.0.1:8083")
	}
	if len(c.Backends) != 1 {
		t.Fatalf("len(Backends) = %d, want 1", len(c.Backends))
	}
	b := c.Backends[0]
	if b.Name != "ollama" || b.Kind != llm.KindOllama || b.URL != "http://127.0.0.1:11434" {
		t.Errorf("backend = %+v, want ollama/%s/http://127.0.0.1:11434", b, llm.KindOllama)
	}
	if !reflect.DeepEqual(b.Command, []string{"ollama", "serve"}) {
		t.Errorf("Command = %q, want [ollama serve]", b.Command)
	}
	if !b.PullMissing {
		t.Error("PullMissing = false, want true")
	}
	if c.Intent.Backend != "ollama" || c.Intent.Model != "llama3.2:1b" {
		t.Errorf("Intent = %+v, want ollama/llama3.2:1b", c.Intent)
	}
	if c.DefaultRoute != "general" {
		t.Errorf("DefaultRoute = %q, want general", c.DefaultRoute)
	}
	want := []llm.Route{
		{Name: "general", Description: "Conversation, questions, writing, and anything no other route fits.", Backend: "ollama", Model: "llama3:8b"},
		{Name: "code", Description: "Writing, explaining, reviewing or debugging source code.", Backend: "ollama", Model: "qwen2.5-coder:1.5b"},
		{Name: "quick", Description: "Greetings and short factual questions with a one-line answer.", Backend: "ollama", Model: "llama3.2:1b"},
	}
	if !reflect.DeepEqual(c.Routes, want) {
		t.Errorf("Routes = %+v, want %+v", c.Routes, want)
	}
}

func TestModelsForDefault(t *testing.T) {
	got := Default().ModelsFor("ollama")
	want := []string{"llama3.2:1b", "llama3:8b", "qwen2.5-coder:1.5b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ModelsFor(ollama) = %q, want %q", got, want)
	}
}

func TestModelsForOtherBackends(t *testing.T) {
	c := Config{
		Addr: "127.0.0.1:8083",
		Backends: []Backend{
			{Name: "a", Kind: llm.KindOllama, URL: "http://a:1"},
			{Name: "b", Kind: llm.KindOpenAI, URL: "http://b:2"},
		},
		Intent:       Intent{Backend: "a", Model: "m1"},
		DefaultRoute: "x",
		Routes: []llm.Route{
			{Name: "x", Backend: "b", Model: "m2"},
			{Name: "y", Backend: "a", Model: "m3"},
			{Name: "z", Backend: "a", Model: "m1"}, // repeat of the intent model
		},
	}
	if got, want := c.ModelsFor("a"), []string{"m1", "m3"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ModelsFor(a) = %q, want %q", got, want)
	}
	if got, want := c.ModelsFor("b"), []string{"m2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ModelsFor(b) = %q, want %q", got, want)
	}
	if got := c.ModelsFor("nope"); len(got) != 0 {
		t.Errorf("ModelsFor(nope) = %q, want none", got)
	}
}

func TestModelsForIntentOnOtherBackend(t *testing.T) {
	c := Default()
	c.Backends = append(c.Backends, Backend{Name: "other", Kind: llm.KindOpenAI, URL: "http://o:3"})
	c.Intent.Backend = "other"
	if got, want := c.ModelsFor("ollama"), []string{"llama3:8b", "qwen2.5-coder:1.5b", "llama3.2:1b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ModelsFor(ollama) = %q, want %q", got, want)
	}
	if got, want := c.ModelsFor("other"), []string{"llama3.2:1b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ModelsFor(other) = %q, want %q", got, want)
	}
}

func TestLoadEmptyPathIsDefault(t *testing.T) {
	clearAddrEnv(t)
	c, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if !reflect.DeepEqual(c, Default()) {
		t.Errorf("Load(\"\") = %+v, want Default()", c)
	}
}

func TestLoadFile(t *testing.T) {
	clearAddrEnv(t)
	path := writeFile(t, `{
		"addr": "0.0.0.0:9000",
		"backends": [
			{"name": "ollama", "kind": "ollama", "url": "http://127.0.0.1:11434",
			 "command": ["ollama", "serve"], "pull_missing": true, "start_timeout_seconds": 30},
			{"name": "llama", "kind": "openai", "url": "http://127.0.0.1:8080",
			 "command": ["llama-server", "-m", "x.gguf"], "dir": "/opt/llama",
			 "env": ["A=1"], "stop_command": ["pkill", "llama-server"]}
		],
		"intent": {"backend": "ollama", "model": "llama3.2:1b", "timeout_seconds": 5},
		"default_route": "chat",
		"routes": [
			{"name": "chat", "description": "Talk.", "backend": "llama", "model": "x"},
			{"name": "quick", "description": "Short.", "backend": "ollama", "model": "llama3.2:1b"}
		]
	}`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load(%s) error: %v", path, err)
	}
	if c.Addr != "0.0.0.0:9000" {
		t.Errorf("Addr = %q, want 0.0.0.0:9000", c.Addr)
	}
	if len(c.Backends) != 2 {
		t.Fatalf("len(Backends) = %d, want 2", len(c.Backends))
	}
	ol := c.Backends[0]
	if !ol.PullMissing || ol.StartTimeoutSeconds != 30 {
		t.Errorf("ollama backend = %+v, want PullMissing and StartTimeoutSeconds 30", ol)
	}
	ll := c.Backends[1]
	if ll.Dir != "/opt/llama" || !reflect.DeepEqual(ll.Env, []string{"A=1"}) ||
		!reflect.DeepEqual(ll.StopCommand, []string{"pkill", "llama-server"}) ||
		!reflect.DeepEqual(ll.Command, []string{"llama-server", "-m", "x.gguf"}) {
		t.Errorf("openai backend = %+v, did not decode fields", ll)
	}
	if c.Intent.TimeoutSeconds != 5 {
		t.Errorf("Intent.TimeoutSeconds = %d, want 5", c.Intent.TimeoutSeconds)
	}
	if c.DefaultRoute != "chat" || len(c.Routes) != 2 {
		t.Errorf("DefaultRoute = %q, len(Routes) = %d; want chat, 2", c.DefaultRoute, len(c.Routes))
	}
}

func TestLoadUnknownFieldFails(t *testing.T) {
	clearAddrEnv(t)
	path := writeFile(t, `{"addr": "127.0.0.1:8083", "bogus": true}`)
	if _, err := Load(path); err == nil {
		t.Fatal("Load with unknown field: error = nil, want error")
	}
}

func TestLoadMissingFileFails(t *testing.T) {
	clearAddrEnv(t)
	path := filepath.Join(t.TempDir(), "does-not-exist.json")
	if _, err := Load(path); err == nil {
		t.Fatal("Load of missing file: error = nil, want error")
	}
}

func TestLoadInvalidFileFails(t *testing.T) {
	clearAddrEnv(t)
	path := writeFile(t, `{"addr": "127.0.0.1:8083", "backends": []}`)
	_, err := Load(path)
	if !errors.Is(err, llm.ErrInvalid) {
		t.Fatalf("Load with no backends: error = %v, want wrapping llm.ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "backends") {
		t.Errorf("error %q does not name field backends", err)
	}
}

func TestLoadEmptyAddrBecomesDefault(t *testing.T) {
	clearAddrEnv(t)
	path := writeFile(t, `{
		"backends": [{"name": "ollama", "kind": "ollama", "url": "http://127.0.0.1:11434"}],
		"intent": {"backend": "ollama", "model": "m"},
		"default_route": "r",
		"routes": [{"name": "r", "backend": "ollama", "model": "m"}]
	}`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Addr != "127.0.0.1:8083" {
		t.Errorf("Addr = %q, want 127.0.0.1:8083", c.Addr)
	}
}

func TestLoadEnvOverridesAddr(t *testing.T) {
	t.Setenv("LLMROUTER_ADDR", "127.0.0.1:7777")

	c, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error: %v", err)
	}
	if c.Addr != "127.0.0.1:7777" {
		t.Errorf("Load(\"\") Addr = %q, want 127.0.0.1:7777", c.Addr)
	}

	path := writeFile(t, `{
		"addr": "127.0.0.1:9999",
		"backends": [{"name": "ollama", "kind": "ollama", "url": "http://127.0.0.1:11434"}],
		"intent": {"backend": "ollama", "model": "m"},
		"default_route": "r",
		"routes": [{"name": "r", "backend": "ollama", "model": "m"}]
	}`)
	c, err = Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Addr != "127.0.0.1:7777" {
		t.Errorf("Load(file) Addr = %q, want env value 127.0.0.1:7777", c.Addr)
	}
}

// validConfig returns a config that passes Validate; each Validate case
// breaks one rule of it.
func validConfig() Config {
	return Default()
}

func TestValidateRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *Config)
		field  string
	}{
		{"empty addr", func(c *Config) { c.Addr = "" }, "addr"},
		{"no backends", func(c *Config) { c.Backends = nil }, "backends"},
		{"backend name with slash", func(c *Config) { c.Backends[0].Name = "a/b" }, "backends[0].name"},
		{"backend name uppercase", func(c *Config) { c.Backends[0].Name = "Ollama" }, "backends[0].name"},
		{"backend name empty", func(c *Config) { c.Backends[0].Name = "" }, "backends[0].name"},
		{"duplicate backend name", func(c *Config) {
			c.Backends = append(c.Backends, Backend{Name: "ollama", Kind: llm.KindOllama, URL: "http://x:1"})
		}, "backends[1].name"},
		{"unknown kind", func(c *Config) { c.Backends[0].Kind = "gemini" }, "backends[0].kind"},
		{"url without scheme", func(c *Config) { c.Backends[0].URL = "127.0.0.1:11434" }, "backends[0].url"},
		{"url with ftp scheme", func(c *Config) { c.Backends[0].URL = "ftp://127.0.0.1:11434" }, "backends[0].url"},
		{"url without host", func(c *Config) { c.Backends[0].URL = "http://" }, "backends[0].url"},
		{"url that does not parse", func(c *Config) { c.Backends[0].URL = "http://[::1" }, "backends[0].url"},
		{"pull_missing on openai", func(c *Config) {
			c.Backends[0].Kind = llm.KindOpenAI
			c.Backends[0].PullMissing = true
		}, "backends[0].pull_missing"},
		{"negative start timeout", func(c *Config) { c.Backends[0].StartTimeoutSeconds = -1 }, "backends[0].start_timeout_seconds"},
		{"negative intent timeout", func(c *Config) { c.Intent.TimeoutSeconds = -1 }, "intent.timeout_seconds"},
		{"intent backend unknown", func(c *Config) { c.Intent.Backend = "x" }, "intent.backend"},
		{"intent model empty", func(c *Config) { c.Intent.Model = "" }, "intent.model"},
		{"no routes", func(c *Config) { c.Routes = nil }, "routes"},
		{"route name empty", func(c *Config) { c.Routes[0].Name = "" }, "routes[0].name"},
		{"route name auto", func(c *Config) { c.Routes[0].Name = llm.AutoModel }, "routes[0].name"},
		{"duplicate route name", func(c *Config) { c.Routes[1].Name = c.Routes[0].Name }, "routes[1].name"},
		{"route backend unknown", func(c *Config) { c.Routes[1].Backend = "nope" }, "routes[1].backend"},
		{"route model empty", func(c *Config) { c.Routes[0].Model = "" }, "routes[0].model"},
		{"default route unknown", func(c *Config) { c.DefaultRoute = "missing" }, "default_route"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validConfig()
			tt.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("Validate() = nil, want error")
			}
			if !errors.Is(err, llm.ErrInvalid) {
				t.Errorf("Validate() error = %v, want wrapping llm.ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tt.field) {
				t.Errorf("Validate() error = %q, want it to name field %q", err, tt.field)
			}
		})
	}
}

func TestValidateAcceptsOpenAIBackend(t *testing.T) {
	c := Default()
	c.Backends = append(c.Backends, Backend{Name: "llama-server", Kind: llm.KindOpenAI, URL: "https://h:8080"})
	c.Routes = append(c.Routes, llm.Route{Name: "big", Backend: "llama-server", Model: "x"})
	if err := c.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}
