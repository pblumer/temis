package dmn

import (
	"fmt"
	"strings"

	dmnxml "github.com/pblumer/temis/internal/xml"
)

// LiteralView is a decision's literal-expression logic for the modeler: the FEEL
// text and its declared result type.
type LiteralView struct {
	DecisionID string `json:"decisionId"`
	Name       string `json:"name"`
	Text       string `json:"text"`
	TypeRef    string `json:"typeRef,omitempty"`
}

// LiteralExpression returns the decision's literal-expression view. ok is false
// when no such decision exists or its logic is not a literal expression.
func (d *Definitions) LiteralExpression(idOrName string) (LiteralView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.LiteralExpression == nil {
		return LiteralView{}, false
	}
	le := dec.LiteralExpression
	return LiteralView{DecisionID: dec.ID, Name: dec.Name, Text: le.Text, TypeRef: canonicalType(le.TypeRef)}, true
}

// SetLiteralExpression sets (or creates) a decision's literal-expression logic and
// returns the updated XML. The text is stored verbatim; an empty text is rejected
// (a literal decision must have an expression). It errors when the decision is
// unknown or already carries non-literal logic (e.g. a decision table — use the
// table editor for that).
func SetLiteralExpression(src []byte, decisionID, text, typeRef string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("dmn: literal expression must not be empty")
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetLiteralExpression(decisionID, strings.TrimSpace(text), strings.TrimSpace(typeRef)) {
		return nil, fmt.Errorf("dmn: cannot set a literal expression for decision %q (unknown or has non-literal logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
