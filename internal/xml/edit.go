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
