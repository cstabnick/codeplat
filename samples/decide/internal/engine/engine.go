// Package engine evaluates JDM decision graphs. It walks the nodes in
// topological order, runs decision tables and expression nodes, and returns
// the merged result of the output nodes.
package engine

import (
	"context"
	"fmt"
	"strings"

	"decide/internal/core"
	"decide/internal/jdm"
)

// Expr is the expression language the engine needs.
type Expr interface {
	Evaluate(expr string, env map[string]any) (any, error)
	Test(cell string, value any, env map[string]any) (bool, error)
}

// Engine evaluates decision graphs with an Expr.
type Engine struct {
	x Expr
}

var _ core.Engine = (*Engine)(nil)

// New returns an Engine that evaluates expressions with x.
func New(x Expr) *Engine {
	return &Engine{x: x}
}

// Evaluate runs the graph g on input and returns the merged output of its output nodes.
func (e *Engine) Evaluate(ctx context.Context, g *jdm.Graph, input map[string]any) (map[string]any, error) {
	if g == nil {
		return nil, fmt.Errorf("%w: nil graph", core.ErrInvalid)
	}
	order, err := g.Order()
	if err != nil {
		return nil, err
	}
	if input == nil {
		input = map[string]any{}
	}
	outputs := make(map[string]map[string]any, len(order))
	result := map[string]any{}
	for _, node := range order {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if node.Type == jdm.TypeInput {
			outputs[node.ID] = input
			continue
		}
		nodeInput := sourceInput(g, node.ID, outputs)
		var out map[string]any
		switch node.Type {
		case jdm.TypeTable:
			out, err = e.table(node, nodeInput)
		case jdm.TypeExpression:
			out, err = e.expressions(node, nodeInput)
		case jdm.TypeOutput:
			out = nodeInput
			result = deepMerge(result, out)
		default:
			return nil, fmt.Errorf("%w: node %q: unknown type %q", core.ErrInvalid, node.Name, node.Type)
		}
		if err != nil {
			return nil, err
		}
		outputs[node.ID] = out
	}
	return result, nil
}

// sourceInput returns the deep merge of the outputs of the sources of the node
// with the given ID, in edge order.
func sourceInput(g *jdm.Graph, id string, outputs map[string]map[string]any) map[string]any {
	in := map[string]any{}
	for _, edge := range g.Edges {
		if edge.TargetID != id {
			continue
		}
		out, ok := outputs[edge.SourceID]
		if !ok {
			continue
		}
		in = deepMerge(in, out)
	}
	return in
}

// table runs a decision table node with the first hit policy.
func (e *Engine) table(node jdm.Node, nodeInput map[string]any) (map[string]any, error) {
	t, err := node.Table()
	if err != nil {
		return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
	}
	values := make([]any, len(t.Inputs))
	evaluated := make([]bool, len(t.Inputs))
	for _, rule := range t.Rules {
		matched := true
		for i, col := range t.Inputs {
			cell := rule[col.ID]
			if cell == "" {
				continue
			}
			if !evaluated[i] {
				v, err := e.value(col.Field, nodeInput)
				if err != nil {
					return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
				}
				values[i] = v
				evaluated[i] = true
			}
			ok, err := e.x.Test(cell, values[i], nodeInput)
			if err != nil {
				return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
			}
			if !ok {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		out := map[string]any{}
		for _, col := range t.Outputs {
			cell := rule[col.ID]
			if cell == "" {
				continue
			}
			v, err := e.x.Evaluate(cell, nodeInput)
			if err != nil {
				return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
			}
			out = setPath(out, col.Field, v)
		}
		return out, nil
	}
	return map[string]any{}, nil
}

// value evaluates an input column field. An empty field has the value nil.
func (e *Engine) value(field string, env map[string]any) (any, error) {
	if field == "" {
		return nil, nil
	}
	return e.x.Evaluate(field, env)
}

// expressions runs an expression node, assigning each expression in order.
func (e *Engine) expressions(node jdm.Node, nodeInput map[string]any) (map[string]any, error) {
	c, err := node.Expressions()
	if err != nil {
		return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
	}
	out := map[string]any{}
	for _, expr := range c.Expressions {
		env := make(map[string]any, len(nodeInput)+1)
		for k, v := range nodeInput {
			env[k] = v
		}
		env["$"] = out
		v, err := e.x.Evaluate(expr.Value, env)
		if err != nil {
			return nil, fmt.Errorf("%w: node %q: %v", core.ErrInvalid, node.Name, err)
		}
		out = setPath(out, expr.Key, v)
	}
	return out, nil
}

// setPath sets v at the dotted path in m and returns the map. Intermediate maps
// are copied rather than modified, so values shared with the input stay unchanged.
func setPath(m map[string]any, path string, v any) map[string]any {
	keys := strings.Split(path, ".")
	return setKeys(m, keys, v)
}

func setKeys(m map[string]any, keys []string, v any) map[string]any {
	res := make(map[string]any, len(m)+1)
	for k, val := range m {
		res[k] = val
	}
	if len(keys) == 1 {
		res[keys[0]] = v
		return res
	}
	inner, _ := res[keys[0]].(map[string]any)
	res[keys[0]] = setKeys(inner, keys[1:], v)
	return res
}

// deepMerge returns a new map with the entries of src merged into a copy of dst.
// Maps merge recursively; otherwise the value from src wins.
func deepMerge(dst, src map[string]any) map[string]any {
	res := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		res[k] = v
	}
	for k, sv := range src {
		if sm, ok := sv.(map[string]any); ok {
			if dm, ok := res[k].(map[string]any); ok {
				res[k] = deepMerge(dm, sm)
				continue
			}
		}
		res[k] = sv
	}
	return res
}
