package dmn

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// Anchor names the model element whose boxed-expression logic an editor targets:
// a decision's own logic (Kind "decision") or a business knowledge model's
// encapsulated-logic body (Kind "bkm"). ID is the element's id or name. It is the
// generalisation of the decision-only routes (ADR-0016, WP-66) that lets the same
// per-kind editors edit a BKM's boxed body — the read-only wall in the simple BKM
// editor.
type Anchor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// exprKind names the boxed kind of a model expression, matching the {kind} path
// segment of the logic routes and the frontend editor registry. It returns "" for
// a nil expression.
func exprKind(e model.Expression) string {
	switch e.(type) {
	case *model.LiteralExpression:
		return "literal"
	case *model.DecisionTable:
		return "table"
	case *model.ContextExpr:
		return "context"
	case *model.Invocation:
		return "invocation"
	case *model.FunctionDef:
		return "function"
	case *model.ListExpr:
		return "list"
	case *model.RelationExpr:
		return "relation"
	case *model.Conditional:
		return "conditional"
	case *model.ForExpr, *model.Quantified:
		return "iterator"
	case *model.FilterExpr:
		return "filter"
	default:
		return ""
	}
}

// anchoredExpr resolves the boxed-expression logic of an anchor to its model form
// plus the element's id and name. ok is false when the anchor is unknown; expr is
// nil when the element exists but has no logic yet.
func (d *Definitions) anchoredExpr(a Anchor) (expr model.Expression, id, name string, ok bool) {
	switch a.Kind {
	case "decision":
		dec := d.decisionModel(a.ID)
		if dec == nil {
			return nil, "", "", false
		}
		return dec.Logic(), dec.ID, dec.Name, true
	case "bkm":
		for _, b := range d.model.BKMs {
			if b.ID == a.ID || b.Name == a.ID {
				if b.EncapsulatedLogic == nil {
					return nil, b.ID, b.Name, true
				}
				return b.EncapsulatedLogic.Body, b.ID, b.Name, true
			}
		}
		return nil, "", "", false
	default:
		return nil, "", "", false
	}
}

// LogicView returns the anchored element's boxed logic as the typed view for the
// requested kind (the same view shapes the decision routes return), or ok=false
// when the anchor is unknown or its logic is not of that kind. It is how the
// modeler reads a BKM's boxed body into the matching kind's editor (WP-66).
func (d *Definitions) LogicView(a Anchor, kind string) (any, bool) {
	expr, id, name, ok := d.anchoredExpr(a)
	if !ok || expr == nil {
		return nil, false
	}
	switch kind {
	case "context":
		c, ok := expr.(*model.ContextExpr)
		if !ok {
			return nil, false
		}
		return contextViewFromModel(id, name, c), true
	case "list":
		l, ok := expr.(*model.ListExpr)
		if !ok {
			return nil, false
		}
		return listViewFromModel(id, name, l), true
	case "relation":
		r, ok := expr.(*model.RelationExpr)
		if !ok {
			return nil, false
		}
		return relationViewFromModel(id, name, r), true
	case "invocation":
		iv, ok := expr.(*model.Invocation)
		if !ok {
			return nil, false
		}
		return invocationViewFromModel(id, name, iv), true
	case "conditional":
		c, ok := expr.(*model.Conditional)
		if !ok {
			return nil, false
		}
		return conditionalViewFromModel(id, name, c), true
	case "filter":
		f, ok := expr.(*model.FilterExpr)
		if !ok {
			return nil, false
		}
		return filterViewFromModel(id, name, f), true
	case "iterator":
		v, ok := iteratorViewFromModel(id, name, expr)
		if !ok {
			return nil, false
		}
		return v, true
	case "table":
		t, ok := expr.(*model.DecisionTable)
		if !ok {
			return nil, false
		}
		return tableViewFromModel(id, name, t), true
	default:
		return nil, false
	}
}

// SetLogic writes an edited boxed expression back to the anchored element and
// returns the recompiled XML. For a decision anchor it delegates to the existing
// per-kind setters (unchanged behaviour); for a BKM anchor it rewrites the
// encapsulated-logic body, preserving the function's formal parameters. raw is
// the kind's typed edit payload as JSON.
func SetLogic(src []byte, a Anchor, kind string, raw json.RawMessage) ([]byte, error) {
	switch a.Kind {
	case "decision":
		return setDecisionLogic(src, a.ID, kind, raw)
	case "bkm":
		return setBKMBodyLogic(src, a.ID, kind, raw)
	default:
		return nil, fmt.Errorf("dmn: unknown anchor kind %q", a.Kind)
	}
}

// setDecisionLogic decodes raw for the given kind and delegates to the decision's
// existing setter, so the anchored route matches the decision route exactly.
func setDecisionLogic(src []byte, decisionID, kind string, raw json.RawMessage) ([]byte, error) {
	switch kind {
	case "context":
		var e ContextEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedContext(src, decisionID, e)
	case "list":
		var e ListEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedList(src, decisionID, e)
	case "relation":
		var e RelationEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedRelation(src, decisionID, e)
	case "invocation":
		var e InvocationEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedInvocation(src, decisionID, e)
	case "conditional":
		var e ConditionalEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedConditional(src, decisionID, e)
	case "filter":
		var e FilterEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedFilter(src, decisionID, e)
	case "iterator":
		var e IteratorEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return SetBoxedIterator(src, decisionID, e)
	case "table":
		var e TableEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return ApplyTableEdit(src, decisionID, e)
	default:
		return nil, fmt.Errorf("dmn: unknown logic kind %q", kind)
	}
}

// setBKMBodyLogic rewrites a BKM's encapsulated-logic body to the edited boxed
// expression. The table kind patches the existing table (columns/rules); every
// other kind replaces the whole body expression via SetLogicBody, which refuses
// to switch the body to a different boxed kind.
func setBKMBodyLogic(src []byte, bkmID, kind string, raw json.RawMessage) ([]byte, error) {
	if kind == "table" {
		var e TableEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return nil, err
		}
		return applyTableEditAt(src, "bkm", bkmID, e)
	}
	expr, err := buildBodyExpr(kind, raw)
	if err != nil {
		return nil, err
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetLogicBody("bkm", bkmID, expr) {
		return nil, fmt.Errorf("dmn: cannot set a %s body for BKM %q (unknown or a different boxed kind)", kind, bkmID)
	}
	return dmnxml.Encode(def)
}

// buildBodyExpr validates a kind's typed edit and builds the corresponding XML
// boxed child, mirroring the decision setters' validation (the recompile that
// follows a save reports any remaining FEEL problem). Nested boxed values are not
// authored here — every leaf is a literal, matching the text editors (WP-66).
func buildBodyExpr(kind string, raw json.RawMessage) (dmnxml.Expression, error) {
	var zero dmnxml.Expression
	switch kind {
	case "context":
		var e ContextEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		var entries []dmnxml.ContextEntry
		for _, en := range e.Entries {
			name := strings.TrimSpace(en.Name)
			if name == "" {
				return zero, fmt.Errorf("dmn: a boxed-context entry must have a name")
			}
			text := strings.TrimSpace(en.Text)
			if text == "" {
				return zero, fmt.Errorf("dmn: boxed-context entry %q must have an expression", name)
			}
			entries = append(entries, dmnxml.ContextEntry{
				Variable:   &dmnxml.Variable{Name: name, TypeRef: strings.TrimSpace(en.TypeRef)},
				Expression: litXML(text, ""),
			})
		}
		if result := strings.TrimSpace(e.Result); result != "" {
			entries = append(entries, dmnxml.ContextEntry{Expression: litXML(result, strings.TrimSpace(e.ResultTypeRef))})
		}
		if len(entries) == 0 {
			return zero, fmt.Errorf("dmn: a boxed context needs at least one entry or a result")
		}
		return dmnxml.Expression{Context: &dmnxml.Context{Entries: entries}}, nil
	case "list":
		var e ListEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		var items []dmnxml.Expression
		for _, it := range e.Items {
			if s := strings.TrimSpace(it); s != "" {
				items = append(items, litXML(s, ""))
			}
		}
		if len(items) == 0 {
			return zero, fmt.Errorf("dmn: a list must have at least one item")
		}
		return dmnxml.Expression{List: &dmnxml.List{Items: items}}, nil
	case "relation":
		var e RelationEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		return relationExprXML(e)
	case "invocation":
		var e InvocationEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		return invocationExprXML(e)
	case "conditional":
		var e ConditionalEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		i, t, el := strings.TrimSpace(e.If), strings.TrimSpace(e.Then), strings.TrimSpace(e.Else)
		if i == "" || t == "" || el == "" {
			return zero, fmt.Errorf("dmn: every conditional branch (if/then/else) must be a non-empty expression")
		}
		return dmnxml.Expression{Conditional: &dmnxml.Conditional{If: childXML(i), Then: childXML(t), Else: childXML(el)}}, nil
	case "filter":
		var e FilterEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		in, match := strings.TrimSpace(e.In), strings.TrimSpace(e.Match)
		if in == "" || match == "" {
			return zero, fmt.Errorf("dmn: a filter needs a non-empty collection (in) and predicate (match)")
		}
		return dmnxml.Expression{Filter: &dmnxml.Filter{In: childXML(in), Match: childXML(match)}}, nil
	case "iterator":
		var e IteratorEdit
		if err := unmarshalEdit(raw, &e); err != nil {
			return zero, err
		}
		return iteratorExprXML(e)
	default:
		return zero, fmt.Errorf("dmn: unknown logic kind %q", kind)
	}
}

func relationExprXML(e RelationEdit) (dmnxml.Expression, error) {
	var zero dmnxml.Expression
	cols := make([]dmnxml.Column, 0, len(e.Columns))
	seen := map[string]bool{}
	for _, c := range e.Columns {
		name := strings.TrimSpace(c)
		if name == "" {
			return zero, fmt.Errorf("dmn: a relation column must have a name")
		}
		if seen[name] {
			return zero, fmt.Errorf("dmn: duplicate relation column %q", name)
		}
		seen[name] = true
		cols = append(cols, dmnxml.Column{Name: name})
	}
	if len(cols) == 0 {
		return zero, fmt.Errorf("dmn: a relation needs at least one column")
	}
	var rows []dmnxml.Row
	for i, r := range e.Rows {
		trimmed := make([]string, len(r))
		blank := true
		for j, cell := range r {
			trimmed[j] = strings.TrimSpace(cell)
			if trimmed[j] != "" {
				blank = false
			}
		}
		if blank {
			continue
		}
		if len(trimmed) != len(cols) {
			return zero, fmt.Errorf("dmn: row %d has %d cells, want %d", i+1, len(trimmed), len(cols))
		}
		cells := make([]dmnxml.Expression, len(trimmed))
		for j, cell := range trimmed {
			if cell == "" {
				return zero, fmt.Errorf("dmn: row %d, column %q is empty", i+1, cols[j].Name)
			}
			cells[j] = litXML(cell, "")
		}
		rows = append(rows, dmnxml.Row{Cells: cells})
	}
	return dmnxml.Expression{Relation: &dmnxml.Relation{Columns: cols, Rows: rows}}, nil
}

func invocationExprXML(e InvocationEdit) (dmnxml.Expression, error) {
	var zero dmnxml.Expression
	called := strings.TrimSpace(e.Called)
	if called == "" {
		return zero, fmt.Errorf("dmn: the invocation must name a function or BKM to call")
	}
	inv := &dmnxml.Invocation{Expression: litXML(called, "")}
	seen := map[string]bool{}
	for _, b := range e.Bindings {
		p, val := strings.TrimSpace(b.Parameter), strings.TrimSpace(b.Value)
		if p == "" && val == "" {
			continue
		}
		if p == "" {
			return zero, fmt.Errorf("dmn: a binding argument is set without a parameter name")
		}
		if val == "" {
			return zero, fmt.Errorf("dmn: parameter %q has no argument", p)
		}
		if seen[p] {
			return zero, fmt.Errorf("dmn: duplicate binding for parameter %q", p)
		}
		seen[p] = true
		inv.Bindings = append(inv.Bindings, dmnxml.Binding{Parameter: &dmnxml.Parameter{Name: p}, Expression: litXML(val, "")})
	}
	return dmnxml.Expression{Invocation: inv}, nil
}

func iteratorExprXML(e IteratorEdit) (dmnxml.Expression, error) {
	var zero dmnxml.Expression
	kind := strings.TrimSpace(e.Kind)
	if kind != "for" && kind != "some" && kind != "every" {
		return zero, fmt.Errorf("dmn: unknown iteration kind %q (want for, some or every)", e.Kind)
	}
	variable, inText, body := strings.TrimSpace(e.Variable), strings.TrimSpace(e.In), strings.TrimSpace(e.Body)
	if variable == "" || inText == "" || body == "" {
		return zero, fmt.Errorf("dmn: an iteration needs a variable, a collection (in) and a body")
	}
	it := &dmnxml.Iterator{IteratorVariable: variable, In: childXML(inText)}
	switch kind {
	case "for":
		it.Return = childXML(body)
		return dmnxml.Expression{For: it}, nil
	case "some":
		it.Satisfies = childXML(body)
		return dmnxml.Expression{Some: it}, nil
	default: // "every"
		it.Satisfies = childXML(body)
		return dmnxml.Expression{Every: it}, nil
	}
}

// litXML wraps FEEL text (with an optional result type) as an Expression holding a
// literal expression — the leaf every text editor authors.
func litXML(text, typeRef string) dmnxml.Expression {
	return dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: text, TypeRef: typeRef}}
}

// childXML wraps FEEL text as a boxed-expression child (a conditional/iterator/
// filter branch) holding a literal expression.
func childXML(text string) *dmnxml.ChildExpr {
	return &dmnxml.ChildExpr{Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: text}}}
}

// unmarshalEdit decodes a kind's typed edit payload from the raw request body.
func unmarshalEdit(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("dmn: invalid edit payload: %w", err)
	}
	return nil
}

// --- View builders shared by the decision routes and the anchored logic route.
// Each mirrors the corresponding Boxed<Type> reader so a BKM body reads exactly
// like a decision's logic; a parity test (logic_test.go) guards against drift.

func contextViewFromModel(id, name string, ctx *model.ContextExpr) ContextView {
	v := ContextView{DecisionID: id, Name: name, Simple: true}
	for _, e := range ctx.Entries {
		lit, ok := e.Value.(*model.LiteralExpression)
		if !ok {
			// A nested boxed entry this text view can't carry; keep a named
			// placeholder so the structure stays visible and mark it not simple.
			v.Simple = false
			if e.Name != "" {
				v.Entries = append(v.Entries, ContextEntryView{Name: e.Name})
			}
			continue
		}
		if e.Name == "" {
			v.Result = lit.Text
			v.ResultTypeRef = canonicalType(lit.TypeRef)
			continue
		}
		v.Entries = append(v.Entries, ContextEntryView{Name: e.Name, Text: lit.Text, TypeRef: canonicalType(e.TypeRef)})
	}
	return v
}

func listViewFromModel(id, name string, l *model.ListExpr) ListView {
	v := ListView{DecisionID: id, Name: name, Simple: true}
	for _, it := range l.Items {
		if lit, ok := it.(*model.LiteralExpression); ok {
			v.Items = append(v.Items, lit.Text)
		} else {
			v.Items = append(v.Items, "")
			v.Simple = false
		}
	}
	return v
}

func relationViewFromModel(id, name string, rel *model.RelationExpr) RelationView {
	v := RelationView{DecisionID: id, Name: name, Columns: append([]string(nil), rel.Columns...), Simple: true}
	for _, row := range rel.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			if lit, ok := c.(*model.LiteralExpression); ok {
				cells = append(cells, lit.Text)
			} else {
				cells = append(cells, "")
				v.Simple = false
			}
		}
		v.Rows = append(v.Rows, cells)
	}
	return v
}

func invocationViewFromModel(id, name string, inv *model.Invocation) InvocationView {
	v := InvocationView{DecisionID: id, Name: name, Simple: true}
	if lit, ok := inv.Called.(*model.LiteralExpression); ok {
		v.Called = lit.Text
	} else if inv.Called != nil {
		v.Simple = false
	}
	for _, b := range inv.Bindings {
		bv := InvocationBindingView{Parameter: b.Parameter}
		if lit, ok := b.Value.(*model.LiteralExpression); ok {
			bv.Value = lit.Text
		} else if b.Value != nil {
			v.Simple = false
		}
		v.Bindings = append(v.Bindings, bv)
	}
	return v
}

func conditionalViewFromModel(id, name string, c *model.Conditional) ConditionalView {
	v := ConditionalView{DecisionID: id, Name: name, Simple: true}
	v.If = branchText(c.If, &v.Simple)
	v.Then = branchText(c.Then, &v.Simple)
	v.Else = branchText(c.Else, &v.Simple)
	return v
}

func filterViewFromModel(id, name string, f *model.FilterExpr) FilterView {
	v := FilterView{DecisionID: id, Name: name, Simple: true}
	v.In = branchText(f.In, &v.Simple)
	v.Match = branchText(f.Match, &v.Simple)
	return v
}

func iteratorViewFromModel(id, name string, expr model.Expression) (IteratorView, bool) {
	v := IteratorView{DecisionID: id, Name: name, Simple: true}
	switch it := expr.(type) {
	case *model.ForExpr:
		v.Kind, v.Variable = "for", it.IteratorVariable
		v.In = branchText(it.In, &v.Simple)
		v.Body = branchText(it.Return, &v.Simple)
	case *model.Quantified:
		v.Kind, v.Variable = it.Kind, it.IteratorVariable
		v.In = branchText(it.In, &v.Simple)
		v.Body = branchText(it.Satisfies, &v.Simple)
	default:
		return IteratorView{}, false
	}
	return v, true
}

func tableViewFromModel(id, name string, dt *model.DecisionTable) TableView {
	v := TableView{
		DecisionID:  id,
		Name:        name,
		HitPolicy:   string(dt.HitPolicy),
		Aggregation: string(dt.Aggregation),
	}
	for _, in := range dt.Inputs {
		v.Inputs = append(v.Inputs, TableInput{Label: in.Label, Expression: in.Expression, TypeRef: canonicalType(in.TypeRef)})
	}
	for _, out := range dt.Outputs {
		v.Outputs = append(v.Outputs, TableOutput{Name: out.Name, Label: out.Label, TypeRef: canonicalType(out.TypeRef)})
	}
	for _, r := range dt.Rules {
		v.Rules = append(v.Rules, TableRule{InputEntries: r.InputEntries, OutputEntries: r.OutputEntries, Annotations: r.Annotations})
	}
	return v
}
