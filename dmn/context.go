package dmn

import (
	"fmt"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ContextEntryView is one entry of a boxed context for the modeler. Name is empty
// for the final result cell (Result is then true). Text is the FEEL source when
// the entry's value is a literal expression; Kind names the value's boxed kind
// ("literal", "decisionTable", "context", …) so a non-literal entry can be shown
// read-only.
type ContextEntryView struct {
	Name    string `json:"name,omitempty"`
	Text    string `json:"text"`
	TypeRef string `json:"typeRef,omitempty"`
	Kind    string `json:"kind"`
	Result  bool   `json:"result,omitempty"`
}

// ContextView is a decision's boxed-context logic for the modeler: its ordered
// entries. Simple is false when any entry's value is a non-literal boxed
// expression (a nested table/context/…), which the simple editor shows read-only
// so it never clobbers logic it cannot represent.
type ContextView struct {
	DecisionID string             `json:"decisionId"`
	Name       string             `json:"name"`
	Entries    []ContextEntryView `json:"entries"`
	Simple     bool               `json:"simple"`
}

// ContextOf returns the boxed-context view of the decision identified by
// idOrName, decoding the document so the entries' declared types survive a
// round-trip. ok is false when no such decision exists or its logic is not a
// boxed context.
func ContextOf(src []byte, idOrName string) (ContextView, bool, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return ContextView{}, false, err
	}
	for _, dec := range def.Decisions {
		if dec.ID != idOrName && dec.Name != idOrName {
			continue
		}
		if dec.Context == nil {
			return ContextView{}, false, nil
		}
		v := ContextView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
		for _, e := range dec.Context.Entries {
			ev := ContextEntryView{Result: e.Variable == nil}
			if e.Variable != nil {
				ev.Name = e.Variable.Name
				ev.TypeRef = canonicalType(e.Variable.TypeRef)
			}
			if le := e.LiteralExpression; le != nil && e.BoxedKind() == "literalExpression" {
				ev.Kind = "literal"
				ev.Text = le.Text
				if ev.TypeRef == "" {
					ev.TypeRef = canonicalType(le.TypeRef)
				}
			} else {
				ev.Kind = e.BoxedKind()
				v.Simple = false
			}
			v.Entries = append(v.Entries, ev)
		}
		return v, true, nil
	}
	return ContextView{}, false, nil
}

// ContextEdit is the editable payload for a decision's boxed context: its ordered
// entries. A named entry binds the result of its FEEL text to that name; the one
// entry with an empty name is the result cell.
type ContextEdit struct {
	Entries []ContextEntryEdit `json:"entries"`
}

// ContextEntryEdit mirrors one editable context entry.
type ContextEntryEdit struct {
	Name    string `json:"name"`
	Text    string `json:"text"`
	TypeRef string `json:"typeRef,omitempty"`
}

// SetContext sets (or creates) a decision's boxed-context logic from the given
// entries, returning the updated XML. Entries with an empty expression are
// dropped; at most one result cell (an entry with no name) is kept, and it must
// come last. It errors when the resulting context is empty, when the decision is
// unknown, or when the decision already carries a different boxed logic (which
// this editor must not overwrite).
func SetContext(src []byte, decisionID string, edit ContextEdit) ([]byte, error) {
	entries := make([]dmnxml.ContextEntryEdit, 0, len(edit.Entries))
	var resultSeen bool
	for _, e := range edit.Entries {
		text := strings.TrimSpace(e.Text)
		name := strings.TrimSpace(e.Name)
		if text == "" {
			continue // a blank row the user never filled in
		}
		if name == "" {
			resultSeen = true // collected below, after the named entries
		}
		entries = append(entries, dmnxml.ContextEntryEdit{Name: name, Text: text, TypeRef: strings.TrimSpace(e.TypeRef)})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("dmn: a context must have at least one entry")
	}
	// The result cell, if any, must be the final entry (DMN 1.5 §7.3.4).
	if resultSeen {
		last := len(entries) - 1
		if entries[last].Name != "" {
			return nil, fmt.Errorf("dmn: the result cell (unnamed entry) must be the last entry")
		}
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetContext(decisionID, entries) {
		return nil, fmt.Errorf("dmn: cannot set a context for decision %q (unknown or has non-context logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
