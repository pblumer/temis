package dmn_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestBoxedFilterView exposes a filter's collection and predicate, and reports
// absent for a non-filter decision.
func TestBoxedFilterView(t *testing.T) {
	defs := compileModel(t, "boxed_collections_15.dmn")
	v, ok := defs.BoxedFilter("BigNumbers")
	if !ok {
		t.Fatal("BigNumbers should be a boxed filter")
	}
	if !v.Simple {
		t.Error("BigNumbers filter should be simple (literal branches)")
	}
	if v.In != "[1, 2, 3, 4]" || v.Match != "item > 2" {
		t.Errorf("filter view = %+v, want [1, 2, 3, 4] / item > 2", v)
	}

	if _, ok := defs.BoxedFilter("Numbers"); ok {
		t.Error("Numbers is a list; BoxedFilter should report absent")
	}
}

// TestSetBoxedFilter edits the branches and checks the recompiled model evaluates
// with the new logic, and the view round-trips.
func TestSetBoxedFilter(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	out, err := dmn.SetBoxedFilter(src, "id_bignums", dmn.FilterEdit{In: "[1, 2, 3, 4, 5]", Match: "item > 3"})
	if err != nil {
		t.Fatalf("SetBoxedFilter: %v", err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), out)
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: err=%v diags=%+v", err, diags)
	}
	v, _ := defs.BoxedFilter("id_bignums")
	if v.In != "[1, 2, 3, 4, 5]" || v.Match != "item > 3" {
		t.Errorf("branches not updated: %+v", v)
	}
	// item > 3 over [1..5] → [4, 5].
	dec, _ := defs.Decision("BigNumbers")
	res, err := dec.Evaluate(context.Background(), dmn.Input{})
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := fmt.Sprint(res.Outputs["BigNumbers"]); got != "[4 5]" {
		t.Errorf("BigNumbers = %v, want [4 5]", got)
	}
}

// TestSetBoxedFilterRefuses rejects an empty branch and a decision that already
// carries non-filter logic.
func TestSetBoxedFilterRefuses(t *testing.T) {
	src := readModel(t, "boxed_collections_15.dmn")
	if _, err := dmn.SetBoxedFilter(src, "id_bignums", dmn.FilterEdit{In: "[1]", Match: "  "}); err == nil {
		t.Error("SetBoxedFilter should reject an empty predicate")
	}
	if _, err := dmn.SetBoxedFilter(readModel(t, "dish_15.dmn"), "Dish", dmn.FilterEdit{In: "[1]", Match: "item > 0"}); err == nil {
		t.Error("SetBoxedFilter should refuse a decision-table decision")
	}
}

// TestCreateBoxedFilter gives an undecided decision a fresh filter and refuses one
// that already has logic.
func TestCreateBoxedFilter(t *testing.T) {
	if _, err := dmn.CreateBoxedFilter(readModel(t, "dish_15.dmn"), "Dish"); err == nil {
		t.Error("CreateBoxedFilter should refuse a decision that already has logic")
	}
	const undecided = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="d" name="d" namespace="urn:x">
  <decision id="id_d" name="D"><variable name="D"/></decision>
</definitions>`
	out, err := dmn.CreateBoxedFilter([]byte(undecided), "id_d")
	if err != nil {
		t.Fatalf("CreateBoxedFilter: %v", err)
	}
	v, ok := mustCompile(t, string(out)).BoxedFilter("id_d")
	if !ok || v.In == "" || v.Match == "" {
		t.Errorf("fresh filter not created: ok=%v view=%+v", ok, v)
	}
}

// TestBoxedFilterNestedReadOnly reports a filter with a nested non-literal branch
// as not simple, so the editor opens read-only.
func TestBoxedFilterNestedReadOnly(t *testing.T) {
	// Adults' `in` branch is a nested list of contexts (non-literal).
	v, ok := compileModel(t, "boxed_collections_15.dmn").BoxedFilter("Adults")
	if !ok {
		t.Fatal("Adults should be a boxed filter")
	}
	if v.Simple {
		t.Error("a filter with a nested list collection should not be simple")
	}
}
