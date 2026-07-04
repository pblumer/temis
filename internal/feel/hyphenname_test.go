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
