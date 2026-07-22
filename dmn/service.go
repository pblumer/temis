package dmn

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// CompiledService is a compiled DMN decision service: a reusable unit that
// evaluates its output (and any encapsulated) decisions and returns the output
// decisions' results. It is immutable and safe to evaluate concurrently.
type CompiledService struct {
	id, name string
	outputs  []*CompiledDecision
	// boundary holds the names of the service's input decisions, which are
	// supplied by the caller rather than computed.
	boundary map[string]bool
	// outType is the service's declared output type, applied to a single output
	// decision's value (DMN §10.3.2.9.4 coercion; nil = Any).
	outType *feel.Type
	// limits are the resource bounds enforced for an evaluation of this service
	// (WP-34), resolved from the engine configuration at compile time.
	limits feel.Limits
}

// Name returns the service's name.
func (s *CompiledService) Name() string { return s.name }

// ID returns the service's identifier.
func (s *CompiledService) ID() string { return s.id }

// Service returns the compiled decision service identified by idOrName.
func (d *Definitions) Service(idOrName string) (*CompiledService, error) {
	s, ok := d.servicesByID[idOrName]
	if !ok {
		s, ok = d.servicesByNam[idOrName]
	}
	if !ok {
		return nil, fmt.Errorf("dmn: no decision service %q", idOrName)
	}
	return s, nil
}

// Evaluate runs the decision service against in and returns its result. Output
// decisions (and the encapsulated decisions they require) are evaluated; the
// service's input decisions are treated as caller-supplied boundaries and are
// not computed. Result.Outputs is keyed by output-decision name; Result.Decisions
// holds every decision the service actually evaluated.
func (s *CompiledService) Evaluate(ctx context.Context, in Input) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	base, err := inputToValues(in)
	if err != nil {
		return Result{}, err
	}

	ev := newEvaluator(base, s.limits)
	ev.boundary = s.boundary

	outputs := make(map[string]any, len(s.outputs))
	for _, out := range s.outputs {
		v, err := ev.eval(out)
		if err != nil {
			return Result{}, err
		}
		// A single-output service coerces the output to its declared type (e.g. a
		// non-conforming value becomes null, a singleton list unwraps).
		if len(s.outputs) == 1 && s.outType != nil {
			v = coerceToType(v, s.outType)
		}
		outputs[out.name] = fromValue(v)
	}
	return Result{Outputs: outputs, Decisions: ev.decisions}, nil
}

// compileServices resolves each decision service's references into compiled
// decisions. References to unknown or non-executable decisions are reported as
// diagnostics; the service still compiles with the resolvable ones.
func compileServices(defs *Definitions, m *model.Definitions, items map[string]*feel.Type) Diagnostics {
	defs.servicesByID = make(map[string]*CompiledService, len(m.Services))
	defs.servicesByNam = make(map[string]*CompiledService, len(m.Services))

	var diags Diagnostics
	for _, ds := range m.Services {
		cs := &CompiledService{id: ds.ID, name: ds.Name, boundary: map[string]bool{}}
		if ds.VariableTypeRef != "" {
			cs.outType = resolveType(ds.VariableTypeRef, items)
		}
		for _, id := range ds.OutputDecisions {
			out, ok := defs.byID[id]
			if !ok || out.expr == nil {
				diags = append(diags, Diagnostic{
					Severity: SevError,
					Code:     CodeServiceOutputUnresolved,
					Message:  fmt.Sprintf("decision service %q references unknown or non-executable output decision %q", serviceLabel(ds), id),
				})
				continue
			}
			cs.outputs = append(cs.outputs, out)
		}
		for _, id := range ds.InputDecisions {
			if in, ok := defs.byID[id]; ok && in.name != "" {
				cs.boundary[in.name] = true
			}
		}

		defs.serviceOrder = append(defs.serviceOrder, cs)
		if cs.id != "" {
			defs.servicesByID[cs.id] = cs
		}
		if cs.name != "" {
			defs.servicesByNam[cs.name] = cs
		}
	}
	return diags
}

// registerServiceInvocables adds one callable FEEL function per decision service
// to funcs, so a decision expression can invoke it by name (DMN §10.4). The
// function's parameters are the service's input data followed by its input
// decisions (in declared order); calling it binds the arguments to those names,
// evaluates the output decision(s) with the service's inputs as boundaries, and
// returns the single output's value (or a context keyed by output name). The
// closure looks the compiled service up lazily because services are compiled
// after the decisions that may call them.
func registerServiceInvocables(m *model.Definitions, defs *Definitions, funcs map[string]*feel.Func, items map[string]*feel.Type, lim feel.Limits) {
	// A service-invocable's parameters bind under the FEEL identifiers (variable
	// name, else display name), matching how the encapsulated decisions reference
	// their inputs during evaluation.
	nameByID := make(map[string]string, len(m.InputData)+len(m.Decisions))
	typeByID := make(map[string]string, len(m.InputData)+len(m.Decisions))
	for _, in := range m.InputData {
		nameByID[in.ID] = in.RefName()
		typeByID[in.ID] = in.TypeRef
	}
	for _, d := range m.Decisions {
		nameByID[d.ID] = d.RefName()
		typeByID[d.ID] = d.VariableTypeRef
	}
	for _, ds := range m.Services {
		if ds.Name == "" {
			continue
		}
		var params []string
		var paramTypes []*feel.Type
		for _, id := range ds.InputData {
			params = append(params, nameByID[id])
			paramTypes = append(paramTypes, resolveType(typeByID[id], items))
		}
		for _, id := range ds.InputDecisions {
			params = append(params, nameByID[id])
			paramTypes = append(paramTypes, resolveType(typeByID[id], items))
		}
		// A single-output service coerces its result to the service's declared type
		// (or, absent one, the output decision's type) — e.g. a singleton list
		// unwraps, or a non-conforming value becomes null. Multiple outputs yield a
		// context, uncoerced.
		var resultType *feel.Type
		if len(ds.OutputDecisions) == 1 {
			if ds.VariableTypeRef != "" {
				resultType = resolveType(ds.VariableTypeRef, items)
			} else {
				resultType = resolveType(typeByID[ds.OutputDecisions[0]], items)
			}
		}
		name := ds.Name
		funcs[name] = &feel.Func{
			Name:       name,
			Params:     params,
			ParamTypes: paramTypes,
			ResultType: resultType,
			Native:     serviceInvoke(defs, name, params, lim),
		}
	}
}

// serviceInvoke builds the native body of a decision-service function value.
func serviceInvoke(defs *Definitions, name string, params []string, lim feel.Limits) func([]value.Value) (value.Value, error) {
	return func(args []value.Value) (value.Value, error) {
		cs, err := defs.Service(name)
		if err != nil {
			return value.Null, nil
		}
		base := make(map[string]value.Value, len(params))
		for i, p := range params {
			if i < len(args) && p != "" {
				base[p] = args[i]
			}
		}
		ev := newEvaluator(base, lim)
		ev.boundary = cs.boundary
		switch len(cs.outputs) {
		case 0:
			return value.Null, nil
		case 1:
			return ev.eval(cs.outputs[0])
		default:
			out := value.NewContext()
			for _, od := range cs.outputs {
				v, err := ev.eval(od)
				if err != nil {
					return nil, err
				}
				out.Put(od.name, v)
			}
			return out, nil
		}
	}
}

// serviceLabel is the human-facing identifier of a decision service.
func serviceLabel(ds *model.DecisionService) string {
	if ds.Name != "" {
		return ds.Name
	}
	return ds.ID
}
