package dmn_test

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func TestInputSchema(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	got := map[string]dmn.InputField{}
	for _, f := range dec.InputSchema() {
		got[f.Name] = f
	}
	if len(got) != 2 {
		t.Fatalf("want 2 input fields, got %d (%+v)", len(got), dec.InputSchema())
	}
	if f := got["Season"]; f.Type != "string" || !f.Required {
		t.Errorf("Season = %+v, want type string, required", f)
	}
	if f := got["Guest Count"]; f.Type != "number" || !f.Required {
		t.Errorf("Guest Count = %+v, want type number, required", f)
	}
}

func TestInputSchemaSuggestsTableCellValues(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	got := map[string]dmn.InputField{}
	for _, f := range dec.InputSchema() {
		got[f.Name] = f
	}

	// Season's cells are the string literals "Fall"/"Winter"/"Spring" (+ a "-"
	// catch-all) — inferred as suggestions, with free entry still allowed.
	season := got["Season"]
	want := map[string]bool{"Fall": true, "Winter": true, "Spring": true}
	if len(season.Values) != 3 {
		t.Fatalf("Season values = %v, want the three seasons", season.Values)
	}
	for _, v := range season.Values {
		if !want[v] {
			t.Errorf("unexpected Season value %q", v)
		}
	}
	if season.ValuesClosed {
		t.Error("Season values are inferred suggestions, should not be a closed set")
	}
	// Guest Count's cells are ranges/comparisons (<= 8, [5..8], > 8) — no discrete
	// values to suggest.
	if gc := got["Guest Count"]; len(gc.Values) != 0 {
		t.Errorf("Guest Count should have no suggested values, got %v", gc.Values)
	}
}

func TestValidateInputByErrorClass(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")

	tests := []struct {
		name     string
		input    dmn.Input
		wantCode string // "" → expect no problems
		wantOn   string // the input the problem is about
	}{
		{"valid", dmn.Input{"Season": "Winter", "Guest Count": 8}, "", ""},
		{"type mismatch number↞string", dmn.Input{"Season": "Winter", "Guest Count": "8"}, "TYPE_MISMATCH", "Guest Count"},
		{"type mismatch string↞number", dmn.Input{"Season": 7, "Guest Count": 8}, "TYPE_MISMATCH", "Season"},
		{"unknown input", dmn.Input{"Season": "Winter", "Guest Count": 8, "Seasn": "x"}, "UNKNOWN_INPUT", "Seasn"},
		{"missing required", dmn.Input{"Season": "Winter"}, "MISSING_INPUT", "Guest Count"},
		{"null is not a type clash", dmn.Input{"Season": "Winter", "Guest Count": nil}, "MISSING_INPUT", "Guest Count"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			probs := dec.ValidateInput(tc.input)
			if tc.wantCode == "" {
				if len(probs) != 0 {
					t.Fatalf("want no problems, got %+v", probs)
				}
				return
			}
			found := false
			for _, p := range probs {
				if p.Code == tc.wantCode && p.Input == tc.wantOn {
					found = true
				}
			}
			if !found {
				t.Errorf("want a %s on %q, got %+v", tc.wantCode, tc.wantOn, probs)
			}
		})
	}
}

func TestStrictInputFailsEvaluate(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")

	// Lenient (default): a string for a number input silently yields no match
	// (the wrong-but-quiet result WP-52 guards against).
	res, err := dec.Evaluate(context.Background(), dmn.Input{"Season": "Winter", "Guest Count": "8"})
	if err != nil {
		t.Fatalf("lenient evaluate errored: %v", err)
	}
	if res.Outputs["Dish"] != nil {
		t.Errorf("lenient: expected null Dish for mistyped input, got %v", res.Outputs["Dish"])
	}

	// Strict: the same input is rejected with a structured InputError.
	_, err = dec.Evaluate(context.Background(),
		dmn.Input{"Season": "Winter", "Guest Count": "8"}, dmn.WithStrictInput())
	var ie *dmn.InputError
	if !errors.As(err, &ie) {
		t.Fatalf("strict: want *InputError, got %v", err)
	}
	if len(ie.Problems) != 1 || ie.Problems[0].Code != "TYPE_MISMATCH" ||
		ie.Problems[0].Expected != "number" || ie.Problems[0].Got != "string" {
		t.Errorf("strict problems = %+v, want one number↞string TYPE_MISMATCH", ie.Problems)
	}
}

func TestStrictInputPassesValid(t *testing.T) {
	dec := compileDecision(t, "dish_15.dmn", "Dish")
	res, err := dec.Evaluate(context.Background(),
		dmn.Input{"Season": "Winter", "Guest Count": 8}, dmn.WithStrictInput())
	if err != nil {
		t.Fatalf("strict evaluate on valid input errored: %v", err)
	}
	if res.Outputs["Dish"] != "Roastbeef" {
		t.Errorf("Dish = %v, want Roastbeef", res.Outputs["Dish"])
	}
}

// sortedInputNames returns the field names in sorted order, for order-independent
// set comparison in the reachable-schema tests.
func sortedInputNames(fs []dmn.InputField) []string {
	names := make([]string, len(fs))
	for i, f := range fs {
		names[i] = f.Name
	}
	sort.Strings(names)
	return names
}

// TestReachableInputSchema covers the cone-scoped union: direct + transitive
// leaf inputs, dedup of a shared input, exclusion of inputs outside the cone
// (unlike the model-wide schema), and the unknown-decision error.
func TestReachableInputSchema(t *testing.T) {
	defs := compileModel(t, "cone_15.dmn")

	// Top's cone is {Top, Sub1, Sub2}: direct A plus transitive A,B (Sub1) and
	// B,C (Sub2). Deduped to A,B,C — and NOT D, which only Isolated needs.
	top, err := defs.ReachableInputSchema("Top")
	if err != nil {
		t.Fatalf("ReachableInputSchema(Top): %v", err)
	}
	if got, want := sortedInputNames(top), []string{"A", "B", "C"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Top reachable inputs = %v, want %v", got, want)
	}
	for _, f := range top {
		if !f.Required {
			t.Errorf("reachable input %q should be required", f.Name)
		}
		if f.Type != "number" {
			t.Errorf("reachable input %q type = %q, want number", f.Name, f.Type)
		}
	}

	// A leaf decision's cone is just its own direct inputs.
	sub1, err := defs.ReachableInputSchema("Sub1")
	if err != nil {
		t.Fatalf("ReachableInputSchema(Sub1): %v", err)
	}
	if got, want := sortedInputNames(sub1), []string{"A", "B"}; !reflect.DeepEqual(got, want) {
		t.Errorf("Sub1 reachable inputs = %v, want %v", got, want)
	}

	// The model-wide schema is strictly wider: it includes D (Isolated), which
	// Top's cone deliberately excludes.
	if got, want := sortedInputNames(defs.ModelInputSchema()), []string{"A", "B", "C", "D"}; !reflect.DeepEqual(got, want) {
		t.Errorf("model input schema = %v, want %v", got, want)
	}

	if _, err := defs.ReachableInputSchema("Nope"); err == nil {
		t.Errorf("ReachableInputSchema on unknown decision: want error, got nil")
	}
}

// TestReachableInputSchemaDeterministic proves the cone union is emitted in a
// stable order (model declaration order, then first-seen input order), so
// re-audit is byte-identical (ADR-0007/0023).
func TestReachableInputSchemaDeterministic(t *testing.T) {
	defs := compileModel(t, "cone_15.dmn")
	first, err := defs.ReachableInputSchema("Top")
	if err != nil {
		t.Fatalf("ReachableInputSchema: %v", err)
	}
	for i := 0; i < 5; i++ {
		got, err := defs.ReachableInputSchema("Top")
		if err != nil {
			t.Fatalf("ReachableInputSchema: %v", err)
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("non-deterministic reachable schema: %+v vs %+v", got, first)
		}
	}
}

// TestValidateReachableInput covers the cone-scoped validation: a complete cone
// input is accepted, an input outside the cone is UNKNOWN_INPUT, a missing
// required leaf is MISSING_INPUT, and an unknown decision errs.
func TestValidateReachableInput(t *testing.T) {
	defs := compileModel(t, "cone_15.dmn")

	probs, err := defs.ValidateReachableInput("Top", dmn.Input{"A": 1, "B": 2, "C": 3})
	if err != nil {
		t.Fatalf("ValidateReachableInput: %v", err)
	}
	if len(probs) != 0 {
		t.Errorf("complete cone input: want no problems, got %+v", probs)
	}

	// D is outside Top's cone (UNKNOWN_INPUT); C is required but absent
	// (MISSING_INPUT).
	probs, err = defs.ValidateReachableInput("Top", dmn.Input{"A": 1, "B": 2, "D": 9})
	if err != nil {
		t.Fatalf("ValidateReachableInput: %v", err)
	}
	codes := map[string]string{}
	for _, p := range probs {
		codes[p.Input] = p.Code
	}
	if codes["D"] != "UNKNOWN_INPUT" {
		t.Errorf("D: want UNKNOWN_INPUT, got %q (all: %+v)", codes["D"], probs)
	}
	if codes["C"] != "MISSING_INPUT" {
		t.Errorf("C: want MISSING_INPUT, got %q (all: %+v)", codes["C"], probs)
	}

	if _, err := defs.ValidateReachableInput("Nope", dmn.Input{}); err == nil {
		t.Errorf("ValidateReachableInput on unknown decision: want error, got nil")
	}
}
