package dmn

import (
	"fmt"
	"strings"

	"github.com/pblumer/temis/internal/model"
	dmnxml "github.com/pblumer/temis/internal/xml"
)

// RelationView is a decision's boxed-relation logic for the modeler: named columns
// and rows of FEEL cells (reference/lookup data). Simple is false when any cell is
// itself a nested boxed expression (not a literal), which this text grid cannot
// represent — the editor then opens read-only so it never clobbers the nesting.
type RelationView struct {
	DecisionID string     `json:"decisionId"`
	Name       string     `json:"name"`
	Columns    []string   `json:"columns"`
	Rows       [][]string `json:"rows"`
	Simple     bool       `json:"simple"`
}

// BoxedRelation returns the decision's boxed-relation view. ok is false when no
// such decision exists or its logic is not a boxed relation.
func (d *Definitions) BoxedRelation(idOrName string) (RelationView, bool) {
	dec := d.decisionModel(idOrName)
	if dec == nil || dec.Relation == nil {
		return RelationView{}, false
	}
	rel := dec.Relation
	v := RelationView{DecisionID: dec.ID, Name: dec.Name, Columns: append([]string(nil), rel.Columns...), Simple: true}
	for _, row := range rel.Rows {
		cells := make([]string, 0, len(row.Cells))
		for _, c := range row.Cells {
			if lit, ok := c.(*model.LiteralExpression); ok {
				cells = append(cells, lit.Text)
			} else {
				// A nested boxed cell this text grid can't carry; keep a placeholder
				// so the grid stays rectangular and mark the relation not editable.
				cells = append(cells, "")
				v.Simple = false
			}
		}
		v.Rows = append(v.Rows, cells)
	}
	return v, true
}

// RelationEdit is the editable payload for a boxed relation: the column names and
// the rows of FEEL cells (each row aligned to the columns).
type RelationEdit struct {
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// SetBoxedRelation sets (or replaces) a decision's boxed-relation logic from edit
// and returns the updated XML. Columns must be non-empty and uniquely named;
// fully blank rows are dropped, and every remaining row must have exactly one
// non-empty FEEL cell per column. It errors when the decision is unknown or
// already carries non-relation logic (use the matching editor for that).
func SetBoxedRelation(src []byte, decisionID string, edit RelationEdit) ([]byte, error) {
	cols := make([]string, 0, len(edit.Columns))
	seen := map[string]bool{}
	for _, c := range edit.Columns {
		name := strings.TrimSpace(c)
		if name == "" {
			return nil, fmt.Errorf("dmn: a relation column must have a name")
		}
		if seen[name] {
			return nil, fmt.Errorf("dmn: duplicate relation column %q", name)
		}
		seen[name] = true
		cols = append(cols, name)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("dmn: a relation needs at least one column")
	}

	var rows [][]string
	for i, r := range edit.Rows {
		trimmed := make([]string, len(r))
		blank := true
		for j, cell := range r {
			trimmed[j] = strings.TrimSpace(cell)
			if trimmed[j] != "" {
				blank = false
			}
		}
		if blank {
			continue // an untouched row
		}
		if len(trimmed) != len(cols) {
			return nil, fmt.Errorf("dmn: row %d has %d cells, want %d", i+1, len(trimmed), len(cols))
		}
		for j, cell := range trimmed {
			if cell == "" {
				return nil, fmt.Errorf("dmn: row %d, column %q is empty", i+1, cols[j])
			}
		}
		rows = append(rows, trimmed)
	}

	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.SetRelation(decisionID, cols, rows) {
		return nil, fmt.Errorf("dmn: cannot set a relation for decision %q (unknown or has non-relation logic)", decisionID)
	}
	return dmnxml.Encode(def)
}

// CreateBoxedRelation gives an undecided decision a fresh boxed relation (one
// column, one placeholder cell) and returns the updated XML. It errors when the
// decision is unknown or already has logic.
func CreateBoxedRelation(src []byte, decisionID string) ([]byte, error) {
	def, err := dmnxml.Decode(src)
	if err != nil {
		return nil, err
	}
	if !def.CreateRelation(decisionID) {
		return nil, fmt.Errorf("dmn: cannot create a relation for decision %q (unknown or already has logic)", decisionID)
	}
	return dmnxml.Encode(def)
}
