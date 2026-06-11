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
