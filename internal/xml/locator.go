package xml

import (
	"fmt"
	"strconv"
	"strings"
)

// Step is one descent from a boxed expression into a nested child. Op names the
// child position; I and J index it (I for a context entry, list item, relation
// row or invocation binding; J for the relation cell's column). A branch step
// (if/then/else, in/return/satisfies, match, called) uses neither index.
type Step struct {
	Op string
	I  int
	J  int
}

// ParseSteps parses a logic locator like "entry.2/item.0" or "cell.1.3" into an
// ordered list of steps. An empty string is the root (no steps). Each slash-
// separated step is an op optionally followed by ".I" and ".J" indices.
func ParseSteps(s string) ([]Step, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	var steps []Step
	for _, part := range strings.Split(s, "/") {
		fields := strings.Split(part, ".")
		if fields[0] == "" || len(fields) > 3 {
			return nil, fmt.Errorf("xml: malformed locator step %q", part)
		}
		st := Step{Op: fields[0], I: -1, J: -1}
		for k, dst := range []*int{&st.I, &st.J} {
			if len(fields) > k+1 {
				n, err := strconv.Atoi(fields[k+1])
				if err != nil || n < 0 {
					return nil, fmt.Errorf("xml: invalid locator index in %q", part)
				}
				*dst = n
			}
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// exprSlotAt resolves the anchor's root logic slot (logicSlot) then walks the
// steps to a nested child slot. create allows creating a BKM function shell for
// the root only — nested steps require the parent structure to already exist. The
// returned pointer aliases the model, so writing through it mutates it in place.
func (d *Definitions) exprSlotAt(anchorKind, anchorID string, steps []Step, create bool) (*Expression, bool) {
	root, ok := d.logicSlot(anchorKind, anchorID, create && len(steps) == 0)
	if !ok {
		return nil, false
	}
	return walkSlot(root, steps)
}

// walkSlot follows steps from an expression slot to a nested child slot. ok is
// false for a step whose op does not match the current expression's kind, whose
// index is out of range, or whose branch is absent.
func walkSlot(slot *Expression, steps []Step) (*Expression, bool) {
	cur := slot
	for _, s := range steps {
		switch {
		case cur.Context != nil:
			if s.Op != "entry" || s.I < 0 || s.I >= len(cur.Context.Entries) {
				return nil, false
			}
			cur = &cur.Context.Entries[s.I].Expression
		case cur.List != nil:
			if s.Op != "item" || s.I < 0 || s.I >= len(cur.List.Items) {
				return nil, false
			}
			cur = &cur.List.Items[s.I]
		case cur.Relation != nil:
			if s.Op != "cell" || s.I < 0 || s.I >= len(cur.Relation.Rows) || s.J < 0 || s.J >= len(cur.Relation.Rows[s.I].Cells) {
				return nil, false
			}
			cur = &cur.Relation.Rows[s.I].Cells[s.J]
		case cur.Invocation != nil:
			switch s.Op {
			case "called":
				cur = &cur.Invocation.Expression
			case "binding":
				if s.I < 0 || s.I >= len(cur.Invocation.Bindings) {
					return nil, false
				}
				cur = &cur.Invocation.Bindings[s.I].Expression
			default:
				return nil, false
			}
		case cur.Conditional != nil:
			child := branchByOp(s.Op, map[string]*ChildExpr{"if": cur.Conditional.If, "then": cur.Conditional.Then, "else": cur.Conditional.Else})
			if child == nil {
				return nil, false
			}
			cur = &child.Expression
		case cur.For != nil:
			child := branchByOp(s.Op, map[string]*ChildExpr{"in": cur.For.In, "return": cur.For.Return})
			if child == nil {
				return nil, false
			}
			cur = &child.Expression
		case cur.Some != nil:
			child := branchByOp(s.Op, map[string]*ChildExpr{"in": cur.Some.In, "satisfies": cur.Some.Satisfies})
			if child == nil {
				return nil, false
			}
			cur = &child.Expression
		case cur.Every != nil:
			child := branchByOp(s.Op, map[string]*ChildExpr{"in": cur.Every.In, "satisfies": cur.Every.Satisfies})
			if child == nil {
				return nil, false
			}
			cur = &child.Expression
		case cur.Filter != nil:
			child := branchByOp(s.Op, map[string]*ChildExpr{"in": cur.Filter.In, "match": cur.Filter.Match})
			if child == nil {
				return nil, false
			}
			cur = &child.Expression
		default:
			return nil, false
		}
	}
	return cur, true
}

// branchByOp selects a named branch child, returning nil when the op is unknown
// or the branch is absent.
func branchByOp(op string, branches map[string]*ChildExpr) *ChildExpr {
	return branches[op]
}
