package dmn_test

import (
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestHitPolicyPriorityAndOutputOrder covers WP-27 end to end: PRIORITY returns
// the highest-ranked matching output, OUTPUT ORDER returns all matches ranked.
func TestHitPolicyPriorityAndOutputOrder(t *testing.T) {
	defs := compileModel(t, "hitpolicy_15.dmn")

	for _, c := range []struct {
		score int
		want  any
	}{
		{90, "High"},
		{60, "Medium"},
		{10, "Low"},
	} {
		if got := evalDecision(t, defs, "Risk Level", dmn.Input{"Score": c.score}); got != c.want {
			t.Errorf("Risk Level(%d) = %v, want %v", c.score, got, c.want)
		}
	}

	if got := evalDecision(t, defs, "Levels", dmn.Input{"Score": 90}); !reflect.DeepEqual(got, []any{"High", "Medium", "Low"}) {
		t.Errorf("Levels(90) = %#v, want [High Medium Low]", got)
	}
	if got := evalDecision(t, defs, "Levels", dmn.Input{"Score": 60}); !reflect.DeepEqual(got, []any{"Medium", "Low"}) {
		t.Errorf("Levels(60) = %#v, want [Medium Low]", got)
	}
}
