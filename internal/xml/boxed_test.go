package xml

import (
	"encoding/xml"
	"strings"
	"testing"
)

// allExprList holds one of every boxed-expression kind, to exercise every
// branch of encodeExprElement and decodeExprElement.
func allExprItems() []Expression {
	return []Expression{
		{LiteralExpression: &LiteralExpression{Text: "1"}},
		{DecisionTable: &DecisionTable{HitPolicy: "UNIQUE"}},
		{Context: &Context{Entries: []ContextEntry{{Variable: &Variable{Name: "e"}, Expression: Expression{LiteralExpression: &LiteralExpression{Text: "0"}}}}}},
		{Invocation: &Invocation{}},
		{FunctionDefinition: &FunctionDefinition{Kind: "FEEL"}},
		{List: &List{Items: []Expression{{LiteralExpression: &LiteralExpression{Text: "nested"}}}}},
		{Relation: &Relation{Columns: []Column{{Name: "c"}}}},
		{Conditional: &Conditional{If: &ChildExpr{Expression{LiteralExpression: &LiteralExpression{Text: "c"}}}}},
		{For: &Iterator{IteratorVariable: "x"}},
		{Every: &Iterator{IteratorVariable: "x"}},
		{Some: &Iterator{IteratorVariable: "x"}},
		{Filter: &Filter{}},
		{}, // empty slot: encodeExprElement default branch, emits nothing
	}
}

// TestListRoundTripAllExprKinds encodes a List containing every boxed-expression
// kind and decodes it back, asserting each kind survives. This drives every case
// of encodeExprElement / decodeExprElement.
func TestListRoundTripAllExprKinds(t *testing.T) {
	l := List{ID: "L1", Items: allExprItems()}
	out, err := xml.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(out)
	for _, want := range []string{
		"<list", `id="L1"`, "<literalExpression>", "<decisionTable", "<context>",
		"<invocation", "<functionDefinition", "<relation", "<conditional",
		"<for", "<every", "<some", "<filter",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("encoded list missing %q:\n%s", want, s)
		}
	}

	var got List
	if err := xml.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "L1" {
		t.Errorf("ID = %q, want L1", got.ID)
	}
	// The empty slot encodes nothing, so it does not re-decode: expect 12 items.
	if len(got.Items) != 12 {
		t.Fatalf("decoded %d items, want 12: %+v", len(got.Items), got.Items)
	}
	present := func(e Expression) bool { return e.present() }
	for i, it := range got.Items {
		if !present(it) {
			t.Errorf("item %d is empty", i)
		}
	}
	// Spot-check the per-kind decode landed on the right field.
	if got.Items[0].LiteralExpression == nil || got.Items[0].LiteralExpression.Text != "1" {
		t.Errorf("item 0 literal = %+v", got.Items[0].LiteralExpression)
	}
	if got.Items[1].DecisionTable == nil {
		t.Error("item 1 not a decisionTable")
	}
	if got.Items[11].Filter == nil {
		t.Error("item 11 not a filter")
	}
}

// TestDecodeExprSeqSkipsUnknown verifies decodeExprSeq skips unrecognised
// child elements rather than failing.
func TestDecodeExprSeqSkipsUnknown(t *testing.T) {
	const src = `<list id="L">` +
		`<somethingUnknown><deep>x</deep></somethingUnknown>` +
		`<literalExpression><text>kept</text></literalExpression>` +
		`</list>`
	var l List
	if err := xml.Unmarshal([]byte(src), &l); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(l.Items) != 1 || l.Items[0].LiteralExpression == nil {
		t.Fatalf("unknown not skipped, items = %+v", l.Items)
	}
	if l.Items[0].LiteralExpression.Text != "kept" {
		t.Errorf("text = %q, want kept", l.Items[0].LiteralExpression.Text)
	}
}

// TestDecodeExprSeqTruncated drives the d.Token() error return of decodeExprSeq:
// the <list> is opened but never closed, so the decoder hits EOF mid-sequence.
func TestDecodeExprSeqTruncated(t *testing.T) {
	dec := xml.NewDecoder(strings.NewReader(`<list><literalExpression><text>1</text></literalExpression>`))
	// advance to the <list> start element
	var start xml.StartElement
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("priming token: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			start = se
			break
		}
	}
	var items []Expression
	if err := decodeExprSeq(dec, start, &items); err == nil {
		t.Error("decodeExprSeq(truncated) = nil error, want error")
	}
}

// TestRowRoundTrip exercises Row.UnmarshalXML / MarshalXML directly.
func TestRowRoundTrip(t *testing.T) {
	r := Row{Cells: []Expression{
		{LiteralExpression: &LiteralExpression{Text: "a"}},
		{LiteralExpression: &LiteralExpression{Text: "b"}},
	}}
	out, err := xml.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.HasPrefix(string(out), "<row>") {
		t.Errorf("row marshal = %s", out)
	}
	var got Row
	if err := xml.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Cells) != 2 || got.Cells[1].LiteralExpression.Text != "b" {
		t.Errorf("cells = %+v", got.Cells)
	}
}

// TestListMarshalNoID covers the branch where List has no ID attribute.
func TestListMarshalNoID(t *testing.T) {
	out, err := xml.Marshal(List{Items: []Expression{{LiteralExpression: &LiteralExpression{Text: "1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "id=") {
		t.Errorf("unexpected id attribute: %s", out)
	}
}

// TestRawMarshalError ensures Raw.MarshalXML surfaces an encoder error. We feed a
// token stream the encoder rejects (an EndElement with no matching start).
func TestRawMarshalError(t *testing.T) {
	r := &Raw{Tokens: []xml.Token{xml.EndElement{Name: xml.Name{Local: "x"}}}}
	var sb strings.Builder
	enc := xml.NewEncoder(&sb)
	if err := r.MarshalXML(enc, xml.StartElement{}); err == nil {
		t.Error("Raw.MarshalXML(bad token) = nil error, want error")
	}
}
