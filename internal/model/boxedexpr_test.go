package model_test

import (
	"testing"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// decisionWith builds a single-decision definition whose logic is x.
func decisionWith(x dmnxml.Expression) *model.Definitions {
	def := defWithNS(ns15)
	def.Decisions = []dmnxml.Decision{{ID: "d1", Name: "D", Expression: x}}
	m, _, _ := model.FromXML(def)
	return m
}

func TestMapBoxedContext(t *testing.T) {
	m := decisionWith(dmnxml.Expression{Context: &dmnxml.Context{
		ID: "ctx1",
		Entries: []dmnxml.ContextEntry{
			{Variable: &dmnxml.Variable{Name: "a"}, Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "1"}}},
			{Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "a + 1"}}},
		},
	}})
	dec := m.Decisions[0]
	ctx := dec.Context
	if ctx == nil {
		t.Fatal("decision context not mapped")
	}
	if dec.Logic() != ctx {
		t.Error("Logic() should return the context expression")
	}
	if len(ctx.Entries) != 2 || ctx.Entries[0].Name != "a" || ctx.Entries[1].Name != "" {
		t.Errorf("context entries mapped wrong: %+v", ctx.Entries)
	}
	if _, ok := ctx.Entries[0].Value.(*model.LiteralExpression); !ok {
		t.Errorf("entry value type %T, want *LiteralExpression", ctx.Entries[0].Value)
	}
}

func TestMapInvocation(t *testing.T) {
	m := decisionWith(dmnxml.Expression{Invocation: &dmnxml.Invocation{
		ID:         "inv1",
		Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "Rate"}},
		Bindings: []dmnxml.Binding{
			{Parameter: &dmnxml.Parameter{Name: "total"}, Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "Order Total"}}},
		},
	}})
	inv := m.Decisions[0].Invocation
	if inv == nil {
		t.Fatal("invocation not mapped")
	}
	if c, ok := inv.Called.(*model.LiteralExpression); !ok || c.Text != "Rate" {
		t.Errorf("called = %+v, want literal Rate", inv.Called)
	}
	if len(inv.Bindings) != 1 || inv.Bindings[0].Parameter != "total" {
		t.Errorf("bindings mapped wrong: %+v", inv.Bindings)
	}
}

func TestMapFunctionDefinitionDecision(t *testing.T) {
	m := decisionWith(dmnxml.Expression{FunctionDefinition: &dmnxml.FunctionDefinition{
		Kind:       "FEEL",
		Parameters: []dmnxml.FormalParameter{{Name: "x", TypeRef: "number"}},
		Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "x + 1"}},
	}})
	fn := m.Decisions[0].FunctionDef
	if fn == nil {
		t.Fatal("function definition not mapped")
	}
	if fn.Kind != "FEEL" || len(fn.Parameters) != 1 || fn.Parameters[0].Name != "x" || fn.Parameters[0].TypeRef != "number" {
		t.Errorf("function definition mapped wrong: %+v", fn)
	}
}

func TestMapBKMEncapsulatedLogic(t *testing.T) {
	def := defWithNS(ns15)
	def.BKMs = []dmnxml.BKM{{
		ID:       "bkm1",
		Name:     "Rate",
		Variable: &dmnxml.Variable{TypeRef: "number"},
		EncapsulatedLogic: &dmnxml.FunctionDefinition{
			Parameters: []dmnxml.FormalParameter{{Name: "total"}},
			Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: "total * 0.1"}},
		},
		KnowledgeRequirts: []dmnxml.KnowledgeRequirt{{RequiredKnowledge: &dmnxml.Ref{Href: "#other"}}},
	}}
	m, _, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	b := m.BKMs[0]
	if b.VariableTypeRef != "number" {
		t.Errorf("BKM variable type = %q, want number", b.VariableTypeRef)
	}
	if b.EncapsulatedLogic == nil || len(b.EncapsulatedLogic.Parameters) != 1 {
		t.Fatalf("encapsulated logic mapped wrong: %+v", b.EncapsulatedLogic)
	}
	if len(b.RequiredKnowledge) != 1 || b.RequiredKnowledge[0] != "other" {
		t.Errorf("BKM RequiredKnowledge = %v, want [other]", b.RequiredKnowledge)
	}
}

func TestLogicNilWhenNoExpression(t *testing.T) {
	dec := &model.Decision{ID: "d"}
	if dec.Logic() != nil {
		t.Error("Logic() of a decision without logic should be nil")
	}
}
