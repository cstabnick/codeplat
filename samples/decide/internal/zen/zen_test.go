package zen

import (
	"reflect"
	"testing"
)

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name string
		expr string
		env  map[string]any
		want any
	}{
		// literals
		{"integer", "12", nil, 12.0},
		{"decimal", "1.5", nil, 1.5},
		{"double quoted string", `"abc"`, nil, "abc"},
		{"single quoted string", `'abc'`, nil, "abc"},
		{"escaped double quote", `"say \"hi\""`, nil, `say "hi"`},
		{"escaped single quote", `'it\'s'`, nil, "it's"},
		{"escaped backslash", `"a\\b"`, nil, `a\b`},
		{"true", "true", nil, true},
		{"false", "false", nil, false},
		{"null", "null", nil, nil},
		{"list", `[1, "a"]`, nil, []any{1.0, "a"}},
		{"empty list", `[]`, nil, []any{}},
		{"parentheses", "(1 + 2) * 3", nil, 9.0},

		// precedence and associativity
		{"mul binds tighter than add", "1 + 2 * 3", nil, 7.0},
		{"subtraction is left associative", "10 - 4 - 3", nil, 3.0},
		{"division is left associative", "8 / 4 / 2", nil, 1.0},
		{"ternary is lowest", "1 == 1 ? 2 + 3 : 0", nil, 5.0},
		{"comparison binds tighter than and", "1 == 1 and 2 == 2", nil, true},
		{"not binds looser than comparison", "not 1 == 2", nil, true},
		{"and binds tighter than or", "true or false and false", nil, true},
		{"nested ternary", "true ? false ? 1 : 2 : 3", nil, 2.0},

		// arithmetic
		{"add", "1 + 2", nil, 3.0},
		{"subtract", "5 - 8", nil, -3.0},
		{"multiply", "1.5 * 2", nil, 3.0},
		{"divide", "7 / 2", nil, 3.5},
		{"modulo", "7 % 4", nil, 3.0},
		{"unary minus", "-3 + 5", nil, 2.0},
		{"double unary minus", "- -3", nil, 3.0},
		{"string concatenation", `"a" + "b"`, nil, "ab"},

		// comparison
		{"less than", "1 < 2", nil, true},
		{"less than or equal", "2 <= 2", nil, true},
		{"greater than", "3 > 4", nil, false},
		{"greater than or equal", "4 >= 4", nil, true},
		{"string less than", `"a" < "b"`, nil, true},
		{"string greater or equal", `"b" >= "a"`, nil, true},
		{"equal numbers", "1 == 1.0", nil, true},
		{"not equal", "1 != 2", nil, true},
		{"null equals null", "null == null", nil, true},
		{"equal lists", "[1, 2] == [1, 2]", nil, true},
		{"unequal lists", "[1, 2] == [1, 3]", nil, false},
		{"string not equal number", `"1" == 1`, nil, false},

		// logic
		{"and", "true and false", nil, false},
		{"&&", "true && true", nil, true},
		{"or", "false or true", nil, true},
		{"||", "false || false", nil, false},
		{"not", "not false", nil, true},
		{"!", "!true", nil, false},
		{"null is false in and", "null and true", nil, false},
		{"null is true under not", "!null", nil, true},
		{"true short circuits or", "true or null", nil, true},

		// in, not in and ranges
		{"in list", "2 in [1, 2]", nil, true},
		{"not in list", "3 not in [1, 2]", nil, true},
		{"in string", `"at" in "cat"`, nil, true},
		{"in inclusive range", "5 in [1..10]", nil, true},
		{"range lower bound inclusive", "1 in [1..10]", nil, true},
		{"exclusive lower bound", "1 in (1..10]", nil, false},
		{"exclusive upper bound", "10 in [1..10)", nil, false},
		{"upper bound inclusive", "10 in (1..10]", nil, true},
		{"exclusive range", "5 in (1..10)", nil, true},
		{"non-number in range", "x in [1..10]", nil, false},
		{"not in range", "11 not in [1..10]", nil, true},
		{"range bounds from env", "5 in [a..b]", map[string]any{"a": 1, "b": 5}, true},

		// members, indexes and identifiers
		{"member", "a.b", map[string]any{"a": map[string]any{"b": 3}}, 3.0},
		{"missing member", "a.c", map[string]any{"a": map[string]any{"b": 3}}, nil},
		{"member of missing identifier", "missing.x", nil, nil},
		{"list index", `xs[0]`, map[string]any{"xs": []any{"p", "q"}}, "p"},
		{"list index out of range", "xs[5]", map[string]any{"xs": []any{"p"}}, nil},
		{"map index", `m["k"]`, map[string]any{"m": map[string]any{"k": "v"}}, "v"},
		{"unknown identifier is nil", "missing", nil, nil},
		{"unknown identifier equals null", "missing == null", nil, true},
		{"identifier with dollar", "$x + 1", map[string]any{"$x": 1}, 2.0},

		// Go integers in env become float64
		{"int in env", "n + 1", map[string]any{"n": 3}, 4.0},
		{"uint8 in env", "u * 2", map[string]any{"u": uint8(4)}, 8.0},
		{"nested int slice in env", "sum(xs)", map[string]any{"xs": []int{1, 2, 3}}, 6.0},

		// functions
		{"len of string counts runes", `len("héllo")`, nil, 5.0},
		{"len of list", "len(xs)", map[string]any{"xs": []any{1, 2}}, 2.0},
		{"abs", "abs(-2)", nil, 2.0},
		{"floor", "floor(1.7)", nil, 1.0},
		{"ceil", "ceil(1.2)", nil, 2.0},
		{"round", "round(2.5)", nil, 3.0},
		{"min", "min([3, 1, 2])", nil, 1.0},
		{"max", "max([3, 1, 2])", nil, 3.0},
		{"sum", "sum([1, 2, 3])", nil, 6.0},
		{"sum of empty list", "sum([])", nil, 0.0},
		{"contains", `contains("abc", "b")`, nil, true},
		{"lower", `lower("AbC")`, nil, "abc"},
		{"upper", `upper("AbC")`, nil, "ABC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Lang{}.Evaluate(tc.expr, tc.env)
			if err != nil {
				t.Fatalf("Evaluate(%q) error: %v", tc.expr, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Evaluate(%q) = %#v, want %#v", tc.expr, got, tc.want)
			}
		})
	}
}

func TestEvaluateErrors(t *testing.T) {
	cases := []struct {
		name string
		expr string
		env  map[string]any
	}{
		{"empty expression", "", nil},
		{"dangling operator", "1 +", nil},
		{"unclosed parenthesis", "(1", nil},
		{"two values in a row", "1 2", nil},
		{"unterminated string", `"abc`, nil},
		{"unknown escape", `"\x"`, nil},
		{"unexpected character", "1 = 2", nil},
		{"keyword as value", "in", nil},
		{"unclosed list", "[1, 2", nil},
		{"range without closing bracket", "3 in [1..2 + 1", nil},
		{"division by zero", "1 / 0", nil},
		{"modulo by zero", "1 % 0", nil},
		{"compare number with string", `1 < "a"`, nil},
		{"compare null", "null < 1", nil},
		{"add number and string", `"a" + 1`, nil},
		{"multiply strings", `"a" * 2`, nil},
		{"negate string", `-"a"`, nil},
		{"condition not bool", "1 ? 2 : 3", nil},
		{"and with number", "1 and true", nil},
		{"not with number", "not 1", nil},
		{"member of number", "x.y", map[string]any{"x": 3}},
		{"index with fraction", "xs[1.5]", map[string]any{"xs": []any{1, 2}}},
		{"negative index", "xs[-1]", map[string]any{"xs": []any{1, 2}}},
		{"index of number", "x[0]", map[string]any{"x": 3}},
		{"map key not string", "m[1]", map[string]any{"m": map[string]any{}}},
		{"in with number on right", "1 in 2", nil},
		{"string in with number", `1 in "a"`, nil},
		{"range bound not number", `5 in ["a".."z"]`, nil},
		{"unknown function", "nope(1)", nil},
		{"wrong argument count", "len()", nil},
		{"len of number", "len(1)", nil},
		{"abs of string", `abs("a")`, nil},
		{"min of empty list", "min([])", nil},
		{"min of mixed list", `min([1, "a"])`, nil},
		{"contains with number", "contains(1, 2)", nil},
		{"lower of number", "lower(1)", nil},
		{"call a value", "(1)(2)", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Lang{}.Evaluate(tc.expr, tc.env)
			if err == nil {
				t.Fatalf("Evaluate(%q) = %#v, want an error", tc.expr, got)
			}
		})
	}
}

func TestTest(t *testing.T) {
	cases := []struct {
		name  string
		cell  string
		value any
		env   map[string]any
		want  bool
	}{
		{"empty cell matches", "", "anything", nil, true},
		{"whitespace cell matches", "   ", 1.0, nil, true},
		{"string equal", `"US"`, "US", nil, true},
		{"string not equal", `"US"`, "UK", nil, false},
		{"greater than", "> 5", 7.0, nil, true},
		{"greater than fails", "> 5", 5.0, nil, false},
		{"less than or equal", "<= 5", 5.0, nil, true},
		{"range inside", "[1..10]", 5.0, nil, true},
		{"range outside", "[1..10]", 11.0, nil, false},
		{"list of values matches second", `"a", "b"`, "b", nil, true},
		{"list of values matches none", `"a", "b"`, "c", nil, false},
		{"comma inside quotes is not a split", `"a", "b, c"`, "b, c", nil, true},
		{"list literal item", "[1, 2], 3", []any{1.0, 2.0}, nil, true},
		{"dollar expression", "$ > 2 and $ < 4", 3.0, nil, true},
		{"dollar expression fails", "$ > 2 and $ < 4", 5.0, nil, false},
		{"dollar with arithmetic", "$ % 2 == 0", 4.0, nil, true},
		{"boolean true matches", "true", true, nil, true},
		{"boolean false does not match true", "true", false, nil, false},
		{"bare value compared with env", "limit", 3.0, map[string]any{"limit": 3}, true},
		{"Go integer value is converted", "> 5", 7, nil, true},
		{"env used in dollar expression", "$ < max", 2.0, map[string]any{"max": 10}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Lang{}.Test(tc.cell, tc.value, tc.env)
			if err != nil {
				t.Fatalf("Test(%q, %#v) error: %v", tc.cell, tc.value, err)
			}
			if got != tc.want {
				t.Fatalf("Test(%q, %#v) = %v, want %v", tc.cell, tc.value, got, tc.want)
			}
		})
	}
}

func TestTestErrors(t *testing.T) {
	cases := []struct {
		name  string
		cell  string
		value any
	}{
		{"dollar expression not bool", "$ + 1", 1.0},
		{"comparison with wrong type", "> 1", "x"},
		{"dollar expression with bad syntax", "$ >", 1.0},
		{"empty item in list", `"a",`, "b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Lang{}.Test(tc.cell, tc.value, nil)
			if err == nil {
				t.Fatalf("Test(%q, %#v) = %v, want an error", tc.cell, tc.value, got)
			}
		})
	}
}
