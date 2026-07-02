package flow

import (
	"context"
	"fmt"
	"strconv"

	"github.com/pblumer/temis/dmn"
)

// DefaultMaxSteps bounds how many steps a flow may evaluate unless overridden
// with WithMaxSteps. It is a guard against a runaway descriptor (ADR-0008); a
// well-formed flow evaluates each step exactly once.
const DefaultMaxSteps = 100

// Option tunes a single Evaluate call.
type Option func(*evalConfig)

type evalConfig struct {
	maxSteps int
	trace    bool
}

// WithMaxSteps overrides the per-evaluation step guard (ADR-0008). Non-positive
// values are ignored.
func WithMaxSteps(n int) Option {
	return func(c *evalConfig) {
		if n > 0 {
			c.maxSteps = n
		}
	}
}

// WithTrace makes Evaluate attach a decision trace to Result.Trace: the table
// traces of every decision step, in evaluation order, so a caller can see which
// rules fired across the whole flow. Service steps contribute no trace (the dmn
// service API produces none). Without it, Result.Trace stays nil.
func WithTrace() Option {
	return func(c *evalConfig) { c.trace = true }
}

// Validate runs model-aware validation: every step's model resolves, its target
// decision/service exists, and — for a decision — its required inputs are wired
// and no wiring targets an input the decision cannot reach (typed against the
// target's reachable input schema — its direct plus transitively-required leaf
// inputs, ADR-0026 L2a — so a composed decision is wireable in a flow). Structural
// diagnostics from Compile are included first. A non-empty result means the flow
// must not be evaluated.
func (f *Flow) Validate(ctx context.Context, r Resolver) Diagnostics {
	diags := append(Diagnostics{}, f.diags...)
	for _, id := range f.sortedIDs() {
		s := f.desc.Steps[f.stepIdx[id]]
		if s.Model == "" || s.Decision == "" {
			continue // already reported by Compile
		}
		defs, err := r.Resolve(ctx, s.Model)
		if err != nil {
			diags = append(diags, Diagnostic{Code: CodeModelUnresolved, Step: id, Message: fmt.Sprintf("cannot resolve model %q: %v", s.Model, err)})
			continue
		}
		if _, decErr := defs.Decision(s.Decision); decErr != nil {
			if _, svcErr := defs.Service(s.Decision); svcErr != nil {
				diags = append(diags, Diagnostic{Code: CodeTargetNotFound, Step: id, Message: fmt.Sprintf("model has no decision or service %q", s.Decision)})
			}
			continue // a service: no public input schema to type-check against
		}
		// Type the wiring against the target's REACHABLE inputs — its direct inputs
		// plus those reached transitively through required decisions (ADR-0026) — not
		// just dec.InputSchema()'s directly declared ones. A composed decision needs
		// a leaf input only via a sub-decision; wiring it must be legal, not
		// FLOW_UNKNOWN_INPUT.
		schema, serr := defs.ReachableInputSchema(s.Decision)
		if serr != nil {
			diags = append(diags, Diagnostic{Code: CodeTargetNotFound, Step: id, Message: serr.Error()})
			continue
		}
		diags = append(diags, checkWiring(id, s, schema)...)
	}
	return diags
}

// checkWiring type-checks a decision step's wiring against its input schema.
func checkWiring(step string, s Step, schema []dmn.InputField) Diagnostics {
	var diags Diagnostics
	known := make(map[string]bool, len(schema))
	for _, fld := range schema {
		known[fld.Name] = true
	}
	for target := range s.In {
		if !known[target] {
			diags = append(diags, Diagnostic{Code: CodeUnknownInput, Step: step, Message: fmt.Sprintf("wires input %q, which decision %q does not declare", target, s.Decision)})
		}
	}
	for _, fld := range schema {
		if fld.Required {
			if _, ok := s.In[fld.Name]; !ok {
				diags = append(diags, Diagnostic{Code: CodeInputUnwired, Step: step, Message: fmt.Sprintf("required input %q is not wired", fld.Name)})
			}
		}
	}
	return diags
}

// Evaluate runs the flow statelessly and returns a dmn.Result whose Outputs are
// the assembled flow output and whose Decisions map holds every step output
// keyed as "stepID.output". It validates first and refuses (an *Error carrying
// Diagnostics) before evaluating anything if the flow is not sound.
func (f *Flow) Evaluate(ctx context.Context, in dmn.Input, r Resolver, opts ...Option) (dmn.Result, error) {
	cfg := evalConfig{maxSteps: DefaultMaxSteps}
	for _, o := range opts {
		o(&cfg)
	}

	if d := f.Validate(ctx, r); d.HasErrors() {
		return dmn.Result{}, &Error{Diagnostics: d}
	}
	if len(f.order) > cfg.maxSteps {
		return dmn.Result{}, &Error{Diagnostics: Diagnostics{{Code: CodeMaxSteps, Message: fmt.Sprintf("flow has %d steps, exceeds MaxSteps=%d", len(f.order), cfg.maxSteps)}}}
	}

	stepOut := make(map[string]map[string]any, len(f.order))
	all := make(map[string]any)
	var traceTables []dmn.TableTrace

	for _, idx := range f.order {
		if err := ctx.Err(); err != nil {
			return dmn.Result{}, err
		}
		s := f.desc.Steps[idx]
		defs, err := r.Resolve(ctx, s.Model)
		if err != nil {
			return dmn.Result{}, &Error{Diagnostics: Diagnostics{{Code: CodeModelUnresolved, Step: s.ID, Message: err.Error()}}}
		}

		var res dmn.Result
		if dec, decErr := defs.Decision(s.Decision); decErr == nil {
			// Build and validate against the target's REACHABLE input schema (direct +
			// transitively-required leaf inputs), so coerce() also fires for transitive
			// numeric inputs and a transitively-reached input is accepted rather than
			// rejected as unknown. We deliberately do NOT use dec.Evaluate(
			// WithStrictInput): that validates only the narrow per-decision schema and
			// would reject the transitive inputs. Nor EvaluateGraph, whose whole-model
			// schema is too wide (it would demand inputs unrelated decisions need) and
			// which re-evaluates every decision in the model per step (a budget
			// multiplication, ADR-0008). Instead we validate once against the cone here,
			// then evaluate leniently so the evaluator itself feeds the transitive
			// inputs down into the required sub-decisions (dmn/eval.go).
			schema, serr := defs.ReachableInputSchema(s.Decision)
			if serr != nil {
				return dmn.Result{}, &Error{Diagnostics: Diagnostics{{Code: CodeTargetNotFound, Step: s.ID, Message: serr.Error()}}}
			}
			stepIn, berr := f.buildInput(ctx, idx, schema, in, stepOut)
			if berr != nil {
				return dmn.Result{}, berr
			}
			if probs, verr := defs.ValidateReachableInput(s.Decision, stepIn); verr != nil {
				return dmn.Result{}, &Error{Diagnostics: Diagnostics{{Code: CodeTargetNotFound, Step: s.ID, Message: verr.Error()}}}
			} else if len(probs) > 0 {
				return dmn.Result{}, fmt.Errorf("flow: step %q: %w", s.ID, &dmn.InputError{Problems: probs})
			}
			var evalOpts []dmn.EvalOption
			if cfg.trace {
				evalOpts = append(evalOpts, dmn.WithTrace())
			}
			res, err = dec.Evaluate(ctx, stepIn, evalOpts...)
		} else if svc, svcErr := defs.Service(s.Decision); svcErr == nil {
			stepIn, berr := f.buildInput(ctx, idx, nil, in, stepOut)
			if berr != nil {
				return dmn.Result{}, berr
			}
			res, err = svc.Evaluate(ctx, stepIn)
		} else {
			return dmn.Result{}, &Error{Diagnostics: Diagnostics{{Code: CodeTargetNotFound, Step: s.ID, Message: decErr.Error()}}}
		}
		if err != nil {
			return dmn.Result{}, fmt.Errorf("flow: step %q: %w", s.ID, err)
		}

		stepOut[s.ID] = res.Outputs
		for k, v := range res.Outputs {
			all[s.ID+"."+k] = v
		}
		if cfg.trace && res.Trace != nil {
			traceTables = append(traceTables, res.Trace.Tables...)
		}
	}

	outputs, err := f.assembleOutput(ctx, in, stepOut)
	if err != nil {
		return dmn.Result{}, err
	}
	result := dmn.Result{Outputs: outputs, Decisions: all}
	if cfg.trace && len(traceTables) > 0 {
		result.Trace = &dmn.Trace{Tables: traceTables}
	}
	return result, nil
}

// buildInput resolves a step's compiled wirings into a dmn.Input, coercing each
// value to the target input's declared FEEL type (so a numeric output — rendered
// by dmn as a decimal string — feeds a numeric input as a number, not a string).
func (f *Flow) buildInput(ctx context.Context, idx int, schema []dmn.InputField, in dmn.Input, stepOut map[string]map[string]any) (dmn.Input, error) {
	typeOf := make(map[string]string, len(schema))
	for _, fld := range schema {
		typeOf[fld.Name] = fld.Type
	}
	wirings := f.compiledIn[idx]
	out := make(dmn.Input, len(wirings))
	for target, m := range wirings {
		v, ok, err := f.resolveMapping(ctx, m, in, stepOut)
		if err != nil {
			return nil, &Error{Diagnostics: Diagnostics{{Code: CodeMappingInvalid, Step: f.desc.Steps[idx].ID, Message: fmt.Sprintf("input %q: evaluating %q: %v", target, m.raw, err)}}}
		}
		if !ok {
			return nil, &Error{Diagnostics: Diagnostics{{Code: CodeUnknownRef, Step: f.desc.Steps[idx].ID, Message: fmt.Sprintf("input %q references %q, which did not resolve", target, m.raw)}}}
		}
		out[target] = coerce(v, typeOf[target])
	}
	return out, nil
}

// assembleOutput builds the flow's output map from the descriptor's output
// wirings, or falls back to the last step's outputs when none are declared.
func (f *Flow) assembleOutput(ctx context.Context, in dmn.Input, stepOut map[string]map[string]any) (map[string]any, error) {
	if len(f.compiledOut) > 0 {
		out := make(map[string]any, len(f.compiledOut))
		for name, m := range f.compiledOut {
			v, ok, err := f.resolveMapping(ctx, m, in, stepOut)
			if err != nil {
				return nil, &Error{Diagnostics: Diagnostics{{Code: CodeMappingInvalid, Message: fmt.Sprintf("output %q: evaluating %q: %v", name, m.raw, err)}}}
			}
			if !ok {
				return nil, &Error{Diagnostics: Diagnostics{{Code: CodeUnknownRef, Message: fmt.Sprintf("output %q references %q, which did not resolve", name, m.raw)}}}
			}
			out[name] = v
		}
		return out, nil
	}
	if len(f.order) == 0 {
		return map[string]any{}, nil
	}
	last := f.desc.Steps[f.order[len(f.order)-1]]
	out := make(map[string]any, len(stepOut[last.ID]))
	for k, v := range stepOut[last.ID] {
		out[k] = v
	}
	return out, nil
}

// resolveMapping produces a mapping's value. A step reference reads the earlier
// step's output (ok=false when the key is absent — a real wiring fault); a flow
// input reads the input (absent → nil, i.e. FEEL null, which is legitimate); a
// FEEL expression is evaluated against the flow inputs plus each prior step's
// outputs (exposed as a context named by the step id).
func (f *Flow) resolveMapping(ctx context.Context, m mapping, in dmn.Input, stepOut map[string]map[string]any) (any, bool, error) {
	switch m.kind {
	case mStep:
		v, ok := stepOut[m.stepID][m.key]
		return v, ok, nil
	case mInput:
		return in[m.name], true, nil
	default: // mFeel
		v, err := m.expr.Evaluate(ctx, f.feelContext(in, stepOut))
		return v, true, err
	}
}

// feelContext builds the variable context a FEEL mapping evaluates against: every
// flow input, plus each completed step's outputs exposed as a context under the
// step's id (so "risk.Risk Level" is FEEL path access into step "risk").
func (f *Flow) feelContext(in dmn.Input, stepOut map[string]map[string]any) dmn.Input {
	ctx := make(dmn.Input, len(in)+len(stepOut))
	for k, v := range in {
		ctx[k] = v
	}
	for stepID, outs := range stepOut {
		ctx[stepID] = map[string]any(outs)
	}
	return ctx
}

// coerce converts a value to the target FEEL type where a round-trip would
// otherwise mistype it. dmn renders numbers as exact decimal strings; when the
// target input is a number, parse the string back to an integer (exact) or float
// so it is fed as a number. All other values pass through unchanged.
func coerce(v any, feelType string) any {
	if feelType != "number" {
		return v
	}
	s, ok := v.(string)
	if !ok {
		return v
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if fv, err := strconv.ParseFloat(s, 64); err == nil {
		return fv
	}
	return v
}
