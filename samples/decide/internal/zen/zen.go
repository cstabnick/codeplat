// Package zen evaluates a small expression language compatible with the subset
// of GoRules ZEN that decision models use, and decision-table cells written in
// it. It depends only on the standard library.
package zen

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Lang evaluates expressions and decision-table cells.
type Lang struct{}

// Evaluate parses and evaluates expr against env.
func (Lang) Evaluate(expr string, env map[string]any) (any, error) {
	return evaluate(expr, normalizeEnv(env))
}

// Test evaluates a decision-table cell against the column value.
func (Lang) Test(cell string, value any, env map[string]any) (bool, error) {
	c := strings.TrimSpace(cell)
	if c == "" {
		return true, nil
	}
	scope := normalizeEnv(env)
	scope["$"] = normalizeValue(value)
	switch {
	case strings.Contains(c, "$"):
		return evalBool(c, scope)
	case hasComparisonPrefix(c):
		return evalBool("$ "+c, scope)
	case isRange(c):
		return evalBool("$ in "+c, scope)
	}
	for _, part := range splitTopLevel(c) {
		part = strings.TrimSpace(part)
		expr := "$ == " + part
		if isRange(part) {
			expr = "$ in " + part
		}
		ok, err := evalBool(expr, scope)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func evalBool(expr string, scope map[string]any) (bool, error) {
	v, err := evaluate(expr, scope)
	if err != nil {
		return false, err
	}
	b, ok := v.(bool)
	if !ok {
		return false, fail("%q must be a bool, got %s", expr, typeName(v))
	}
	return b, nil
}

func hasComparisonPrefix(c string) bool {
	for _, op := range []string{"<=", ">=", "==", "!=", "<", ">"} {
		if strings.HasPrefix(c, op) {
			return true
		}
	}
	return false
}

func isRange(s string) bool {
	return (strings.HasPrefix(s, "[") || strings.HasPrefix(s, "(")) && strings.Contains(s, "..")
}

// splitTopLevel splits s on commas that are outside quotes and brackets.
func splitTopLevel(s string) []string {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[' || c == '(':
			depth++
		case c == ']' || c == ')':
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

func fail(format string, args ...any) error {
	return fmt.Errorf("zen: "+format, args...)
}

func evaluate(expr string, scope map[string]any) (any, error) {
	toks, err := lex(expr)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	root, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, p.errorf(t, "unexpected %s", t.describe())
	}
	return root.eval(scope)
}

// normalizeEnv returns a copy of env whose values use the canonical types.
func normalizeEnv(env map[string]any) map[string]any {
	out := make(map[string]any, len(env)+1)
	for k, v := range env {
		out[k] = normalizeValue(v)
	}
	return out
}

// normalizeValue converts Go integers and floats to float64, and slices and
// string-keyed maps to []any and map[string]any. Other values are kept as is.
func normalizeValue(v any) any {
	switch x := v.(type) {
	case nil, bool, float64, string:
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalizeValue(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalizeValue(e)
		}
		return out
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return rv.String()
	case reflect.Bool:
		return rv.Bool()
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = normalizeValue(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if rv.Type().Key().Kind() == reflect.String {
			out := make(map[string]any, rv.Len())
			iter := rv.MapRange()
			for iter.Next() {
				out[iter.Key().String()] = normalizeValue(iter.Value().Interface())
			}
			return out
		}
	case reflect.Ptr:
		if rv.IsNil() {
			return nil
		}
		return normalizeValue(rv.Elem().Interface())
	}
	return v
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64:
		return "number"
	case string:
		return "string"
	case []any:
		return "list"
	case map[string]any:
		return "map"
	}
	return fmt.Sprintf("%T", v)
}

// ---- lexer ----

type tokKind int

const (
	tokEOF tokKind = iota
	tokNum
	tokStr
	tokName // identifiers and keywords
	tokSym  // punctuation and operators
)

type token struct {
	kind tokKind
	text string // source text, identifier, symbol, or decoded string contents
	num  float64
	pos  int
}

func (t token) describe() string {
	if t.kind == tokEOF {
		return "end of expression"
	}
	return strconv.Quote(t.text)
}

var twoCharSymbols = []string{"==", "!=", "<=", ">=", "&&", "||", ".."}

const oneCharSymbols = "()[],.?:+-*/%<>!"

func lex(src string) ([]token, error) {
	var toks []token
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case isDigit(c):
			start := i
			for i < len(src) && isDigit(src[i]) {
				i++
			}
			if i+1 < len(src) && src[i] == '.' && isDigit(src[i+1]) {
				i++
				for i < len(src) && isDigit(src[i]) {
					i++
				}
			}
			n, err := strconv.ParseFloat(src[start:i], 64)
			if err != nil {
				return nil, fail("invalid number %q at offset %d", src[start:i], start)
			}
			toks = append(toks, token{kind: tokNum, text: src[start:i], num: n, pos: start})
		case c == '"' || c == '\'':
			s, end, err := lexString(src, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tokStr, text: s, pos: i})
			i = end
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			toks = append(toks, token{kind: tokName, text: src[start:i], pos: start})
		default:
			sym, ok := matchSymbol(src[i:])
			if !ok {
				return nil, fail("unexpected character %q at offset %d", c, i)
			}
			toks = append(toks, token{kind: tokSym, text: sym, pos: i})
			i += len(sym)
		}
	}
	return append(toks, token{kind: tokEOF, pos: len(src)}), nil
}

// lexString decodes the quoted string that starts at src[start] and returns
// its contents and the offset just past the closing quote.
func lexString(src string, start int) (string, int, error) {
	quote := src[start]
	var b strings.Builder
	for i := start + 1; i < len(src); {
		ch := src[i]
		if ch == quote {
			return b.String(), i + 1, nil
		}
		if ch != '\\' {
			b.WriteByte(ch)
			i++
			continue
		}
		if i+1 >= len(src) {
			break
		}
		next := src[i+1]
		if next != '"' && next != '\'' && next != '\\' {
			return "", 0, fail("unknown escape \\%c at offset %d", next, i)
		}
		b.WriteByte(next)
		i += 2
	}
	return "", 0, fail("unterminated string at offset %d", start)
}

func matchSymbol(s string) (string, bool) {
	for _, sym := range twoCharSymbols {
		if strings.HasPrefix(s, sym) {
			return sym, true
		}
	}
	if len(s) > 0 && strings.IndexByte(oneCharSymbols, s[0]) >= 0 {
		return s[:1], true
	}
	return "", false
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || isDigit(c)
}

// ---- parser ----

type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token {
	return p.toks[p.pos]
}

func (p *parser) peekAt(n int) token {
	if p.pos+n < len(p.toks) {
		return p.toks[p.pos+n]
	}
	return p.toks[len(p.toks)-1]
}

func (p *parser) next() token {
	t := p.toks[p.pos]
	if t.kind != tokEOF {
		p.pos++
	}
	return t
}

func (p *parser) isSym(s string) bool {
	t := p.peek()
	return t.kind == tokSym && t.text == s
}

func (p *parser) isName(s string) bool {
	t := p.peek()
	return t.kind == tokName && t.text == s
}

func (p *parser) expectSym(s string) error {
	if !p.isSym(s) {
		return p.errorf(p.peek(), "expected %q, found %s", s, p.peek().describe())
	}
	p.next()
	return nil
}

func (p *parser) errorf(t token, format string, args ...any) error {
	return fail("syntax error at offset %d: %s", t.pos, fmt.Sprintf(format, args...))
}

// parseTernary handles c ? a : b, the lowest precedence level.
func (p *parser) parseTernary() (node, error) {
	cond, err := p.parseOr()
	if err != nil || !p.isSym("?") {
		return cond, err
	}
	p.next()
	yes, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if err := p.expectSym(":"); err != nil {
		return nil, err
	}
	no, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	return ternary{cond: cond, yes: yes, no: no}, nil
}

func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isName("or") || p.isSym("||") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = logic{and: false, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseAnd() (node, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.isName("and") || p.isSym("&&") {
		p.next()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = logic{and: true, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseNot() (node, error) {
	if p.isName("not") || p.isSym("!") {
		p.next()
		x, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return not{x: x}, nil
	}
	return p.parseComparison()
}

func isComparison(s string) bool {
	switch s {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	}
	return false
}

func (p *parser) parseComparison() (node, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		t := p.peek()
		switch {
		case t.kind == tokSym && isComparison(t.text):
			p.next()
			right, err := p.parseAdditive()
			if err != nil {
				return nil, err
			}
			left = binary{op: t.text, left: left, right: right}
		case p.isName("in"):
			p.next()
			set, err := p.parseIn()
			if err != nil {
				return nil, err
			}
			left = in{value: left, set: set}
		case p.isName("not") && p.peekAt(1).kind == tokName && p.peekAt(1).text == "in":
			p.next()
			p.next()
			set, err := p.parseIn()
			if err != nil {
				return nil, err
			}
			left = in{value: left, set: set, negate: true}
		default:
			return left, nil
		}
	}
}

// parseIn parses the right side of in: a range such as [a..b) or an ordinary
// expression. A bracket that does not start a range is re-parsed as an
// expression.
func (p *parser) parseIn() (node, error) {
	if p.isSym("[") || p.isSym("(") {
		save := p.pos
		open := p.next()
		lo, err := p.parseAdditive()
		if err == nil && p.isSym("..") {
			p.next()
			hi, err := p.parseAdditive()
			if err != nil {
				return nil, err
			}
			closing := p.next()
			if closing.kind != tokSym || (closing.text != "]" && closing.text != ")") {
				return nil, p.errorf(closing, "expected ] or ) to close the range")
			}
			return rangeNode{lo: lo, hi: hi, loIncl: open.text == "[", hiIncl: closing.text == "]"}, nil
		}
		p.pos = save
	}
	return p.parseAdditive()
}

func (p *parser) parseAdditive() (node, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.isSym("+") || p.isSym("-") {
		op := p.next().text
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		left = binary{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseMultiplicative() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.isSym("*") || p.isSym("/") || p.isSym("%") {
		op := p.next().text
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = binary{op: op, left: left, right: right}
	}
	return left, nil
}

func (p *parser) parseUnary() (node, error) {
	if p.isSym("-") {
		p.next()
		x, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return negate{x: x}, nil
	}
	return p.parsePostfix()
}

func (p *parser) parsePostfix() (node, error) {
	x, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isSym("."):
			p.next()
			t := p.next()
			if t.kind != tokName {
				return nil, p.errorf(t, "expected a member name after '.', found %s", t.describe())
			}
			x = member{base: x, name: t.text}
		case p.isSym("["):
			p.next()
			idx, err := p.parseTernary()
			if err != nil {
				return nil, err
			}
			if err := p.expectSym("]"); err != nil {
				return nil, err
			}
			x = index{base: x, idx: idx}
		case p.isSym("("):
			id, ok := x.(identifier)
			if !ok {
				return nil, p.errorf(p.peek(), "only a named function can be called")
			}
			p.next()
			var args []node
			if !p.isSym(")") {
				for {
					arg, err := p.parseTernary()
					if err != nil {
						return nil, err
					}
					args = append(args, arg)
					if !p.isSym(",") {
						break
					}
					p.next()
				}
			}
			if err := p.expectSym(")"); err != nil {
				return nil, err
			}
			x = call{name: id.name, args: args}
		default:
			return x, nil
		}
	}
}

func (p *parser) parsePrimary() (node, error) {
	t := p.next()
	switch t.kind {
	case tokNum:
		return literal{value: t.num}, nil
	case tokStr:
		return literal{value: t.text}, nil
	case tokName:
		switch t.text {
		case "true":
			return literal{value: true}, nil
		case "false":
			return literal{value: false}, nil
		case "null":
			return literal{value: nil}, nil
		case "and", "or", "not", "in":
			return nil, p.errorf(t, "unexpected keyword %q", t.text)
		}
		return identifier{name: t.text}, nil
	case tokSym:
		switch t.text {
		case "(":
			x, err := p.parseTernary()
			if err != nil {
				return nil, err
			}
			if err := p.expectSym(")"); err != nil {
				return nil, err
			}
			return x, nil
		case "[":
			var items []node
			if !p.isSym("]") {
				for {
					item, err := p.parseTernary()
					if err != nil {
						return nil, err
					}
					items = append(items, item)
					if !p.isSym(",") {
						break
					}
					p.next()
				}
			}
			if err := p.expectSym("]"); err != nil {
				return nil, err
			}
			return list{items: items}, nil
		}
	}
	return nil, p.errorf(t, "unexpected %s", t.describe())
}

// ---- syntax tree and evaluation ----

type node interface {
	eval(scope map[string]any) (any, error)
}

type literal struct{ value any }

func (n literal) eval(map[string]any) (any, error) { return n.value, nil }

type identifier struct{ name string }

func (n identifier) eval(scope map[string]any) (any, error) { return scope[n.name], nil }

type list struct{ items []node }

func (n list) eval(scope map[string]any) (any, error) {
	out := make([]any, len(n.items))
	for i, item := range n.items {
		v, err := item.eval(scope)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

type ternary struct{ cond, yes, no node }

func (n ternary) eval(scope map[string]any) (any, error) {
	c, err := n.cond.eval(scope)
	if err != nil {
		return nil, err
	}
	b, err := truthy(c)
	if err != nil {
		return nil, err
	}
	if b {
		return n.yes.eval(scope)
	}
	return n.no.eval(scope)
}

// logic is a short-circuiting and or or.
type logic struct {
	and         bool
	left, right node
}

func (n logic) eval(scope map[string]any) (any, error) {
	l, err := n.left.eval(scope)
	if err != nil {
		return nil, err
	}
	lb, err := truthy(l)
	if err != nil {
		return nil, err
	}
	if n.and && !lb {
		return false, nil
	}
	if !n.and && lb {
		return true, nil
	}
	r, err := n.right.eval(scope)
	if err != nil {
		return nil, err
	}
	return truthy(r)
}

type not struct{ x node }

func (n not) eval(scope map[string]any) (any, error) {
	v, err := n.x.eval(scope)
	if err != nil {
		return nil, err
	}
	b, err := truthy(v)
	if err != nil {
		return nil, err
	}
	return !b, nil
}

type negate struct{ x node }

func (n negate) eval(scope map[string]any) (any, error) {
	v, err := n.x.eval(scope)
	if err != nil {
		return nil, err
	}
	f, ok := v.(float64)
	if !ok {
		return nil, fail("unary - needs a number, got %s", typeName(v))
	}
	return -f, nil
}

type binary struct {
	op          string
	left, right node
}

func (n binary) eval(scope map[string]any) (any, error) {
	l, err := n.left.eval(scope)
	if err != nil {
		return nil, err
	}
	r, err := n.right.eval(scope)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "==":
		return equalValues(l, r), nil
	case "!=":
		return !equalValues(l, r), nil
	case "<", "<=", ">", ">=":
		return compare(n.op, l, r)
	}
	return arithmetic(n.op, l, r)
}

func arithmetic(op string, l, r any) (any, error) {
	if ls, ok := l.(string); ok && op == "+" {
		if rs, ok := r.(string); ok {
			return ls + rs, nil
		}
	}
	lf, lok := l.(float64)
	rf, rok := r.(float64)
	if !lok || !rok {
		return nil, fail("%s cannot combine %s and %s", op, typeName(l), typeName(r))
	}
	switch op {
	case "+":
		return lf + rf, nil
	case "-":
		return lf - rf, nil
	case "*":
		return lf * rf, nil
	case "/":
		if rf == 0 {
			return nil, fail("division by zero")
		}
		return lf / rf, nil
	case "%":
		if rf == 0 {
			return nil, fail("modulo by zero")
		}
		return math.Mod(lf, rf), nil
	}
	return nil, fail("unknown operator %s", op)
}

func compare(op string, l, r any) (bool, error) {
	lf, lIsNum := l.(float64)
	rf, rIsNum := r.(float64)
	ls, lIsStr := l.(string)
	rs, rIsStr := r.(string)
	var less, greater bool
	switch {
	case lIsNum && rIsNum:
		less, greater = lf < rf, lf > rf
	case lIsStr && rIsStr:
		less, greater = ls < rs, ls > rs
	default:
		return false, fail("%s needs two numbers or two strings, got %s and %s", op, typeName(l), typeName(r))
	}
	switch op {
	case "<":
		return less, nil
	case "<=":
		return !greater, nil
	case ">":
		return greater, nil
	}
	return !less, nil
}

// equalValues compares any two values: numbers numerically, lists element-wise
// and maps key-wise.
func equalValues(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equalValues(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !equalValues(v, w) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(a, b)
}

type in struct {
	value  node
	set    node
	negate bool
}

func (n in) eval(scope map[string]any) (any, error) {
	v, err := n.value.eval(scope)
	if err != nil {
		return nil, err
	}
	var found bool
	if r, ok := n.set.(rangeNode); ok {
		found, err = r.contains(v, scope)
	} else {
		var set any
		set, err = n.set.eval(scope)
		if err == nil {
			found, err = contains(set, v)
		}
	}
	if err != nil {
		return nil, err
	}
	return found != n.negate, nil
}

// contains reports whether v is an element of a list, or a substring of a string.
func contains(set, v any) (bool, error) {
	switch s := set.(type) {
	case []any:
		for _, e := range s {
			if equalValues(e, v) {
				return true, nil
			}
		}
		return false, nil
	case string:
		sub, ok := v.(string)
		if !ok {
			return false, fail("a string can only contain a string, got %s", typeName(v))
		}
		return strings.Contains(s, sub), nil
	}
	return false, fail("in needs a list or a string on the right, got %s", typeName(set))
}

// rangeNode is a numeric range. It is only valid as the right side of in.
type rangeNode struct {
	lo, hi         node
	loIncl, hiIncl bool
}

func (n rangeNode) eval(map[string]any) (any, error) {
	return nil, fail("a range is only valid after in")
}

func (n rangeNode) contains(v any, scope map[string]any) (bool, error) {
	x, ok := v.(float64)
	if !ok {
		return false, nil
	}
	lo, err := numberOf(n.lo, scope)
	if err != nil {
		return false, err
	}
	hi, err := numberOf(n.hi, scope)
	if err != nil {
		return false, err
	}
	aboveLo := x > lo || (n.loIncl && x == lo)
	belowHi := x < hi || (n.hiIncl && x == hi)
	return aboveLo && belowHi, nil
}

func numberOf(n node, scope map[string]any) (float64, error) {
	v, err := n.eval(scope)
	if err != nil {
		return 0, err
	}
	f, ok := v.(float64)
	if !ok {
		return 0, fail("range bound must be a number, got %s", typeName(v))
	}
	return f, nil
}

type member struct {
	base node
	name string
}

func (n member) eval(scope map[string]any) (any, error) {
	b, err := n.base.eval(scope)
	if err != nil {
		return nil, err
	}
	switch base := b.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return base[n.name], nil
	}
	return nil, fail("cannot read .%s of %s", n.name, typeName(b))
}

type index struct {
	base, idx node
}

func (n index) eval(scope map[string]any) (any, error) {
	b, err := n.base.eval(scope)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, nil
	}
	k, err := n.idx.eval(scope)
	if err != nil {
		return nil, err
	}
	switch base := b.(type) {
	case map[string]any:
		key, ok := k.(string)
		if !ok {
			return nil, fail("a map key must be a string, got %s", typeName(k))
		}
		return base[key], nil
	case []any:
		f, ok := k.(float64)
		if !ok {
			return nil, fail("a list index must be a number, got %s", typeName(k))
		}
		if f < 0 || f != math.Trunc(f) {
			return nil, fail("a list index must be a non-negative integer, got %v", f)
		}
		if f >= float64(len(base)) {
			return nil, nil
		}
		return base[int(f)], nil
	}
	return nil, fail("cannot index %s", typeName(b))
}

type call struct {
	name string
	args []node
}

func (n call) eval(scope map[string]any) (any, error) {
	args := make([]any, len(n.args))
	for i, a := range n.args {
		v, err := a.eval(scope)
		if err != nil {
			return nil, err
		}
		args[i] = v
	}
	return callFunction(n.name, args)
}

var arity = map[string]int{
	"len": 1, "abs": 1, "floor": 1, "ceil": 1, "round": 1,
	"min": 1, "max": 1, "sum": 1, "contains": 2, "lower": 1, "upper": 1,
}

var mathFuncs = map[string]func(float64) float64{
	"abs": math.Abs, "floor": math.Floor, "ceil": math.Ceil, "round": math.Round,
}

func callFunction(name string, args []any) (any, error) {
	n, known := arity[name]
	if !known {
		return nil, fail("unknown function %s", name)
	}
	if len(args) != n {
		return nil, fail("%s takes %d argument(s), got %d", name, n, len(args))
	}
	if f, ok := mathFuncs[name]; ok {
		x, ok := args[0].(float64)
		if !ok {
			return nil, fail("%s needs a number, got %s", name, typeName(args[0]))
		}
		return f(x), nil
	}
	switch name {
	case "len":
		switch x := args[0].(type) {
		case string:
			return float64(utf8.RuneCountInString(x)), nil
		case []any:
			return float64(len(x)), nil
		}
		return nil, fail("len needs a string or a list, got %s", typeName(args[0]))
	case "min", "max", "sum":
		nums, err := numberList(name, args[0])
		if err != nil {
			return nil, err
		}
		return reduceNumbers(name, nums)
	case "contains":
		s, ok1 := args[0].(string)
		sub, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return nil, fail("contains needs two strings, got %s and %s", typeName(args[0]), typeName(args[1]))
		}
		return strings.Contains(s, sub), nil
	case "lower", "upper":
		s, ok := args[0].(string)
		if !ok {
			return nil, fail("%s needs a string, got %s", name, typeName(args[0]))
		}
		if name == "lower" {
			return strings.ToLower(s), nil
		}
		return strings.ToUpper(s), nil
	}
	return nil, fail("unknown function %s", name)
}

func numberList(name string, v any) ([]float64, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fail("%s needs a list, got %s", name, typeName(v))
	}
	out := make([]float64, len(list))
	for i, e := range list {
		f, ok := e.(float64)
		if !ok {
			return nil, fail("%s needs a list of numbers, found %s", name, typeName(e))
		}
		out[i] = f
	}
	return out, nil
}

func reduceNumbers(name string, nums []float64) (any, error) {
	if name == "sum" {
		total := 0.0
		for _, f := range nums {
			total += f
		}
		return total, nil
	}
	if len(nums) == 0 {
		return nil, fail("%s of an empty list", name)
	}
	best := nums[0]
	for _, f := range nums[1:] {
		if (name == "min" && f < best) || (name == "max" && f > best) {
			best = f
		}
	}
	return best, nil
}

// truthy reads a condition: nil counts as false, and anything but a bool is an error.
func truthy(v any) (bool, error) {
	switch x := v.(type) {
	case nil:
		return false, nil
	case bool:
		return x, nil
	}
	return false, fail("expected a bool, got %s", typeName(v))
}
