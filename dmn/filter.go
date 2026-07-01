package dmn

import (
	"fmt"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// FilterView is a decision's boxed-filter logic for the modeler: the collection
// (`in`) and the predicate (`match`), which is evaluated for each element with
// `item` bound to it. Simple is false when either branch is itself a nested boxed
// expression (not a literal), which this text view cannot represent — the editor
// then opens read-only so it never clobbers the nesting.
type FilterView struct {
	DecisionID string `json:"decisionId"`
	Name       string `json:"name"`
	In         string `json:"in"`
	Match      string `json:"match"`
	Simple     bool   `json:"simple"`
}

// BoxedFilter returns the decision's boxed-filter view. ok is false when no such
// decision exists or its logic is not a boxed filter.
func (d *Definitions) BoxedFilter(idOrName string) (FilterView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.Filter == nil {
		return FilterView{}, false
	}
	f := dec.Filter
	v := FilterView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
	v.In = branchText(f.In, &v.Simple)
	v.Match = branchText(f.Match, &v.Simple)
	return v, true
}

// FilterEdit is the editable payload for a boxed filter: the two FEEL branches.
// Both are required.
type FilterEdit struct {
	In    string `json:"in"`
	Match string `json:"match"`
}

// SetBoxedFilter sets (or replaces) a decision's boxed-filter logic from edit and
// returns the updated XML. Both branches must be non-empty FEEL expressions. It
// errors when the decision is unknown or already carries non-filter logic (use
// the matching editor for that).
func SetBoxedFilter(src []byte, decisionID string, edit FilterEdit) ([]byte, error) {
	inText, matchText := strings.TrimSpace(edit.In), strings.TrimSpace(edit.Match)
	if inText == "" {
		return nil, fmt.Errorf("dmn: the filter collection (in) must not be empty")
	}
	if matchText == "" {
		return nil, fmt.Errorf("dmn: the filter predicate (match) must not be empty")
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetFilter(decisionID, inText, matchText) {
		return nil, fmt.Errorf("dmn: cannot set a filter for decision %q (unknown or has non-filter logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedFilter gives an undecided decision a fresh boxed filter (placeholder
// branches, ready to edit) and returns the updated XML. It errors when the
// decision is unknown or already has logic.
func CreateBoxedFilter(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateFilter(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a filter for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
