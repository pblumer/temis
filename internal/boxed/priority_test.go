package boxed

import (
	"testing"

	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// mkTableOut builds a single-input table whose one output carries a priority
// list (outputValues), for exercising the Priority and Output Order policies.
func mkTableOut(hp model.HitPolicy, outValues string, rules ...r) *model.DecisionTable {
	dt := mkTable(hp, model.AggNone, []string{"x"}, []string{"o"}, rules...)
	dt.Outputs[0].AllowedValues = outValues
	return dt
}

func TestPriority(t *testing.T) {
	// Two rules match for x=5; priority list ranks "high" above "low".
	dt := mkTableOut(model.HitPriority, `"high","low"`,
		r{[]string{"> 0"}, []string{`"low"`}},
		r{[]string{"> 0"}, []string{`"high"`}},
	)
	if got := evalT(t, dt, x("5")); got.String() != "high" {
		t.Errorf("PRIORITY = %s, want high (outranks low despite table order)", got)
	}
}

func TestPriorityNoMatchIsNull(t *testing.T) {
	dt := mkTableOut(model.HitPriority, `"high","low"`,
		r{[]string{"> 100"}, []string{`"high"`}},
	)
	if got := evalT(t, dt, x("5")); !value.IsNull(got) {
		t.Errorf("PRIORITY no match = %s, want null", got)
	}
}

func TestOutputOrder(t *testing.T) {
	// All three rules match; Output Order returns every output ranked by priority.
	dt := mkTableOut(model.HitOutputOrder, `"high","medium","low"`,
		r{[]string{"> 0"}, []string{`"low"`}},
		r{[]string{"> 0"}, []string{`"high"`}},
		r{[]string{"> 0"}, []string{`"medium"`}},
	)
	if got := evalT(t, dt, x("5")); got.String() != `[high, medium, low]` {
		t.Errorf("OUTPUT ORDER = %s, want [high, medium, low]", got)
	}
}

func TestOutputOrderNoMatchIsEmptyList(t *testing.T) {
	dt := mkTableOut(model.HitOutputOrder, `"high","low"`,
		r{[]string{"> 100"}, []string{`"high"`}},
	)
	got := evalT(t, dt, x("5"))
	if l, ok := got.(value.List); !ok || len(l.Elements) != 0 {
		t.Errorf("OUTPUT ORDER no match = %s, want empty list", got)
	}
}

func TestPriorityValueNotInListRanksLast(t *testing.T) {
	// "other" is absent from the priority list, so the ranked value wins.
	dt := mkTableOut(model.HitPriority, `"win"`,
		r{[]string{"> 0"}, []string{`"other"`}},
		r{[]string{"> 0"}, []string{`"win"`}},
	)
	if got := evalT(t, dt, x("5")); got.String() != "win" {
		t.Errorf("PRIORITY = %s, want win (listed value outranks unlisted)", got)
	}
}

func TestPriorityMultiOutput(t *testing.T) {
	// Priority compares outputs in column order: first output dominates.
	dt := mkTable(model.HitPriority, model.AggNone, []string{"x"}, []string{"a", "b"},
		r{[]string{"> 0"}, []string{`"lo"`, `"x"`}},
		r{[]string{"> 0"}, []string{`"hi"`, `"y"`}},
	)
	dt.Outputs[0].AllowedValues = `"hi","lo"`
	dt.Outputs[1].AllowedValues = `"x","y"`
	got := evalT(t, dt, x("5"))
	ctx, ok := got.(*value.Context)
	if !ok {
		t.Fatalf("multi-output priority = %s, want a context", got)
	}
	if a, _ := ctx.Get("a"); a.String() != "hi" {
		t.Errorf("priority a = %s, want hi", a)
	}
}

func TestParsePriorityListError(t *testing.T) {
	dt := mkTableOut(model.HitPriority, "1 +", r{[]string{"-"}, []string{"1"}})
	if _, err := CompileTable(dt, envForVars(map[string]value.Value{"x": value.MustNumber("0")}), nil); err == nil {
		t.Error("malformed output values should be a compile error")
	}
}
