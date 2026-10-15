package core

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"decide/internal/jdm"
)

type fakeModels struct {
	graphs map[string]*jdm.Graph
	keys   []string
	listed bool
	getErr error
	gotKey string
}

func (f *fakeModels) Get(_ context.Context, key string) (*jdm.Graph, error) {
	f.gotKey = key
	if f.getErr != nil {
		return nil, f.getErr
	}
	g, ok := f.graphs[key]
	if !ok {
		return nil, ErrNotFound
	}
	return g, nil
}

func (f *fakeModels) List(context.Context) ([]string, error) {
	f.listed = true
	return f.keys, nil
}

type fakeEngine struct {
	out     map[string]any
	err     error
	called  bool
	gotIn   map[string]any
	gotNode *jdm.Graph
}

func (f *fakeEngine) Evaluate(_ context.Context, g *jdm.Graph, input map[string]any) (map[string]any, error) {
	f.called = true
	f.gotNode = g
	f.gotIn = input
	return f.out, f.err
}

func newFixture() (*fakeModels, *fakeEngine, *Service) {
	g := &jdm.Graph{Nodes: []jdm.Node{{ID: "i", Type: jdm.TypeInput}, {ID: "o", Type: jdm.TypeOutput}}}
	models := &fakeModels{graphs: map[string]*jdm.Graph{"loan-check": g}, keys: []string{"a", "loan-check"}}
	engine := &fakeEngine{out: map[string]any{"approved": true}}
	return models, engine, NewService(models, engine)
}

func TestEvaluateRejectsBadKey(t *testing.T) {
	for _, key := range []string{"", "Loan", "-leading", "has space", "under_score", "../etc"} {
		models, engine, svc := newFixture()
		_, err := svc.Evaluate(context.Background(), key, nil)
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("key %q: err = %v, want ErrInvalid", key, err)
		}
		if models.gotKey != "" {
			t.Errorf("key %q: model source was called with %q", key, models.gotKey)
		}
		if engine.called {
			t.Errorf("key %q: engine was called", key)
		}
	}
}

func TestEvaluateMissingModel(t *testing.T) {
	models, engine, svc := newFixture()
	_, err := svc.Evaluate(context.Background(), "missing", nil)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if models.gotKey != "missing" {
		t.Errorf("source got key %q, want missing", models.gotKey)
	}
	if engine.called {
		t.Error("engine was called for a missing model")
	}
}

func TestEvaluateNilInputBecomesEmptyMap(t *testing.T) {
	_, engine, svc := newFixture()
	out, err := svc.Evaluate(context.Background(), "loan-check", nil)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if engine.gotIn == nil || len(engine.gotIn) != 0 {
		t.Errorf("engine got input %#v, want empty non-nil map", engine.gotIn)
	}
	if !reflect.DeepEqual(out, map[string]any{"approved": true}) {
		t.Errorf("out = %#v", out)
	}
}

func TestEvaluatePassesInputThrough(t *testing.T) {
	_, engine, svc := newFixture()
	in := map[string]any{"age": 30}
	if _, err := svc.Evaluate(context.Background(), "loan-check", in); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !reflect.DeepEqual(engine.gotIn, in) {
		t.Errorf("engine got %#v, want %#v", engine.gotIn, in)
	}
	if engine.gotNode == nil {
		t.Error("engine got a nil graph")
	}
}

func TestEvaluateNilResultBecomesEmptyMap(t *testing.T) {
	_, engine, svc := newFixture()
	engine.out = nil
	out, err := svc.Evaluate(context.Background(), "loan-check", map[string]any{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if out == nil || len(out) != 0 {
		t.Errorf("out = %#v, want empty non-nil map", out)
	}
}

func TestEvaluateReturnsEngineErrorUnchanged(t *testing.T) {
	_, engine, svc := newFixture()
	boom := errors.New("boom")
	engine.err = boom
	out, err := svc.Evaluate(context.Background(), "loan-check", nil)
	if err != boom {
		t.Errorf("err = %v, want the engine error itself", err)
	}
	if out != nil {
		t.Errorf("out = %#v, want nil on error", out)
	}
}

func TestModelsReturnsList(t *testing.T) {
	_, _, svc := newFixture()
	got, err := svc.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if !reflect.DeepEqual(got, []string{"a", "loan-check"}) {
		t.Errorf("Models = %v", got)
	}
}

func TestModelsNeverNil(t *testing.T) {
	models, _, svc := newFixture()
	models.keys = nil
	got, err := svc.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("Models = %#v, want empty non-nil slice", got)
	}
}

func TestKeyPattern(t *testing.T) {
	for _, ok := range []string{"a", "0", "loan-check", "v2-rules"} {
		if !KeyPattern.MatchString(ok) {
			t.Errorf("%q should match", ok)
		}
	}
}
