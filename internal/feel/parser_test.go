package feel

import "testing"

// sexpr parses src and returns the AST's S-expression form, failing on error.
func sexpr(t *testing.T, src string) string {
	t.Helper()
	e, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", src, err)
	}
	return e.String()
}

func TestLiterals(t *testing.T) {
	cases := map[string]string{
		"42":              "42",
		"3.14":            "3.14",
		`"hi"`:            `"hi"`,
		"true":            "true",
		"false":           "false",
		"null":            "null",
		`@"2024-01-01"`:   `@"2024-01-01"`,
		"Applicant Age":   "Applicant Age",
		"x":               "x",
		"[]":              "(list)",
		"[1, 2, 3]":       "(list 1 2 3)",
		"{a: 1, b: 2}":    "(context (a: 1) (b: 2))",
		`{"k": 1}`:        "(context (k: 1))",
		"{first name: 1}": "(context (first name: 1))",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestPrecedence(t *testing.T) {
	cases := map[string]string{
		"1 + 2 * 3":     "(+ 1 (* 2 3))",
		"1 * 2 + 3":     "(+ (* 1 2) 3)",
		"(1 + 2) * 3":   "(* (+ 1 2) 3)",
		"2 ** 3 ** 2":   "(** (** 2 3) 2)", // left-associative (TCK 0100)
		"-2 ** 2":       "(** (- 2) 2)",    // unary minus binds tighter than ** (TCK 0100)
		"-3 * 4":        "(* (- 3) 4)",
		"a or b and c":  "(or a (and b c))",
		"a and b or c":  "(or (and a b) c)",
		"1 < 2 = true":  "(= (< 1 2) true)",
		"a + b < c * d": "(< (+ a b) (* c d))",
		"2 ** -3":       "(** 2 (- 3))",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestComparisonsAndTests(t *testing.T) {
	cases := map[string]string{
		"x between 1 and 10":   "(between x 1 10)",
		"x in (1, 2, 3)":       "(in x 1 2 3)",
		"x in [1..10]":         "(in x [1..10])",
		"x instance of number": "(instance-of x number)",
		"a != b":               "(!= a b)",
		"a <= b":               "(<= a b)",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestIntervals(t *testing.T) {
	cases := map[string]string{
		"[1..10]":                "[1..10]",
		"[1..10)":                "[1..10)",
		"(1..10]":                "(1..10]",
		"(1..10)":                "(1..10)",
		"]1..10[":                "(1..10)",
		"]1..10]":                "(1..10]",
		`[date("a")..date("b")]`: `[(call date "a")..(call date "b")]`,
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestControlFlow(t *testing.T) {
	cases := map[string]string{
		`if x > 0 then "pos" else "neg"`:      `(if (> x 0) "pos" "neg")`,
		"for i in [1..3] return i * 2":        "(for ((i in [1..3])) (* i 2))",
		"for i in a, j in b return i + j":     "(for ((i in a) (j in b)) (+ i j))",
		"some x in xs satisfies x > 0":        "(some ((x in xs)) (> x 0))",
		"every n in ns satisfies n >= 0":      "(every ((n in ns)) (>= n 0))",
		`if a then if b then 1 else 2 else 3`: "(if a (if b 1 2) 3)",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestPostfix(t *testing.T) {
	cases := map[string]string{
		"a.b.c":         "(. (. a b) c)",
		"L[1]":          "(filter L 1)",
		"L[item > 2]":   "(filter L (> item 2))",
		"f(1, 2)":       "(call f 1 2)",
		"f()":           "(call f)",
		"f(a: 1, b: 2)": "(call f (a: 1) (b: 2))",
		"not(x)":        "(call not x)",
		"a.b[1].c":      "(. (filter (. a b) 1) c)",
		// Without the oracle 'and' stays an operator: "date and time" is a
		// boolean and of the names date and time, not a single multi-word name.
		"date and time": "(and date time)",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestFunctionDefinition(t *testing.T) {
	cases := map[string]string{
		"function(a, b) a + b":        "(function (a b) (+ a b))",
		"function(a: number) a":       "(function (a:number) a)",
		`function(x) external "java"`: `(function-ext (x) "java")`,
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

type nameSet map[string]bool

func (n nameSet) Has(s string) bool { return n[s] }

func TestNameOracleLongestMatch(t *testing.T) {
	names := nameSet{"date and time": true, "Applicant Age": true}
	cases := map[string]string{
		`date and time("x")`: `(call date and time "x")`, // keyword 'and' absorbed into the name
		"Applicant Age + 1":  "(+ Applicant Age 1)",
	}
	for src, want := range cases {
		e, err := ParseWithNames(src, names)
		if err != nil {
			t.Fatalf("ParseWithNames(%q) error: %v", src, err)
		}
		if got := e.String(); got != want {
			t.Errorf("ParseWithNames(%q) = %s, want %s", src, got, want)
		}
	}

	// 'a and b' must remain a boolean and even with an oracle, since "a and b"
	// is not a known name.
	e, _ := ParseWithNames("a and b", names)
	if got := e.String(); got != "(and a b)" {
		t.Errorf(`ParseWithNames("a and b") = %s, want (and a b)`, got)
	}
}

func TestErrorsReportPosition(t *testing.T) {
	cases := []struct {
		src       string
		line, col int
	}{
		{"1 +", 1, 4},          // EOF after operator
		{"if x then 1", 1, 12}, // missing else
		{"(1 + 2", 1, 7},       // missing close paren
		{"1 2", 1, 3},          // trailing token
		{"@bad", 1, 1},         // lexer error surfaced
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		if err == nil {
			t.Errorf("Parse(%q) = nil error, want error", c.src)
			continue
		}
		pe, ok := err.(*ParseError)
		if !ok {
			t.Errorf("Parse(%q) error type %T, want *ParseError", c.src, err)
			continue
		}
		if pe.Line != c.line || pe.Col != c.col {
			t.Errorf("Parse(%q) error at %d:%d, want %d:%d (%s)", c.src, pe.Line, pe.Col, c.line, c.col, pe.Msg)
		}
	}
}

func TestGenericTypes(t *testing.T) {
	cases := map[string]string{
		"x instance of list<number>":       "(instance-of x list<number>)",
		"x instance of context<a: number>": "(instance-of x context<a:number>)",
		"function(xs: list<string>) xs":    "(function (xs:list<string>) xs)",
	}
	for src, want := range cases {
		if got := sexpr(t, src); got != want {
			t.Errorf("Parse(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestParseErrorMessage(t *testing.T) {
	_, err := Parse("1 +")
	pe, ok := err.(*ParseError)
	if !ok {
		t.Fatalf("error type %T, want *ParseError", err)
	}
	if got := pe.Error(); got != "1:4: "+pe.Msg {
		t.Errorf("Error() = %q, want %q", got, "1:4: "+pe.Msg)
	}
}

func TestPositionsOnNodes(t *testing.T) {
	e, err := Parse("  1 + 2")
	if err != nil {
		t.Fatal(err)
	}
	// The BinaryExpr is positioned at the operator.
	if p := e.Pos(); p.Line != 1 || p.Col != 5 {
		t.Errorf("operator position = %d:%d, want 1:5", p.Line, p.Col)
	}
}
