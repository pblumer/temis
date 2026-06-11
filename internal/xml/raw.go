package xml

import "encoding/xml"

// Raw captures an XML element subtree verbatim as a token stream and replays it
// on encoding. It is used to preserve the DMNDI diagram interchange section
// across a decode/encode round-trip without losing any of its content.
//
// Namespace prefixes may be rewritten by the encoder, but element structure,
// attribute values and character data are reproduced faithfully — which is what
// "DMNDI bleibt erhalten" requires (see ADR-0010 and docs/50-testing-strategy.md).
type Raw struct {
	// Tokens is the captured stream, including the wrapping start and end
	// elements. Each token is copied so it stays valid after decoding.
	Tokens []xml.Token
}

// UnmarshalXML captures every token of the element (inclusive of start and end)
// so the subtree can be replayed verbatim later.
func (r *Raw) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	r.Tokens = append(r.Tokens, xml.CopyToken(start))
	depth := 1
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
		r.Tokens = append(r.Tokens, xml.CopyToken(tok))
		if depth == 0 {
			return nil
		}
	}
}

// MarshalXML replays the captured token stream. The start element passed by the
// encoder is ignored in favour of the originally captured one.
func (r *Raw) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	for _, tok := range r.Tokens {
		if err := e.EncodeToken(tok); err != nil {
			return err
		}
	}
	return nil
}

// UnknownElem captures any element not recognised by the schema structs. It is
// reported as a diagnostic by the model mapper rather than causing a hard
// failure, keeping decoding forward-compatible (ADR-0002).
type UnknownElem struct {
	XMLName xml.Name
	Inner   string `xml:",innerxml"`
}
