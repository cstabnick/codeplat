package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"decide/internal/core"
	"decide/internal/jdm"
)

// fakeExpr resolves a dotted path in env when it exists, "boom" fails, and any
// other expression is a JSON literal. A cell is a JSON literal the value must equal.
type fakeExpr struct{}

func (fakeExpr) Evaluate(expr string, env map[string]any) (any, error) {
	if expr == "boom" {
		return nil, errors.New("kaboom")
	}
	if v, ok := lookup(expr, env); ok {
		return v, nil
	}
	var v any
	if err := json.Unmarshal([]byte(expr), &v); err != nil {
		return nil, err
	}
	return v, nil
}

func (fakeExpr) Test(cell string, value any, env map[string]any) (bool, error) {
	var want any
	if err := json.Unmarshal([]byte(cell), &want); err != nil {
		return false, err
	}
	return reflect.DeepEqual(want, value), nil
}

func lookup(path string, env map[string]any) (any, bool) {
	var cur any = env
	for _, k := range strings.Split(path, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[k]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func parse(t *testing.T, doc string) *jdm.Graph {
	t.Helper()
	g, err := jdm.Parse([]byte(doc))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return g
}

// tableGraph returns input -> decision table -> output, with content as the table.
func tableGraph(content string) string {
	return `{"nodes":[
		{"id":"in","type":"inputNode","name":"Input"},
		{"id":"t","type":"decisionTableNode","name":"Discount","content":` + content + `},
		{"id":"out","type":"outputNode","name":"Output"}],
		"edges":[{"id":"e1","sourceId":"in","targetId":"t"},{"id":"e2","sourceId":"t","targetId":"out"}]}`
}

// exprGraph returns input -> expression node -> output, with content as the expressions.
func exprGraph(content string) string {
	return `{"nodes":[
		{"id":"in","type":"inputNode","name":"Input"},
		{"id":"x","type":"expressionNode","name":"Calc","content":` + content + `},
		{"id":"out","type":"outputNode","name":"Output"}],
		"edges":[{"id":"e1","sourceId":"in","targetId":"x"},{"id":"e2","sourceId":"x","targetId":"out"}]}`
}

func evaluate(t *testing.T, g *jdm.Graph, input map[string]any) (map[string]any, error) {
	t.Helper()
	return New(fakeExpr{}).Evaluate(context.Background(), g, input)
}

func TestFirstMatchWins(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first",
		"inputs":[{"id":"c1","name":"Tier","field":"tier"}],
		"outputs":[{"id":"o1","name":"Discount","field":"discount"}],
		"rules":[{"c1":"\"gold\"","o1":"20"},{"c1":"\"gold\"","o1":"30"}]}`))
	got, err := evaluate(t, g, map[string]any{"tier": "gold"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"discount": float64(20)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestNoMatchGivesEmptyMap(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first",
		"inputs":[{"id":"c1","name":"Tier","field":"tier"}],
		"outputs":[{"id":"o1","name":"Discount","field":"discount"}],
		"rules":[{"c1":"\"gold\"","o1":"20"}]}`))
	got, err := evaluate(t, g, map[string]any{"tier": "bronze"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if !reflect.DeepEqual(got, map[string]any{}) {
		t.Fatalf("got %v, want empty map", got)
	}
}

func TestEmptyCellsMatchAndSkipOutputs(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first",
		"inputs":[{"id":"c1","name":"Tier","field":"tier"},{"id":"c2","name":"Any","field":""}],
		"outputs":[{"id":"o1","name":"Discount","field":"discount"},{"id":"o2","name":"Note","field":"note"}],
		"rules":[{"c1":"\"gold\"","c2":"null","o1":"5","o2":""}]}`))
	got, err := evaluate(t, g, map[string]any{"tier": "gold"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"discount": float64(5)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestEmptyInputCellMatchesAnyValue(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first",
		"inputs":[{"id":"c1","name":"Tier","field":"tier"}],
		"outputs":[{"id":"o1","name":"Discount","field":"discount"}],
		"rules":[{"c1":"","o1":"9"}]}`))
	got, err := evaluate(t, g, map[string]any{"tier": "anything"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"discount": float64(9)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDottedOutputPath(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first",
		"inputs":[{"id":"c1","name":"Tier","field":"tier"}],
		"outputs":[{"id":"o1","name":"Deep","field":"a.b.c"}],
		"rules":[{"c1":"\"gold\"","o1":"3"}]}`))
	got, err := evaluate(t, g, map[string]any{"tier": "gold"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"a": map[string]any{"b": map[string]any{"c": float64(3)}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpressionNodeUsesDollar(t *testing.T) {
	g := parse(t, exprGraph(`{"expressions":[
		{"id":"e1","key":"x","value":"base"},
		{"id":"e2","key":"y","value":"$.x"}]}`))
	got, err := evaluate(t, g, map[string]any{"base": float64(2)})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"x": float64(2), "y": float64(2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestTwoSourcesAreMerged(t *testing.T) {
	g := parse(t, `{"nodes":[
		{"id":"in","type":"inputNode","name":"Input"},
		{"id":"p","type":"expressionNode","name":"P","content":{"expressions":[{"id":"e1","key":"a.p","value":"1"}]}},
		{"id":"q","type":"expressionNode","name":"Q","content":{"expressions":[{"id":"e2","key":"a.q","value":"2"}]}},
		{"id":"out","type":"outputNode","name":"Output"}],
		"edges":[
		{"id":"e1","sourceId":"in","targetId":"p"},
		{"id":"e2","sourceId":"in","targetId":"q"},
		{"id":"e3","sourceId":"p","targetId":"out"},
		{"id":"e4","sourceId":"q","targetId":"out"}]}`)
	got, err := evaluate(t, g, map[string]any{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]any{"a": map[string]any{"p": float64(1), "q": float64(2)}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExprErrorWrappedAsInvalid(t *testing.T) {
	g := parse(t, exprGraph(`{"expressions":[{"id":"e1","key":"x","value":"boom"}]}`))
	_, err := evaluate(t, g, map[string]any{})
	if err == nil {
		t.Fatal("want an error")
	}
	if !errors.Is(err, core.ErrInvalid) {
		t.Fatalf("error %v does not wrap core.ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), `"Calc"`) || !strings.Contains(err.Error(), "kaboom") {
		t.Fatalf("error %q does not name the node and the cause", err)
	}
}

func TestCancelledContext(t *testing.T) {
	g := parse(t, tableGraph(`{"hitPolicy":"first","inputs":[],"outputs":[],"rules":[]}`))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(fakeExpr{}).Evaluate(ctx, g, map[string]any{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
