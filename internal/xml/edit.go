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

// SetDecisionTableRules replaces the rule rows of the decision-table logic of the
// decision identified by id. The table's columns (inputs/outputs) and hit policy
// are left untouched — only the rows change. It reports whether a matching
// decision with decision-table logic was found.
func (d *Definitions) SetDecisionTableRules(id string, rules []Rule) bool {
	for i := range d.Decisions {
		if d.Decisions[i].ID == id {
			dt := d.Decisions[i].DecisionTable
			if dt == nil {
				return false
			}
			dt.Rules = rules
			return true
		}
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
