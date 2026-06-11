// Package xml decodes DMN XML (1.5, tolerant towards 1.3/1.4) into schema
// structs and encodes them back, preserving the DMNDI subtree verbatim.
//
// Decoding is namespace-tolerant: struct tags carry local names only, so Go's
// encoding/xml matches elements in any DMN namespace. The detected version and
// any unrecognised elements are surfaced by the model mapper as diagnostics
// (see internal/model), keeping the decoder forward-compatible (ADR-0002).
package xml

import (
	"encoding/xml"
	"fmt"
)

// Decode parses a DMN document into Definitions. Unknown elements are retained
// (see UnknownElem) rather than rejected; only malformed XML is a hard error.
func Decode(data []byte) (*Definitions, error) {
	var def Definitions
	if err := xml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("decode DMN XML: %w", err)
	}
	return &def, nil
}
