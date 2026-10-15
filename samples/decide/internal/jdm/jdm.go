// Package jdm parses and validates decision models in the JDM file format.
package jdm

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Node types.
const (
	TypeInput      = "inputNode"
	TypeOutput     = "outputNode"
	TypeTable      = "decisionTableNode"
	TypeExpression = "expressionNode"
)

// ErrInvalid is wrapped by every error Parse, Validate, Order, Table and Expressions return.
var ErrInvalid = errors.New("invalid model")

// Graph is a decision model: nodes connected by edges.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Node is one step of a decision model. Content depends on Type.
type Node struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Name    string          `json:"name"`
	Content json.RawMessage `json:"content,omitempty"`
}

// Edge connects two nodes by ID.
type Edge struct {
	ID       string `json:"id"`
	SourceID string `json:"sourceId"`
	TargetID string `json:"targetId"`
}

// Table is the content of a decision table node.
type Table struct {
	HitPolicy string              `json:"hitPolicy"`
	Inputs    []Column            `json:"inputs"`
	Outputs   []Column            `json:"outputs"`
	Rules     []map[string]string `json:"rules"` // column ID -> cell text; also "_id", "_description"
}

// Column is an input or output column of a decision table.
type Column struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Field string `json:"field"` // an expression for inputs, a dotted path for outputs
}

// ExpressionContent is the content of an expression node.
type ExpressionContent struct {
	Expressions []Expression `json:"expressions"`
}

// Expression assigns the value of an expression to a dotted output path.
type Expression struct {
	ID    string `json:"id"`
	Key   string `json:"key"`   // dotted output path
	Value string `json:"value"` // expression
}

// Parse decodes a JDM document and validates it. Unknown fields are ignored.
func Parse(data []byte) (*Graph, error) {
	var g Graph
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := g.Validate(); err != nil {
		return nil, err
	}
	return &g, nil
}

// Validate reports the first rule the graph breaks.
func (g *Graph) Validate() error {
	if len(g.Nodes) == 0 {
		return fmt.Errorf("%w: no nodes", ErrInvalid)
	}
	ids := make(map[string]bool, len(g.Nodes))
	inputs, outputs := 0, 0
	for _, n := range g.Nodes {
		if n.ID == "" {
			return fmt.Errorf("%w: a node has an empty id", ErrInvalid)
		}
		if ids[n.ID] {
			return fmt.Errorf("%w: duplicate node %q", ErrInvalid, n.ID)
		}
		ids[n.ID] = true
		switch n.Type {
		case TypeInput:
			inputs++
		case TypeOutput:
			outputs++
		case TypeTable:
			if err := n.validateTable(); err != nil {
				return err
			}
		case TypeExpression:
			if err := n.validateExpressions(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: node %q: unknown type %q", ErrInvalid, n.ID, n.Type)
		}
	}
	if inputs != 1 {
		return fmt.Errorf("%w: want exactly one input node, have %d", ErrInvalid, inputs)
	}
	if outputs == 0 {
		return fmt.Errorf("%w: no output node", ErrInvalid)
	}
	for _, e := range g.Edges {
		if !ids[e.SourceID] {
			return fmt.Errorf("%w: edge %q: source %q is not a node", ErrInvalid, e.ID, e.SourceID)
		}
		if !ids[e.TargetID] {
			return fmt.Errorf("%w: edge %q: target %q is not a node", ErrInvalid, e.ID, e.TargetID)
		}
	}
	_, err := g.Order()
	return err
}

// Order returns the nodes in topological order. Ties keep the order of Nodes.
// It returns an error wrapping ErrInvalid if the edges form a cycle.
func (g *Graph) Order() ([]Node, error) {
	n := len(g.Nodes)
	index := make(map[string]int, n)
	for i, node := range g.Nodes {
		if _, ok := index[node.ID]; !ok {
			index[node.ID] = i
		}
	}
	indegree := make([]int, n)
	successors := make([][]int, n)
	for _, e := range g.Edges {
		src, ok := index[e.SourceID]
		if !ok {
			return nil, fmt.Errorf("%w: edge %q: source %q is not a node", ErrInvalid, e.ID, e.SourceID)
		}
		dst, ok := index[e.TargetID]
		if !ok {
			return nil, fmt.Errorf("%w: edge %q: target %q is not a node", ErrInvalid, e.ID, e.TargetID)
		}
		successors[src] = append(successors[src], dst)
		indegree[dst]++
	}
	done := make([]bool, n)
	out := make([]Node, 0, n)
	for len(out) < n {
		pick := -1
		for i := range g.Nodes {
			if !done[i] && indegree[i] == 0 {
				pick = i
				break
			}
		}
		if pick < 0 {
			for i := range g.Nodes {
				if !done[i] {
					return nil, fmt.Errorf("%w: cycle through node %q", ErrInvalid, g.Nodes[i].ID)
				}
			}
		}
		done[pick] = true
		out = append(out, g.Nodes[pick])
		for _, s := range successors[pick] {
			indegree[s]--
		}
	}
	return out, nil
}

// Table decodes the content of a decision table node.
func (n Node) Table() (Table, error) {
	if n.Type != TypeTable {
		return Table{}, fmt.Errorf("%w: node %q is %q, not a decision table", ErrInvalid, n.ID, n.Type)
	}
	var t Table
	if len(n.Content) == 0 {
		return t, nil
	}
	if err := json.Unmarshal(n.Content, &t); err != nil {
		return Table{}, fmt.Errorf("%w: node %q: %v", ErrInvalid, n.ID, err)
	}
	return t, nil
}

// Expressions decodes the content of an expression node.
func (n Node) Expressions() (ExpressionContent, error) {
	if n.Type != TypeExpression {
		return ExpressionContent{}, fmt.Errorf("%w: node %q is %q, not an expression", ErrInvalid, n.ID, n.Type)
	}
	var c ExpressionContent
	if len(n.Content) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(n.Content, &c); err != nil {
		return ExpressionContent{}, fmt.Errorf("%w: node %q: %v", ErrInvalid, n.ID, err)
	}
	return c, nil
}

func (n Node) validateTable() error {
	t, err := n.Table()
	if err != nil {
		return err
	}
	if t.HitPolicy != "" && t.HitPolicy != "first" {
		return fmt.Errorf("%w: node %q: hitPolicy %q is not supported", ErrInvalid, n.ID, t.HitPolicy)
	}
	for _, c := range t.Inputs {
		if c.ID == "" {
			return fmt.Errorf("%w: node %q: an input column has an empty id", ErrInvalid, n.ID)
		}
	}
	for _, c := range t.Outputs {
		if c.ID == "" {
			return fmt.Errorf("%w: node %q: an output column has an empty id", ErrInvalid, n.ID)
		}
		if c.Field == "" {
			return fmt.Errorf("%w: node %q: output column %q has an empty field", ErrInvalid, n.ID, c.ID)
		}
	}
	return nil
}

func (n Node) validateExpressions() error {
	c, err := n.Expressions()
	if err != nil {
		return err
	}
	for _, e := range c.Expressions {
		if e.Key == "" {
			return fmt.Errorf("%w: node %q: expression %q has an empty key", ErrInvalid, n.ID, e.ID)
		}
	}
	return nil
}
