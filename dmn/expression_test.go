package dmn

import (
	"context"
	"reflect"
	"testing"
)

func TestCompileExpression(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name  string
		expr  string
		names []string
		in    Input
		want  any
	}{
		{"arithmetic on inputs", "Credit Score - 50", []string{"Credit Score"}, Input{"Credit Score": 750}, "700"},
		{"conditional", `if Age >= 18 then "adult" else "minor"`, []string{"Age"}, Input{"Age": 16}, "minor"},
		{"builtin", "upper case(Region)", []string{"Region"}, Input{"Region": "eu"}, "EU"},
		{"path access on a context", "risk.Level", []string{"risk"}, Input{"risk": map[string]any{"Level": "high"}}, "high"},
		{"absent name is null", "x", []string{"x"}, Input{}, nil},
		// A runtime type mismatch is a spec-conformant null, not an error.
		{"type mismatch is null", `x + 1`, []string{"x"}, Input{"x": "abc"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ce, err := CompileExpression(tc.expr, tc.names...)
			if err != nil {
				t.Fatalf("compile %q: %v", tc.expr, err)
			}
			got, err := ce.Evaluate(ctx, tc.in)
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("%q = %v (%T), want %v", tc.expr, got, got, tc.want)
			}
		})
	}
}

func TestCompileExpressionReferences(t *testing.T) {
	ce, err := CompileExpression("Credit Score - 50 + Bonus", "Credit Score", "Bonus", "Unused")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got := ce.References()
	want := []string{"Bonus", "Credit Score"} // sorted, only referenced names
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("References() = %v, want %v", got, want)
	}
}

func TestCompileExpressionUnknownVariable(t *testing.T) {
	if _, err := CompileExpression("Missing + 1", "Known"); err == nil {
		t.Fatal("expected a compile error for an undeclared variable")
	}
}
