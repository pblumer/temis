package dmn

import (
	"fmt"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ContextView is a decision's boxed-context logic for the modeler: an ordered
// list of named entries (each a literal FEEL expression) and an optional
// result-cell expression. Simple is false when any entry's value is itself a
// nested boxed expression (not a literal), which this text-based view cannot
// represent — the editor then opens read-only so it never clobbers the nesting.
type ContextView struct {
	DecisionID    string             `json:"decisionId"`
	Name          string             `json:"name"`
	Entries       []ContextEntryView `json:"entries"`
	Result        string             `json:"result,omitempty"`
	ResultTypeRef string             `json:"resultTypeRef,omitempty"`
	Simple        bool               `json:"simple"`
}

// ContextEntryView is one boxed-context entry: a bound name and its literal FEEL
// expression with an optional declared result type. Index is the entry's position
// in the context (used as the `entry.N` locator step for a drill-in). ChildKind is
// set when the entry's value is itself a nested boxed expression rather than a
// literal, naming which boxed editor edits it in place (WP-66 Phase 2); Text is
// then empty.
type ContextEntryView struct {
	Name      string `json:"name"`
	Text      string `json:"text"`
	TypeRef   string `json:"typeRef,omitempty"`
	Index     int    `json:"index"`
	ChildKind string `json:"childKind,omitempty"`
}

// BoxedContext returns the decision's boxed-context view. ok is false when no
// such decision exists or its logic is not a boxed context.
func (d *Definitions) BoxedContext(idOrName string) (ContextView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.Context == nil {
		return ContextView{}, false
	}
	return contextViewFromModel(dec.ID, dec.Name, dec.Context), true
}

// ContextEdit is the editable payload for a boxed context: named entries (each a
// literal FEEL expression) and an optional result-cell expression. It replaces
// the decision's context entries wholesale.
type ContextEdit struct {
	Entries       []ContextEntryView `json:"entries"`
	Result        string             `json:"result,omitempty"`
	ResultTypeRef string             `json:"resultTypeRef,omitempty"`
}

// SetBoxedContext sets (or replaces) a decision's boxed-context logic from edit
// and returns the updated XML. Each named entry must have a non-empty name and
// expression; the optional result cell is the context's value (otherwise the
// value is a context keyed by the entry names). It errors when the decision is
// unknown or already carries non-context logic (use the matching editor for
// that).
func SetBoxedContext(src []byte, decisionID string, edit ContextEdit) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	var entries []dmnxml.ContextEntry
	for _, e := range edit.Entries {
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, fmt.Errorf("dmn: a boxed-context entry must have a name")
		}
		text := strings.TrimSpace(e.Text)
		if text == "" {
			return nil, fmt.Errorf("dmn: boxed-context entry %q must have an expression", name)
		}
		entries = append(entries, dmnxml.ContextEntry{
			Variable:   &dmnxml.Variable{Name: name, TypeRef: strings.TrimSpace(e.TypeRef)},
			Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: text}},
		})
	}
	if result := strings.TrimSpace(edit.Result); result != "" {
		entries = append(entries, dmnxml.ContextEntry{
			Expression: dmnxml.Expression{LiteralExpression: &dmnxml.LiteralExpression{Text: result, TypeRef: strings.TrimSpace(edit.ResultTypeRef)}},
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("dmn: a boxed context needs at least one entry or a result")
	}
	if !def.SetBoxedContext(decisionID, entries) {
		return nil, fmt.Errorf("dmn: cannot set a boxed context for decision %q (unknown or has non-context logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedContext gives an undecided decision a fresh boxed context (a single
// named entry, ready to edit) and returns the updated XML. It errors when the
// decision is unknown or already has logic.
func CreateBoxedContext(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateBoxedContext(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a boxed context for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
