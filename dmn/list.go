package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ListView is a decision's boxed-list logic for the modeler: an ordered list of
// FEEL item expressions. Simple is false when any item is itself a nested boxed
// expression (not a literal), which this text view cannot represent — the editor
// then opens read-only so it never clobbers the nesting.
type ListView struct {
	DecisionID string   `json:"decisionId"`
	Name       string   `json:"name"`
	Items      []string `json:"items"`
	Simple     bool     `json:"simple"`
}

// BoxedList returns the decision's boxed-list view. ok is false when no such
// decision exists or its logic is not a boxed list.
func (d *Definitions) BoxedList(idOrName string) (ListView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.List == nil {
		return ListView{}, false
	}
	v := ListView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
	for _, it := range dec.List.Items {
		switch lit := it.(type) {
		case *model.LiteralExpression:
			v.Items = append(v.Items, lit.Text)
		default:
			// A nested boxed item this text view can't carry; keep a placeholder so
			// the item count stays right and mark the list not simply editable.
			v.Items = append(v.Items, "")
			v.Simple = false
		}
	}
	return v, true
}

// ListEdit is the editable payload for a boxed list: the ordered FEEL items.
type ListEdit struct {
	Items []string `json:"items"`
}

// SetBoxedList sets (or replaces) a decision's boxed-list logic from edit and
// returns the updated XML. Blank items are dropped; the list must end up with at
// least one item. It errors when the decision is unknown or already carries
// non-list logic (use the matching editor for that).
func SetBoxedList(src []byte, decisionID string, edit ListEdit) ([]byte, error) {
	items := make([]string, 0, len(edit.Items))
	for _, it := range edit.Items {
		if s := strings.TrimSpace(it); s != "" {
			items = append(items, s)
		}
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("dmn: a list must have at least one item")
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetList(decisionID, items) {
		return nil, fmt.Errorf("dmn: cannot set a list for decision %q (unknown or has non-list logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedList gives an undecided decision a fresh boxed list (a single
// placeholder item, ready to edit) and returns the updated XML. It errors when the
// decision is unknown or already has logic.
func CreateBoxedList(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateList(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a list for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
