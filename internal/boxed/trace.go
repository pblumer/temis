package boxed

import "github.com/pblumer/temis/internal/value"

// Recorder collects a decision table's evaluation steps so the engine can hand
// back a structured explanation (ADR-0012, WP-51). It is attached to a Scope via
// (*feel.Scope).WithTrace before evaluation and read back afterwards. A nil
// Recorder means tracing is off and the evaluator takes its normal,
// allocation-free path; the recorder is written only on the opt-in trace path.
//
// A Recorder is single-use and not safe for concurrent use: each traced
// evaluation creates its own and never shares it across goroutines.
type Recorder struct {
	tables []TableTrace
}

// NewRecorder returns an empty Recorder ready to attach to a scope.
func NewRecorder() *Recorder { return &Recorder{} }

// Tables returns the recorded table evaluations in evaluation order.
func (r *Recorder) Tables() []TableTrace { return r.tables }

func (r *Recorder) add(tt TableTrace) { r.tables = append(r.tables, tt) }

// TableTrace explains one decision table's evaluation: its hit policy, the input
// values it tested, every rule's condition results, and which rules matched.
type TableTrace struct {
	HitPolicy   string
	Aggregation string // "" for a plain (non-aggregating) collect or a single-hit policy
	Inputs      []InputTrace
	Rules       []RuleTrace
	Matched     []int // indices (0-based) of the rules that matched
}

// InputTrace is one input column: the FEEL expression and the value it produced.
type InputTrace struct {
	Expression string
	Value      value.Value
}

// RuleTrace is one rule's contribution: whether it matched, the per-condition
// results that led to that verdict, and the outputs it produced when it fired.
type RuleTrace struct {
	Index      int    // 0-based row position in the table
	ID         string // model rule id, when the source provides one
	Matched    bool
	Conditions []ConditionTrace
	// Outputs holds the rule's output values, set only when the rule actually
	// contributed to the result under the hit policy; nil otherwise.
	Outputs []value.Value
}

// ConditionTrace is one input cell's test: the column expression, the rule's
// unary-test text and whether the input value satisfied it. Conditions are
// recorded up to and including the first one that fails, mirroring the
// evaluator's short-circuit — cells after a miss are not evaluated and so are
// not reported.
type ConditionTrace struct {
	Input   string
	Entry   string
	Matched bool
}
