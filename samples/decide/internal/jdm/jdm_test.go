package jdm

import (
	"errors"
	"strings"
	"testing"
)

const validGraph = `{
  "nodes": [
    {"id": "in", "type": "inputNode", "name": "request", "position": {"x": 1}},
    {"id": "table", "type": "decisionTableNode", "name": "policy",
     "content": {"hitPolicy": "first",
       "inputs": [{"id": "c1", "name": "age", "field": "age"}],
       "outputs": [{"id": "c2", "name": "tier", "field": "tier"}],
       "rules": [{"_id": "r1", "c1": "> 18", "c2": "\"adult\""}]}},
    {"id": "out", "type": "outputNode", "name": "response"}
  ],
  "edges": [
    {"id": "e1", "sourceId": "in", "targetId": "table"},
    {"id": "e2", "sourceId": "table", "targetId": "out"}
  ]
}`

func mustNode(t *testing.T, g *Graph, id string) Node {
	t.Helper()
	for _, n := range g.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("no node %q", id)
	return Node{}
}

func TestParseValidGraphAndOrder(t *testing.T) {
	g, err := Parse([]byte(validGraph))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(g.Nodes) != 3 || len(g.Edges) != 2 {
		t.Fatalf("got %d nodes and %d edges, want 3 and 2", len(g.Nodes), len(g.Edges))
	}
	order, err := g.Order()
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	want := []string{"in", "table", "out"}
	if len(order) != len(want) {
		t.Fatalf("Order returned %d nodes, want %d", len(order), len(want))
	}
	for i, id := range want {
		if order[i].ID != id {
			t.Errorf("order[%d] = %q, want %q", i, order[i].ID, id)
		}
	}
}

func TestOrderKeepsNodeOrderForTies(t *testing.T) {
	g := &Graph{
		Nodes: []Node{
			{ID: "in", Type: TypeInput},
			{ID: "b", Type: TypeExpression},
			{ID: "a", Type: TypeExpression},
			{ID: "out", Type: TypeOutput},
		},
		Edges: []Edge{
			{ID: "1", SourceID: "in", TargetID: "a"},
			{ID: "2", SourceID: "in", TargetID: "b"},
			{ID: "3", SourceID: "a", TargetID: "out"},
			{ID: "4", SourceID: "b", TargetID: "out"},
		},
	}
	order, err := g.Order()
	if err != nil {
		t.Fatalf("Order: %v", err)
	}
	var got []string
	for _, n := range order {
		got = append(got, n.ID)
	}
	if strings.Join(got, ",") != "in,b,a,out" {
		t.Errorf("Order = %v, want [in b a out]", got)
	}
}

func TestParseRejectsInvalidModels(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantMsg string
	}{
		{
			name:    "no nodes",
			json:    `{"nodes": [], "edges": []}`,
			wantMsg: "no nodes",
		},
		{
			name:    "empty node id",
			json:    `{"nodes": [{"id": "", "type": "inputNode"}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "empty id",
		},
		{
			name:    "duplicate node id",
			json:    `{"nodes": [{"id": "x", "type": "inputNode"}, {"id": "x", "type": "outputNode"}]}`,
			wantMsg: "duplicate node \"x\"",
		},
		{
			name:    "unknown type",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "o", "type": "outputNode"}, {"id": "p", "type": "functionNode"}]}`,
			wantMsg: "unknown type \"functionNode\"",
		},
		{
			name:    "no input node",
			json:    `{"nodes": [{"id": "o", "type": "outputNode"}]}`,
			wantMsg: "exactly one input node, have 0",
		},
		{
			name:    "two input nodes",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "j", "type": "inputNode"}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "exactly one input node, have 2",
		},
		{
			name:    "no output node",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}]}`,
			wantMsg: "no output node",
		},
		{
			name:    "edge source missing",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "o", "type": "outputNode"}], "edges": [{"id": "e", "sourceId": "nope", "targetId": "o"}]}`,
			wantMsg: "source \"nope\" is not a node",
		},
		{
			name:    "edge target missing",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "o", "type": "outputNode"}], "edges": [{"id": "e", "sourceId": "i", "targetId": "nope"}]}`,
			wantMsg: "target \"nope\" is not a node",
		},
		{
			name: "cycle",
			json: `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "a", "type": "expressionNode"}, {"id": "b", "type": "expressionNode"}, {"id": "o", "type": "outputNode"}],
				"edges": [{"id": "1", "sourceId": "i", "targetId": "a"}, {"id": "2", "sourceId": "a", "targetId": "b"}, {"id": "3", "sourceId": "b", "targetId": "a"}, {"id": "4", "sourceId": "b", "targetId": "o"}]}`,
			wantMsg: "cycle",
		},
		{
			name:    "hit policy not first",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "policy", "type": "decisionTableNode", "content": {"hitPolicy": "collect"}}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "node \"policy\": hitPolicy \"collect\" is not supported",
		},
		{
			name:    "table input column without id",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "t", "type": "decisionTableNode", "content": {"inputs": [{"name": "x"}]}}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "input column has an empty id",
		},
		{
			name:    "table output column without id",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "t", "type": "decisionTableNode", "content": {"outputs": [{"field": "x"}]}}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "output column has an empty id",
		},
		{
			name:    "table output column without field",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "t", "type": "decisionTableNode", "content": {"outputs": [{"id": "c1"}]}}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "output column \"c1\" has an empty field",
		},
		{
			name:    "expression without key",
			json:    `{"nodes": [{"id": "i", "type": "inputNode"}, {"id": "e", "type": "expressionNode", "content": {"expressions": [{"id": "x", "value": "1"}]}}, {"id": "o", "type": "outputNode"}]}`,
			wantMsg: "expression \"x\" has an empty key",
		},
		{
			name:    "malformed json",
			json:    `{"nodes": [`,
			wantMsg: "invalid model",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.json))
			if err == nil {
				t.Fatal("Parse succeeded, want error")
			}
			if !errors.Is(err, ErrInvalid) {
				t.Errorf("error %v does not wrap ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	if _, err := Parse([]byte(validGraph)); err != nil {
		t.Fatalf("Parse with position fields: %v", err)
	}
}

func TestTableOnTableNode(t *testing.T) {
	g, err := Parse([]byte(validGraph))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	tbl, err := mustNode(t, g, "table").Table()
	if err != nil {
		t.Fatalf("Table: %v", err)
	}
	if tbl.HitPolicy != "first" {
		t.Errorf("HitPolicy = %q, want first", tbl.HitPolicy)
	}
	if len(tbl.Inputs) != 1 || tbl.Inputs[0].ID != "c1" || tbl.Inputs[0].Field != "age" {
		t.Errorf("Inputs = %+v", tbl.Inputs)
	}
	if len(tbl.Outputs) != 1 || tbl.Outputs[0].ID != "c2" || tbl.Outputs[0].Field != "tier" {
		t.Errorf("Outputs = %+v", tbl.Outputs)
	}
	if len(tbl.Rules) != 1 || tbl.Rules[0]["_id"] != "r1" || tbl.Rules[0]["c2"] != "\"adult\"" {
		t.Errorf("Rules = %+v", tbl.Rules)
	}
}

func TestTableOnNonTableNode(t *testing.T) {
	g, err := Parse([]byte(validGraph))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	_, err = mustNode(t, g, "in").Table()
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("Table on input node: err = %v, want ErrInvalid", err)
	}
}
