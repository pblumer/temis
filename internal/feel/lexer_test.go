package feel

import (
	"testing"
)

// kinds tokenizes src and returns the token kinds without the trailing EOF.
func kinds(src string) []Kind {
	toks := Tokenize(src)
	ks := make([]Kind, 0, len(toks))
	for _, t := range toks {
		if t.Kind == EOF {
			break
		}
		ks = append(ks, t.Kind)
	}
	return ks
}

func TestNumbers(t *testing.T) {
	cases := map[string][]Kind{
		"0":        {Number},
		"42":       {Number},
		"3.14":     {Number},
		".5":       {Number},
		"1.2e10":   {Number},
		"6.022E23": {Number},
		"1e-9":     {Number},
		// "1." has no fractional digit: number 1, then a dot.
		"1.":      {Number, Dot},
		"1..10":   {Number, DotDot, Number},
		"[1..10]": {LBracket, Number, DotDot, Number, RBracket},
		// a bare 'e' with no exponent digits is not part of the number.
		"1e":  {Number, Name},
		"1e+": {Number, Name, Plus},
	}
	for src, want := range cases {
		if got := kinds(src); !equalKinds(got, want) {
			t.Errorf("kinds(%q) = %v, want %v", src, got, want)
		}
	}
}

func TestNumberText(t *testing.T) {
	toks := Tokenize("1.2e10")
	if toks[0].Kind != Number || toks[0].Text != "1.2e10" {
		t.Errorf("got %v %q, want Number 1.2e10", toks[0].Kind, toks[0].Text)
	}
}

func TestStrings(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"hello"`, "hello"},
		{`"a\"b"`, `a"b`},
		{`"line\nbreak"`, "line\nbreak"},
		{`"tab\tend"`, "tab\tend"},
		{`"slash\/"`, "slash/"},
		{`"unicA"`, "unicA"},
		{`"\u0041"`, "A"},                // 4-hex \u escape
		{`"\U01F40E"`, "\U0001F40E"},     // 6-hex \U escape → horse
		{`"\uD83D\uDCA9"`, "\U0001F4A9"}, // UTF-16 surrogate pair → poo
		{`""`, ""},
	}
	for _, c := range cases {
		toks := Tokenize(c.src)
		if toks[0].Kind != String {
			t.Errorf("Tokenize(%q)[0].Kind = %v, want String", c.src, toks[0].Kind)
			continue
		}
		if toks[0].Value != c.want {
			t.Errorf("Tokenize(%q) value = %q, want %q", c.src, toks[0].Value, c.want)
		}
	}
}

func TestStringErrors(t *testing.T) {
	for _, src := range []string{`"unterminated`, `"bad\xescape"`, `"\u00ZZ"`} {
		toks := Tokenize(src)
		if toks[0].Kind != Error {
			t.Errorf("Tokenize(%q)[0].Kind = %v, want Error", src, toks[0].Kind)
		}
		if toks[0].Value == "" {
			t.Errorf("Tokenize(%q): error token has no message", src)
		}
	}
}

func TestAtLiteral(t *testing.T) {
	toks := Tokenize(`@"2024-01-01"`)
	if toks[0].Kind != At || toks[0].Value != "2024-01-01" {
		t.Errorf("got %v %q, want At 2024-01-01", toks[0].Kind, toks[0].Value)
	}
	if bad := Tokenize(`@foo`); bad[0].Kind != Error {
		t.Errorf("@foo: kind = %v, want Error", bad[0].Kind)
	}
}

func TestNamesWithSpaces(t *testing.T) {
	// The lexer emits one Name per fragment; the parser assembles "Applicant Age".
	toks := Tokenize("Applicant Age")
	if len(toks) != 3 || toks[0].Kind != Name || toks[1].Kind != Name || toks[2].Kind != EOF {
		t.Fatalf("got %v, want Name Name EOF", toks)
	}
	if toks[0].Text != "Applicant" || toks[1].Text != "Age" {
		t.Errorf("fragments = %q %q, want Applicant Age", toks[0].Text, toks[1].Text)
	}
}

func TestKeywords(t *testing.T) {
	want := []Kind{If, Name, Gte, Number, Then, String, Else, String}
	got := kinds(`if x >= 18 then "a" else "b"`)
	if !equalKinds(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
}

func TestNamesContainingKeywordPrefix(t *testing.T) {
	// "format" must not be split into the keyword "for" plus "mat".
	toks := Tokenize("format")
	if toks[0].Kind != Name || toks[0].Text != "format" {
		t.Errorf("got %v %q, want Name format", toks[0].Kind, toks[0].Text)
	}
}

func TestOperators(t *testing.T) {
	want := []Kind{
		Plus, Minus, Star, Slash, Pow,
		Eq, Neq, Lt, Lte, Gt, Gte,
		LParen, RParen, LBracket, RBracket, LBrace, RBrace,
		Comma, Colon, Dot, DotDot,
	}
	got := kinds(`+ - * / ** = != < <= > >= ( ) [ ] { } , : . ..`)
	if !equalKinds(got, want) {
		t.Errorf("kinds = %v, want %v", got, want)
	}
}

func TestPositions(t *testing.T) {
	toks := Tokenize("a\n  bb")
	if toks[0].Line != 1 || toks[0].Col != 1 {
		t.Errorf("token 0 at %d:%d, want 1:1", toks[0].Line, toks[0].Col)
	}
	if toks[1].Line != 2 || toks[1].Col != 3 {
		t.Errorf("token 1 at %d:%d, want 2:3", toks[1].Line, toks[1].Col)
	}
}

func TestIllegalCharacter(t *testing.T) {
	toks := Tokenize("a ; b")
	var sawError bool
	for _, tk := range toks {
		if tk.Kind == Error {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("expected an Error token for ';', got %v", toks)
	}
}

func TestComplexExpression(t *testing.T) {
	src := `for i in [1..3] return i * 2`
	want := []Kind{For, Name, In, LBracket, Number, DotDot, Number, RBracket, Return, Name, Star, Number}
	if got := kinds(src); !equalKinds(got, want) {
		t.Errorf("kinds(%q) = %v, want %v", src, got, want)
	}
}

func TestEOFIsTerminal(t *testing.T) {
	l := New("a")
	_ = l.Next()
	if k := l.Next().Kind; k != EOF {
		t.Errorf("second Next = %v, want EOF", k)
	}
	if k := l.Next().Kind; k != EOF {
		t.Errorf("repeated Next = %v, want EOF", k)
	}
}

func TestKindString(t *testing.T) {
	if Plus.String() != "+" || Name.String() != "Name" || Kind(9999).String() != "Kind(?)" {
		t.Errorf("Kind.String mismatch: %q %q %q", Plus, Name, Kind(9999))
	}
}

func equalKinds(a, b []Kind) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
