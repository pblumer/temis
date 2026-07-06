package dmn_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// These benchmarks back the WP-42 performance budget (docs/50-testing-strategy.md
// §6): compiling a mid-size decision table, evaluating it warm, a bare FEEL
// arithmetic expression, and a 10-deep DRG chain. The budget assertions live in
// budget_test.go.

// midTableModel builds a decision table with the given number of rules over four
// numeric inputs (A..D), each rule matching a distinct range of A.
func midTableModel(rules int) []byte {
	var b strings.Builder
	b.WriteString(`<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/bench" name="Bench" id="def_b">`)
	for _, n := range []string{"A", "B", "C", "D"} {
		fmt.Fprintf(&b, `<inputData id="i_%s" name="%s"><variable name="%s" typeRef="number"/></inputData>`, n, n, n)
	}
	b.WriteString(`<decision id="d_grade" name="Grade"><variable name="Grade" typeRef="string"/>`)
	for _, n := range []string{"A", "B", "C", "D"} {
		fmt.Fprintf(&b, `<informationRequirement><requiredInput href="#i_%s"/></informationRequirement>`, n)
	}
	b.WriteString(`<decisionTable hitPolicy="UNIQUE">`)
	for _, n := range []string{"A", "B", "C", "D"} {
		fmt.Fprintf(&b, `<input id="in_%s"><inputExpression typeRef="number"><text>%s</text></inputExpression></input>`, n, n)
	}
	b.WriteString(`<output name="Grade" typeRef="string"/>`)
	for i := 0; i < rules; i++ {
		fmt.Fprintf(&b, `<rule><inputEntry><text>[%d..%d]</text></inputEntry>`+
			`<inputEntry><text>-</text></inputEntry><inputEntry><text>-</text></inputEntry><inputEntry><text>-</text></inputEntry>`+
			`<outputEntry><text>"g%d"</text></outputEntry></rule>`, i*10, i*10+9, i)
	}
	b.WriteString(`</decisionTable></decision></definitions>`)
	return []byte(b.String())
}

const arithmeticModel = `<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/arith" name="Arith" id="def_a">
  <inputData id="i_a" name="A"><variable name="A" typeRef="number"/></inputData>
  <inputData id="i_b" name="B"><variable name="B" typeRef="number"/></inputData>
  <decision id="d_r" name="R"><variable name="R" typeRef="number"/>
    <informationRequirement><requiredInput href="#i_a"/></informationRequirement>
    <informationRequirement><requiredInput href="#i_b"/></informationRequirement>
    <literalExpression><text>(A * B + 3) / 2 - 1</text></literalExpression>
  </decision>
</definitions>`

// drgChainModel builds n decisions D1..Dn where each adds 1 to the previous,
// seeded by the Seed input; evaluating Dn chains through all of them.
func drgChainModel(n int) []byte {
	var b strings.Builder
	b.WriteString(`<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/drg" name="Drg" id="def_d">`)
	b.WriteString(`<inputData id="i_seed" name="Seed"><variable name="Seed" typeRef="number"/></inputData>`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, `<decision id="d%d" name="D%d"><variable name="D%d" typeRef="number"/>`, i, i, i)
		if i == 1 {
			b.WriteString(`<informationRequirement><requiredInput href="#i_seed"/></informationRequirement>`)
			b.WriteString(`<literalExpression><text>Seed + 1</text></literalExpression>`)
		} else {
			fmt.Fprintf(&b, `<informationRequirement><requiredDecision href="#d%d"/></informationRequirement>`, i-1)
			fmt.Fprintf(&b, `<literalExpression><text>D%d + 1</text></literalExpression>`, i-1)
		}
		b.WriteString(`</decision>`)
	}
	b.WriteString(`</definitions>`)
	return []byte(b.String())
}

func mustCompileDecision(b *testing.B, xml []byte, decision string) *dmn.CompiledDecision {
	b.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil || diags.HasErrors() {
		b.Fatalf("compile: %v %+v", err, diags)
	}
	dec, err := defs.Decision(decision)
	if err != nil {
		b.Fatal(err)
	}
	return dec
}

func BenchmarkCompileMidTable(b *testing.B) {
	xml := midTableModel(10)
	eng := dmn.New()
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := eng.Compile(ctx, xml); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluateMidTable(b *testing.B) {
	dec := mustCompileDecision(b, midTableModel(10), "Grade")
	ctx := context.Background()
	in := dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

// stringTableModel builds a decision table that matches two string inputs
// (Season, Region) by equality — the most common real-world DMN table shape —
// with the given number of rules, returning a string.
func stringTableModel(rules int) []byte {
	seasons := []string{"Winter", "Spring", "Summer", "Autumn"}
	var b strings.Builder
	b.WriteString(`<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/str" name="Str" id="def_s">`)
	for _, n := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `<inputData id="i_%s" name="%s"><variable name="%s" typeRef="string"/></inputData>`, n, n, n)
	}
	b.WriteString(`<decision id="d_menu" name="Menu"><variable name="Menu" typeRef="string"/>`)
	for _, n := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `<informationRequirement><requiredInput href="#i_%s"/></informationRequirement>`, n)
	}
	b.WriteString(`<decisionTable hitPolicy="UNIQUE">`)
	for _, n := range []string{"Season", "Region"} {
		fmt.Fprintf(&b, `<input id="in_%s"><inputExpression typeRef="string"><text>%s</text></inputExpression></input>`, n, n)
	}
	b.WriteString(`<output name="Menu" typeRef="string"/>`)
	for i := 0; i < rules; i++ {
		fmt.Fprintf(&b, `<rule><inputEntry><text>"%s"</text></inputEntry>`+
			`<inputEntry><text>"R%d"</text></inputEntry>`+
			`<outputEntry><text>"m%d"</text></outputEntry></rule>`, seasons[i%len(seasons)], i, i)
	}
	b.WriteString(`</decisionTable></decision></definitions>`)
	return []byte(b.String())
}

func BenchmarkEvaluateStringTable(b *testing.B) {
	dec := mustCompileDecision(b, stringTableModel(10), "Menu")
	ctx := context.Background()
	in := dmn.Input{"Season": "Winter", "Region": "R8"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluateArithmetic(b *testing.B) {
	dec := mustCompileDecision(b, []byte(arithmeticModel), "R")
	ctx := context.Background()
	in := dmn.Input{"A": 6, "B": 7}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEvaluateDRGChain10(b *testing.B) {
	dec := mustCompileDecision(b, drgChainModel(10), "D10")
	ctx := context.Background()
	in := dmn.Input{"Seed": 0}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}
