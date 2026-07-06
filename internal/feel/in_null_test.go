package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestInNullSemantics covers WP-41.16: the `in` operator is a 3-valued
// disjunction. A null tested value against a range, or an explicit null range
// endpoint, makes the membership test null (not false); an omitted, unbounded
// endpoint is unaffected.
func TestInNullSemantics(t *testing.T) {
	cases := []struct {
		src  string
		want value.Value
	}{
		{"null in [1..10]", value.Null},
		{"5 in [null..10]", value.Null},
		{"5 in (null..10]", value.Null},
		{"5 in [1..null]", value.Null},
		{"5 in [1..null)", value.Null},
		// A bounded, non-null range still decides normally.
		{"5 in [1..10]", value.True},
		{"50 in [1..10]", value.False},
		// An unbounded (omitted) endpoint is not null: 5 < 10 holds.
		{"5 in (< 10)", value.True},
	}
	for _, c := range cases {
		ce, err := CompileString(c.src, NewEnv())
		if err != nil {
			t.Errorf("compile %q: %v", c.src, err)
			continue
		}
		got, err := ce(NewEnv().NewScope(nil))
		if err != nil {
			t.Errorf("eval %q: %v", c.src, err)
			continue
		}
		if value.IsNull(c.want) {
			if !value.IsNull(got) {
				t.Errorf("%s = %s, want null", c.src, got.String())
			}
			continue
		}
		if got != c.want {
			t.Errorf("%s = %s, want %s", c.src, got.String(), c.want.String())
		}
	}
}
