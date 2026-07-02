package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// ItemType is a model's item definition (a named DMN type) as the modeler edits
// it: a base FEEL type with an optional collection flag and allowed-values
// constraint. Structured types (with item components) are reported with
// Structured=true and are read-only here — the simple editor does not touch them.
// Components carries a structured type's fields (name + type, nested), so a
// consumer can show the shape a caller must supply for that type — turning an
// opaque type name like "tDriverList" into the list of fields it actually wants.
type ItemType struct {
	Name          string     `json:"name"`
	TypeRef       string     `json:"typeRef,omitempty"`
	IsCollection  bool       `json:"isCollection,omitempty"`
	AllowedValues string     `json:"allowedValues,omitempty"`
	Structured    bool       `json:"structured,omitempty"`
	Components    []ItemType `json:"components,omitempty"`
}

// ItemDefinitions returns the model's named type definitions, for the modeler's
// type manager and the type pickers.
func (d *Definitions) ItemDefinitions() []ItemType {
	out := make([]ItemType, 0, len(d.model.ItemDefinitions))
	for _, it := range d.model.ItemDefinitions {
		out = append(out, itemType(it))
	}
	return out
}

// itemType maps one item definition to its DTO, recursing into a structured
// type's components so the whole shape (fields, nested fields) travels with it.
func itemType(it *model.ItemDefinition) ItemType {
	t := ItemType{
		Name:          it.Name,
		TypeRef:       it.TypeRef,
		IsCollection:  it.IsCollection,
		AllowedValues: it.AllowedValues,
		Structured:    len(it.Components) > 0,
	}
	if len(it.Components) > 0 {
		t.Components = make([]ItemType, 0, len(it.Components))
		for _, c := range it.Components {
			t.Components = append(t.Components, itemType(c))
		}
	}
	return t
}

// SetItemDefinition creates or updates a simple item definition and returns the
// updated XML. It errors on an empty name or when the named definition is
// structured (has components), which the simple editor must not overwrite.
func SetItemDefinition(src []byte, t ItemType) ([]byte, error) {
	if strings.TrimSpace(t.Name) == "" {
		return nil, fmt.Errorf("dmn: type name must not be empty")
	}
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.UpsertItemDefinition(t.Name, t.TypeRef, t.IsCollection, t.AllowedValues) {
		return nil, fmt.Errorf("dmn: cannot edit structured type %q here", t.Name)
	}
	return dmnxml.Encode(def)
}

// RemoveItemDefinition removes the named item definition and returns the updated
// XML. References to the type elsewhere are left as-is (the author's concern). It
// errors when no such type exists.
func RemoveItemDefinition(src []byte, name string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.RemoveItemDefinition(name) {
		return nil, fmt.Errorf("dmn: no type %q", name)
	}
	return dmnxml.Encode(def)
}
