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

// Validate runs model-aware validation: every step's model resolves, its target
// decision/service exists, and — for a decision — its required inputs are wired
// and no wiring targets an input the decision does not declare (typed against
// InputSchema, WP-52). Structural diagnostics from Compile are included first.
// A non-empty result means the flow must not be evaluated.
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
		dec, decErr := defs.Decision(s.Decision)
		if decErr != nil {
			if _, svcErr := defs.Service(s.Decision); svcErr != nil {
				diags = append(diags, Diagnostic{Code: CodeTargetNotFound, Step: id, Message: fmt.Sprintf("model has no decision or service %q", s.Decision)})
			}
			continue // a service: no public input schema to type-check against
		}
		diags = append(diags, checkWiring(id, s, dec.InputSchema())...)
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
			stepIn, berr := f.buildInput(s, dec.InputSchema(), in, stepOut)
			if berr != nil {
				return dmn.Result{}, berr
			}
			res, err = dec.Evaluate(ctx, stepIn, dmn.WithStrictInput())
		} else if svc, svcErr := defs.Service(s.Decision); svcErr == nil {
			stepIn, berr := f.buildInput(s, nil, in, stepOut)
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
	}

	outputs, err := f.assembleOutput(in, stepOut)
	if err != nil {
		return dmn.Result{}, err
	}
	return dmn.Result{Outputs: outputs, Decisions: all}, nil
}

// buildInput resolves a step's wiring into a dmn.Input, coercing each value to
// the target input's declared FEEL type (so a numeric output — rendered by dmn
// as a decimal string — feeds a numeric input as a number, not a string).
func (f *Flow) buildInput(s Step, schema []dmn.InputField, in dmn.Input, stepOut map[string]map[string]any) (dmn.Input, error) {
	typeOf := make(map[string]string, len(schema))
	for _, fld := range schema {
		typeOf[fld.Name] = fld.Type
	}
	out := make(dmn.Input, len(s.In))
	for target, ref := range s.In {
		v, ok := f.resolveRef(ref, in, stepOut)
		if !ok {
			return nil, &Error{Diagnostics: Diagnostics{{Code: CodeUnknownRef, Step: s.ID, Message: fmt.Sprintf("input %q references %q, which did not resolve", target, ref)}}}
		}
		out[target] = coerce(v, typeOf[target])
	}
	return out, nil
}

// assembleOutput builds the flow's output map from the descriptor's output
// references, or falls back to the last step's outputs when none are declared.
func (f *Flow) assembleOutput(in dmn.Input, stepOut map[string]map[string]any) (map[string]any, error) {
	if len(f.desc.Output) > 0 {
		out := make(map[string]any, len(f.desc.Output))
		for name, ref := range f.desc.Output {
			v, ok := f.resolveRef(ref, in, stepOut)
			if !ok {
				return nil, &Error{Diagnostics: Diagnostics{{Code: CodeUnknownRef, Message: fmt.Sprintf("output %q references %q, which did not resolve", name, ref)}}}
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

// resolveRef looks a reference up against the flow inputs and prior step outputs.
// A "stepID.key" reference resolves against that step's outputs (ok=false when
// the key is absent — a real wiring fault). Any other reference is a flow input;
// an absent flow input resolves to nil (FEEL null), which is legitimate.
func (f *Flow) resolveRef(ref string, in dmn.Input, stepOut map[string]map[string]any) (any, bool) {
	if stepID, key, isStep := f.parseStepRef(ref); isStep {
		v, ok := stepOut[stepID][key]
		return v, ok
	}
	return in[ref], true
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
