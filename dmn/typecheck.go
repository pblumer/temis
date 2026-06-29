package dmn

import (
	"strings"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
)

// typecheckModel statically type-checks every decision's FEEL expressions
// against the types declared on input data, decision variables and item
// definitions, returning advisory warning diagnostics (with position) for the
// provable mismatches it finds (WP-30). It checks literal expressions and
// decision-table input expressions and output cells — the places typed FEEL text
// appears; other boxed forms are left to evaluation's null semantics. The
// checker is conservative: where a type is unknown it infers Any and stays
// silent, so a well-typed model produces no findings.
func typecheckModel(m *model.Definitions, funcs map[string]*feel.Func) Diagnostics {
	items := buildItemTypes(m)
	var diags Diagnostics
	for _, dec := range m.Decisions {
		env := buildTypeEnv(m, dec, items)
		diags = append(diags, typecheckDecision(dec, env, funcs)...)
	}
	return diags
}

// typecheckDecision checks one decision's typed FEEL text.
func typecheckDecision(dec *model.Decision, env *feel.TypeEnv, funcs map[string]*feel.Func) Diagnostics {
	var diags Diagnostics
	report := func(src string) {
		for _, te := range feel.TypecheckString(src, env, funcs) {
			diags = append(diags, Diagnostic{
				Severity:   SevWarning,
				Code:       CodeTypeError,
				Message:    typeWarnMessage(dec, te),
				DecisionID: dec.ID,
				Line:       te.Line,
				Col:        te.Col,
			})
		}
	}

	switch {
	case dec.LiteralExpression != nil:
		report(dec.LiteralExpression.Text)
	case dec.DecisionTable != nil:
		dt := dec.DecisionTable
		for _, in := range dt.Inputs {
			report(in.Expression)
		}
		for _, r := range dt.Rules {
			for _, cell := range r.OutputEntries {
				if strings.TrimSpace(cell) != "" {
					report(cell)
				}
			}
		}
	}
	return diags
}

func typeWarnMessage(dec *model.Decision, te feel.TypeError) string {
	return "decision " + quote(decisionLabel(dec)) + ": " + te.Msg
}

func quote(s string) string { return "\"" + s + "\"" }

// buildTypeEnv maps a decision's required inputs and required decisions to their
// declared FEEL types, so the checker can reason about the names the
// expressions reference. Input types come from the input-data typeRef, falling
// back to the decision-table input clause whose expression is exactly that
// input's name (the dmn-js style where types live on the table).
func buildTypeEnv(m *model.Definitions, dec *model.Decision, items map[string]*feel.Type) *feel.TypeEnv {
	env := feel.NewTypeEnv()

	typeByExpr := map[string]string{}
	if dec.DecisionTable != nil {
		for _, in := range dec.DecisionTable.Inputs {
			if in.TypeRef != "" {
				typeByExpr[strings.TrimSpace(in.Expression)] = in.TypeRef
			}
		}
	}

	inputByID := make(map[string]*model.InputData, len(m.InputData))
	for _, in := range m.InputData {
		inputByID[in.ID] = in
	}
	decByID := make(map[string]*model.Decision, len(m.Decisions))
	for _, d := range m.Decisions {
		decByID[d.ID] = d
	}

	for _, id := range dec.RequiredInputs {
		in, ok := inputByID[id]
		if !ok || in.Name == "" {
			continue
		}
		ref := in.TypeRef
		if ref == "" {
			ref = typeByExpr[in.Name]
		}
		env.Set(in.Name, resolveType(ref, items))
	}
	for _, id := range dec.RequiredDecisions {
		d, ok := decByID[id]
		if !ok || d.Name == "" {
			continue
		}
		env.Set(d.Name, resolveType(d.VariableTypeRef, items))
	}
	return env
}

// buildItemTypes resolves each named item definition into a FEEL type. Component
// definitions become context field types; a type reference resolves to a
// built-in or another item definition; isCollection wraps the result in a list.
// A reference that cannot be resolved (forward/cyclic) yields Any, keeping the
// checker conservative.
func buildItemTypes(m *model.Definitions) map[string]*feel.Type {
	types := make(map[string]*feel.Type, len(m.ItemDefinitions))
	var resolve func(it *model.ItemDefinition, seen map[string]bool) *feel.Type
	resolve = func(it *model.ItemDefinition, seen map[string]bool) *feel.Type {
		var base *feel.Type
		switch {
		case len(it.Components) > 0:
			fields := make(map[string]*feel.Type, len(it.Components))
			for _, c := range it.Components {
				if c.Name != "" {
					fields[c.Name] = resolve(c, seen)
				}
			}
			base = feel.ContextOf(fields)
		default:
			base = resolveTypeRefSeen(it.TypeRef, m, types, seen)
		}
		if it.IsCollection {
			base = feel.ListOf(base)
		}
		return base
	}
	for _, it := range m.ItemDefinitions {
		if it.Name != "" {
			types[it.Name] = resolve(it, map[string]bool{it.Name: true})
		}
	}
	return types
}

// resolveType maps a type reference to a FEEL type using the built-ins and the
// resolved item-definition types; an unknown reference is Any (nil).
func resolveType(ref string, items map[string]*feel.Type) *feel.Type {
	if ref == "" {
		return nil
	}
	if t, ok := feel.BuiltinType(ref); ok {
		return t
	}
	return items[strings.TrimSpace(ref)]
}

// resolveTypeRefSeen resolves a type reference during item-definition building,
// resolving references to other (not-yet-built) item definitions on demand while
// guarding against cycles.
func resolveTypeRefSeen(ref string, m *model.Definitions, types map[string]*feel.Type, seen map[string]bool) *feel.Type {
	if ref == "" {
		return nil
	}
	if t, ok := feel.BuiltinType(ref); ok {
		return t
	}
	name := strings.TrimSpace(ref)
	if t, ok := types[name]; ok {
		return t
	}
	if seen[name] {
		return nil // cycle
	}
	for _, it := range m.ItemDefinitions {
		if it.Name == name {
			seen[name] = true
			s2 := make(map[string]bool, len(seen))
			for k, v := range seen {
				s2[k] = v
			}
			var inner *feel.Type
			if len(it.Components) > 0 {
				fields := make(map[string]*feel.Type, len(it.Components))
				for _, c := range it.Components {
					if c.Name != "" {
						fields[c.Name] = resolveTypeRefSeen(c.TypeRef, m, types, s2)
					}
				}
				inner = feel.ContextOf(fields)
			} else {
				inner = resolveTypeRefSeen(it.TypeRef, m, types, s2)
			}
			if it.IsCollection {
				inner = feel.ListOf(inner)
			}
			return inner
		}
	}
	return nil
}
