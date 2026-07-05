package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestInstanceOfGenerics covers WP-41.23: `instance of` resolves parametrized
// generics (list<T>, context<fields>, nested) and user-defined item types, and
// matches structurally; null is not an instance of any type.
func TestInstanceOfGenerics(t *testing.T) {
	// A user-defined type tRec = context<a: string, b: string>.
	tRec := ContextOf(map[string]*Type{"a": TString, "b": TString})
	env := NewEnv().WithTypes(map[string]*Type{"tRec": tRec})

	cases := []struct {
		src  string
		want value.Value
	}{
		{`{a: "x", b: "y"} instance of context<a: string, b: string>`, value.True},
		{`{a: "x"} instance of context<a: string, b: string>`, value.False}, // missing b
		{`{a: "123"} instance of context<a: number>`, value.False},          // wrong field type
		{`{a: {b: 123}} instance of context<a: context<b: number>>`, value.True},
		{`{a: {b: 123}} instance of context<a: context<b: string>>`, value.False},
		{`[1, 2, 3] instance of list<number>`, value.True},
		{`[1, "2"] instance of list<number>`, value.False},
		{`{a: "x", b: "y"} instance of tRec`, value.True},
		{`{a: "x"} instance of tRec`, value.False},
		{`null instance of Any`, value.False},
		{`5 instance of Any`, value.True},
	}
	for _, c := range cases {
		ce, err := CompileString(c.src, env)
		if err != nil {
			t.Errorf("compile %q: %v", c.src, err)
			continue
		}
		got, err := ce(env.NewScope(nil))
		if err != nil {
			t.Errorf("eval %q: %v", c.src, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.src, got.String(), c.want.String())
		}
	}
}
