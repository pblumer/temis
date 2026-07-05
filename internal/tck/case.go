package tck

import (
	"encoding/xml"
	"strings"

	"github.com/pblumer/temis/internal/value"
)

// testCases mirrors the root of a DMN TCK test-definition document (the standard
// `testCases` XML). Tags use local names only, so the testcase namespace and the
// xsi attributes decode regardless of their declared prefixes.
type testCases struct {
	XMLName   xml.Name   `xml:"testCases"`
	ModelName string     `xml:"modelName"`
	Cases     []testCase `xml:"testCase"`
}

type testCase struct {
	ID      string       `xml:"id,attr"`
	Type    string       `xml:"type,attr"`
	Invoked string       `xml:"invocableName,attr"`
	Inputs  []inputNode  `xml:"inputNode"`
	Results []resultNode `xml:"resultNode"`
}

// inputNode is a supplied input value, given directly as a value/list/components.
type inputNode struct {
	Name string `xml:"name,attr"`
	valueContent
}

// resultNode is an expected decision (or expression) result.
type resultNode struct {
	Name     string       `xml:"name,attr"`
	Expected expectedNode `xml:"expected"`
}

type expectedNode struct {
	valueContent
}

// valueContent is the value carried by a node: a scalar/structured <value>, a
// <list>, or a set of <component>s (a context). At most one applies.
type valueContent struct {
	Value      *tckValue      `xml:"value"`
	List       *tckList       `xml:"list"`
	Components []tckComponent `xml:"component"`
}

// toValue converts a node's content to a FEEL value (null when empty).
func (c valueContent) toValue() value.Value {
	switch {
	case c.Value != nil:
		return c.Value.toValue()
	case c.List != nil:
		return c.List.toValue()
	case len(c.Components) > 0:
		return componentsToContext(c.Components)
	default:
		return value.Null
	}
}

// tckValue is a <value>: a scalar (xsi:type + character data, or xsi:nil) or a
// nested structured value (list / components).
type tckValue struct {
	Type string `xml:"type,attr"`
	Nil  string `xml:"nil,attr"`
	Text string `xml:",chardata"`
	valueContent
}

func (v tckValue) toValue() value.Value {
	if strings.EqualFold(strings.TrimSpace(v.Nil), "true") {
		return value.Null
	}
	if v.List != nil {
		return v.List.toValue()
	}
	if len(v.Components) > 0 {
		return componentsToContext(v.Components)
	}
	if v.Value != nil {
		return v.Value.toValue()
	}
	return scalarValue(v.Type, strings.TrimSpace(v.Text))
}

// tckList decodes a TCK <list>. The corpus uses two element encodings: direct
// <value> children, and <item> wrappers (each holding a value, a nested list or
// context components). Both are accepted so nested and context-valued lists decode
// correctly rather than collapsing to empty.
type tckList struct {
	Values []tckValue     `xml:"value"`
	Items  []valueContent `xml:"item"`
}

func (l *tckList) toValue() value.Value {
	elems := make([]value.Value, 0, len(l.Values)+len(l.Items))
	for i := range l.Values {
		elems = append(elems, l.Values[i].toValue())
	}
	for i := range l.Items {
		elems = append(elems, l.Items[i].toValue())
	}
	return value.NewList(elems...)
}

type tckComponent struct {
	Name string `xml:"name,attr"`
	tckValue
}

func componentsToContext(comps []tckComponent) value.Value {
	ctx := value.NewContext()
	for _, c := range comps {
		ctx.Put(c.Name, c.toValue())
	}
	return ctx
}

// scalarValue converts a scalar TCK value (its xsi:type and text) to a FEEL
// value. An empty, untyped value is null; an unparsable typed value is null too,
// so a malformed expectation simply fails to match rather than panicking.
func scalarValue(typ, text string) value.Value {
	switch normType(typ) {
	case "string":
		return value.Str(text)
	case "boolean":
		return value.BoolOf(text == "true" || text == "1")
	case "number":
		if n, err := value.ParseNumber(text); err == nil {
			return n
		}
		return value.Null
	case "date":
		if d, err := value.ParseDate(text); err == nil {
			return d
		}
		return value.Null
	case "time":
		if t, err := value.ParseTime(text); err == nil {
			return t
		}
		return value.Null
	case "datetime":
		if dt, err := value.ParseDateTime(text); err == nil {
			return dt
		}
		return value.Null
	case "duration":
		if dur, err := value.ParseDuration(text); err == nil {
			return dur
		}
		return value.Null
	default:
		if text == "" {
			return value.Null
		}
		return value.Str(text)
	}
}

// normType normalises an xsi:type / feel type name to a coarse kind: the
// namespace prefix is dropped and numeric XSD types collapse to "number".
func normType(typ string) string {
	t := strings.TrimSpace(typ)
	if i := strings.IndexByte(t, ':'); i >= 0 {
		t = t[i+1:]
	}
	switch strings.ToLower(t) {
	case "string":
		return "string"
	case "boolean":
		return "boolean"
	case "decimal", "double", "float", "integer", "int", "long", "short", "number":
		return "number"
	case "date":
		return "date"
	case "time":
		return "time"
	case "datetime", "dateandtime":
		return "datetime"
	case "duration", "daytimeduration", "yearmonthduration", "dayspriortimeduration",
		"daysandtimeduration", "yearsandmonthsduration":
		return "duration"
	default:
		return ""
	}
}
