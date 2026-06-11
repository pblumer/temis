package value

import (
	"testing"
	"time"
)

func TestEqual(t *testing.T) {
	cases := []struct {
		a, b Value
		want Value
	}{
		{MustNumber("1"), MustNumber("1.0"), True},
		{MustNumber("1"), MustNumber("2"), False},
		{Str("a"), Str("a"), True},
		{Str("a"), Str("b"), False},
		{True, True, True},
		{True, False, False},
		{Null, Null, True},
		{Null, MustNumber("1"), False},
		{MustNumber("1"), Null, False},
		{MustNumber("1"), Str("1"), False}, // different kinds
		{NewList(MustNumber("1"), MustNumber("2")), NewList(MustNumber("1"), MustNumber("2")), True},
		{NewList(MustNumber("1")), NewList(MustNumber("2")), False},
	}
	for _, c := range cases {
		if got := Equal(c.a, c.b); got != c.want {
			t.Errorf("Equal(%s, %s) = %s, want %s", c.a, c.b, got, c.want)
		}
	}
}

func TestContextEqualityOrderIndependent(t *testing.T) {
	a := NewContext().Put("x", MustNumber("1")).Put("y", MustNumber("2"))
	b := NewContext().Put("y", MustNumber("2")).Put("x", MustNumber("1"))
	if Equal(a, b) != True {
		t.Error("contexts with same entries in different order should be equal")
	}
	c := NewContext().Put("x", MustNumber("1"))
	if Equal(a, c) != False {
		t.Error("contexts with different entries should be unequal")
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b   Value
		want   int
		wantOK bool
	}{
		{MustNumber("1"), MustNumber("2"), -1, true},
		{MustNumber("2"), MustNumber("2"), 0, true},
		{Str("a"), Str("b"), -1, true},
		{NewDate(2024, time.January, 1), NewDate(2024, time.February, 1), -1, true},
		{Null, MustNumber("1"), 0, false},     // null is not comparable
		{MustNumber("1"), Str("1"), 0, false}, // different kinds
		{True, False, 0, false},               // booleans are unordered
	}
	for _, c := range cases {
		got, ok := Compare(c.a, c.b)
		if ok != c.wantOK || (ok && got != c.want) {
			t.Errorf("Compare(%s, %s) = (%d, %v), want (%d, %v)", c.a, c.b, got, ok, c.want, c.wantOK)
		}
	}
}

func TestCrossDurationNotComparable(t *testing.T) {
	ym, _ := ParseDuration("P1Y")
	dt, _ := ParseDuration("P365D")
	if _, ok := Compare(ym, dt); ok {
		t.Error("years-months and days-time durations are not comparable")
	}
}

func TestArithmeticNullPropagation(t *testing.T) {
	for _, op := range []func(a, b Value) Value{Add, Sub, Mul, Div, Exp} {
		if !IsNull(op(Null, MustNumber("1"))) {
			t.Error("op(null, 1) should be null")
		}
		if !IsNull(op(MustNumber("1"), Null)) {
			t.Error("op(1, null) should be null")
		}
	}
}
