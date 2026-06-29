package dmn

import "github.com/pblumer/temis/internal/boxed"

// Trace is an optional, structured explanation of an evaluation: which decision
// tables ran, the input values they tested, which rules matched (and why), and
// what those rules produced. It lets a caller — notably an AI agent (ADR-0013,
// WP-51) — justify a decision rather than merely read its output.
//
// A Trace is derived from the actual evaluation, never reconstructed after the
// fact. It is present on a Result only when WithTrace is passed to Evaluate; the
// default Evaluate path produces no Trace and stays allocation-free.
//
// Unlike the other dmn types, the Trace tree carries JSON tags: it is the
// agent-facing explanation that the HTTP and MCP adapters serialise verbatim, so
// its field names are part of that wire contract.
type Trace struct {
	// Tables holds one entry per decision table evaluated, in evaluation order.
	// A decision whose logic is a literal expression (not a table) produces no
	// table entries.
	Tables []TableTrace `json:"tables"`
}

// TableTrace explains one decision table's evaluation.
type TableTrace struct {
	HitPolicy   string       `json:"hitPolicy"`             // the table's hit policy (U/A/F/R/C)
	Aggregation string       `json:"aggregation,omitempty"` // collect aggregation (SUM/MIN/MAX/COUNT), or "" if none
	Inputs      []TraceInput `json:"inputs"`                // the input columns and the values they produced
	Rules       []TraceRule  `json:"rules"`                 // every rule, with its condition results
	Matched     []int        `json:"matched"`               // indices (0-based) of the rules that matched
}

// TraceInput is one input column: its FEEL expression and the value it evaluated
// to, in the same Go form Evaluate uses for outputs.
type TraceInput struct {
	Expression string `json:"expression"`
	Value      any    `json:"value"`
}

// TraceRule is one rule's contribution to the result.
type TraceRule struct {
	Index      int              `json:"index"`        // 0-based row position in the table
	ID         string           `json:"id,omitempty"` // model rule id, when the source provides one
	Matched    bool             `json:"matched"`
	Conditions []TraceCondition `json:"conditions"`
	// Outputs holds the rule's output values, set only when the rule actually
	// contributed to the result under the hit policy; nil otherwise.
	Outputs []any `json:"outputs,omitempty"`
}

// TraceCondition is one input cell's test: the column expression, the rule's
// unary-test text and whether the input satisfied it. Conditions are reported up
// to and including the first one that fails (the evaluator short-circuits), so a
// non-matching rule shows exactly which condition ruled it out.
type TraceCondition struct {
	Input   string `json:"input"`
	Entry   string `json:"entry"`
	Matched bool   `json:"matched"`
}

// traceFromRecorder maps the internal recorder's tables into the public Trace,
// converting FEEL values to Go via the same mapping as outputs.
func traceFromRecorder(rec *boxed.Recorder) *Trace {
	t := &Trace{}
	for _, bt := range rec.Tables() {
		tt := TableTrace{
			HitPolicy:   bt.HitPolicy,
			Aggregation: bt.Aggregation,
			Matched:     bt.Matched,
		}
		for _, in := range bt.Inputs {
			tt.Inputs = append(tt.Inputs, TraceInput{Expression: in.Expression, Value: fromValue(in.Value)})
		}
		for _, r := range bt.Rules {
			tr := TraceRule{Index: r.Index, ID: r.ID, Matched: r.Matched}
			for _, c := range r.Conditions {
				tr.Conditions = append(tr.Conditions, TraceCondition{Input: c.Input, Entry: c.Entry, Matched: c.Matched})
			}
			for _, o := range r.Outputs {
				tr.Outputs = append(tr.Outputs, fromValue(o))
			}
			tt.Rules = append(tt.Rules, tr)
		}
		t.Tables = append(t.Tables, tt)
	}
	return t
}
