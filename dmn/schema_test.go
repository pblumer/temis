package dmn_test

import (
	"context"
	"errors"
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
