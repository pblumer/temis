package boxed

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

type r struct{ in, out []string }

func mkTable(hp model.HitPolicy, agg model.Aggregation, inExprs, outNames []string, rules ...r) *model.DecisionTable {
	dt := &model.DecisionTable{HitPolicy: hp, Aggregation: agg}
	for _, e := range inExprs {
		dt.Inputs = append(dt.Inputs, &model.InputClause{Expression: e})
	}
	for _, n := range outNames {
		dt.Outputs = append(dt.Outputs, &model.OutputClause{Name: n})
	}
	for _, rule := range rules {
		dt.Rules = append(dt.Rules, &model.Rule{InputEntries: rule.in, OutputEntries: rule.out})
	}
	return dt
}

func envForVars(vars map[string]value.Value) *feel.Env {
	names := make([]string, 0, len(vars))
	for k := range vars {
		names = append(names, k)
	}
	sort.Strings(names)
	return feel.NewEnv(names...)
}

func evalT(t *testing.T, dt *model.DecisionTable, vars map[string]value.Value) value.Value {
	t.Helper()
	env := envForVars(vars)
	ce, err := CompileTable(dt, env, nil)
	if err != nil {
		t.Fatalf("compile table: %v", err)
	}
	v, err := ce(env.NewScope(vars))
	if err != nil {
		t.Fatalf("evaluate table: %v", err)
	}
	return v
}

func evalErr(t *testing.T, dt *model.DecisionTable, vars map[string]value.Value) error {
	t.Helper()
	env := envForVars(vars)
	ce, err := CompileTable(dt, env, nil)
	if err != nil {
		t.Fatalf("compile table: %v", err)
	}
	_, err = ce(env.NewScope(vars))
	return err
}

func x(n string) map[string]value.Value { return map[string]value.Value{"x": value.MustNumber(n)} }

func TestUnique(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"low"`}},
		r{[]string{"[10..20]"}, []string{`"mid"`}},
		r{[]string{"> 20"}, []string{`"high"`}},
	)
	for in, want := range map[string]string{"5": "low", "15": "mid", "25": "high"} {
		if got := evalT(t, dt, x(in)); got.String() != want {
			t.Errorf("x=%s -> %s, want %s", in, got, want)
		}
	}
	// no match -> null
	dt2 := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 0"}, []string{`"neg"`}})
	if got := evalT(t, dt2, x("5")); !value.IsNull(got) {
		t.Errorf("no match -> %s, want null", got)
	}
}

func TestUniqueMultipleMatchIsError(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"a"`}},
		r{[]string{"< 20"}, []string{`"b"`}},
	)
	if err := evalErr(t, dt, x("5")); err == nil {
		t.Error("UNIQUE with two matches should error")
	}
}

func TestFirst(t *testing.T) {
	dt := mkTable(model.HitFirst, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"a"`}},
		r{[]string{"< 20"}, []string{`"b"`}},
	)
	if got := evalT(t, dt, x("5")); got.String() != "a" {
		t.Errorf("FIRST -> %s, want a", got)
	}
}

func TestAny(t *testing.T) {
	same := mkTable(model.HitAny, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"same"`}},
		r{[]string{"< 20"}, []string{`"same"`}},
	)
	if got := evalT(t, same, x("5")); got.String() != "same" {
		t.Errorf("ANY (equal outputs) -> %s, want same", got)
	}
	diff := mkTable(model.HitAny, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"a"`}},
		r{[]string{"< 20"}, []string{`"b"`}},
	)
	if err := evalErr(t, diff, x("5")); err == nil {
		t.Error("ANY with divergent outputs should error")
	}
}

func TestRuleOrder(t *testing.T) {
	dt := mkTable(model.HitRuleOrder, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"a"`}},
		r{[]string{"< 20"}, []string{`"b"`}},
	)
	if got := evalT(t, dt, x("5")); got.String() != "[a, b]" {
		t.Errorf("RULE ORDER -> %s, want [a, b]", got)
	}
	if got := evalT(t, dt, x("15")); got.String() != "[b]" {
		t.Errorf("RULE ORDER -> %s, want [b]", got)
	}
}

func TestCollectList(t *testing.T) {
	dt := mkTable(model.HitCollect, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{"1"}},
		r{[]string{"< 20"}, []string{"2"}},
	)
	if got := evalT(t, dt, x("5")); got.String() != "[1, 2]" {
		t.Errorf("COLLECT -> %s, want [1, 2]", got)
	}
	// no match -> empty list
	if got := evalT(t, dt, x("100")); got.String() != "[]" {
		t.Errorf("COLLECT no match -> %s, want []", got)
	}
}

func TestCollectAggregations(t *testing.T) {
	rules := []r{
		{[]string{"< 10"}, []string{"10"}},
		{[]string{"< 20"}, []string{"5"}},
	}
	cases := map[model.Aggregation]string{
		model.AggSum:   "15",
		model.AggMin:   "5",
		model.AggMax:   "10",
		model.AggCount: "2",
	}
	for agg, want := range cases {
		dt := mkTable(model.HitCollect, agg, []string{"x"}, []string{"out"}, rules...)
		if got := evalT(t, dt, x("5")); got.String() != want {
			t.Errorf("COLLECT %s -> %s, want %s", agg, got, want)
		}
	}
	// COUNT of no matches is 0; SUM of no matches is null.
	sumDT := mkTable(model.HitCollect, model.AggSum, []string{"x"}, []string{"out"}, rules...)
	if got := evalT(t, sumDT, x("100")); !value.IsNull(got) {
		t.Errorf("COLLECT SUM no match -> %s, want null", got)
	}
	countDT := mkTable(model.HitCollect, model.AggCount, []string{"x"}, []string{"out"}, rules...)
	if got := evalT(t, countDT, x("100")); got.String() != "0" {
		t.Errorf("COLLECT COUNT no match -> %s, want 0", got)
	}
}

func TestMultipleOutputsAndDash(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"a", "b"},
		r{[]string{"-"}, []string{"1", `"two"`}}, // dash always matches
	)
	got := evalT(t, dt, x("999"))
	ctx, ok := got.(*value.Context)
	if !ok {
		t.Fatalf("multi-output result = %T, want context", got)
	}
	if ctx.String() != `{a: 1, b: two}` {
		t.Errorf("context = %s", ctx.String())
	}
}

func TestCompileErrors(t *testing.T) {
	// unsupported hit policy
	if _, err := CompileTable(mkTable(model.HitPriority, model.AggNone, []string{"x"}, []string{"o"}), feel.NewEnv("x"), nil); err == nil {
		t.Error("PRIORITY should be unsupported")
	}
	// aggregation with multiple outputs
	bad := mkTable(model.HitCollect, model.AggSum, []string{"x"}, []string{"a", "b"},
		r{[]string{"-"}, []string{"1", "2"}})
	if _, err := CompileTable(bad, feel.NewEnv("x"), nil); err == nil {
		t.Error("aggregation with two outputs should error")
	}
	// wrong number of input entries
	mism := mkTable(model.HitUnique, model.AggNone, []string{"x", "y"}, []string{"o"},
		r{[]string{"-"}, []string{"1"}}) // only one input entry for two inputs
	if _, err := CompileTable(mism, feel.NewEnv("x", "y"), nil); err == nil {
		t.Error("mismatched input entries should error")
	}
}

func TestEmptyOutputCellIsNull(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"-"}, []string{""}}) // empty output cell
	if got := evalT(t, dt, x("1")); !value.IsNull(got) {
		t.Errorf("empty output cell -> %s, want null", got)
	}
}

func TestMoreCompileErrors(t *testing.T) {
	// malformed input expression
	if _, err := CompileTable(mkTable(model.HitUnique, model.AggNone, []string{"1 +"}, []string{"o"},
		r{[]string{"-"}, []string{"1"}}), feel.NewEnv(), nil); err == nil {
		t.Error("malformed input expression should error")
	}
	// malformed output expression
	if _, err := CompileTable(mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"o"},
		r{[]string{"-"}, []string{"1 +"}}), feel.NewEnv("x"), nil); err == nil {
		t.Error("malformed output expression should error")
	}
	// wrong number of output entries
	if _, err := CompileTable(mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"o"},
		r{[]string{"-"}, []string{"1", "2"}}), feel.NewEnv("x"), nil); err == nil {
		t.Error("mismatched output entries should error")
	}
}

func TestAggregationIncomparableIsNull(t *testing.T) {
	// MIN over a string and a number cannot compare -> null.
	dt := mkTable(model.HitCollect, model.AggMin, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"a"`}},
		r{[]string{"< 20"}, []string{"1"}},
	)
	if got := evalT(t, dt, x("5")); !value.IsNull(got) {
		t.Errorf("MIN of incomparable values -> %s, want null", got)
	}
}

// TestDishFixtureEndToEnd loads the WP-02 dish_15.dmn fixture, compiles its
// decision table and evaluates it — the first full XML→model→compile→evaluate path.
func TestDishFixtureEndToEnd(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "xml", "testdata", "models", "dish_15.dmn"))
	if err != nil {
		t.Fatal(err)
	}
	def, err := dmnxml.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	m, _, err := model.FromXML(def)
	if err != nil {
		t.Fatal(err)
	}
	var dish *model.DecisionTable
	for _, d := range m.Decisions {
		if d.Name == "Dish" {
			dish = d.DecisionTable
		}
	}
	if dish == nil {
		t.Fatal("Dish decision table not found")
	}

	env := feel.NewEnv("Season", "Guest Count")
	ce, err := CompileTable(dish, env, nil)
	if err != nil {
		t.Fatalf("compile dish table: %v", err)
	}

	cases := []struct {
		season string
		guests string
		want   string
	}{
		{"Fall", "4", "Spareribs"},
		{"Winter", "4", "Roastbeef"},
		{"Spring", "6", "Steak"},
		{"Summer", "10", "Stew"}, // matches only the "-, > 8" rule
	}
	for _, c := range cases {
		scope := env.NewScope(map[string]value.Value{
			"Season":      value.Str(c.season),
			"Guest Count": value.MustNumber(c.guests),
		})
		got, err := ce(scope)
		if err != nil {
			t.Fatalf("evaluate dish for %s/%s: %v", c.season, c.guests, err)
		}
		if got.String() != c.want {
			t.Errorf("dish(%s, %s) = %s, want %s", c.season, c.guests, got, c.want)
		}
	}
}
