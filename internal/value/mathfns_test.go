package value

import "testing"

func TestNumberSqrtLnExp(t *testing.T) {
	if r, ok := MustNumber("16").Sqrt(); !ok || r.String() != "4" {
		t.Errorf("sqrt(16) = %v ok=%v, want 4", r, ok)
	}
	if r, ok := MustNumber("2").Sqrt(); !ok || r.Cmp(MustNumber("1.4142135623730950488016887242096981")) != 0 {
		t.Errorf("sqrt(2) = %v ok=%v", r, ok)
	}
	if _, ok := MustNumber("-1").Sqrt(); ok {
		t.Error("sqrt(-1) should be invalid (null)")
	}
	if r, ok := MustNumber("1").Ln(); !ok || !r.IsZero() {
		t.Errorf("ln(1) = %v ok=%v, want 0", r, ok)
	}
	if _, ok := MustNumber("0").Ln(); ok {
		t.Error("ln(0) should be invalid (null)")
	}
	if _, ok := MustNumber("-3").Ln(); ok {
		t.Error("ln(-3) should be invalid (null)")
	}
	if r, ok := MustNumber("0").Exp(); !ok || r.String() != "1" {
		t.Errorf("exp(0) = %v ok=%v, want 1", r, ok)
	}
}

func TestNumberModulo(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"12", "5", "2"},
		{"-12", "5", "3"},  // sign follows divisor
		{"12", "-5", "-3"}, // sign follows divisor
		{"-12", "-5", "-2"},
		{"10", "2", "0"},
		{"5.5", "2", "1.5"},
	}
	for _, c := range cases {
		r, ok := MustNumber(c.a).Modulo(MustNumber(c.b))
		if !ok || r.String() != c.want {
			t.Errorf("modulo(%s,%s) = %v ok=%v, want %s", c.a, c.b, r, ok, c.want)
		}
	}
	if _, ok := MustNumber("1").Modulo(MustNumber("0")); ok {
		t.Error("modulo by zero should be invalid (null)")
	}
}

func TestNumberRounding(t *testing.T) {
	if r, ok := MustNumber("3.14159").RoundHalfEven(2); !ok || r.String() != "3.14" {
		t.Errorf("decimal(3.14159,2) = %v ok=%v, want 3.14", r, ok)
	}
	if r, ok := MustNumber("2.5").RoundHalfEven(0); !ok || r.String() != "2" { // half-even
		t.Errorf("decimal(2.5,0) = %v, want 2 (half-even)", r)
	}
	if r, ok := MustNumber("1.5").RoundHalfEven(0); !ok || r.String() != "2" {
		t.Errorf("decimal(1.5,0) = %v, want 2 (half-even)", r)
	}
	if r, ok := MustNumber("5.5").RoundHalfUp(0); !ok || r.String() != "6" {
		t.Errorf("round half up(5.5,0) = %v, want 6", r)
	}
	if r, ok := MustNumber("5.5").RoundHalfDown(0); !ok || r.String() != "5" {
		t.Errorf("round half down(5.5,0) = %v, want 5", r)
	}
	if r, ok := MustNumber("1.121").RoundUp(2); !ok || r.String() != "1.13" {
		t.Errorf("round up(1.121,2) = %v, want 1.13", r)
	}
	if r, ok := MustNumber("1.126").RoundDown(2); !ok || r.String() != "1.12" {
		t.Errorf("round down(1.126,2) = %v, want 1.12", r)
	}
	if r, ok := MustNumber("12345.6789").RoundHalfEven(-2); !ok || r.String() != "12300" {
		t.Errorf("decimal(12345.6789,-2) = %v, want 12300", r)
	}
}

func TestNumberParity(t *testing.T) {
	if e, ok := MustNumber("2").Even(); !ok || !e {
		t.Errorf("even(2) = %v ok=%v, want true", e, ok)
	}
	if e, ok := MustNumber("5").Even(); !ok || e {
		t.Errorf("even(5) = %v ok=%v, want false", e, ok)
	}
	if o, ok := MustNumber("5").Odd(); !ok || !o {
		t.Errorf("odd(5) = %v ok=%v, want true", o, ok)
	}
	if _, ok := MustNumber("2.5").Even(); ok {
		t.Error("even(2.5) should be invalid (null)")
	}
	if !MustNumber("4").IsInteger() || MustNumber("4.5").IsInteger() {
		t.Error("IsInteger mismatch")
	}
}
