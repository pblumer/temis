package xml

import (
	"encoding/xml"
	"strconv"
	"strings"
)

// SetElementName sets the name attribute of the inputData, decision or business
// knowledge model identified by id. References to the element elsewhere in the
// document are left untouched — keeping a downstream FEEL reference valid is the
// author's concern, not this patch's. It reports whether a matching element was
// found.
func (d *Definitions) SetElementName(id, name string) bool {
	for i := range d.InputData {
		if d.InputData[i].ID == id {
			d.InputData[i].Name = name
			return true
		}
	}
	for i := range d.Decisions {
		if d.Decisions[i].ID == id {
			d.Decisions[i].Name = name
			return true
		}
	}
	for i := range d.BKMs {
		if d.BKMs[i].ID == id {
			d.BKMs[i].Name = name
			return true
		}
	}
	return false
}

// SetInputType sets the declared FEEL type (the <variable> typeRef) of the
// inputData identified by id, creating the <variable> element if absent. An empty
// typeRef clears it. It reports whether a matching inputData was found.
func (d *Definitions) SetInputType(id, typeRef string) bool {
	for i := range d.InputData {
		if d.InputData[i].ID == id {
			in := &d.InputData[i]
			typeRef = strings.TrimSpace(typeRef)
			if in.Variable == nil {
				if typeRef == "" {
					return true // nothing to clear, no variable to create
				}
				in.Variable = &Variable{Name: in.Name}
			}
			in.Variable.TypeRef = typeRef
			return true
		}
	}
	return false
}

// UpdateDecisionTable rewrites the decision-table logic of the decision
// identified by id. Rules are always replaced. A non-empty hitPolicy sets the
// policy and aggregation; columns are replaced only when replaceColumns is set
// (otherwise the existing inputs/outputs are kept). It reports whether a matching
// decision with decision-table logic was found.
func (d *Definitions) UpdateDecisionTable(id, hitPolicy, aggregation string, inputs []Input, outputs []Output, rules []Rule, replaceColumns bool) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID == id {
			dt := d.Decisions[i].DecisionTable
			if dt == nil {
				return false
			}
			if hitPolicy != "" {
				dt.HitPolicy = hitPolicy
				dt.Aggregation = aggregation
			}
			if replaceColumns {
				dt.Inputs = inputs
				dt.Outputs = outputs
			}
			dt.Rules = rules
			return true
		}
	}
	return false
}

// UpsertItemDefinition creates or updates a (simple) item definition by name: its
// base type, collection flag and allowed-values constraint. It refuses (returns
// false) for an empty name or when an existing definition of that name is
// structured (has item components), so the simple editor never clobbers a
// structured type.
func (d *Definitions) UpsertItemDefinition(name, typeRef string, isCollection bool, allowedValues string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for i := range d.ItemDefs {
		if d.ItemDefs[i].Name == name {
			if len(d.ItemDefs[i].Components) > 0 {
				return false
			}
			d.ItemDefs[i].TypeRef = strings.TrimSpace(typeRef)
			d.ItemDefs[i].IsCollection = isCollection
			d.ItemDefs[i].AllowedValues = textOrNil(allowedValues)
			return true
		}
	}
	d.ItemDefs = append(d.ItemDefs, ItemDef{Name: name, TypeRef: strings.TrimSpace(typeRef), IsCollection: isCollection, AllowedValues: textOrNil(allowedValues)})
	return true
}

// RemoveItemDefinition removes the item definition with the given name, reporting
// whether one was found. References to it elsewhere are left untouched.
func (d *Definitions) RemoveItemDefinition(name string) bool {
	for i := range d.ItemDefs {
		if d.ItemDefs[i].Name == name {
			d.ItemDefs = append(d.ItemDefs[:i], d.ItemDefs[i+1:]...)
			return true
		}
	}
	return false
}

func textOrNil(s string) *Text {
	if s = strings.TrimSpace(s); s == "" {
		return nil
	}
	return &Text{Value: s}
}

// SetLiteralExpression sets (or creates) the literal-expression logic of the
// decision identified by id, with the given FEEL text and result type. It refuses
// (returns false) when the decision is unknown or already carries a different
// boxed logic (decision table, context, …), which would conflict.
func (d *Definitions) SetLiteralExpression(id, text, typeRef string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.LiteralExpression == nil {
			return false // some other boxed logic is present
		}
		if dec.LiteralExpression == nil {
			dec.LiteralExpression = &LiteralExpression{}
		}
		dec.LiteralExpression.Text = text
		dec.LiteralExpression.TypeRef = typeRef
		return true
	}
	return false
}

// SetBoxedContext sets (or replaces) the boxed-context logic of the decision
// identified by id with the given entries. It refuses (returns false) when the
// decision is unknown or already carries a different boxed logic (decision
// table, literal expression, …), which would conflict.
func (d *Definitions) SetBoxedContext(id string, entries []ContextEntry) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.Context == nil {
			return false // some other boxed logic is present
		}
		dec.Context = &Context{Entries: entries}
		return true
	}
	return false
}

// CreateBoxedContext gives an undecided decision a fresh boxed context with a
// single named entry, ready to edit in the modeler. It refuses (returns false)
// when the decision is unknown or already has logic.
func (d *Definitions) CreateBoxedContext(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.Context = &Context{Entries: []ContextEntry{{
		Variable:   &Variable{Name: "Eintrag 1"},
		Expression: Expression{LiteralExpression: &LiteralExpression{Text: "0"}},
	}}}
	return true
}

// litChild wraps a FEEL text as a boxed-expression child holding a literal
// expression — the branch of a conditional (its if/then/else) as the simple
// editor authors it.
func litChild(text string) *ChildExpr {
	return &ChildExpr{Expression: Expression{LiteralExpression: &LiteralExpression{Text: text}}}
}

// SetConditional sets (or replaces) the boxed-conditional logic of the decision
// identified by id with literal if/then/else branches. It refuses (returns false)
// when the decision is unknown or already carries a different boxed logic (a
// table, a context, …), which would conflict.
func (d *Definitions) SetConditional(id, ifText, thenText, elseText string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.Conditional == nil {
			return false // some other boxed logic is present
		}
		dec.Conditional = &Conditional{If: litChild(ifText), Then: litChild(thenText), Else: litChild(elseText)}
		return true
	}
	return false
}

// CreateConditional gives an undecided decision a fresh boxed conditional with
// placeholder branches, ready to edit in the modeler. It refuses (returns false)
// when the decision is unknown or already has logic.
func (d *Definitions) CreateConditional(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.Conditional = &Conditional{If: litChild("true"), Then: litChild("0"), Else: litChild("0")}
	return true
}

// SetIterator sets (or replaces) the boxed-iteration logic of the decision
// identified by id. kind is "for" (which carries a return branch, yielding a
// list), or "some"/"every" (which carry a satisfies branch, yielding a boolean).
// variable is the iterator variable, inText the collection and bodyText the
// return/satisfies expression, all as literals. It refuses (returns false) for an
// unknown kind, an unknown decision, or a decision that already carries a
// different (non-iterator) boxed logic.
func (d *Definitions) SetIterator(id, kind, variable, inText, bodyText string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		hasIter := dec.For != nil || dec.Every != nil || dec.Some != nil
		if dec.present() && !hasIter {
			return false // some other boxed logic is present
		}
		it := &Iterator{IteratorVariable: variable, In: litChild(inText)}
		switch kind {
		case "for":
			it.Return = litChild(bodyText)
		case "some", "every":
			it.Satisfies = litChild(bodyText)
		default:
			return false
		}
		// Replace whichever iterator kind was there before with the chosen one.
		dec.For, dec.Every, dec.Some = nil, nil, nil
		switch kind {
		case "for":
			dec.For = it
		case "every":
			dec.Every = it
		case "some":
			dec.Some = it
		}
		return true
	}
	return false
}

// CreateIterator gives an undecided decision a fresh boxed iteration (a `for`
// with placeholder branches), ready to edit in the modeler. It refuses (returns
// false) when the decision is unknown or already has logic.
func (d *Definitions) CreateIterator(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.For = &Iterator{IteratorVariable: "x", In: litChild("[1, 2, 3]"), Return: litChild("x * 2")}
	return true
}

// SetFilter sets (or replaces) the boxed-filter logic of the decision identified
// by id with literal `in` (collection) and `match` (predicate) branches. It
// refuses (returns false) when the decision is unknown or already carries a
// different boxed logic (a table, a list, …), which would conflict.
func (d *Definitions) SetFilter(id, inText, matchText string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.Filter == nil {
			return false // some other boxed logic is present
		}
		dec.Filter = &Filter{In: litChild(inText), Match: litChild(matchText)}
		return true
	}
	return false
}

// CreateFilter gives an undecided decision a fresh boxed filter with placeholder
// branches, ready to edit in the modeler. It refuses (returns false) when the
// decision is unknown or already has logic.
func (d *Definitions) CreateFilter(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.Filter = &Filter{In: litChild("[1, 2, 3]"), Match: litChild("item > 1")}
	return true
}

// SetList sets (or replaces) the boxed-list logic of the decision identified by
// id with the given literal FEEL items (in order). It refuses (returns false)
// when the decision is unknown or already carries a different boxed logic (a
// table, a context, …), which would conflict.
func (d *Definitions) SetList(id string, items []string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.List == nil {
			return false // some other boxed logic is present
		}
		lst := &List{Items: make([]Expression, 0, len(items))}
		for _, it := range items {
			lst.Items = append(lst.Items, Expression{LiteralExpression: &LiteralExpression{Text: it}})
		}
		dec.List = lst
		return true
	}
	return false
}

// CreateList gives an undecided decision a fresh boxed list with a single
// placeholder item, ready to edit in the modeler. It refuses (returns false) when
// the decision is unknown or already has logic.
func (d *Definitions) CreateList(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.List = &List{Items: []Expression{{LiteralExpression: &LiteralExpression{Text: "0"}}}}
	return true
}

// SetRelation sets (or replaces) the boxed-relation logic of the decision
// identified by id with the given column names and rows of literal FEEL cells (in
// order). It refuses (returns false) when the decision is unknown or already
// carries a different boxed logic (a table, a list, …), which would conflict.
func (d *Definitions) SetRelation(id string, columns []string, rows [][]string) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID != id {
			continue
		}
		dec := &d.Decisions[i]
		if dec.present() && dec.Relation == nil {
			return false // some other boxed logic is present
		}
		rel := &Relation{Columns: make([]Column, 0, len(columns))}
		for _, c := range columns {
			rel.Columns = append(rel.Columns, Column{Name: c})
		}
		for _, r := range rows {
			row := Row{Cells: make([]Expression, 0, len(r))}
			for _, cell := range r {
				row.Cells = append(row.Cells, Expression{LiteralExpression: &LiteralExpression{Text: cell}})
			}
			rel.Rows = append(rel.Rows, row)
		}
		dec.Relation = rel
		return true
	}
	return false
}

// CreateRelation gives an undecided decision a fresh boxed relation with a single
// column and one placeholder cell, ready to edit in the modeler. It refuses
// (returns false) when the decision is unknown or already has logic.
func (d *Definitions) CreateRelation(id string) bool {
	i := indexDecision(d.Decisions, id)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}
	dec.Relation = &Relation{
		Columns: []Column{{Name: "Spalte 1"}},
		Rows:    []Row{{Cells: []Expression{{LiteralExpression: &LiteralExpression{Text: "0"}}}}},
	}
	return true
}

// SetBKMFunction sets the encapsulated logic of the business knowledge model
// identified by id to a function with the given formal parameters and a literal
// FEEL body. It refuses (returns false) when the BKM is unknown or its current
// body is a non-literal boxed expression, which the simple editor must not
// overwrite.
func (d *Definitions) SetBKMFunction(id string, params []FormalParameter, bodyText, bodyTypeRef string) bool {
	for i := range d.BKMs {
		if d.BKMs[i].ID != id {
			continue
		}
		b := &d.BKMs[i]
		if b.EncapsulatedLogic != nil && b.EncapsulatedLogic.present() && b.EncapsulatedLogic.LiteralExpression == nil {
			return false
		}
		b.EncapsulatedLogic = &FunctionDefinition{
			Kind:       "FEEL",
			Parameters: params,
			Expression: Expression{LiteralExpression: &LiteralExpression{Text: bodyText, TypeRef: bodyTypeRef}},
		}
		return true
	}
	return false
}

// MoveShape repositions the DMNShape bound to element id within a captured DMNDI
// token stream, rewriting its <Bounds> x/y attributes in place (width and height
// are preserved). It is the inverse of ParseDI: it matches local element and
// attribute names, so it patches DMN 1.3/1.4/1.5 DI regardless of namespace
// prefix, and it touches only the shape's own Bounds, never a nested DMNLabel's.
// A nil stream, or a model without a DMNShape for id, yields false.
func MoveShape(r *Raw, id string, x, y float64) bool {
	if r == nil {
		return false
	}
	inShape := false
	for i, tok := range r.Tokens {
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "DMNShape":
				inShape = attrLocal(t, "dmnElementRef") == id
			case "Bounds":
				if inShape {
					r.Tokens[i] = setBoundsXY(t, x, y)
					return true
				}
			}
		case xml.EndElement:
			if t.Name.Local == "DMNShape" {
				inShape = false
			}
		}
	}
	return false
}

// setBoundsXY returns a copy of a <Bounds> start element with its x and y
// attributes set to the given coordinates; all other attributes are preserved.
func setBoundsXY(se xml.StartElement, x, y float64) xml.StartElement {
	out := xml.StartElement{Name: se.Name, Attr: make([]xml.Attr, len(se.Attr))}
	copy(out.Attr, se.Attr)
	for i := range out.Attr {
		switch out.Attr[i].Name.Local {
		case "x":
			out.Attr[i].Value = formatCoord(x)
		case "y":
			out.Attr[i].Value = formatCoord(y)
		}
	}
	return out
}

// formatCoord renders a coordinate as a plain decimal without an exponent,
// dropping a redundant fractional part (e.g. 180 not 180.0).
func formatCoord(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
