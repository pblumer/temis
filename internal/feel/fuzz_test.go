package feel

import (
	"testing"
	"unicode/utf8"
)

// FuzzLexer encodes the WP-03 acceptance criterion: no input makes the lexer
// panic or hang. It seeds with representative FEEL snippets and then asserts the
// invariants for arbitrary input: tokenisation terminates, ends in exactly one
// EOF, and every token carries a valid 1-based position.
func FuzzLexer(f *testing.F) {
	seeds := []string{
		"",
		"1 + 2 * 3",
		`if Applicant Age >= 18 then "yes" else "no"`,
		"for i in [1..10] return i ** 2",
		`@"2024-01-01"`,
		`some x in L satisfies x > 0`,
		`{ a: 1, b: [2, 3] }`,
		`"escapes \n \t \\ \" é"`,
		"a.b.c[1]",
		"日本語 name",
		"!@#$%^&",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		if !utf8.ValidString(src) {
			// Invalid UTF-8 still must not panic, but skip the position
			// invariants which assume rune-countable input.
			Tokenize(src)
			return
		}

		toks := Tokenize(src)
		if len(toks) == 0 {
			t.Fatal("Tokenize returned no tokens")
		}
		for i, tk := range toks {
			if tk.Line < 1 || tk.Col < 1 {
				t.Fatalf("token %d %q has invalid position %d:%d", i, tk.Text, tk.Line, tk.Col)
			}
			if (tk.Kind == EOF) != (i == len(toks)-1) {
				t.Fatalf("EOF appears at index %d of %d", i, len(toks))
			}
		}
	})
}
