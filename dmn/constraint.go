package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/feel"
	"github.com/pblumer/feel/value"
	"github.com/pblumer/temis/internal/model"
)

// unaryEnv compiles and evaluates allowed-values matchers, which are FEEL unary
// tests over the implicit input variable "?".
var unaryEnv = feel.NewEnv(feel.InputVar)

// inputConstraint is the resolved validation constraint on one input: its
// structural type (for kind checks of custom struct/list types) and an optional
// allowed-values matcher compiled from the type's or the input clause's
// allowedValues (WP-31).
type inputConstraint struct {
	typ         *feel.Type
	allowedText string
	matcher     feel.CompiledExpr // unary test over "?"; nil = no allowed-values constraint
}

// check returns an InputProblem when v violates the constraint, or nil when it
// conforms (or the constraint is empty). A null value is treated as absent and
// is handled by the missing-input check, not here.
func (c *inputConstraint) check(name string, v value.Value) *InputProblem {
	if value.IsNull(v) {
		return nil
	}
	if c.typ != nil && !structurallyConforms(c.typ, v) {
		return &InputProblem{
			Input:    name,
			Code:     "TYPE_MISMATCH",
			Expected: c.typ.String(),
			Got:      v.Kind().String(),
			Message:  fmt.Sprintf("input %q expects %s, got %s", name, c.typ.String(), v.Kind()),
		}
	}
	if c.matcher != nil {
		ok, err := feel.Matches(c.matcher, unaryEnv.NewScope(map[string]value.Value{feel.InputVar: v}))
		if err == nil && !ok {
			return &InputProblem{
				Input:    name,
				Code:     "VALUE_NOT_ALLOWED",
				Expected: c.allowedText,
				Got:      v.String(),
				Message:  fmt.Sprintf("input %q value %s is not among the allowed values %s", name, v.String(), c.allowedText),
			}
		}
	}
	return nil
}

// structurallyConforms reports whether v's kind matches the declared type's kind
// for the structured types (context, list). Scalar conformance is left to the
// existing canonical-type check; an Any type imposes nothing.
func structurallyConforms(t *feel.Type, v value.Value) bool {
	switch t.Kind {
	case value.KindContext:
		return v.Kind() == value.KindContext
	case value.KindList:
		return v.Kind() == value.KindList
	default:
		return true
	}
}

// buildConstraints resolves each required input's structural type and
// allowed-values matcher. Allowed values come from the input clause's own
// allowedValues (the dmn-js inline style) when present, otherwise from the
// item-definition the input's type names. A matcher that fails to compile is
// dropped (no constraint) rather than failing the whole compile.
func buildConstraints(m *model.Definitions, dec *model.Decision, items map[string]*feel.Type) map[string]*inputConstraint {
	itemAllowed := allowedValuesByType(m)

	typeByExpr, allowedByExpr := map[string]string{}, map[string]string{}
	if dec.DecisionTable != nil {
		for _, in := range dec.DecisionTable.Inputs {
			expr := strings.TrimSpace(in.Expression)
			if in.TypeRef != "" {
				typeByExpr[expr] = in.TypeRef
			}
			if in.AllowedValues != "" {
				allowedByExpr[expr] = in.AllowedValues
			}
		}
	}

	inputByID := make(map[string]*model.InputData, len(m.InputData))
	for _, in := range m.InputData {
		inputByID[in.ID] = in
	}

	out := map[string]*inputConstraint{}
	for _, id := range dec.RequiredInputs {
		in, ok := inputByID[id]
		if !ok || in.Name == "" {
			continue
		}
		ref := in.TypeRef
		if ref == "" {
			ref = typeByExpr[in.Name]
		}
		allowed := allowedByExpr[in.Name]
		if allowed == "" {
			allowed = itemAllowed[strings.TrimSpace(ref)]
		}

		c := &inputConstraint{typ: resolveType(ref, items), allowedText: allowed}
		if allowed != "" {
			if matcher, err := feel.CompileUnaryTest(allowed, unaryEnv); err == nil {
				c.matcher = matcher
			} else {
				c.allowedText = "" // unparsable constraint: drop it
			}
		}
		if c.typ != nil || c.matcher != nil {
			out[in.Name] = c
		}
	}
	return out
}

// allowedValuesByType maps each named item definition to its allowed-values
// text, so a typed input inherits its type's constraint.
func allowedValuesByType(m *model.Definitions) map[string]string {
	out := make(map[string]string, len(m.ItemDefinitions))
	for _, it := range m.ItemDefinitions {
		if it.Name != "" && it.AllowedValues != "" {
			out[it.Name] = it.AllowedValues
		}
	}
	return out
}
