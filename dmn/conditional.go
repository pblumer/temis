package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ConditionalView is a decision's boxed-conditional logic for the modeler: the
// three FEEL branches of an if/then/else. Simple is false when any branch is
// itself a nested boxed expression (not a literal), which this text view cannot
// represent — the editor then opens read-only so it never clobbers the nesting.
type ConditionalView struct {
	DecisionID string `json:"decisionId"`
	Name       string `json:"name"`
	If         string `json:"if"`
	Then       string `json:"then"`
	Else       string `json:"else"`
	Simple     bool   `json:"simple"`
}

// BoxedConditional returns the decision's boxed-conditional view. ok is false when
// no such decision exists or its logic is not a boxed conditional.
func (d *Definitions) BoxedConditional(idOrName string) (ConditionalView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.Conditional == nil {
		return ConditionalView{}, false
	}
	c := dec.Conditional
	v := ConditionalView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
	v.If = branchText(c.If, &v.Simple)
	v.Then = branchText(c.Then, &v.Simple)
	v.Else = branchText(c.Else, &v.Simple)
	return v, true
}

// branchText returns a conditional branch's FEEL text when it is a literal
// expression (or empty when the branch is absent); for a nested boxed branch it
// clears simple, since the text editor cannot carry the nesting.
func branchText(e model.Expression, simple *bool) string {
	switch b := e.(type) {
	case nil:
		return ""
	case *model.LiteralExpression:
		return b.Text
	default:
		*simple = false
		return ""
	}
}

// ConditionalEdit is the editable payload for a boxed conditional: the three FEEL
// branches. All three are required.
type ConditionalEdit struct {
	If   string `json:"if"`
	Then string `json:"then"`
	Else string `json:"else"`
}

// SetBoxedConditional sets (or replaces) a decision's boxed-conditional logic from
// edit and returns the updated XML. Each branch must be a non-empty FEEL
// expression. It errors when the decision is unknown or already carries
// non-conditional logic (use the matching editor for that).
func SetBoxedConditional(src []byte, decisionID string, edit ConditionalEdit) ([]byte, error) {
	ifText, thenText, elseText := strings.TrimSpace(edit.If), strings.TrimSpace(edit.Then), strings.TrimSpace(edit.Else)
	for name, s := range map[string]string{"Bedingung (if)": ifText, "Dann (then)": thenText, "Sonst (else)": elseText} {
		if s == "" {
			return nil, fmt.Errorf("dmn: conditional branch %q must not be empty", name)
		}
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetConditional(decisionID, ifText, thenText, elseText) {
		return nil, fmt.Errorf("dmn: cannot set a conditional for decision %q (unknown or has non-conditional logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedConditional gives an undecided decision a fresh boxed conditional
// (placeholder branches, ready to edit) and returns the updated XML. It errors
// when the decision is unknown or already has logic.
func CreateBoxedConditional(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateConditional(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a conditional for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
