package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// BKMParam is one formal parameter of a business knowledge model's function.
type BKMParam struct {
	Name    string `json:"name"`
	TypeRef string `json:"typeRef,omitempty"`
}

// BKMView is a business knowledge model's encapsulated logic for the modeler: its
// formal parameters and a literal FEEL body. Simple is false when the body is a
// non-literal boxed expression (a table/context/…), which the simple editor shows
// read-only.
type BKMView struct {
	BkmID       string     `json:"bkmId"`
	Name        string     `json:"name"`
	Params      []BKMParam `json:"params"`
	BodyText    string     `json:"bodyText"`
	BodyTypeRef string     `json:"bodyTypeRef,omitempty"`
	Simple      bool       `json:"simple"`
}

// BKMFunction returns a business knowledge model's encapsulated-logic view. ok is
// false when no such BKM exists.
func (d *Definitions) BKMFunction(idOrName string) (BKMView, bool) {
	var bkm *model.BKM
	for _, b := range d.model.BKMs {
		if b.ID == idOrName || b.Name == idOrName {
			bkm = b
			break
		}
	}
	if bkm == nil {
		return BKMView{}, false
	}
	v := BKMView{BkmID: bkm.ID, Name: bkm.Name, Simple: true}
	if fn := bkm.EncapsulatedLogic; fn != nil {
		for _, p := range fn.Parameters {
			v.Params = append(v.Params, BKMParam{Name: p.Name, TypeRef: canonicalType(p.TypeRef)})
		}
		switch body := fn.Body.(type) {
		case *model.LiteralExpression:
			v.BodyText = body.Text
			v.BodyTypeRef = canonicalType(body.TypeRef)
		case nil:
			// no body yet — still a simple (empty) function
		default:
			v.Simple = false // boxed body; not editable in the simple editor
		}
	}
	return v, true
}

// BKMFunctionEdit is the editable payload for a BKM's function: its formal
// parameters and a literal FEEL body.
type BKMFunctionEdit struct {
	Params      []BKMParam `json:"params"`
	BodyText    string     `json:"bodyText"`
	BodyTypeRef string     `json:"bodyTypeRef"`
}

// SetBKMFunction sets a business knowledge model's encapsulated logic to a
// function with the given parameters and literal body, returning the updated XML.
// An empty body is rejected; so is a BKM whose current body is a non-literal boxed
// expression (which this editor must not overwrite). Parameters with an empty name
// are dropped.
func SetBKMFunction(src []byte, bkmID string, edit BKMFunctionEdit) ([]byte, error) {
	if strings.TrimSpace(edit.BodyText) == "" {
		return nil, fmt.Errorf("dmn: BKM body must not be empty")
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	params := make([]dmnxml.FormalParameter, 0, len(edit.Params))
	for _, p := range edit.Params {
		if name := strings.TrimSpace(p.Name); name != "" {
			params = append(params, dmnxml.FormalParameter{Name: name, TypeRef: strings.TrimSpace(p.TypeRef)})
		}
	}
	if !def.SetBKMFunction(bkmID, params, strings.TrimSpace(edit.BodyText), strings.TrimSpace(edit.BodyTypeRef)) {
		return nil, fmt.Errorf("dmn: cannot set the function of BKM %q (unknown or non-literal body)", bkmID)
	}
	return dmnxml.Encode(def)
}
