package tck

import (
	"encoding/xml"
	"reflect"
	"testing"

	"github.com/pblumer/feel/value"
)

func TestScalarValue(t *testing.T) {
	cases := []struct {
		typ, text string
		want      string // FEEL String() of the result
	}{
		{"xsd:string", "hi", "hi"},
		{"xsd:boolean", "true", "true"},
		{"xsd:boolean", "false", "false"},
		{"xsd:decimal", "0.10", "0.1"},
		{"xsd:integer", "42", "42"},
		{"feel:number", "7", "7"},
		{"xsd:date", "2024-01-02", "2024-01-02"},
		{"xsd:dateTime", "2024-01-02T03:04:05", "2024-01-02T03:04:05"},
		{"", "plain", "plain"},
		// String whitespace is significant and preserved verbatim (TCK 1105:
		// upper case("xyZ ") → "XYZ "); numeric text is still trimmed.
		{"xsd:string", "XYZ ", "XYZ "},
		{"xsd:string", " leading", " leading"},
		{"xsd:decimal", "  42  ", "42"},
	}
	for _, c := range cases {
		got := scalarValue(c.typ, c.text)
		if got.String() != c.want {
			t.Errorf("scalarValue(%q,%q) = %s, want %s", c.typ, c.text, got, c.want)
		}
	}

	// Unparsable typed values and empty untyped values are null.
	if !value.IsNull(scalarValue("xsd:decimal", "not-a-number")) {
		t.Error("unparsable decimal should be null")
	}
	if !value.IsNull(scalarValue("", "")) {
		t.Error("empty untyped value should be null")
	}
}

func TestNormType(t *testing.T) {
	for in, want := range map[string]string{
		"xsd:string":             "string",
		"feel:boolean":           "boolean",
		"xsd:double":             "number",
		"xsd:long":               "number",
		"xsd:time":               "time",
		"dayTimeDuration":        "duration",
		"yearsAndMonthsDuration": "duration",
		"weird":                  "",
	} {
		if got := normType(in); got != want {
			t.Errorf("normType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGoOf(t *testing.T) {
	cases := []struct {
		v    value.Value
		want any
	}{
		{value.Null, nil},
		{value.BoolOf(true), true},
		{value.Str("x"), "x"},
		{value.NumberFromInt64(5), "5"},
		{value.NewDate(2024, 1, 2), "2024-01-02"},
		{value.NewList(value.NumberFromInt64(1), value.Str("a")), []any{"1", "a"}},
	}
	for _, c := range cases {
		if got := goOf(c.v); !reflect.DeepEqual(got, c.want) {
			t.Errorf("goOf(%s) = %#v, want %#v", c.v, got, c.want)
		}
	}

	ctx := value.NewContext()
	ctx.Put("a", value.NumberFromInt64(1))
	if got := goOf(ctx); !reflect.DeepEqual(got, map[string]any{"a": "1"}) {
		t.Errorf("goOf(context) = %#v", got)
	}
}

func TestToValueShapes(t *testing.T) {
	// A list value.
	list := valueContent{List: &tckList{Values: []tckValue{
		{Type: "xsd:decimal", Text: "1"}, {Type: "xsd:decimal", Text: "2"},
	}}}
	if got := list.toValue(); got.String() != "[1, 2]" {
		t.Errorf("list toValue = %s, want [1, 2]", got)
	}

	// A context via components.
	ctx := valueContent{Components: []tckComponent{
		{Name: "x", tckValue: tckValue{Type: "xsd:string", Text: "hi"}},
	}}
	if got := ctx.toValue(); got.String() != "{x: hi}" {
		t.Errorf("context toValue = %s, want {x: hi}", got)
	}

	// An explicit nil value.
	if got := (tckValue{Nil: "true"}).toValue(); !value.IsNull(got) {
		t.Errorf("nil value toValue = %s, want null", got)
	}

	// Empty content is null.
	if got := (valueContent{}).toValue(); !value.IsNull(got) {
		t.Errorf("empty content toValue = %s, want null", got)
	}
}

// TestItemWrappedListDecoding covers the TCK <list><item>… encoding (as opposed
// to direct <value> children), including nested lists and context items, which
// must decode to a populated list rather than collapsing to empty.
func TestItemWrappedListDecoding(t *testing.T) {
	const xmlSrc = `<expected xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	  <list>
	    <item><value xsi:type="xsd:string">a</value></item>
	    <item><value xsi:type="xsd:string">b</value></item>
	    <item>
	      <list>
	        <item><value xsi:type="xsd:decimal">1</value></item>
	        <item><value xsi:type="xsd:decimal">2</value></item>
	      </list>
	    </item>
	    <item><component name="k"><value xsi:type="xsd:string">v</value></component></item>
	  </list>
	</expected>`
	var vc valueContent
	if err := xml.Unmarshal([]byte(xmlSrc), &vc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := vc.toValue().String(); got != `[a, b, [1, 2], {k: v}]` {
		t.Errorf("item-wrapped list = %s, want [a, b, [1, 2], {k: v}]", got)
	}
}
