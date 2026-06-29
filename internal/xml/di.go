package xml

import (
	"encoding/xml"
	"strconv"
)

// DIShape is a DMNDI shape's bounds, keyed to a DMN element via dmnElementRef.
type DIShape struct {
	Ref                 string
	X, Y, Width, Height float64
}

// DI is the parsed subset of the DMNDI diagram interchange the modeler needs:
// element bounds. Edge waypoints are intentionally not extracted — the modeler
// routes requirement edges from the node bounds itself.
type DI struct {
	Shapes []DIShape
}

// ParseDI extracts DMNShape bounds from a captured DMNDI token stream (the
// verbatim Raw subtree, ADR-0010). It matches local element/attribute names, so
// it reads DMN 1.3/1.4/1.5 DI regardless of namespace prefix. Returns nil when r
// is nil or carries no usable shapes.
func ParseDI(r *Raw) *DI {
	if r == nil {
		return nil
	}
	di := &DI{}
	var cur *DIShape // non-nil while inside a <DMNShape>
	for _, tok := range r.Tokens {
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "DMNShape":
				cur = &DIShape{Ref: attrLocal(t, "dmnElementRef")}
			case "Bounds":
				if cur != nil {
					cur.X = atof(attrLocal(t, "x"))
					cur.Y = atof(attrLocal(t, "y"))
					cur.Width = atof(attrLocal(t, "width"))
					cur.Height = atof(attrLocal(t, "height"))
				}
			}
		case xml.EndElement:
			if t.Name.Local == "DMNShape" && cur != nil {
				if cur.Ref != "" && cur.Width > 0 && cur.Height > 0 {
					di.Shapes = append(di.Shapes, *cur)
				}
				cur = nil
			}
		}
	}
	if len(di.Shapes) == 0 {
		return nil
	}
	return di
}

func attrLocal(t xml.StartElement, local string) string {
	for _, a := range t.Attr {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}

func atof(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
