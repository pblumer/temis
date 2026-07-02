package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// InvocationBindingView is one parameter binding of a boxed invocation: the formal
// parameter name and the FEEL argument bound to it.
type InvocationBindingView struct {
	Parameter string `json:"parameter"`
	Value     string `json:"value"`
}

// InvocationView is a decision's boxed-invocation logic for the modeler: the
// called function/BKM (Called, a name) and the parameter bindings supplying its
// arguments. Simple is false when the called expression or any binding argument
// is itself a nested boxed expression (not a literal), which this text view
// cannot represent — the editor then opens read-only so it never clobbers the
// nesting.
type InvocationView struct {
	DecisionID string                  `json:"decisionId"`
	Name       string                  `json:"name"`
	Called     string                  `json:"called"`
	Bindings   []InvocationBindingView `json:"bindings"`
	Simple     bool                    `json:"simple"`
}

// BoxedInvocation returns the decision's boxed-invocation view. ok is false when
// no such decision exists or its logic is not a boxed invocation.
func (d *Definitions) BoxedInvocation(idOrName string) (InvocationView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.Invocation == nil {
		return InvocationView{}, false
	}
	inv := dec.Invocation
	v := InvocationView{DecisionID: dec.ID, Name: dec.Name, Simple: true}
	if lit, ok := inv.Called.(*model.LiteralExpression); ok {
		v.Called = lit.Text
	} else if inv.Called != nil {
		v.Simple = false
	}
	for _, b := range inv.Bindings {
		bv := InvocationBindingView{Parameter: b.Parameter}
		if lit, ok := b.Value.(*model.LiteralExpression); ok {
			bv.Value = lit.Text
		} else if b.Value != nil {
			v.Simple = false
		}
		v.Bindings = append(v.Bindings, bv)
	}
	return v, true
}

// InvocationEdit is the editable payload for a boxed invocation: the called
// function/BKM and its parameter bindings.
type InvocationEdit struct {
	Called   string                  `json:"called"`
	Bindings []InvocationBindingView `json:"bindings"`
}

// SetBoxedInvocation sets (or replaces) a decision's boxed-invocation logic from
// edit and returns the updated XML. The called function must be named; bindings
// with a blank parameter and value are dropped, and every remaining binding needs
// a unique parameter name and a non-empty argument. It errors when the decision
// is unknown or already carries non-invocation logic (use the matching editor).
func SetBoxedInvocation(src []byte, decisionID string, edit InvocationEdit) ([]byte, error) {
	called := strings.TrimSpace(edit.Called)
	if called == "" {
		return nil, fmt.Errorf("dmn: the invocation must name a function or BKM to call")
	}
	var params, values []string
	seen := map[string]bool{}
	for _, b := range edit.Bindings {
		p, val := strings.TrimSpace(b.Parameter), strings.TrimSpace(b.Value)
		if p == "" && val == "" {
			continue // an untouched binding row
		}
		if p == "" {
			return nil, fmt.Errorf("dmn: a binding argument is set without a parameter name")
		}
		if val == "" {
			return nil, fmt.Errorf("dmn: parameter %q has no argument", p)
		}
		if seen[p] {
			return nil, fmt.Errorf("dmn: duplicate binding for parameter %q", p)
		}
		seen[p] = true
		params = append(params, p)
		values = append(values, val)
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetInvocation(decisionID, called, params, values) {
		return nil, fmt.Errorf("dmn: cannot set an invocation for decision %q (unknown or has non-invocation logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedInvocation gives an undecided decision a fresh boxed invocation
// (placeholder called function and one binding) and returns the updated XML. It
// errors when the decision is unknown or already has logic.
func CreateBoxedInvocation(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateInvocation(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create an invocation for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
