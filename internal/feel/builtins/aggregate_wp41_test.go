package builtins

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestModeNullAndSubstringFraction covers WP-41.24 at the builtin level: mode(null)
// is null, and substring's position/length may be non-integer (floored) (TCK
// 0062/1103).
func TestModeNullAndSubstringFraction(t *testing.T) {
	run(t, []tc{
		{name: "mode", args: []value.Value{value.Null}, wantNull: true},
		{name: "substring", args: []value.Value{str("foobar"), num("3"), num("3.8")}, want: "oba"},
		{name: "substring", args: []value.Value{str("foobar"), num("2.9")}, want: "oobar"},
	})
}
