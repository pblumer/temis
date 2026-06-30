package xml

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// BuildDI synthesises a minimal DMNDI diagram — one DMNShape (with bounds) per
// shape — as a captured Raw token stream, so a model authored WITHOUT diagram
// interchange can have the modeler's layout persisted on save. Edge DI is omitted
// on purpose: the modeler routes requirement edges from the node bounds (see the
// note on DI). Returns nil when there are no shapes.
func BuildDI(shapes []DIShape) (*Raw, error) {
	if len(shapes) == 0 {
		return nil, nil
	}
	var sb strings.Builder
	sb.WriteString(`<dmndi:DMNDI xmlns:dmndi="https://www.omg.org/spec/DMN/20191111/DMNDI/"` +
		` xmlns:dc="http://www.omg.org/spec/DMN/20180521/DC/"` +
		` xmlns:di="http://www.omg.org/spec/DMN/20180521/DI/">` +
		`<dmndi:DMNDiagram id="DMNDiagram_temis">`)
	for i, s := range shapes {
		fmt.Fprintf(&sb, `<dmndi:DMNShape id="DMNShape_%d" dmnElementRef="%s"><dc:Bounds x="%s" y="%s" width="%s" height="%s"/></dmndi:DMNShape>`,
			i, escapeAttr(s.Ref), ftoa(s.X), ftoa(s.Y), ftoa(s.Width), ftoa(s.Height))
	}
	sb.WriteString(`</dmndi:DMNDiagram></dmndi:DMNDI>`)

	dec := xml.NewDecoder(strings.NewReader(sb.String()))
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		if se, ok := tok.(xml.StartElement); ok {
			var raw Raw
			if err := raw.UnmarshalXML(dec, se); err != nil {
				return nil, err
			}
			return &raw, nil
		}
	}
}

// ftoa formats a coordinate without scientific notation or a trailing ".0".
func ftoa(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// escapeAttr escapes the characters that must not appear raw in an XML attribute.
func escapeAttr(s string) string {
	return strings.NewReplacer(`&`, "&amp;", `<`, "&lt;", `>`, "&gt;", `"`, "&quot;").Replace(s)
}

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
