package model

import "testing"

// TestLogicAllBranches exercises every branch of Decision.Logic, including the
// nil/default case, ensuring each boxed-expression field is returned when set.
func TestLogicAllBranches(t *testing.T) {
	lit := &LiteralExpression{}
	dt := &DecisionTable{}
	ctx := &ContextExpr{}
	inv := &Invocation{}
	fn := &FunctionDef{}
	list := &ListExpr{}
	rel := &RelationExpr{}
	cond := &Conditional{}
	forExpr := &ForExpr{}
	quant := &Quantified{}
	filter := &FilterExpr{}

	cases := []struct {
		name string
		dec  Decision
		want Expression
	}{
		{"literal", Decision{LiteralExpression: lit}, lit},
		{"table", Decision{DecisionTable: dt}, dt},
		{"context", Decision{Context: ctx}, ctx},
		{"invocation", Decision{Invocation: inv}, inv},
		{"function", Decision{FunctionDef: fn}, fn},
		{"list", Decision{List: list}, list},
		{"relation", Decision{Relation: rel}, rel},
		{"conditional", Decision{Conditional: cond}, cond},
		{"for", Decision{For: forExpr}, forExpr},
		{"quantified", Decision{Quantified: quant}, quant},
		{"filter", Decision{Filter: filter}, filter},
		{"none", Decision{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := tc.dec
			if got := d.Logic(); got != tc.want {
				t.Errorf("Logic() = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestIsExpressionMarkers calls the unexported marker method on every concrete
// Expression implementation so each is exercised. The method is a no-op; the
// test just verifies the types satisfy Expression and the methods run.
func TestIsExpressionMarkers(t *testing.T) {
	exprs := []Expression{
		&LiteralExpression{},
		&DecisionTable{},
		&ContextExpr{},
		&Invocation{},
		&FunctionDef{},
		&ListExpr{},
		&RelationExpr{},
		&Conditional{},
		&ForExpr{},
		&Quantified{},
		&FilterExpr{},
	}
	for _, e := range exprs {
		e.isExpression()
	}
	if len(exprs) != 11 {
		t.Fatalf("expected 11 expression types, got %d", len(exprs))
	}
}

// TestLocalRefEmpty covers the empty-href branch of localRef.
func TestLocalRefEmpty(t *testing.T) {
	if got := localRef("   "); got != "" {
		t.Errorf("localRef(blank) = %q, want empty", got)
	}
	if got := localRef("model.dmn#dec_1"); got != "dec_1" {
		t.Errorf("localRef = %q, want dec_1", got)
	}
	if got := localRef("plain"); got != "plain" {
		t.Errorf("localRef(plain) = %q, want plain", got)
	}
}

// TestVariableNameNil covers the nil branch of variableName (and variableTypeRef).
func TestVariableNameNil(t *testing.T) {
	if got := variableName(nil); got != "" {
		t.Errorf("variableName(nil) = %q, want empty", got)
	}
	if got := variableTypeRef(nil); got != "" {
		t.Errorf("variableTypeRef(nil) = %q, want empty", got)
	}
}
