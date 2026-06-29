package dmn

import (
	"fmt"

	"github.com/pblumer/temis/internal/model"
)

// wireRequirements resolves each decision's required-decision references into
// direct edges between the compiled decisions, then reports any dependency
// cycle as an error diagnostic (a cyclic decision cannot be evaluated). The
// edges drive the topological evaluation in Evaluate.
func wireRequirements(defs *Definitions, m *model.Definitions) Diagnostics {
	for i, dec := range m.Decisions {
		cd := defs.order[i]
		seen := make(map[*CompiledDecision]bool)
		for _, reqID := range dec.RequiredDecisions {
			req, ok := defs.byID[reqID]
			if !ok || req == cd || seen[req] {
				continue
			}
			seen[req] = true
			cd.requires = append(cd.requires, req)
		}
	}
	return detectCycles(defs)
}

// detectCycles finds dependency cycles via depth-first search, emitting one
// error diagnostic per decision that participates in a cycle.
func detectCycles(defs *Definitions) Diagnostics {
	const (
		white = 0 // unvisited
		gray  = 1 // on the current DFS stack
		black = 2 // fully explored
	)
	color := make(map[*CompiledDecision]int, len(defs.order))

	var diags Diagnostics
	var visit func(cd *CompiledDecision)
	visit = func(cd *CompiledDecision) {
		color[cd] = gray
		for _, req := range cd.requires {
			switch color[req] {
			case gray:
				diags = append(diags, Diagnostic{
					Severity:   SevError,
					Code:       CodeDecisionCycle,
					Message:    fmt.Sprintf("decision %q is part of a dependency cycle (via %q)", label(cd), label(req)),
					DecisionID: cd.id,
				})
			case white:
				visit(req)
			}
		}
		color[cd] = black
	}

	for _, cd := range defs.order {
		if color[cd] == white {
			visit(cd)
		}
	}
	return diags
}

// label is the human-facing identifier of a compiled decision.
func label(cd *CompiledDecision) string {
	if cd.name != "" {
		return cd.name
	}
	return cd.id
}
