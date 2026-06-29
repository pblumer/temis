package dmn

import dmnxml "github.com/pblumer/temis/internal/xml"

// NodeEdit describes a change to one decision requirements graph node, addressed
// by its DMN element id. Only the non-nil fields are applied, so a client can
// persist a move without touching the name, or a rename without touching the
// position. It mirrors the editable subset of GraphNode (ADR-0016, Edit→Save).
type NodeEdit struct {
	ID       string   `json:"id"`
	Name     *string  `json:"name,omitempty"`
	DataType *string  `json:"dataType,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
}

// ApplyEdits applies position, name and type edits to a DMN XML document and
// returns the updated XML. It patches the existing document in place rather than
// regenerating it, so all decision logic (decision tables, FEEL, boxed
// expressions) and the rest of the DMNDI diagram are preserved untouched — only
// the named, retyped or repositioned elements change.
//
// For each edit, a non-nil Name sets the element's name attribute (inputData,
// decision or businessKnowledgeModel); a non-nil DataType sets an inputData's
// variable typeRef; non-nil X and Y reposition the element's DMNShape in the
// diagram interchange (a no-op when the model carries no DMNDI). Edits for
// unknown ids are ignored. Renaming an element does not rewrite references to it
// elsewhere — keeping a downstream FEEL reference valid is the author's concern.
func ApplyEdits(src []byte, edits []NodeEdit) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	for _, e := range edits {
		if e.ID == "" {
			continue
		}
		if e.Name != nil {
			def.SetElementName(e.ID, *e.Name)
		}
		if e.DataType != nil {
			def.SetInputType(e.ID, *e.DataType)
		}
		if e.X != nil && e.Y != nil {
			dmnxml.MoveShape(def.DMNDI, e.ID, *e.X, *e.Y)
		}
	}
	return dmnxml.Encode(def)
}
