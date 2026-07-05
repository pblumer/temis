// Package boxed compiles DMN boxed expressions into FEEL closures. WP-09 covers
// decision tables with hit policies Unique, Any, First, Rule order and Collect
// (incl. SUM/MIN/MAX/COUNT aggregation); the remaining boxed forms and hit
// policies arrive in later work packages.
package boxed

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// MultipleMatchError reports that a UNIQUE hit-policy decision table matched
// more than one rule, which DMN forbids. It is a typed runtime error so the API
// edge (dmn.Evaluate) can classify it precisely as UNIQUE_MULTIPLE_MATCH via
// errors.As, instead of falling back to a generic runtime code.
type MultipleMatchError struct {
	Matched int // number of rules that matched
}

func (e *MultipleMatchError) Error() string {
	return fmt.Sprintf("UNIQUE hit policy: %d rules matched", e.Matched)
}

// CompileTable compiles a decision table into a CompiledExpr over env (the
// decision's input variables). The evaluated result depends on the table's
// shape and hit policy:
//
//   - single output, single-hit policy (U/A/F): the bare output value;
//   - multiple outputs: a context {outputName: value};
//   - Rule order / Collect without aggregation: a list (of values or contexts);
//   - Collect with aggregation: the aggregated value (single output only).
//
// Per DMN, no matching rule yields null (an empty list for the collecting
// policies). A Unique table with more than one match, or an Any table with
// divergent outputs, is an evaluation error.
func CompileTable(dt *model.DecisionTable, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	switch dt.HitPolicy {
	case model.HitUnique, model.HitAny, model.HitFirst, model.HitRuleOrder,
		model.HitCollect, model.HitPriority, model.HitOutputOrder:
	default:
		return nil, fmt.Errorf("hit policy %q is not supported", dt.HitPolicy)
	}
	if dt.Aggregation != model.AggNone && len(dt.Outputs) != 1 {
		return nil, fmt.Errorf("collect aggregation requires exactly one output, got %d", len(dt.Outputs))
	}

	ct := &compiledTable{
		hitPolicy:   dt.HitPolicy,
		aggregation: dt.Aggregation,
	}

	for _, out := range dt.Outputs {
		ct.outputNames = append(ct.outputNames, out.Name)
		prio, err := parsePriorityList(out.AllowedValues)
		if err != nil {
			return nil, fmt.Errorf("output %q values %q: %w", out.Name, out.AllowedValues, err)
		}
		ct.priorities = append(ct.priorities, prio)

		// The optional default output entry is the column's value when no rule
		// matches (DMN 8.2.11); nil when the column declares none.
		var def feel.CompiledExpr
		if text := strings.TrimSpace(out.DefaultOutput); text != "" {
			def, err = feel.CompileStringWith(text, env, funcs)
			if err != nil {
				return nil, fmt.Errorf("output %q default %q: %w", out.Name, out.DefaultOutput, err)
			}
		}
		ct.defaults = append(ct.defaults, def)
	}

	for i, in := range dt.Inputs {
		ce, err := feel.CompileStringWith(in.Expression, env, funcs)
		if err != nil {
			return nil, fmt.Errorf("input %d expression %q: %w", i+1, in.Expression, err)
		}
		ct.inputs = append(ct.inputs, ce)
		ct.inputExprs = append(ct.inputExprs, in.Expression)
	}

	unaryEnv := env.Derive(feel.InputVar)
	for ri, r := range dt.Rules {
		if len(r.InputEntries) != len(dt.Inputs) {
			return nil, fmt.Errorf("rule %d has %d input entries, want %d", ri+1, len(r.InputEntries), len(dt.Inputs))
		}
		if len(r.OutputEntries) != len(dt.Outputs) {
			return nil, fmt.Errorf("rule %d has %d output entries, want %d", ri+1, len(r.OutputEntries), len(dt.Outputs))
		}
		cr := compiledRule{index: ri, id: r.ID, inputEntries: r.InputEntries}
		for ci, entry := range r.InputEntries {
			test, err := feel.CompileUnaryTest(entry, unaryEnv)
			if err != nil {
				return nil, fmt.Errorf("rule %d input %d %q: %w", ri+1, ci+1, entry, err)
			}
			cr.tests = append(cr.tests, test)
		}
		for ci, entry := range r.OutputEntries {
			out, err := compileOutput(entry, env, funcs)
			if err != nil {
				return nil, fmt.Errorf("rule %d output %d %q: %w", ri+1, ci+1, entry, err)
			}
			cr.outputs = append(cr.outputs, out)
		}
		ct.rules = append(ct.rules, cr)
	}

	return ct.evaluate, nil
}

// compileOutput compiles an output cell; an empty cell evaluates to null.
func compileOutput(entry string, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if strings.TrimSpace(entry) == "" {
		return feel.CompileString("null", env)
	}
	return feel.CompileStringWith(entry, env, funcs)
}

type compiledRule struct {
	index        int                 // 0-based row position, for tracing
	id           string              // model rule id, for tracing
	inputEntries []string            // raw unary-test texts, for tracing
	tests        []feel.CompiledExpr // one per input column (unary test, references "?")
	outputs      []feel.CompiledExpr // one per output column
}

type compiledTable struct {
	inputs      []feel.CompiledExpr
	inputExprs  []string // raw input-column expressions, for tracing
	outputNames []string
	priorities  [][]value.Value     // per output: allowed values in priority order (P/O)
	defaults    []feel.CompiledExpr // per output: default output entry (nil when none)
	rules       []compiledRule
	hitPolicy   model.HitPolicy
	aggregation model.Aggregation
}

func (ct *compiledTable) evaluate(s *feel.Scope) (value.Value, error) {
	// Tracing is opt-in: a nil recorder means the normal, allocation-free path.
	rec, _ := s.Trace().(*Recorder)
	var tt *TableTrace
	if rec != nil {
		tt = &TableTrace{HitPolicy: string(ct.hitPolicy), Aggregation: string(ct.aggregation)}
	}

	// Evaluate each input column once; the unary tests run in the decision scope
	// with "?" bound to the column's value. A single reusable scope carries "?",
	// rebound per column (BindInput), so the table matches without allocating a
	// scope per column.
	colVals := make([]value.Value, len(ct.inputs))
	for i, in := range ct.inputs {
		v, err := in(s)
		if err != nil {
			return nil, err
		}
		colVals[i] = v
		if tt != nil {
			tt.Inputs = append(tt.Inputs, InputTrace{Expression: ct.inputExprs[i], Value: v})
		}
	}
	us := s.WithInput()

	var matched []int
	for ri, r := range ct.rules {
		ok := true
		var conds []ConditionTrace
		for ci, test := range r.tests {
			us.BindInput(colVals[ci])
			m, err := feel.Matches(test, us)
			if err != nil {
				return nil, err
			}
			if tt != nil {
				conds = append(conds, ConditionTrace{Input: ct.inputExprs[ci], Entry: r.inputEntries[ci], Matched: m})
			}
			if !m {
				ok = false
				break // short-circuit: cells after a miss are not evaluated (and not traced)
			}
		}
		if ok {
			matched = append(matched, ri)
		}
		if tt != nil {
			tt.Rules = append(tt.Rules, RuleTrace{Index: r.index, ID: r.id, Matched: ok, Conditions: conds})
		}
	}
	if tt != nil {
		tt.Matched = append(tt.Matched, matched...)
	}

	out, err := ct.applyHitPolicy(s, matched, tt)
	if rec != nil {
		rec.add(*tt)
	}
	return out, err
}

func (ct *compiledTable) applyHitPolicy(s *feel.Scope, matched []int, tt *TableTrace) (value.Value, error) {
	switch ct.hitPolicy {
	case model.HitUnique:
		if len(matched) == 0 {
			return ct.noMatch(s)
		}
		if len(matched) > 1 {
			return nil, &MultipleMatchError{Matched: len(matched)}
		}
		return ct.ruleOutput(s, matched[0], tt)

	case model.HitFirst:
		if len(matched) == 0 {
			return ct.noMatch(s)
		}
		return ct.ruleOutput(s, matched[0], tt)

	case model.HitAny:
		if len(matched) == 0 {
			return ct.noMatch(s)
		}
		first, err := ct.ruleOutput(s, matched[0], tt)
		if err != nil {
			return nil, err
		}
		for _, ri := range matched[1:] {
			v, err := ct.ruleOutput(s, ri, tt)
			if err != nil {
				return nil, err
			}
			if value.Equal(first, v) != value.True {
				return nil, fmt.Errorf("ANY hit policy: matched rules have divergent outputs")
			}
		}
		return first, nil

	case model.HitRuleOrder:
		return ct.collectList(s, matched, tt)

	case model.HitCollect:
		if ct.aggregation == model.AggNone {
			return ct.collectList(s, matched, tt)
		}
		return ct.aggregate(s, matched, tt)

	case model.HitPriority:
		return ct.prioritized(s, matched, false, tt)

	case model.HitOutputOrder:
		return ct.prioritized(s, matched, true, tt)

	default:
		return nil, fmt.Errorf("unsupported hit policy %q", ct.hitPolicy)
	}
}

// ruleOutput builds a matched rule's output: the bare value for a single output,
// or a context keyed by output name for multiple outputs.
func (ct *compiledTable) ruleOutput(s *feel.Scope, ri int, tt *TableTrace) (value.Value, error) {
	cells, err := ct.ruleCells(s, ri, tt)
	if err != nil {
		return nil, err
	}
	return ct.outputValue(cells), nil
}

// ruleCells evaluates a rule's output cells, one value per output column. When
// tracing, it records those values against the rule that produced them — the
// single chokepoint every hit policy routes output evaluation through.
func (ct *compiledTable) ruleCells(s *feel.Scope, ri int, tt *TableTrace) ([]value.Value, error) {
	r := ct.rules[ri]
	cells := make([]value.Value, len(r.outputs))
	for i, out := range r.outputs {
		v, err := out(s)
		if err != nil {
			return nil, err
		}
		cells[i] = v
	}
	if tt != nil {
		tt.Rules[ri].Outputs = cells
	}
	return cells, nil
}

// outputValue assembles evaluated output cells into a rule's result: the bare
// value for a single output, or a context keyed by output name otherwise.
func (ct *compiledTable) outputValue(cells []value.Value) value.Value {
	if len(ct.outputNames) == 1 {
		return cells[0]
	}
	ctx := value.NewContext()
	for i, name := range ct.outputNames {
		ctx.Put(name, cells[i])
	}
	return ctx
}

// noMatch is the result when no rule matches a single-hit (or aggregating)
// table: the declared default output entry, or null when none is declared.
func (ct *compiledTable) noMatch(s *feel.Scope) (value.Value, error) {
	if v, ok, err := ct.defaultValue(s); err != nil {
		return nil, err
	} else if ok {
		return v, nil
	}
	return value.Null, nil
}

// defaultValue assembles the table's default output: the bare value for a single
// output, or a context keyed by output name. ok is false when no output column
// declares a default entry.
func (ct *compiledTable) defaultValue(s *feel.Scope) (value.Value, bool, error) {
	any := false
	for _, d := range ct.defaults {
		if d != nil {
			any = true
			break
		}
	}
	if !any {
		return value.Null, false, nil
	}
	cells := make([]value.Value, len(ct.defaults))
	for i, d := range ct.defaults {
		if d == nil {
			cells[i] = value.Null
			continue
		}
		v, err := d(s)
		if err != nil {
			return nil, false, err
		}
		cells[i] = v
	}
	return ct.outputValue(cells), true, nil
}

func (ct *compiledTable) collectList(s *feel.Scope, matched []int, tt *TableTrace) (value.Value, error) {
	elems := make([]value.Value, 0, len(matched))
	for _, ri := range matched {
		v, err := ct.ruleOutput(s, ri, tt)
		if err != nil {
			return nil, err
		}
		elems = append(elems, v)
	}
	return value.NewList(elems...), nil
}

func (ct *compiledTable) aggregate(s *feel.Scope, matched []int, tt *TableTrace) (value.Value, error) {
	if ct.aggregation == model.AggCount {
		return value.NumberFromInt64(int64(len(matched))), nil
	}
	if len(matched) == 0 {
		return ct.noMatch(s)
	}
	vals := make([]value.Value, 0, len(matched))
	for _, ri := range matched {
		v, err := ct.ruleOutput(s, ri, tt)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}

	switch ct.aggregation {
	case model.AggSum:
		total := value.Value(value.NumberFromInt64(0))
		for _, v := range vals {
			total = value.Add(total, v)
		}
		return total, nil
	case model.AggMin:
		return extremum(vals, -1), nil
	case model.AggMax:
		return extremum(vals, 1), nil
	default:
		return nil, fmt.Errorf("unsupported aggregation %q", ct.aggregation)
	}
}

// extremum returns the most extreme value in the given direction (-1 min, +1
// max); an incomparable pair yields null.
func extremum(vals []value.Value, dir int) value.Value {
	best := vals[0]
	for _, v := range vals[1:] {
		cmp, ok := value.Compare(v, best)
		if !ok {
			return value.Null
		}
		if (dir < 0 && cmp < 0) || (dir > 0 && cmp > 0) {
			best = v
		}
	}
	return best
}
