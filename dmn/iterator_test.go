package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedIteratorView exposes a for/every/some iteration and reports absent for
// a non-iteration decision.
func TestBoxedIteratorView(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")

	f, ok := defs.BoxedIterator("Doubled")
	if !ok || f.Kind != "for" || f.Variable != "x" || f.In != "[1, 2, 3]" || f.Body != "x * 2" {
		t.Errorf("Doubled for-view = %+v ok=%v", f, ok)
	}
	e, ok := defs.BoxedIterator("AllPositive")
	if !ok || e.Kind != "every" || e.Body != "x > 0" {
		t.Errorf("AllPositive every-view = %+v ok=%v", e, ok)
	}
	s, ok := defs.BoxedIterator("AnyBig")
	if !ok || s.Kind != "some" || s.Body != "x > Threshold" {
		t.Errorf("AnyBig some-view = %+v ok=%v", s, ok)
	}
	if !f.Simple {
		t.Error("Doubled should be simple")
	}

	if _, ok := defs.BoxedIterator("Numbers"); ok {
		t.Error("Numbers is a list; BoxedIterator should report absent")
	}
}

// TestSetBoxedIterator edits an iteration (including switching its kind) and checks
// the recompiled model evaluates with the new logic.
func TestSetBoxedIterator(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	// Change Doubled from `for x in [1,2,3] return x*2` to `for x in [1,2,3,4] return x*10`.
	out, err := dmn.SetBoxedIterator(src, "id_doubled", dmn.IteratorEdit{Kind: "for", Variable: "x", In: "[1, 2, 3, 4]", Body: "x * 10"})
	if err != nil {
		t.Fatalf("SetBoxedIterator: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	v, _ := defs.BoxedIterator("id_doubled")
	if v.In != "[1, 2, 3, 4]" || v.Body != "x * 10" {
		t.Errorf("not updated: %+v", v)
	}
	dec, _ := defs.Decision("Doubled")
	res, _ := dec.Evaluate(context.Background(), dmn.Input{})
	if got := fmt.Sprint(res.Outputs["Doubled"]); got != "[10 20 30 40]" {
		t.Errorf("Doubled = %v, want [10 20 30 40]", got)
	}

	// Switch AllPositive from `every` to `some` and verify the boolean flips logic.
	out2, err := dmn.SetBoxedIterator(readModel(t, "boxed_collections_15.dmn"), "id_allpos", dmn.IteratorEdit{Kind: "some", Variable: "x", In: "[-1, -2, 3]", Body: "x > 0"})
	if err != nil {
		t.Fatalf("SetBoxedIterator(some): %v", err)
	}
	defs2, diags2, err := dmn.New().Compile(context.Background(), out2)
	if err != nil || diags2.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags2)
	}
	if v, _ := defs2.BoxedIterator("id_allpos"); v.Kind != "some" {
		t.Errorf("kind not switched to some: %+v", v)
	}
	dec2, _ := defs2.Decision("AllPositive")
	res2, _ := dec2.Evaluate(context.Background(), dmn.Input{})
	if got := fmt.Sprint(res2.Outputs["AllPositive"]); got != "true" {
		t.Errorf("AllPositive (some x>0 over [-1,-2,3]) = %v, want true", got)
	}
}

// TestSetBoxedIteratorRefuses rejects a bad kind, empty branches and a decision
// that already carries non-iteration logic.
func TestSetBoxedIteratorRefuses(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	if _, err := dmn.SetBoxedIterator(src, "id_doubled", dmn.IteratorEdit{Kind: "loop", Variable: "x", In: "[1]", Body: "x"}); err == nil {
		t.Error("SetBoxedIterator should reject an unknown kind")
	}
	if _, err := dmn.SetBoxedIterator(src, "id_doubled", dmn.IteratorEdit{Kind: "for", Variable: "", In: "[1]", Body: "x"}); err == nil {
		t.Error("SetBoxedIterator should reject an empty iterator variable")
	}
	if _, err := dmn.SetBoxedIterator(readModel(t, "dish_15.dmn"), "Dish", dmn.IteratorEdit{Kind: "for", Variable: "x", In: "[1]", Body: "x"}); err == nil {
		t.Error("SetBoxedIterator should refuse a decision-table decision")
	}
}

// TestCreateBoxedIterator gives an undecided decision a fresh iteration and refuses
// one that already has logic.
func TestCreateBoxedIterator(t *testing.T) {
	if _, err := dmn.CreateBoxedIterator(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedIterator should refuse a decision that already has logic")
	}
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedIterator([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedIterator: %v", err)
	}
	v, ok := mustCompile(t, string(out)).BoxedIterator("id_d")
	if !ok || v.Kind != "for" || v.Variable == "" {
		t.Errorf("fresh iteration not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedIteratorNestedReadOnly reports an iteration with a nested non-literal
// branch as not simple, so the editor opens read-only.
func TestBoxedIteratorNestedReadOnly(t *testing.T) {
	const nested = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D">
    <variable name="D"/>
    <for iteratorVariable="x">
      <in><list><literalExpression><text>1</text></literalExpression></list></in>
      <return><literalExpression><text>x</text></literalExpression></return>
    </for>
  </decision>
</definitions>`
	v, ok := mustCompile(t, nested).BoxedIterator("D")
	if !ok {
		t.Fatal("D should be a boxed iteration")
	}
	if v.Simple {
		t.Error("an iteration with a nested list collection should not be simple")
	}
}
