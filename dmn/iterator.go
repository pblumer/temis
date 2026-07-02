package dmn

import (
	"fmt"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// IteratorView is a decision's boxed-iteration logic for the modeler: a `for`
// (which yields a list via its return branch) or a `some`/`every` quantifier
// (which yields a boolean via its satisfies branch). Body is that branch's FEEL
// text; the iterator Variable is bound in it while In is the collection iterated.
// Simple is false when either branch is a nested boxed expression (not a
// literal), which this text view cannot represent — the editor then opens
// read-only so it never clobbers the nesting.
type IteratorView struct {
	DecisionID string `json:"decisionId"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "for" | "some" | "every"
	Variable   string `json:"variable"`
	In         string `json:"in"`
	Body       string `json:"body"`
	Simple     bool   `json:"simple"`
}

// BoxedIterator returns the decision's boxed-iteration view. ok is false when no
// such decision exists or its logic is not a for/some/every iteration.
func (d *Definitions) BoxedIterator(idOrName string) (IteratorView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil {
		return IteratorView{}, false
	}
	v := IteratorView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
	switch {
	case dec.For != nil:
		v.Kind, v.Variable = "for", dec.For.IteratorVariable
		v.In = branchText(dec.For.In, &v.Simple)
		v.Body = branchText(dec.For.Return, &v.Simple)
	case dec.Quantified != nil:
		v.Kind, v.Variable = dec.Quantified.Kind, dec.Quantified.IteratorVariable
		v.In = branchText(dec.Quantified.In, &v.Simple)
		v.Body = branchText(dec.Quantified.Satisfies, &v.Simple)
	default:
		return IteratorView{}, false
	}
	return v, true
}

// IteratorEdit is the editable payload for a boxed iteration: the kind, the
// iterator variable, the collection and the return/satisfies body.
type IteratorEdit struct {
	Kind     string `json:"kind"`
	Variable string `json:"variable"`
	In       string `json:"in"`
	Body     string `json:"body"`
}

// SetBoxedIterator sets (or replaces) a decision's boxed-iteration logic from edit
// and returns the updated XML. kind must be "for", "some" or "every"; the
// variable, collection and body must all be non-empty. It errors when the
// decision is unknown or already carries non-iteration logic (use the matching
// editor for that).
func SetBoxedIterator(src []byte, decisionID string, edit IteratorEdit) ([]byte, error) {
	kind := strings.TrimSpace(edit.Kind)
	if kind != "for" && kind != "some" && kind != "every" {
		return nil, fmt.Errorf("dmn: unknown iteration kind %q (want for, some or every)", edit.Kind)
	}
	variable, inText, body := strings.TrimSpace(edit.Variable), strings.TrimSpace(edit.In), strings.TrimSpace(edit.Body)
	if variable == "" {
		return nil, fmt.Errorf("dmn: the iterator variable must not be empty")
	}
	if inText == "" {
		return nil, fmt.Errorf("dmn: the collection (in) must not be empty")
	}
	if body == "" {
		return nil, fmt.Errorf("dmn: the %s branch must not be empty", map[string]string{"for": "return", "some": "satisfies", "every": "satisfies"}[kind])
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetIterator(decisionID, kind, variable, inText, body) {
		return nil, fmt.Errorf("dmn: cannot set an iteration for decision %q (unknown or has non-iteration logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedIterator gives an undecided decision a fresh boxed iteration (a
// placeholder `for`) and returns the updated XML. It errors when the decision is
// unknown or already has logic.
func CreateBoxedIterator(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateIterator(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create an iteration for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
