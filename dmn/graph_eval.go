package dmn

import "context"

// GraphResult is the outcome of evaluating a whole model (EvaluateGraph): each
// decision's value and, with WithTrace, its trace, keyed by decision name — so a
// caller can show the entire decision requirements graph computed from a single
// set of leaf inputs. A decision that fails to evaluate (e.g. a runtime error)
// has its message in Errors and no entry in Values; a spec-conformant null is a
// nil value, not an error.
type GraphResult struct {
	// Values holds every executable decision's result, keyed by decision name.
	Values map[string]any
	// Traces holds each decision's structured explanation, present only with
	// WithTrace and only for decisions that evaluated without error.
	Traces map[string]*Trace
	// Errors holds the message for each decision that failed to evaluate, keyed by
	// name; nil when every decision succeeded.
	Errors map[string]string
	// Diags collects the runtime diagnostics gathered across all decisions.
	Diags Diagnostics
}

// EvaluateGraph evaluates every executable decision in the model against in and
// returns each one's value (and trace when WithTrace is set), so a caller can
// present the whole decision requirements graph computed from a single set of
// leaf inputs — exactly what an "evaluate the graph" view needs (the user fills
// the input data once and sees every decision's result).
//
// With WithStrictInput, in is validated once against the model's whole-graph
// input schema (ModelInputSchema) and a non-conforming input fails with an
// *InputError before any decision runs; an input reached only transitively
// (named by a downstream decision but not the one being shown) is accepted.
// Without it, evaluation is lenient.
//
// Each decision is evaluated as its own root, so it carries its own trace and a
// failure in one decision (recorded in GraphResult.Errors) does not blank the
// others. Shared sub-decisions are recomputed; this is fine for the interactive
// use this serves. Strict validation, when requested, is applied once at the
// graph level, so the per-decision evaluations here run leniently.
func (d *Definitions) EvaluateGraph(ctx context.Context, in Input, opts ...EvalOption) (GraphResult, error) {
	var cfg evalConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	if cfg.strict {
		if probs := d.ValidateModelInput(in); len(probs) > 0 {
			return GraphResult{}, &InputError{Problems: probs}
		}
	}

	var perOpts []EvalOption
	if cfg.trace {
		perOpts = append(perOpts, WithTrace())
	}

	// A decision is evaluated rooted at itself, so its trace captures every table
	// in its cone — including its dependencies'. Each decision's OWN logic is its
	// last-evaluated table (the evaluator runs required decisions first), and only
	// when the decision is itself a decision table; a literal decision has none.
	// Keep just that, so a decision's reasoning is its own and a literal shows no
	// (foreign) table.
	ownsTable := map[string]bool{}
	for _, dec := range d.model.Decisions {
		if dec.Name != "" {
			ownsTable[dec.Name] = dec.DecisionTable != nil
		}
	}

	res := GraphResult{Values: map[string]any{}}
	for _, cd := range d.order {
		if cd.expr == nil || cd.name == "" {
			continue
		}
		r, err := cd.Evaluate(ctx, in, perOpts...)
		if err != nil {
			if res.Errors == nil {
				res.Errors = map[string]string{}
			}
			res.Errors[cd.name] = err.Error()
			continue
		}
		res.Values[cd.name] = r.Outputs[cd.name]
		if cfg.trace && ownsTable[cd.name] && r.Trace != nil && len(r.Trace.Tables) > 0 {
			if res.Traces == nil {
				res.Traces = map[string]*Trace{}
			}
			res.Traces[cd.name] = &Trace{Tables: []TableTrace{r.Trace.Tables[len(r.Trace.Tables)-1]}}
		}
		res.Diags = append(res.Diags, r.Diags...)
	}
	return res, nil
}
