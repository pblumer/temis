package dmn

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
)

// InputField describes one input a decision expects: its name, its declared FEEL
// type (empty when the model declares none) and whether the decision requires
// it. It is the self-description an agent reads before calling Evaluate
// (ADR-0013, WP-52).
type InputField struct {
	Name     string `json:"name"`
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required"`
	// Constraint is the input's allowed-values text (a FEEL unary-test list, e.g.
	// `"red","green","blue"` or `[1..10]`), empty when unconstrained. It lets an
	// agent see the permitted values before calling Evaluate (WP-31).
	Constraint string `json:"constraint,omitempty"`
	// Values lists the discrete values this input may take, for a picker. They
	// come from the input's declared allowed-values enumeration and/or the literal
	// values used in decision-table cells. Empty when the domain is open or
	// continuous (e.g. a numeric range). A consumer can offer these to send a
	// correct input without guessing.
	Values []string `json:"values,omitempty"`
	// ValuesClosed is true when Values is the exhaustive allowed set (a declared
	// enumeration) — a caller may offer a closed dropdown and reject other input;
	// false when Values are merely suggestions inferred from table cells and other
	// values are still accepted.
	ValuesClosed bool `json:"valuesClosed,omitempty"`
}

// InputProblem is a single, machine-readable input-validation failure. Code is
// one of "TYPE_MISMATCH", "UNKNOWN_INPUT", "MISSING_INPUT" or, for a value
// outside its type's allowed values, "VALUE_NOT_ALLOWED" (WP-31).
type InputProblem struct {
	Input    string `json:"input"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Got      string `json:"got,omitempty"`
}

// InputError is returned by Evaluate under WithStrictInput when the supplied
// input does not satisfy the decision's declared schema. It carries every
// problem found, so a caller (notably an agent) gets the full picture in one go
// rather than one error at a time.
type InputError struct {
	Problems []InputProblem
}

func (e *InputError) Error() string {
	if len(e.Problems) == 1 {
		return "dmn: invalid input: " + e.Problems[0].Message
	}
	msgs := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		msgs[i] = p.Message
	}
	return fmt.Sprintf("dmn: %d input problems: %s", len(e.Problems), strings.Join(msgs, "; "))
}

// InputSchema returns the inputs the decision expects, with their declared types.
func (c *CompiledDecision) InputSchema() []InputField { return c.inputs }

// ValidateInput checks in against the decision's declared schema and returns
// every problem found (an empty slice means the input is valid). It reports
// inputs of the wrong type, inputs the decision does not declare, and missing
// required inputs — turning what would otherwise be a silently wrong result into
// an explicit, actionable list. It never evaluates the decision.
func (c *CompiledDecision) ValidateInput(in Input) []InputProblem {
	return validateInputAgainst(in, c.inputs, c.constraints, fmt.Sprintf("decision %q", c.name))
}

// validateInputAgainst is the shared core of input validation: it checks in
// against the given declared fields and resolved constraints and returns every
// problem found. subject names the schema's owner for the UNKNOWN_INPUT message
// (e.g. `decision "Routing"` or `this model`), so the same logic backs both
// per-decision (ValidateInput) and whole-graph (ValidateModelInput) validation.
func validateInputAgainst(in Input, fields []InputField, constraints map[string]*inputConstraint, subject string) []InputProblem {
	schema := make(map[string]InputField, len(fields))
	for _, f := range fields {
		schema[f.Name] = f
	}

	var probs []InputProblem
	for name, v := range in {
		f, ok := schema[name]
		if !ok {
			probs = append(probs, InputProblem{
				Input:   name,
				Code:    "UNKNOWN_INPUT",
				Message: fmt.Sprintf("input %q is not declared by %s; expected one of %s", name, subject, quoteNames(fields)),
			})
			continue
		}
		if got, want, bad := typeMismatch(f.Type, v); bad {
			probs = append(probs, InputProblem{
				Input:    name,
				Code:     "TYPE_MISMATCH",
				Expected: want,
				Got:      got,
				Message:  fmt.Sprintf("input %q expects %s, got %s", name, want, got),
			})
			continue
		}
		// Structural (custom struct/list) and allowed-values constraints (WP-31).
		if c := constraints[name]; c != nil {
			if fv, err := toValue(v); err == nil {
				if p := c.check(name, fv); p != nil {
					probs = append(probs, *p)
				}
			}
		}
	}
	for _, f := range fields {
		if !f.Required {
			continue
		}
		if v, ok := in[f.Name]; !ok || v == nil {
			probs = append(probs, InputProblem{
				Input:   f.Name,
				Code:    "MISSING_INPUT",
				Message: fmt.Sprintf("required input %q is missing", f.Name),
			})
		}
	}
	return probs
}

// ModelInputSchema returns the input data the whole model consumes — the union of
// every decision's declared input fields, deduped by name. It is the schema for a
// graph-wide evaluation (EvaluateGraph): the leaf inputs a caller fills once to
// drive every decision, including those reached only transitively through other
// decisions (e.g. an input a downstream decision never names directly). A field's
// type/constraint is taken from the first decision that declares it with one, and
// it is required when any decision requires it.
func (d *Definitions) ModelInputSchema() []InputField {
	return unionInputs(d.order)
}

// ReachableInputSchema returns the leaf inputs needed to evaluate the decision
// idOrName: its directly declared inputs plus those reached transitively through
// required decisions — the union over its requirements cone, deduped by name
// exactly as ModelInputSchema but scoped to that one decision (ADR-0026, L2a).
// ModelInputSchema is the union over the whole model and would allow inputs of
// other, unrelated decisions; ReachableInputSchema is the minimal correct
// superset for driving one composed decision — e.g. in a flow, precisely the
// leaf inputs a step targeting the decision may wire. A field's type/constraint
// comes from the first declaring decision in the cone, and it is required when
// any decision in the cone requires it. It errs if no such decision exists.
func (d *Definitions) ReachableInputSchema(idOrName string) ([]InputField, error) {
	cone, err := d.requirementsCone(idOrName)
	if err != nil {
		return nil, err
	}
	return unionInputs(cone), nil
}

// ValidateModelInput checks in against the model's whole-graph input schema
// (ModelInputSchema) and returns every problem found — the model-level
// counterpart of CompiledDecision.ValidateInput, used by EvaluateGraph's strict
// mode so a transitively-reached input is accepted (it feeds some decision) while
// a genuinely unknown one is still reported.
func (d *Definitions) ValidateModelInput(in Input) []InputProblem {
	return validateInputAgainst(in, d.ModelInputSchema(), d.mergedConstraints(), "this model")
}

// ValidateReachableInput checks in against the decision's reachable input schema
// (ReachableInputSchema) and returns every problem found — the cone-scoped
// counterpart of ValidateModelInput. It lets a caller (notably a flow step)
// validate an input that includes transitively-reached leaf inputs while still
// catching genuine unknowns, type mismatches and missing required inputs. It errs
// if no such decision exists.
func (d *Definitions) ValidateReachableInput(idOrName string, in Input) ([]InputProblem, error) {
	cone, err := d.requirementsCone(idOrName)
	if err != nil {
		return nil, err
	}
	subject := fmt.Sprintf("decision %q's reachable inputs", idOrName)
	return validateInputAgainst(in, unionInputs(cone), coneConstraints(cone), subject), nil
}

// requirementsCone returns the decisions in the requirements cone of idOrName —
// the decision itself plus every decision reachable through its requiredDecision
// edges — in the model's declaration order (d.order), so any union built over it
// is deterministic (no map-iteration order, ADR-0007/0023).
func (d *Definitions) requirementsCone(idOrName string) ([]*CompiledDecision, error) {
	root, ok := d.byID[idOrName]
	if !ok {
		root, ok = d.byName[idOrName]
	}
	if !ok {
		return nil, fmt.Errorf("dmn: no decision %q", idOrName)
	}
	inCone := map[*CompiledDecision]bool{}
	var visit func(cd *CompiledDecision)
	visit = func(cd *CompiledDecision) {
		if inCone[cd] {
			return
		}
		inCone[cd] = true
		for _, req := range cd.requires {
			visit(req)
		}
	}
	visit(root)
	var cone []*CompiledDecision
	for _, cd := range d.order {
		if inCone[cd] {
			cone = append(cone, cd)
		}
	}
	return cone, nil
}

// unionInputs merges the declared input fields of decs into one schema, deduped
// by name: a field's type/constraint comes from the first decision that declares
// it with one, and it is required when any decision requires it. It backs both
// ModelInputSchema (the whole model) and ReachableInputSchema (one decision's
// cone). Order follows decs, then first-seen input order — deterministic.
func unionInputs(decs []*CompiledDecision) []InputField {
	var fields []InputField
	idx := map[string]int{}
	for _, cd := range decs {
		for _, f := range cd.inputs {
			if i, ok := idx[f.Name]; ok {
				mergeInputField(&fields[i], f)
				continue
			}
			idx[f.Name] = len(fields)
			fields = append(fields, f)
		}
	}
	return fields
}

// mergeInputField folds f into an already-recorded field of the same name: the
// first declared type/constraint wins, required is OR'd, and Values union unless
// a closed declared enumeration already fixed them.
func mergeInputField(ex *InputField, f InputField) {
	if ex.Type == "" {
		ex.Type = f.Type
	}
	if ex.Constraint == "" {
		ex.Constraint = f.Constraint
	}
	ex.Required = ex.Required || f.Required
	// Values: a closed declared enumeration wins; otherwise union the
	// suggestions from every decision that consumes this input.
	if !ex.ValuesClosed {
		if f.ValuesClosed {
			ex.Values, ex.ValuesClosed = f.Values, true
		} else {
			ex.Values = mergeDistinct(ex.Values, f.Values)
		}
	}
}

// mergedConstraints unions every decision's resolved input constraints, keyed by
// input name (first decision to declare a constraint wins).
func (d *Definitions) mergedConstraints() map[string]*inputConstraint {
	return coneConstraints(d.order)
}

// coneConstraints unions the resolved input constraints of decs, keyed by input
// name (first decision to declare a constraint wins). It backs both the
// whole-model (mergedConstraints) and cone-scoped (ValidateReachableInput) paths.
func coneConstraints(decs []*CompiledDecision) map[string]*inputConstraint {
	out := map[string]*inputConstraint{}
	for _, cd := range decs {
		for name, c := range cd.constraints {
			if _, ok := out[name]; !ok {
				out[name] = c
			}
		}
	}
	return out
}

// InputSchema returns the declared input schema of a decision by id or name.
func (d *Definitions) InputSchema(idOrName string) ([]InputField, error) {
	cd, ok := d.byID[idOrName]
	if !ok {
		cd, ok = d.byName[idOrName]
	}
	if !ok {
		return nil, fmt.Errorf("dmn: no decision %q", idOrName)
	}
	return cd.inputs, nil
}

// buildInputSchema resolves a decision's required inputs into typed fields. A
// type is taken from the input-data's own typeRef when present, otherwise from
// the decision table's input clause whose expression is exactly that input's
// name (the common dmn-js authoring style, where types live on the table). A
// user-defined item-definition type is reported by its name; a resolved
// allowed-values constraint is surfaced on the field (WP-31).
func buildInputSchema(m *model.Definitions, dec *model.Decision, items map[string]*feel.Type, constraints map[string]*inputConstraint) []InputField {
	typeByExpr := map[string]string{}
	if dec.DecisionTable != nil {
		for _, in := range dec.DecisionTable.Inputs {
			if in.TypeRef != "" {
				typeByExpr[strings.TrimSpace(in.Expression)] = in.TypeRef
			}
		}
	}

	byID := make(map[string]*model.InputData, len(m.InputData))
	for _, idata := range m.InputData {
		byID[idata.ID] = idata
	}

	// Distinct literal values used in each input column's cells, keyed by the
	// column expression — the values a picker can suggest for that input.
	cellLits := map[string][]string{}
	if dec.DecisionTable != nil {
		for ci, in := range dec.DecisionTable.Inputs {
			expr := strings.TrimSpace(in.Expression)
			seenLit := map[string]bool{}
			var lits []string
			for _, r := range dec.DecisionTable.Rules {
				if ci >= len(r.InputEntries) {
					continue
				}
				vs, _ := literalsIn(r.InputEntries[ci])
				for _, v := range vs {
					if !seenLit[v] {
						seenLit[v] = true
						lits = append(lits, v)
					}
				}
			}
			if len(lits) > 0 {
				cellLits[expr] = lits
			}
		}
	}

	var fields []InputField
	seen := map[string]bool{}
	for _, id := range dec.RequiredInputs {
		idata, ok := byID[id]
		// Key the field by the input's FEEL identifier (variable name, else display
		// name): that is what the decision-table input expressions reference and
		// what the caller supplies the value under, so schema, constraints and
		// evaluation stay aligned even when the display name is a free-form label.
		ref := ""
		if ok {
			ref = idata.RefName()
		}
		if !ok || ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		typ := idata.TypeRef
		if typ == "" {
			typ = typeByExpr[ref]
		}
		f := InputField{Name: ref, Type: schemaTypeName(typ, items), Required: true}
		if c := constraints[ref]; c != nil {
			f.Constraint = c.allowedText
		}
		f.Values, f.ValuesClosed = suggestValues(f.Constraint, cellLits[ref])
		fields = append(fields, f)
	}
	return fields
}

var numberLiteral = regexp.MustCompile(`^-?\d+(\.\d+)?$`)

// suggestValues picks the discrete values to offer for an input: a declared
// enumeration (every part a literal) is the closed allowed set; otherwise the
// literal values used in the table cells are suggestions (plus any literal parts
// of a non-enumerable constraint), and other values are still accepted.
func suggestValues(constraint string, cellLits []string) (values []string, closed bool) {
	if constraint != "" {
		if vs, complete := literalsIn(constraint); complete {
			return vs, true
		} else if len(vs) > 0 {
			return mergeDistinct(vs, cellLits), false
		}
	}
	if len(cellLits) > 0 {
		return cellLits, false
	}
	return nil, false
}

// literalsIn extracts the discrete literal values from a FEEL unary-test list:
// each top-level, comma-separated part that is a plain string or number literal.
// Non-literal parts (ranges, comparisons, "-", expressions) are skipped. It also
// reports whether EVERY part was such a literal — i.e. a closed enumeration.
func literalsIn(text string) (values []string, complete bool) {
	parts := splitTopLevel(text)
	if len(parts) == 0 {
		return nil, false
	}
	complete = true
	for _, p := range parts {
		if v, ok := literalValue(p); ok {
			values = append(values, v)
		} else {
			complete = false
		}
	}
	return values, complete && len(values) > 0
}

// literalValue returns the value of a FEEL string or number literal (a string
// unquoted), or ok=false for anything else (a range, comparison, "-", …).
func literalValue(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		inner := s[1 : len(s)-1]
		if !strings.Contains(inner, `"`) {
			return inner, true
		}
		return "", false
	}
	if numberLiteral.MatchString(s) {
		return s, true
	}
	return "", false
}

// splitTopLevel splits on commas that are not inside a string or brackets, so a
// list like `"a","b",[1..5]` yields three parts.
func splitTopLevel(s string) []string {
	var parts []string
	depth, start := 0, 0
	inStr := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			inStr = !inStr
		case inStr:
			// inside a string literal: ignore structural characters
		case c == '[' || c == '(':
			depth++
		case (c == ']' || c == ')') && depth > 0:
			depth--
		case c == ',' && depth == 0:
			parts = append(parts, s[start:i])
			start = i + 1
		}
	}
	return append(parts, s[start:])
}

// mergeDistinct appends b's items to a, skipping ones already present.
func mergeDistinct(a, b []string) []string {
	seen := make(map[string]bool, len(a))
	for _, v := range a {
		seen[v] = true
	}
	for _, v := range b {
		if !seen[v] {
			seen[v] = true
			a = append(a, v)
		}
	}
	return a
}

// schemaTypeName is the type name reported in the self-description: the canonical
// FEEL name for a built-in, the item-definition's own name for a user-defined
// type, or "" when neither resolves.
func schemaTypeName(ref string, items map[string]*feel.Type) string {
	if c := canonicalType(ref); c != "" {
		return c
	}
	name := strings.TrimSpace(ref)
	if i := strings.LastIndexByte(name, ':'); i >= 0 {
		name = name[i+1:]
	}
	if items[name] != nil {
		return name
	}
	return ""
}

// canonicalType maps a DMN typeRef (optionally namespace-prefixed) to the
// canonical FEEL type name used for validation and self-description. An
// unrecognised (e.g. custom item-definition) type returns "", meaning "no type
// constraint" until the type system lands (WP-31).
func canonicalType(t string) string {
	t = strings.TrimSpace(t)
	if i := strings.LastIndexByte(t, ':'); i >= 0 {
		t = t[i+1:]
	}
	switch strings.ToLower(strings.ReplaceAll(t, " ", "")) {
	case "number":
		return "number"
	case "string":
		return "string"
	case "boolean":
		return "boolean"
	case "date":
		return "date"
	case "time":
		return "time"
	case "datetime", "dateandtime":
		return "date and time"
	case "duration", "daytimeduration", "yearmonthduration", "daysandtimeduration", "yearsandmonthsduration":
		return "duration"
	default:
		return ""
	}
}

// typeMismatch reports whether v violates the expected canonical FEEL type. It
// returns the value's observed type, the expected type and whether they clash.
// A null value never clashes (absence is handled as MISSING_INPUT), and an
// expected type of "" (undeclared/custom) imposes no constraint. Temporal types
// accept their canonical string form (the form Evaluate parses).
func typeMismatch(expected string, v any) (got, want string, bad bool) {
	if expected == "" {
		return "", "", false
	}
	got = goKind(v)
	if got == "null" {
		return "", "", false
	}
	ok := false
	switch expected {
	case "number", "string", "boolean":
		ok = got == expected
	case "date", "time", "duration":
		ok = got == "string"
	case "date and time":
		ok = got == "string" || got == "date and time"
	default:
		ok = true
	}
	if ok {
		return "", "", false
	}
	return got, expected, true
}

// goKind names the FEEL kind a Go input value maps to (see Evaluate's mapping).
func goKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return "number"
	case time.Time:
		return "date and time"
	case []any:
		return "list"
	case map[string]any:
		return "context"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func quoteNames(fields []InputField) string {
	if len(fields) == 0 {
		return "(none)"
	}
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = fmt.Sprintf("%q", f.Name)
	}
	return strings.Join(names, ", ")
}
