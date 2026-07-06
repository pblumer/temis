package dmn

import (
	"context"
	"testing"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/value"
)

func TestCoerceToType(t *testing.T) {
	num := value.MustNumber
	list := func(vs ...value.Value) value.Value { return value.NewList(vs...) }
	ctx := func() *value.Context { return value.NewContext() }

	cases := []struct {
		name string
		v    value.Value
		t    *feel.Type
		want value.Value
	}{
		{"any keeps value", num("2"), nil, num("2")},
		{"conforming scalar kept", value.Str("foo"), feel.TString, value.Str("foo")},
		{"singleton list unwrapped", list(value.Str("foo")), feel.TString, value.Str("foo")},
		{"singleton number unwrapped", list(num("10")), feel.TNumber, num("10")},
		{"singleton of wrong type ⇒ null", list(num("1")), feel.TString, value.Null},
		{"scalar of wrong type ⇒ null", num("2"), feel.TString, value.Null},
		{"multi-element list ⇒ null", list(num("1"), value.Str("x")), feel.TString, value.Null},
		{"context to scalar ⇒ null", ctx().Put("name", value.Str("foo")), feel.TString, value.Null},
		{"list target keeps list", list(num("1")), feel.ListOf(feel.TNumber), list(num("1"))},
		{"null stays null", value.Null, feel.TString, value.Null},
	}
	for _, c := range cases {
		got := coerceToType(c.v, c.t)
		if got.String() != c.want.String() || got.Kind() != c.want.Kind() {
			t.Errorf("%s: coerceToType(%s, %s) = %s, want %s", c.name, c.v, c.t, got, c.want)
		}
	}
}

// TestOutputCoercionEndToEnd drives a typed decision whose literal returns a
// singleton list, and asserts the engine coerces it to the declared scalar type.
func TestOutputCoercionEndToEnd(t *testing.T) {
	const src = `<?xml version="1.0"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/" id="d" name="d" namespace="n">
  <decision name="pick" id="pick">
    <variable name="pick" typeRef="string"/>
    <literalExpression><text>["foo"]</text></literalExpression>
  </decision>
</definitions>`
	defs, diags, err := New().Compile(context.Background(), []byte(src))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %v", diags)
	}
	cd, err := defs.Decision("pick")
	if err != nil {
		t.Fatal(err)
	}
	res, err := cd.Evaluate(context.Background(), Input{})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Outputs["pick"]; got != "foo" {
		t.Errorf(`["foo"] coerced to string = %v (%T), want "foo"`, got, got)
	}
}
