package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestQuantifierNonBooleanIsNull covers a some/every whose satisfies-result is a
// genuine non-boolean: the whole quantifier is null, even when another element
// satisfied it (TCK 1153). false/null keep the ordinary three-valued behaviour.
func TestQuantifierNonBooleanIsNull(t *testing.T) {
	if got := evalStr(t, `some element in [1,2] satisfies (if element = 2 then true else "x")`, nil); !value.IsNull(got) {
		t.Errorf(`some … non-boolean = %s, want null`, got)
	}
	if got := evalStr(t, `every element in [1,2] satisfies (if element = 2 then true else "x")`, nil); !value.IsNull(got) {
		t.Errorf(`every … non-boolean = %s, want null`, got)
	}
	// Ordinary boolean quantifiers are unaffected.
	if got := evalStr(t, `some x in [1,2,3] satisfies x > 2`, nil); got.String() != "true" {
		t.Errorf(`some x > 2 = %s, want true`, got)
	}
	if got := evalStr(t, `every x in [1,2,3] satisfies x > 5`, nil); got.String() != "false" {
		t.Errorf(`every x > 5 = %s, want false`, got)
	}
}

// TestListReplaceNamedMatch covers the named match-function form of list replace
// (TCK 1155); the positional and named position forms keep working.
func TestListReplaceNamedMatch(t *testing.T) {
	if got := evalStr(t, `list replace(match: function(item, newItem) item = 2, newItem: 4, list: [1,2,3])`, nil); got.String() != "[1, 4, 3]" {
		t.Errorf(`list replace(match:…) = %s, want [1, 4, 3]`, got)
	}
	if got := evalStr(t, `list replace([1,2,3], 2, 9)`, nil); got.String() != "[1, 9, 3]" {
		t.Errorf(`list replace(pos) = %s, want [1, 9, 3]`, got)
	}
}

// TestStringJoinArity covers string join being restricted to its DMN 1- and
// 2-argument forms: a third argument is invalid and yields null (TCK 1140).
func TestStringJoinArity(t *testing.T) {
	if got := evalStr(t, `string join(["a","c"], "X")`, nil); got.String() != "aXc" {
		t.Errorf(`string join(list, delim) = %s, want aXc`, got)
	}
	if got := evalStr(t, `string join(["a","c"], "X", "foo")`, nil); !value.IsNull(got) {
		t.Errorf(`string join with 3 args = %s, want null`, got)
	}
}

// TestDuplicateContextKeysNull covers a context literal with duplicate keys
// evaluating to null (TCK 0057).
func TestDuplicateContextKeysNull(t *testing.T) {
	if got := evalStr(t, `{foo: "bar", foo: "baz"}`, nil); !value.IsNull(got) {
		t.Errorf(`{foo:…, foo:…} = %s, want null`, got)
	}
	// Distinct keys are unaffected.
	if got := evalStr(t, `{a: 1, b: 2}`, nil); got.String() != "{a: 1, b: 2}" {
		t.Errorf(`{a:1, b:2} = %s, want {a: 1, b: 2}`, got)
	}
}
