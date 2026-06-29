package dmn_test

import (
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedListAndRelation covers WP-25.
func TestBoxedListAndRelation(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")

	if got := evalDecision(t, defs, "Numbers", nil); !reflect.DeepEqual(got, []any{"1", "2", "3"}) {
		t.Errorf("Numbers = %#v, want [1 2 3]", got)
	}

	got := evalDecision(t, defs, "People", nil)
	want := []any{
		map[string]any{"name": "Ann", "age": "30"},
		map[string]any{"name": "Bob", "age": "15"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("People = %#v, want %#v", got, want)
	}
}

// TestBoxedConditionalIteratorFilter covers WP-26.
func TestBoxedConditionalIteratorFilter(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")

	if got := evalDecision(t, defs, "Grade", dmn.Input{"Threshold": 10}); got != "high" {
		t.Errorf("Grade(10) = %v, want high", got)
	}
	if got := evalDecision(t, defs, "Grade", dmn.Input{"Threshold": 3}); got != "low" {
		t.Errorf("Grade(3) = %v, want low", got)
	}

	if got := evalDecision(t, defs, "Doubled", nil); !reflect.DeepEqual(got, []any{"2", "4", "6"}) {
		t.Errorf("Doubled = %#v, want [2 4 6]", got)
	}

	if got := evalDecision(t, defs, "AllPositive", nil); got != true {
		t.Errorf("AllPositive = %v, want true", got)
	}

	if got := evalDecision(t, defs, "AnyBig", dmn.Input{"Threshold": 6}); got != true {
		t.Errorf("AnyBig(6) = %v, want true (9 > 6)", got)
	}
	if got := evalDecision(t, defs, "AnyBig", dmn.Input{"Threshold": 100}); got != false {
		t.Errorf("AnyBig(100) = %v, want false", got)
	}

	if got := evalDecision(t, defs, "BigNumbers", nil); !reflect.DeepEqual(got, []any{"3", "4"}) {
		t.Errorf("BigNumbers = %#v, want [3 4]", got)
	}

	got := evalDecision(t, defs, "Adults", nil)
	want := []any{map[string]any{"name": "Ann", "age": "30"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Adults = %#v, want %#v", got, want)
	}
}
