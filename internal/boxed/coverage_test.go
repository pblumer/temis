package boxed

import (
	"errors"
	"testing"

	"github.com/pblumer/feel"
	"github.com/pblumer/feel/value"
	"github.com/pblumer/temis/internal/model"
)

// tightLimits trips the iteration limit on the first iteration step, so any cell
// that iterates (a FEEL `for`/`sum` over a multi-element list) errors at
// evaluation. This is the white-box hook for exercising the evaluator's runtime
// error paths, which FEEL never reaches via bad data alone (FEEL errors yield
// null, not Go errors).
func tightLimits() feel.Limits {
	return feel.Limits{MaxCallDepth: 256, MaxIterations: 1, MaxListSize: 1_000_000}
}

// evalLim compiles dt and evaluates it under tightLimits, returning the
// evaluation error (if any). It mirrors evalErr but uses a limited scope so
// iterating cells fail at runtime.
func evalLim(t *testing.T, dt *model.DecisionTable, vars map[string]value.Value) error {
	t.Helper()
	env := envForVars(vars)
	ce, err := CompileTable(dt, env, nil)
	if err != nil {
		t.Fatalf("compile table: %v", err)
	}
	_, err = ce(env.NewScopeWithLimits(vars, tightLimits()))
	return err
}

// iterCell is an output cell that iterates over several elements; under
// tightLimits it trips the iteration limit and returns a runtime error.
const iterCell = `sum(for i in [1, 2, 3] return i)`

func TestMultipleMatchErrorMessage(t *testing.T) {
	e := &MultipleMatchError{Matched: 3}
	if got := e.Error(); got != "UNIQUE hit policy: 3 rules matched" {
		t.Errorf("Error() = %q", got)
	}
}

// TestInputColumnRuntimeError makes an input column iterate; under tightLimits
// the column evaluation itself errors before any rule is tested.
func TestInputColumnRuntimeError(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{iterCell}, []string{"out"},
		r{[]string{"-"}, []string{"1"}})
	if err := evalLim(t, dt, x("5")); err == nil {
		t.Error("iterating input column under a tight limit should error")
	}
}

// TestOutputCellRuntimeErrorPerPolicy routes a runtime output-cell error through
// every hit policy's output-evaluation path (ruleOutput / ruleCells /
// collectList / aggregate / prioritized).
func TestOutputCellRuntimeErrorPerPolicy(t *testing.T) {
	cases := []struct {
		name string
		hp   model.HitPolicy
		agg  model.Aggregation
		prio bool // attach a priority list (P/O policies need one to score)
	}{
		{"unique", model.HitUnique, model.AggNone, false},
		{"first", model.HitFirst, model.AggNone, false},
		{"any", model.HitAny, model.AggNone, false},
		{"ruleorder", model.HitRuleOrder, model.AggNone, false},
		{"collect", model.HitCollect, model.AggNone, false},
		{"collect-sum", model.HitCollect, model.AggSum, false},
		{"priority", model.HitPriority, model.AggNone, true},
		{"outputorder", model.HitOutputOrder, model.AggNone, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dt := mkTable(c.hp, c.agg, []string{"x"}, []string{"o"},
				r{[]string{"> 0"}, []string{iterCell}})
			if c.prio {
				dt.Outputs[0].AllowedValues = "1, 2"
			}
			if err := evalLim(t, dt, x("5")); err == nil {
				t.Errorf("%s: iterating output cell should surface a runtime error", c.name)
			}
		})
	}
}

// TestAnyDivergentSecondCellRuntimeError exercises the ANY policy's second-rule
// output evaluation (the matched[1:] loop) failing at runtime.
func TestAnyDivergentSecondCellRuntimeError(t *testing.T) {
	dt := mkTable(model.HitAny, model.AggNone, []string{"x"}, []string{"o"},
		r{[]string{"> 0"}, []string{"1"}},
		r{[]string{"> 0"}, []string{iterCell}},
	)
	if err := evalLim(t, dt, x("5")); err == nil {
		t.Error("ANY second-rule output runtime error should surface")
	}
}

// TestTraceRecordsEvaluation drives the opt-in tracing path: NewRecorder, the
// trace branches in evaluate/applyHitPolicy/ruleCells, Tables and add.
func TestTraceRecordsEvaluation(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< 10"}, []string{`"low"`}},
		r{[]string{">= 10"}, []string{`"high"`}},
	)
	env := envForVars(x("5"))
	ce, err := CompileTable(dt, env, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rec := NewRecorder()
	scope := env.NewScope(x("5")).WithTrace(rec)
	got, err := ce(scope)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got.String() != "low" {
		t.Fatalf("result = %s, want low", got)
	}

	tables := rec.Tables()
	if len(tables) != 1 {
		t.Fatalf("recorded %d tables, want 1", len(tables))
	}
	tt := tables[0]
	if tt.HitPolicy != string(model.HitUnique) {
		t.Errorf("HitPolicy = %q", tt.HitPolicy)
	}
	if len(tt.Inputs) != 1 || tt.Inputs[0].Expression != "x" || tt.Inputs[0].Value.String() != "5" {
		t.Errorf("Inputs = %+v", tt.Inputs)
	}
	if len(tt.Matched) != 1 || tt.Matched[0] != 0 {
		t.Errorf("Matched = %v, want [0]", tt.Matched)
	}
	if len(tt.Rules) != 2 {
		t.Fatalf("Rules = %d, want 2", len(tt.Rules))
	}
	// Rule 0 matched and produced an output; rule 1 missed and has no output.
	if !tt.Rules[0].Matched || len(tt.Rules[0].Outputs) != 1 || tt.Rules[0].Outputs[0].String() != "low" {
		t.Errorf("rule 0 trace = %+v", tt.Rules[0])
	}
	if tt.Rules[1].Matched || tt.Rules[1].Outputs != nil {
		t.Errorf("rule 1 trace = %+v, want unmatched with no outputs", tt.Rules[1])
	}
	if len(tt.Rules[0].Conditions) != 1 || !tt.Rules[0].Conditions[0].Matched {
		t.Errorf("rule 0 conditions = %+v", tt.Rules[0].Conditions)
	}
}

// TestTraceAggregationField confirms the aggregation label is carried into the
// trace for a collecting table.
func TestTraceAggregationField(t *testing.T) {
	dt := mkTable(model.HitCollect, model.AggSum, []string{"x"}, []string{"o"},
		r{[]string{"> 0"}, []string{"1"}})
	env := envForVars(x("5"))
	ce, err := CompileTable(dt, env, nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	rec := NewRecorder()
	if _, err := ce(env.NewScope(x("5")).WithTrace(rec)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if got := rec.Tables()[0].Aggregation; got != string(model.AggSum) {
		t.Errorf("Aggregation = %q, want %q", got, model.AggSum)
	}
}

// TestParsePriorityListNonList covers the branch where the allowed-values text
// compiles and evaluates but is not a list.
func TestParsePriorityListNonList(t *testing.T) {
	// "1][2" wraps to "[1][2]", which compiles and evaluates (to null) but is
	// not a list, so parsePriorityList reports the non-list error.
	dt := mkTableOut(model.HitPriority, "1][2", r{[]string{"-"}, []string{"1"}})
	if _, err := CompileTable(dt, feel.NewEnv("x"), nil); err == nil {
		t.Error("non-list allowed-values should be a compile error")
	}
}

// TestLessKeyEqualKeys covers lessKey's fall-through (equal keys -> not less),
// reached when two matched rules share the same priority key and the stable
// sort must keep their table order.
func TestLessKeyEqualKeys(t *testing.T) {
	if lessKey([]int{1, 2}, []int{1, 2}) {
		t.Error("equal keys: lessKey should be false")
	}
	// Through Output Order: two rules with identical (unlisted) outputs both
	// score last, so their keys are equal and original order is preserved.
	dt := mkTableOut(model.HitOutputOrder, `"win"`,
		r{[]string{"> 0"}, []string{`"a"`}},
		r{[]string{"> 0"}, []string{`"b"`}},
	)
	got := evalT(t, dt, x("5"))
	if got.String() != "[a, b]" {
		t.Errorf("equal-priority output order = %s, want [a, b]", got)
	}
}

// TestContextEntryCompileError covers compileContext's error wrap when an entry
// fails to compile.
func TestContextEntryCompileError(t *testing.T) {
	ctx := &model.ContextExpr{Entries: []model.ContextEntry{
		{Name: "a", Value: lit("1 +")},
	}}
	if _, err := Compile(ctx, feel.NewEnv(), nil); err == nil {
		t.Error("malformed context entry should be a compile error")
	}
}

// TestContextEntryRuntimeError covers the context closure's per-entry error
// return (an entry that iterates, evaluated under a tight limit).
func TestContextEntryRuntimeError(t *testing.T) {
	ctx := &model.ContextExpr{Entries: []model.ContextEntry{
		{Name: "a", Value: lit(iterCell)},
	}}
	ce, err := Compile(ctx, feel.NewEnv(), nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := ce(feel.NewEnv().NewScopeWithLimits(nil, tightLimits())); err == nil {
		t.Error("iterating context entry under a tight limit should error")
	}
}

// TestListItemRuntimeError covers compileList's closure error return.
func TestListItemRuntimeError(t *testing.T) {
	l := &model.ListExpr{Items: []model.Expression{lit(iterCell)}}
	ce, err := Compile(l, feel.NewEnv(), nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := ce(feel.NewEnv().NewScopeWithLimits(nil, tightLimits())); err == nil {
		t.Error("iterating list item under a tight limit should error")
	}
}

// TestRelationCellRuntimeError covers compileRelation's closure error return.
func TestRelationCellRuntimeError(t *testing.T) {
	rel := &model.RelationExpr{
		Columns: []string{"a"},
		Rows:    []model.RelationRow{{Cells: []model.Expression{lit(iterCell)}}},
	}
	ce, err := Compile(rel, feel.NewEnv(), nil)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if _, err := ce(feel.NewEnv().NewScopeWithLimits(nil, tightLimits())); err == nil {
		t.Error("iterating relation cell under a tight limit should error")
	}
}

// TestFirstAndAnyNoMatchAreNull covers the no-match (null) arms of the First and
// Any hit policies.
func TestFirstAndAnyNoMatchAreNull(t *testing.T) {
	for _, hp := range []model.HitPolicy{model.HitFirst, model.HitAny} {
		dt := mkTable(hp, model.AggNone, []string{"x"}, []string{"out"},
			r{[]string{"> 100"}, []string{`"x"`}})
		if got := evalT(t, dt, x("5")); !value.IsNull(got) {
			t.Errorf("%s no match = %s, want null", hp, got)
		}
	}
}

// TestUnaryTestCompileError covers CompileTable's error when a rule's input cell
// (unary test) is malformed.
func TestUnaryTestCompileError(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"< <"}, []string{"1"}})
	if _, err := CompileTable(dt, feel.NewEnv("x"), nil); err == nil {
		t.Error("malformed unary test should be a compile error")
	}
}

// TestUnaryTestRuntimeError covers the evaluate loop's error return from
// feel.Matches: a unary test that iterates trips the tight iteration limit.
func TestUnaryTestRuntimeError(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"sum(for i in [1, 2, 3] return i)"}, []string{"1"}})
	if err := evalLim(t, dt, x("5")); err == nil {
		t.Error("iterating unary test under a tight limit should error")
	}
}

// TestUniqueMultipleMatchErrorsAsTyped re-confirms errors.As classification,
// keeping the typed-error path exercised alongside the message test.
func TestUniqueMultipleMatchErrorsAsTyped(t *testing.T) {
	dt := mkTable(model.HitUnique, model.AggNone, []string{"x"}, []string{"out"},
		r{[]string{"> 0"}, []string{"1"}},
		r{[]string{"> 0"}, []string{"2"}},
	)
	err := evalErr(t, dt, x("5"))
	var mm *MultipleMatchError
	if !errors.As(err, &mm) {
		t.Fatalf("error = %v, want *MultipleMatchError", err)
	}
}
