package feel

import (
	"strings"
	"testing"
)

// FuzzParser encodes the WP-04 invariant from docs/50-testing-strategy.md §3 and
// docs/30-feel-spec.md §10: no input makes the parser panic. Any input either
// parses to an AST or returns a *ParseError. On success the AST must render
// without panicking (exercising every String method reachable from the input).
func FuzzParser(f *testing.F) {
	seeds := []string{
		"1 + 2 * 3",
		`if x > 0 then "pos" else "neg"`,
		"for i in [1..10] return i ** 2",
		"some x in xs satisfies x > 0",
		"every n in ns satisfies n >= 0",
		"f(a: 1, b: 2)",
		"a.b.c[1]",
		"{ a: 1, b: [2, 3] }",
		"x between 1 and 10",
		"x in (1, 2, 3)",
		"x instance of number",
		"function(a, b) a + b",
		"]1..10[",
		`date and time("2024-01-01")`,
		"Applicant Age + Guest Count",
		// Deep-nesting seeds pin the K1 guard: the parser must reject these with
		// a *ParseError, never a fatal stack overflow (ADR-0008).
		strings.Repeat("-", DefaultMaxParseDepth+50) + "1",
		strings.Repeat("(", DefaultMaxParseDepth+50) + "1",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		e, err := Parse(src)
		if err != nil {
			if _, ok := err.(*ParseError); !ok {
				t.Fatalf("Parse(%q) returned non-ParseError %T: %v", src, err, err)
			}
			if e != nil {
				t.Fatalf("Parse(%q) returned both an AST and an error", src)
			}
			return
		}
		if e == nil {
			t.Fatalf("Parse(%q) returned nil AST and nil error", src)
		}
		_ = e.String() // must not panic
	})
}
