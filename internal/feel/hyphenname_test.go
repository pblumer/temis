package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestHyphenatedNames covers WP-41.15: a variable whose name embeds a hyphen
// (e.g. the DMN decision output "Date-Time") assembles as one name when the
// oracle knows it, while a bare "a - b" with no such name still parses as a
// subtraction.
func TestHyphenatedNames(t *testing.T) {
	vars := map[string]value.Value{
		"Date-Time":              value.Str("2018-12-08T00:00:00"),
		"Pre-bureauRiskCategory": value.Str("HIGH"),
		"a":                      value.MustNumber("7"),
		"b":                      value.MustNumber("3"),
	}

	// A known hyphenated name resolves to its variable.
	if got := evalStr(t, "Date-Time", vars); got.String() != "2018-12-08T00:00:00" {
		t.Errorf("Date-Time = %s, want the bound value", got.String())
	}
	if got := evalStr(t, "Pre-bureauRiskCategory", vars); got.String() != "HIGH" {
		t.Errorf("Pre-bureauRiskCategory = %s, want HIGH", got.String())
	}

	// A hyphen between two known names that is NOT itself a name stays a
	// subtraction (there is no variable "a-b").
	if got := evalStr(t, "a - b", vars); got.String() != "4" {
		t.Errorf("a - b = %s, want 4", got.String())
	}
	if got := evalStr(t, "a-b", vars); got.String() != "4" {
		t.Errorf("a-b = %s, want 4 (subtraction, no such name)", got.String())
	}
}

// TestHyphenatedNameWithoutOracle confirms the plain parser (no name oracle)
// leaves "a - b" as a subtraction and never over-assembles a hyphenated name.
func TestHyphenatedNameWithoutOracle(t *testing.T) {
	expr, err := Parse("a-b")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := expr.(*BinaryExpr); !ok {
		t.Errorf("a-b without oracle parsed as %T, want *BinaryExpr", expr)
	}
}

// TestNumberWordNames covers WP-41.17: a name may end in (or embed) a number
// word — "Extra days case 1", "K-MatchesFunc-1" — assembled when the oracle
// knows it, while a bare "x 1" juxtaposition never over-assembles.
func TestNumberWordNames(t *testing.T) {
	vars := map[string]value.Value{
		"Extra days case 1": value.MustNumber("5"),
		"K-MatchesFunc-1":   value.Str("ok"),
		"x":                 value.MustNumber("2"),
	}
	if got := evalStr(t, "Extra days case 1", vars); got.String() != "5" {
		t.Errorf(`"Extra days case 1" = %s, want 5`, got.String())
	}
	if got := evalStr(t, "max(Extra days case 1, 3)", vars); got.String() != "5" {
		t.Errorf("max(Extra days case 1, 3) = %s, want 5", got.String())
	}
	if got := evalStr(t, "K-MatchesFunc-1", vars); got.String() != "ok" {
		t.Errorf(`"K-MatchesFunc-1" = %s, want ok`, got.String())
	}
	// Without a matching name, "x - 1" stays a subtraction.
	if got := evalStr(t, "x - 1", vars); got.String() != "1" {
		t.Errorf("x - 1 = %s, want 1", got.String())
	}
}

// TestUnknownCalleeYieldsNull covers WP-41.17: invoking an unknown name or a
// non-function value evaluates to null (FEEL total-function semantics) instead of
// making the whole decision non-executable (1131).
func TestUnknownCalleeYieldsNull(t *testing.T) {
	for _, src := range []string{
		"non_existing_function()",
		`"some_func"()`,
		"123()",
		"true()",
		"null()",
	} {
		ce, err := CompileString(src, NewEnv())
		if err != nil {
			t.Errorf("compile %q: unexpected error %v", src, err)
			continue
		}
		got, err := ce(NewEnv().NewScope(nil))
		if err != nil {
			t.Errorf("eval %q: %v", src, err)
			continue
		}
		if !value.IsNull(got) {
			t.Errorf("%s = %s, want null", src, got.String())
		}
	}
}
