package dmn

import (
	"reflect"
	"testing"

	"github.com/pblumer/feel/value"
)

func TestToValueAndBack(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want any // expected fromValue(toValue(in))
	}{
		{"nil", nil, nil},
		{"bool", true, true},
		{"string", "hi", "hi"},
		{"int", 42, "42"},
		{"int64", int64(-7), "-7"},
		{"float exact", 0.5, "0.5"},
		{"list", []any{1, "x", true}, []any{"1", "x", true}},
		{"context", map[string]any{"a": 1}, map[string]any{"a": "1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := toValue(c.in)
			if err != nil {
				t.Fatalf("toValue: %v", err)
			}
			got := fromValue(v)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("round-trip %v = %#v, want %#v", c.in, got, c.want)
			}
		})
	}
}

func TestToValueUnsupported(t *testing.T) {
	if _, err := toValue(struct{}{}); err == nil {
		t.Error("expected error for unsupported type")
	}
}

func TestFromValueNumberIsExactString(t *testing.T) {
	// 0.1 + 0.2 must round-trip to the exact "0.3", not a float artefact.
	sum := value.Add(value.MustNumber("0.1"), value.MustNumber("0.2"))
	if got := fromValue(sum); got != "0.3" {
		t.Errorf("0.1+0.2 = %v, want \"0.3\"", got)
	}
}
