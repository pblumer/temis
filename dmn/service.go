package dmn

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/internal/model"
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

	ev := newEvaluator(base)
	ev.boundary = s.boundary

	outputs := make(map[string]any, len(s.outputs))
	for _, out := range s.outputs {
		v, err := ev.eval(out)
		if err != nil {
			return Result{}, err
		}
		outputs[out.name] = fromValue(v)
	}
	return Result{Outputs: outputs, Decisions: ev.decisions}, nil
}

// compileServices resolves each decision service's references into compiled
// decisions. References to unknown or non-executable decisions are reported as
// diagnostics; the service still compiles with the resolvable ones.
func compileServices(defs *Definitions, m *model.Definitions) Diagnostics {
	defs.servicesByID = make(map[string]*CompiledService, len(m.Services))
	defs.servicesByNam = make(map[string]*CompiledService, len(m.Services))

	var diags Diagnostics
	for _, ds := range m.Services {
		cs := &CompiledService{id: ds.ID, name: ds.Name, boundary: map[string]bool{}}
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

// serviceLabel is the human-facing identifier of a decision service.
func serviceLabel(ds *model.DecisionService) string {
	if ds.Name != "" {
		return ds.Name
	}
	return ds.ID
}
