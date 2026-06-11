package value

import "testing"

func TestSingletonsAndString(t *testing.T) {
	if Null.String() != "null" || Null.Kind() != KindNull {
		t.Errorf("Null = %s/%s", Null, Null.Kind())
	}
	if True.String() != "true" || False.String() != "false" {
		t.Errorf("booleans = %s/%s", True, False)
	}
	if BoolOf(true) != True || BoolOf(false) != False {
		t.Error("BoolOf mismatch")
	}
	if Str("hi").String() != "hi" || Str("hi").Kind() != KindString {
		t.Errorf("Str = %s/%s", Str("hi"), Str("hi").Kind())
	}
}

func TestIsNull(t *testing.T) {
	if !IsNull(Null) || !IsNull(nil) {
		t.Error("Null and nil should be null")
	}
	if IsNull(True) {
		t.Error("True should not be null")
	}
}

func TestListString(t *testing.T) {
	l := NewList(MustNumber("1"), Str("a"), True)
	if l.String() != "[1, a, true]" || l.Kind() != KindList {
		t.Errorf("List.String() = %q", l.String())
	}
}

func TestContextOrderedAccess(t *testing.T) {
	c := NewContext().Put("b", MustNumber("2")).Put("a", MustNumber("1"))
	if got := c.Keys(); len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Errorf("Keys = %v, want [b a] (insertion order)", got)
	}
	if v, ok := c.Get("a"); !ok || v.String() != "1" {
		t.Errorf("Get(a) = %s/%v", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Error("Get(missing) should be absent")
	}
	// Put on an existing key updates in place without reordering.
	c.Put("b", MustNumber("9"))
	if c.Len() != 2 || c.Keys()[0] != "b" {
		t.Errorf("update changed order/len: %v", c.Keys())
	}
	if c.String() != "{b: 9, a: 1}" {
		t.Errorf("Context.String() = %q", c.String())
	}
}

func TestRangeString(t *testing.T) {
	r := Range{LowClosed: true, Low: MustNumber("1"), High: MustNumber("10"), HighClosed: false}
	if r.String() != "[1..10)" || r.Kind() != KindRange {
		t.Errorf("Range.String() = %q", r.String())
	}
}

func TestKindString(t *testing.T) {
	if KindDateTime.String() != "date and time" || KindYearsMonthsDuration.String() != "years and months duration" {
		t.Errorf("Kind.String mismatch")
	}
}
