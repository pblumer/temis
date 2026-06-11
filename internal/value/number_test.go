package value

import "testing"

func TestNumberParsingAndString(t *testing.T) {
	cases := map[string]string{
		"0":       "0",
		"42":      "42",
		"3.14":    "3.14",
		"-7":      "-7",
		"1000000": "1000000",
		"0.30":    "0.3", // trailing zeros are insignificant in FEEL
	}
	for in, want := range cases {
		n, err := ParseNumber(in)
		if err != nil {
			t.Errorf("ParseNumber(%q) error: %v", in, err)
			continue
		}
		if got := n.String(); got != want {
			t.Errorf("ParseNumber(%q).String() = %q, want %q", in, got, want)
		}
	}
}

func TestParseNumberInvalid(t *testing.T) {
	for _, in := range []string{"", "abc", "0x10", "1.2.3", "Infinity", "NaN"} {
		if _, err := ParseNumber(in); err == nil {
			t.Errorf("ParseNumber(%q) = nil error, want error", in)
		}
	}
}

// TestDecimalExactness is the headline ADR-0007 guarantee: decimal, not float.
func TestDecimalExactness(t *testing.T) {
	got := Add(MustNumber("0.1"), MustNumber("0.2"))
	if got.String() != "0.3" {
		t.Errorf("0.1 + 0.2 = %s, want 0.3", got)
	}
}

func TestNumberArithmetic(t *testing.T) {
	cases := []struct {
		op       string
		a, b     string
		want     string
		wantNull bool
	}{
		{"+", "2", "3", "5", false},
		{"-", "2", "3", "-1", false},
		{"*", "4", "2.5", "10", false},
		{"/", "10", "4", "2.5", false},
		{"/", "1", "0", "", true}, // division by zero is null
		{"**", "2", "10", "1024", false},
		{"**", "9", "0.5", "3", false},
	}
	for _, c := range cases {
		var got Value
		a, b := MustNumber(c.a), MustNumber(c.b)
		switch c.op {
		case "+":
			got = Add(a, b)
		case "-":
			got = Sub(a, b)
		case "*":
			got = Mul(a, b)
		case "/":
			got = Div(a, b)
		case "**":
			got = Exp(a, b)
		}
		if c.wantNull {
			if !IsNull(got) {
				t.Errorf("%s %s %s = %s, want null", c.a, c.op, c.b, got)
			}
			continue
		}
		if IsNull(got) || got.String() != c.want {
			t.Errorf("%s %s %s = %s, want %s", c.a, c.op, c.b, got, c.want)
		}
	}
}

func TestNumberPrecision(t *testing.T) {
	// 1/3 carries 34 significant digits.
	got := Div(MustNumber("1"), MustNumber("3")).String()
	want := "0.3333333333333333333333333333333333"
	if got != want {
		t.Errorf("1/3 = %s (len %d), want %s", got, len(got), want)
	}
}

func TestNumberNegAndCompare(t *testing.T) {
	if Neg(MustNumber("5")).String() != "-5" {
		t.Errorf("Neg(5) = %s, want -5", Neg(MustNumber("5")))
	}
	if MustNumber("2").Cmp(MustNumber("10")) != -1 {
		t.Error("2 cmp 10 should be -1")
	}
	if MustNumber("3.0").Cmp(MustNumber("3")) != 0 {
		t.Error("3.0 cmp 3 should be 0")
	}
}
