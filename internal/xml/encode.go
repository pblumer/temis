package xml

import (
	"encoding/xml"
	"fmt"
)

// Encode serialises Definitions back to DMN XML. The model's default namespace
// (Definitions.XMLName.Space, populated on Decode) is emitted on the root, and
// the DMNDI subtree is replayed verbatim from its captured token stream.
//
// Namespace prefixes are not guaranteed to be identical to the input; the
// output is semantically equivalent and re-decodes to the same structs, which
// is the round-trip guarantee WP-02 targets (ADR-0010).
func Encode(def *Definitions) ([]byte, error) {
	enc := *def // shallow copy so we can set the namespace attribute without mutating the caller
	if enc.Xmlns == "" {
		enc.Xmlns = enc.XMLName.Space
	}
	body, err := xml.MarshalIndent(&enc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode DMN XML: %w", err)
	}
	out := make([]byte, 0, len(xml.Header)+len(body))
	out = append(out, xml.Header...)
	out = append(out, body...)
	return out, nil
}

// MarshalXML encodes a decision-table <rule>, emitting one wrapper element per
// entry. The struct stores entries as []string with `inputEntry>text`-style path
// tags, which decode correctly but, under the default marshaler, collapse all
// texts into a single shared wrapper (<inputEntry><text>a</text><text>b</text>…)
// and materialise an empty <annotationEntry/> for the empty slice — structurally
// invalid DMN even though it re-decodes to the same model. This restores
// byte-structural round-trip fidelity (ADR-0016 / WP-62): one <inputEntry> per
// input, one <outputEntry> per output, <annotationEntry> only when present.
func (r Rule) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	start.Name = xml.Name{Local: "rule"}
	start.Attr = nil
	if r.ID != "" {
		start.Attr = []xml.Attr{{Name: xml.Name{Local: "id"}, Value: r.ID}}
	}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, group := range []struct {
		parent string
		texts  []string
	}{
		{"inputEntry", r.InputEntries},
		{"outputEntry", r.OutputEntries},
		{"annotationEntry", r.Annotations},
	} {
		for _, text := range group.texts {
			if err := encodeTextEntry(e, group.parent, text); err != nil {
				return err
			}
		}
	}
	return e.EncodeToken(start.End())
}

// encodeTextEntry emits <parent><text>text</text></parent>.
func encodeTextEntry(e *xml.Encoder, parent, text string) error {
	p := named(parent)
	if err := e.EncodeToken(p); err != nil {
		return err
	}
	if err := e.EncodeElement(text, named("text")); err != nil {
		return err
	}
	return e.EncodeToken(p.End())
}
