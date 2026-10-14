package router

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"llmrouter/internal/llm"
)

var _ llm.Router = (*Router)(nil)

// fakeClassifier records its arguments and returns a set answer or error.
type fakeClassifier struct {
	mu      sync.Mutex
	answer  string
	err     error
	calls   int
	model   string
	system  string
	prompt  string
	labels  []string
	onCall  func()
	blockOn bool
}

func (f *fakeClassifier) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	f.mu.Lock()
	f.calls++
	f.model, f.system, f.prompt = model, system, prompt
	f.labels = append([]string(nil), labels...)
	onCall, block := f.onCall, f.blockOn
	answer, err := f.answer, f.err
	f.mu.Unlock()
	if onCall != nil {
		onCall()
	}
	if block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return answer, err
}

func (f *fakeClassifier) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// fakeClock returns a time that only moves when the test moves it.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

var testRoutes = []llm.Route{
	{Name: "code", Description: "Writing or debugging code.", Backend: "ollama", Model: "coder"},
	{Name: "general", Description: "Anything else.", Backend: "ollama", Model: "chat"},
}

func validOpts() Options {
	return Options{
		IntentBackend: "ollama",
		IntentModel:   "intent",
		Routes:        testRoutes,
		Default:       "general",
	}
}

func newRouter(t *testing.T, cl *fakeClassifier, opts Options) *Router {
	t.Helper()
	r, err := New(map[string]Classifier{"ollama": cl}, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r
}

func user(s string) llm.Message { return llm.Message{Role: "user", Content: s} }

func TestNewValidation(t *testing.T) {
	cl := &fakeClassifier{}
	classifiers := map[string]Classifier{"ollama": cl}

	cases := []struct {
		name    string
		classes map[string]Classifier
		mutate  func(o *Options)
	}{
		{"empty intent model", classifiers, func(o *Options) { o.IntentModel = "" }},
		{"intent backend not configured", classifiers, func(o *Options) { o.IntentBackend = "llama" }},
		{"no routes", classifiers, func(o *Options) { o.Routes = nil }},
		{"empty route name", classifiers, func(o *Options) {
			o.Routes = []llm.Route{{Name: "", Backend: "ollama", Model: "m"}}
			o.Default = ""
		}},
		{"empty route backend", classifiers, func(o *Options) {
			o.Routes = []llm.Route{{Name: "general", Backend: "", Model: "m"}}
			o.Default = "general"
		}},
		{"empty route model", classifiers, func(o *Options) {
			o.Routes = []llm.Route{{Name: "general", Backend: "ollama", Model: ""}}
			o.Default = "general"
		}},
		{"duplicate route name", classifiers, func(o *Options) {
			o.Routes = []llm.Route{
				{Name: "general", Backend: "ollama", Model: "a"},
				{Name: "general", Backend: "ollama", Model: "b"},
			}
			o.Default = "general"
		}},
		{"route backend not configured", classifiers, func(o *Options) {
			o.Routes = []llm.Route{{Name: "general", Backend: "llama", Model: "m"}}
			o.Default = "general"
		}},
		{"default not a route", classifiers, func(o *Options) { o.Default = "missing" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := validOpts()
			tc.mutate(&opts)
			r, err := New(tc.classes, opts)
			if err == nil {
				t.Fatalf("New returned router %v, want error", r)
			}
			if !errors.Is(err, llm.ErrInvalid) {
				t.Fatalf("error %v does not wrap llm.ErrInvalid", err)
			}
		})
	}

	if _, err := New(classifiers, validOpts()); err != nil {
		t.Fatalf("valid options: %v", err)
	}
}

func TestSystemPrompt(t *testing.T) {
	want := "You route chat messages. Pick the one route that fits the user's message best.\n\n" +
		"Routes:\n" +
		"- code: Writing or debugging code.\n" +
		"- general: Anything else.\n\n" +
		"Answer with JSON: {\"route\": \"<name>\"}"
	if got := SystemPrompt(testRoutes); got != want {
		t.Fatalf("SystemPrompt =\n%q\nwant\n%q", got, want)
	}
}

func TestRouteClassified(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	r := newRouter(t, cl, validOpts())

	d := r.Route(context.Background(), []llm.Message{user("fix this loop")})
	if d.Route != "code" || d.Backend != "ollama" || d.Model != "coder" {
		t.Fatalf("decision route = %q/%q/%q, want code/ollama/coder", d.Route, d.Backend, d.Model)
	}
	if d.Fallback || d.Error != "" {
		t.Fatalf("Fallback=%v Error=%q, want no fallback", d.Fallback, d.Error)
	}
	if d.Prompt != "fix this loop" {
		t.Fatalf("Prompt = %q", d.Prompt)
	}
}

func TestRouteAnswerIsTrimmedAndCaseInsensitive(t *testing.T) {
	cl := &fakeClassifier{answer: "  CODE \n"}
	r := newRouter(t, cl, validOpts())

	d := r.Route(context.Background(), []llm.Message{user("hi")})
	if d.Route != "code" || d.Fallback {
		t.Fatalf("Route=%q Fallback=%v, want code without fallback", d.Route, d.Fallback)
	}
}

func TestRouteUnknownAnswerFallsBack(t *testing.T) {
	cl := &fakeClassifier{answer: "poetry"}
	r := newRouter(t, cl, validOpts())

	d := r.Route(context.Background(), []llm.Message{user("write a sonnet")})
	if d.Route != "general" || d.Backend != "ollama" || d.Model != "chat" {
		t.Fatalf("decision route = %q/%q/%q, want general/ollama/chat", d.Route, d.Backend, d.Model)
	}
	if !d.Fallback {
		t.Fatal("Fallback = false, want true")
	}
	if want := `unknown route "poetry"`; d.Error != want {
		t.Fatalf("Error = %q, want %q", d.Error, want)
	}
}

func TestRouteClassifyErrorFallsBack(t *testing.T) {
	cl := &fakeClassifier{err: errors.New("backend exploded")}
	r := newRouter(t, cl, validOpts())

	d := r.Route(context.Background(), []llm.Message{user("hi")})
	if d.Route != "general" || !d.Fallback {
		t.Fatalf("Route=%q Fallback=%v, want general with fallback", d.Route, d.Fallback)
	}
	if d.Error != "backend exploded" {
		t.Fatalf("Error = %q, want %q", d.Error, "backend exploded")
	}
}

func TestRouteNoUserMessage(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	r := newRouter(t, cl, validOpts())

	d := r.Route(context.Background(), []llm.Message{{Role: "system", Content: "be nice"}})
	if d.Route != "general" || !d.Fallback || d.Error != "no user message" {
		t.Fatalf("Route=%q Fallback=%v Error=%q, want general, fallback, \"no user message\"", d.Route, d.Fallback, d.Error)
	}
	if n := cl.callCount(); n != 0 {
		t.Fatalf("classifier called %d times, want 0", n)
	}
	if got := r.Recent(0); len(got) != 1 {
		t.Fatalf("Recent has %d decisions, want the no-user-message decision recorded", len(got))
	}
}

func TestRouteSingleRouteSkipsClassification(t *testing.T) {
	cl := &fakeClassifier{answer: "anything"}
	opts := validOpts()
	opts.Routes = testRoutes[:1]
	opts.Default = "code"
	r := newRouter(t, cl, opts)

	d := r.Route(context.Background(), []llm.Message{user("hi")})
	if d.Route != "code" || d.Model != "coder" || d.Fallback || d.Error != "" {
		t.Fatalf("decision = %+v, want code without fallback", d)
	}
	if n := cl.callCount(); n != 0 {
		t.Fatalf("classifier called %d times, want 0", n)
	}
	if got := r.Recent(0); len(got) != 1 {
		t.Fatalf("Recent has %d decisions, want the single-route decision recorded", len(got))
	}
}

func TestRouteClassifierArguments(t *testing.T) {
	cl := &fakeClassifier{answer: "general"}
	r := newRouter(t, cl, validOpts())

	r.Route(context.Background(), []llm.Message{
		user("first question"),
		{Role: "assistant", Content: "an answer"},
		user("the last user question"),
		{Role: "assistant", Content: "trailing assistant"},
	})

	if cl.model != "intent" {
		t.Errorf("model = %q, want intent", cl.model)
	}
	if want := SystemPrompt(testRoutes); cl.system != want {
		t.Errorf("system = %q, want %q", cl.system, want)
	}
	if cl.prompt != "the last user question" {
		t.Errorf("prompt = %q, want the last user message", cl.prompt)
	}
	if want := []string{"code", "general"}; !equalStrings(cl.labels, want) {
		t.Errorf("labels = %v, want %v", cl.labels, want)
	}
}

func TestRoutePromptCutTo200Runes(t *testing.T) {
	cl := &fakeClassifier{answer: "general"}
	r := newRouter(t, cl, validOpts())

	long := strings.Repeat("é", 250) // multi-byte runes: cut by rune, not byte
	d := r.Route(context.Background(), []llm.Message{user(long)})
	if got := len([]rune(d.Prompt)); got != 200 {
		t.Fatalf("Prompt has %d runes, want 200", got)
	}
	if d.Prompt != strings.Repeat("é", 200) {
		t.Fatalf("Prompt is not the first 200 runes")
	}
	if got := len([]rune(cl.prompt)); got != 250 {
		t.Fatalf("classifier got %d runes, want the full 250", got)
	}
}

func TestRouteRecordsTimeAndDuration(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)}
	start := clock.Now()
	cl := &fakeClassifier{answer: "code", onCall: func() { clock.Advance(5 * time.Millisecond) }}
	opts := validOpts()
	opts.Now = clock.Now
	r := newRouter(t, cl, opts)

	d := r.Route(context.Background(), []llm.Message{user("hi")})
	if !d.At.Equal(start) {
		t.Fatalf("At = %v, want %v", d.At, start)
	}
	if d.Took != 5*time.Millisecond {
		t.Fatalf("Took = %v, want 5ms", d.Took)
	}
}

func TestRouteTimeout(t *testing.T) {
	cl := &fakeClassifier{blockOn: true}
	opts := validOpts()
	opts.Timeout = 20 * time.Millisecond
	r := newRouter(t, cl, opts)

	d := r.Route(context.Background(), []llm.Message{user("hi")})
	if d.Route != "general" || !d.Fallback {
		t.Fatalf("Route=%q Fallback=%v, want general with fallback", d.Route, d.Fallback)
	}
	if d.Error == "" {
		t.Fatal("Error is empty, want the timeout error")
	}
}

func TestRoutesCopy(t *testing.T) {
	r := newRouter(t, &fakeClassifier{}, validOpts())

	got := r.Routes()
	if len(got) != 2 || got[0].Name != "code" || got[1].Name != "general" {
		t.Fatalf("Routes = %+v, want code then general", got)
	}
	got[0].Name = "changed"
	if again := r.Routes(); again[0].Name != "code" {
		t.Fatal("mutating the returned slice changed the router")
	}
}

func TestDefaultRoute(t *testing.T) {
	r := newRouter(t, &fakeClassifier{}, validOpts())
	if got := r.DefaultRoute(); got != "general" {
		t.Fatalf("DefaultRoute = %q, want general", got)
	}
}

func TestRecentOrderAndLimits(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	r := newRouter(t, cl, validOpts())

	for _, p := range []string{"one", "two", "three"} {
		r.Route(context.Background(), []llm.Message{user(p)})
	}

	all := r.Recent(0)
	if len(all) != 3 {
		t.Fatalf("Recent(0) has %d, want 3", len(all))
	}
	if all[0].Prompt != "three" || all[1].Prompt != "two" || all[2].Prompt != "one" {
		t.Fatalf("Recent(0) prompts = %q %q %q, want newest first", all[0].Prompt, all[1].Prompt, all[2].Prompt)
	}
	if neg := r.Recent(-1); len(neg) != 3 {
		t.Fatalf("Recent(-1) has %d, want 3", len(neg))
	}
	if two := r.Recent(2); len(two) != 2 || two[0].Prompt != "three" || two[1].Prompt != "two" {
		t.Fatalf("Recent(2) = %+v, want three and two", two)
	}
	if big := r.Recent(10); len(big) != 3 {
		t.Fatalf("Recent(10) has %d, want 3", len(big))
	}
}

func TestRecentNeverNil(t *testing.T) {
	r := newRouter(t, &fakeClassifier{}, validOpts())
	if got := r.Recent(0); got == nil || len(got) != 0 {
		t.Fatalf("Recent(0) on empty history = %#v, want empty non-nil slice", got)
	}
	if got := r.Recent(5); got == nil {
		t.Fatal("Recent(5) on empty history returned nil")
	}
}

func TestHistoryCap(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	opts := validOpts()
	opts.History = 2
	r := newRouter(t, cl, opts)

	for _, p := range []string{"one", "two", "three"} {
		r.Route(context.Background(), []llm.Message{user(p)})
	}

	got := r.Recent(0)
	if len(got) != 2 {
		t.Fatalf("Recent(0) has %d, want 2 with History=2", len(got))
	}
	if got[0].Prompt != "three" || got[1].Prompt != "two" {
		t.Fatalf("Recent(0) prompts = %q %q, want three and two", got[0].Prompt, got[1].Prompt)
	}
}

func TestHistoryDefaultCap(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	r := newRouter(t, cl, validOpts())

	for i := 0; i < 105; i++ {
		r.Route(context.Background(), []llm.Message{user("p")})
	}
	if n := len(r.Recent(0)); n != 100 {
		t.Fatalf("Recent(0) has %d with default History, want 100", n)
	}
}

func TestConcurrentUse(t *testing.T) {
	cl := &fakeClassifier{answer: "code"}
	r := newRouter(t, cl, validOpts())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				r.Route(context.Background(), []llm.Message{user("hi")})
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				_ = r.Recent(5)
				_ = r.Routes()
				_ = r.DefaultRoute()
			}
		}()
	}
	wg.Wait()
	if n := len(r.Recent(0)); n != 100 {
		t.Fatalf("Recent(0) has %d after 400 routes, want 100", n)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
