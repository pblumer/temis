// Package xml decodes DMN XML (1.5, tolerant towards 1.3/1.4) into schema
// structs and encodes them back, preserving the DMNDI subtree verbatim.
//
// Decoding is namespace-tolerant: struct tags carry local names only, so Go's
// encoding/xml matches elements in any DMN namespace. The detected version and
// any unrecognised elements are surfaced by the model mapper as diagnostics
// (see internal/model), keeping the decoder forward-compatible (ADR-0002).
package xml

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
)

// DefaultMaxElementDepth bounds how deeply nested the element tree may be before
// decoding is refused. encoding/xml (and the custom UnmarshalXML on boxed lists)
// recurse per level, so unbounded nesting would overflow the stack and crash the
// process on hostile input (audit finding K1, ADR-0008). Real DMN documents nest
// in the low tens; this ceiling sits far above that.
const DefaultMaxElementDepth = 5_000

// Decode parses a DMN document into Definitions. Unknown elements are retained
// (see UnknownElem) rather than rejected; only malformed XML is a hard error.
func Decode(data []byte) (*Definitions, error) {
	if err := checkDepth(data, DefaultMaxElementDepth); err != nil {
		return nil, err
	}
	var def Definitions
	if err := xml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("decode DMN XML: %w", err)
	}
	return &def, nil
}

// checkDepth scans the token stream once, before the recursive unmarshal, and
// rejects documents whose element nesting exceeds max. This is O(tokens) and
// allocation-light, guarding the recursive decode from stack exhaustion.
func checkDepth(data []byte, max int) error {
	dec := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return nil // let xml.Unmarshal produce the canonical error
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
			if depth > max {
				return fmt.Errorf("decode DMN XML: element nesting too deep (limit %d)", max)
			}
		case xml.EndElement:
			depth--
		}
	}
}
