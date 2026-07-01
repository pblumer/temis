package xml

import (
	"encoding/xml"
	"errors"
	"testing"
)

// failWriter fails every Write, so EncodeElement (which flushes) surfaces an
// error. EncodeToken alone buffers and does not, so only the EncodeElement-backed
// error paths are exercised here.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("boom") }

func newFailEncoder() *xml.Encoder { return xml.NewEncoder(failWriter{}) }

func TestListMarshalEncodeError(t *testing.T) {
	l := List{ID: "L", Items: []Expression{{LiteralExpression: &LiteralExpression{Text: "x"}}}}
	if err := l.MarshalXML(newFailEncoder(), named("list")); err == nil {
		t.Error("List.MarshalXML(failing writer) = nil error, want error")
	}
}

func TestRowMarshalEncodeError(t *testing.T) {
	r := Row{Cells: []Expression{{LiteralExpression: &LiteralExpression{Text: "x"}}}}
	if err := r.MarshalXML(newFailEncoder(), named("row")); err == nil {
		t.Error("Row.MarshalXML(failing writer) = nil error, want error")
	}
}

func TestRuleMarshalEncodeError(t *testing.T) {
	r := Rule{ID: "r1", InputEntries: []string{"a"}}
	if err := r.MarshalXML(newFailEncoder(), named("rule")); err == nil {
		t.Error("Rule.MarshalXML(failing writer) = nil error, want error")
	}
}

func TestEncodeExprSeqError(t *testing.T) {
	items := []Expression{{LiteralExpression: &LiteralExpression{Text: "x"}}}
	if err := encodeExprSeq(newFailEncoder(), items); err == nil {
		t.Error("encodeExprSeq(failing writer) = nil error, want error")
	}
}

func TestEncodeTextEntryError(t *testing.T) {
	if err := encodeTextEntry(newFailEncoder(), "inputEntry", "x"); err == nil {
		t.Error("encodeTextEntry(failing writer) = nil error, want error")
	}
}

// TestEncodeExprElementEachKindError drives the error return of every branch of
// encodeExprElement (each calls EncodeElement, which surfaces the writer error).
func TestEncodeExprElementEachKindError(t *testing.T) {
	for i, x := range allExprItems() {
		if !x.present() {
			continue // empty slot returns nil, no EncodeElement
		}
		if err := encodeExprElement(newFailEncoder(), &x); err == nil {
			t.Errorf("encodeExprElement(item %d, failing writer) = nil error, want error", i)
		}
	}
	// empty slot: no error, nothing emitted
	var empty Expression
	if err := encodeExprElement(newFailEncoder(), &empty); err != nil {
		t.Errorf("encodeExprElement(empty) = %v, want nil", err)
	}
}
