package xml

import "encoding/xml"

// List mirrors a boxed <list>: an ordered sequence of element expressions. Its
// children are heterogeneous boxed expressions whose order is significant, so it
// decodes them itself rather than relying on per-type struct fields (which would
// group by type and lose interleaving order).
type List struct {
	ID    string
	Items []Expression
}

// Relation mirrors a boxed <relation>: named columns and rows whose cells line
// up with the columns positionally.
type Relation struct {
	ID      string   `xml:"id,attr,omitempty"`
	Columns []Column `xml:"column"`
	Rows    []Row    `xml:"row"`
}

// Column mirrors a relation <column> heading.
type Column struct {
	Name string `xml:"name,attr,omitempty"`
}

// Row mirrors a relation <row>: an ordered sequence of cell expressions, one per
// column. Like List it decodes its heterogeneous children in order.
type Row struct {
	Cells []Expression
}

// Conditional mirrors a boxed <conditional> (DMN 1.4+): an if/then/else over
// nested expressions.
type Conditional struct {
	ID   string     `xml:"id,attr,omitempty"`
	If   *ChildExpr `xml:"if"`
	Then *ChildExpr `xml:"then"`
	Else *ChildExpr `xml:"else"`
}

// Iterator mirrors a boxed <for>, <every> or <some> (DMN 1.4+): a named iterator
// variable ranging over the In collection, producing Return (for) or testing
// Satisfies (every/some).
type Iterator struct {
	ID               string     `xml:"id,attr,omitempty"`
	IteratorVariable string     `xml:"iteratorVariable,attr,omitempty"`
	In               *ChildExpr `xml:"in"`
	Return           *ChildExpr `xml:"return"`
	Satisfies        *ChildExpr `xml:"satisfies"`
}

// Filter mirrors a boxed <filter> (DMN 1.4+): the In collection filtered by the
// Match predicate.
type Filter struct {
	ID    string     `xml:"id,attr,omitempty"`
	In    *ChildExpr `xml:"in"`
	Match *ChildExpr `xml:"match"`
}

// ChildExpr wraps a single nested expression that DMN places inside a named
// holder element (<if>, <then>, <in>, <return>, <satisfies>, <match>).
type ChildExpr struct {
	Expression
}

// UnmarshalXML decodes a <list>, capturing its element expressions in document
// order.
func (l *List) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, a := range start.Attr {
		if a.Name.Local == "id" {
			l.ID = a.Value
		}
	}
	return decodeExprSeq(d, start, &l.Items)
}

// UnmarshalXML decodes a relation <row>, capturing its cell expressions in order.
func (r *Row) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	return decodeExprSeq(d, start, &r.Cells)
}

// decodeExprSeq reads child elements of start until its matching end tag,
// decoding each recognised boxed-expression element into out in order and
// skipping anything else.
func decodeExprSeq(d *xml.Decoder, start xml.StartElement, out *[]Expression) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			var e Expression
			ok, err := decodeExprElement(d, t, &e)
			if err != nil {
				return err
			}
			if ok {
				*out = append(*out, e)
			} else if err := d.Skip(); err != nil {
				return err
			}
		case xml.EndElement:
			if t.Name == start.Name {
				return nil
			}
		}
	}
}

// decodeExprElement decodes one boxed-expression start element into e, reporting
// whether the element name was a recognised expression. Unrecognised elements
// are left for the caller to skip.
func decodeExprElement(d *xml.Decoder, start xml.StartElement, e *Expression) (bool, error) {
	switch start.Name.Local {
	case "literalExpression":
		e.LiteralExpression = &LiteralExpression{}
		return true, d.DecodeElement(e.LiteralExpression, &start)
	case "decisionTable":
		e.DecisionTable = &DecisionTable{}
		return true, d.DecodeElement(e.DecisionTable, &start)
	case "context":
		e.Context = &Context{}
		return true, d.DecodeElement(e.Context, &start)
	case "invocation":
		e.Invocation = &Invocation{}
		return true, d.DecodeElement(e.Invocation, &start)
	case "functionDefinition":
		e.FunctionDefinition = &FunctionDefinition{}
		return true, d.DecodeElement(e.FunctionDefinition, &start)
	case "list":
		e.List = &List{}
		return true, d.DecodeElement(e.List, &start)
	case "relation":
		e.Relation = &Relation{}
		return true, d.DecodeElement(e.Relation, &start)
	case "conditional":
		e.Conditional = &Conditional{}
		return true, d.DecodeElement(e.Conditional, &start)
	case "for":
		e.For = &Iterator{}
		return true, d.DecodeElement(e.For, &start)
	case "every":
		e.Every = &Iterator{}
		return true, d.DecodeElement(e.Every, &start)
	case "some":
		e.Some = &Iterator{}
		return true, d.DecodeElement(e.Some, &start)
	case "filter":
		e.Filter = &Filter{}
		return true, d.DecodeElement(e.Filter, &start)
	default:
		return false, nil
	}
}
