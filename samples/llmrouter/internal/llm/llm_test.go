package llm

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

// fakeBackend implements Backend.
type fakeBackend struct {
	name    string
	kind    string
	version string
	err     error
}

func (f *fakeBackend) Name() string { return f.name }
func (f *fakeBackend) Kind() string { return f.kind }

func (f *fakeBackend) Health(ctx context.Context) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.version, nil
}

func (f *fakeBackend) ListModels(ctx context.Context) ([]Model, error) { return nil, nil }

func (f *fakeBackend) Classify(ctx context.Context, model, system, prompt string, labels []string) (string, error) {
	return "", nil
}

func (f *fakeBackend) ChatCompletions(ctx context.Context, body []byte) (*http.Response, error) {
	return nil, nil
}

// fakeManager implements Backend and ModelManager.
type fakeManager struct {
	fakeBackend
	loaded    []LoadedModel
	loadedErr error
}

func (f *fakeManager) LoadedModels(ctx context.Context) ([]LoadedModel, error) {
	return f.loaded, f.loadedErr
}

func (f *fakeManager) PullModel(ctx context.Context, name string) error { return nil }

func (f *fakeManager) DeleteModel(ctx context.Context, name string) error { return nil }

func TestCheckBackendUnhealthy(t *testing.T) {
	b := &fakeBackend{name: "ollama", kind: KindOllama, err: errors.New("connection refused")}
	s := CheckBackend(context.Background(), b, false)
	if s.Reachable {
		t.Error("Reachable = true, want false")
	}
	if s.Error != "connection refused" {
		t.Errorf("Error = %q, want %q", s.Error, "connection refused")
	}
	if s.Loaded == nil || len(s.Loaded) != 0 {
		t.Errorf("Loaded = %#v, want non-nil empty slice", s.Loaded)
	}
}

func TestCheckBackendPlainReachable(t *testing.T) {
	b := &fakeBackend{name: "llama", kind: KindOpenAI, version: "1.2.3"}
	s := CheckBackend(context.Background(), b, false)
	if !s.Reachable {
		t.Error("Reachable = false, want true")
	}
	if s.Version != "1.2.3" {
		t.Errorf("Version = %q, want 1.2.3", s.Version)
	}
	if s.Error != "" {
		t.Errorf("Error = %q, want empty", s.Error)
	}
	if s.Loaded == nil || len(s.Loaded) != 0 {
		t.Errorf("Loaded = %#v, want non-nil empty slice", s.Loaded)
	}
}

func TestCheckBackendModelManager(t *testing.T) {
	want := []LoadedModel{{Name: "qwen", SizeVRAM: 42}}
	b := &fakeManager{
		fakeBackend: fakeBackend{name: "ollama", kind: KindOllama, version: "0.9"},
		loaded:      want,
	}
	s := CheckBackend(context.Background(), b, true)
	if !s.Reachable || s.Version != "0.9" {
		t.Errorf("Reachable=%v Version=%q, want true and 0.9", s.Reachable, s.Version)
	}
	if !reflect.DeepEqual(s.Loaded, want) {
		t.Errorf("Loaded = %#v, want %#v", s.Loaded, want)
	}
}

func TestCheckBackendModelManagerNilLoaded(t *testing.T) {
	b := &fakeManager{fakeBackend: fakeBackend{name: "ollama", kind: KindOllama}}
	s := CheckBackend(context.Background(), b, false)
	if s.Loaded == nil || len(s.Loaded) != 0 {
		t.Errorf("Loaded = %#v, want non-nil empty slice", s.Loaded)
	}
}

func TestCheckBackendCopiesFields(t *testing.T) {
	b := &fakeBackend{name: "ollama", kind: KindOllama, version: "x"}
	for _, managed := range []bool{true, false} {
		s := CheckBackend(context.Background(), b, managed)
		if s.Name != "ollama" || s.Kind != KindOllama {
			t.Errorf("Name=%q Kind=%q, want ollama and %s", s.Name, s.Kind, KindOllama)
		}
		if s.Managed != managed {
			t.Errorf("Managed = %v, want %v", s.Managed, managed)
		}
	}
}

func TestNewFleetBackends(t *testing.T) {
	a := &fakeBackend{name: "a"}
	b := &fakeBackend{name: "b"}
	c := &fakeBackend{name: "c"}
	f := NewFleet([]Backend{a, b, c}, nil)
	got := f.Backends()
	if len(got) != 3 || got[0] != a || got[1] != b || got[2] != c {
		t.Errorf("Backends() = %v, want a, b, c in order", got)
	}
	// The returned slice is a copy; changing it must not change the fleet.
	got[0] = nil
	if f.Backends()[0] != a {
		t.Error("Backends() result aliases the fleet's internal slice")
	}
}

func TestNewFleetBackendLookup(t *testing.T) {
	x := &fakeBackend{name: "x"}
	f := NewFleet([]Backend{x}, nil)
	b, ok := f.Backend("x")
	if !ok || b != x {
		t.Errorf("Backend(\"x\") = %v, %v; want the x backend, true", b, ok)
	}
	b, ok = f.Backend("missing")
	if ok || b != nil {
		t.Errorf("Backend(\"missing\") = %v, %v; want nil, false", b, ok)
	}
}

func TestNewFleetManaged(t *testing.T) {
	calls := []string{}
	f := NewFleet(nil, func(name string) bool {
		calls = append(calls, name)
		return name == "x"
	})
	if !f.Managed("x") {
		t.Error("Managed(\"x\") = false, want true")
	}
	if f.Managed("y") {
		t.Error("Managed(\"y\") = true, want false")
	}
	if !reflect.DeepEqual(calls, []string{"x", "y"}) {
		t.Errorf("managed function calls = %v, want [x y]", calls)
	}
}

func TestNewFleetManagedNilFunc(t *testing.T) {
	f := NewFleet([]Backend{&fakeBackend{name: "x"}}, nil)
	if f.Managed("x") {
		t.Error("Managed(\"x\") with nil function = true, want false")
	}
}
