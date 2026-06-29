package boxed

import (
	"fmt"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// compileList compiles a boxed list: its items evaluate to the elements of a
// FEEL list, in order.
func compileList(l *model.ListExpr, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	items := make([]feel.CompiledExpr, len(l.Items))
	for i, it := range l.Items {
		ce, err := Compile(it, env, funcs)
		if err != nil {
			return nil, fmt.Errorf("list item %d: %w", i+1, err)
		}
		items[i] = ce
	}
	return func(s *feel.Scope) (value.Value, error) {
		vs := make([]value.Value, len(items))
		for i, ce := range items {
			v, err := ce(s)
			if err != nil {
				return nil, err
			}
			vs[i] = v
		}
		return value.NewList(vs...), nil
	}, nil
}

// compileRelation compiles a boxed relation: each row evaluates to a context
// keyed by the column names, and the relation's value is the list of those row
// contexts.
func compileRelation(r *model.RelationExpr, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	rows := make([][]feel.CompiledExpr, len(r.Rows))
	for ri, row := range r.Rows {
		if len(row.Cells) != len(r.Columns) {
			return nil, fmt.Errorf("relation row %d has %d cells, want %d", ri+1, len(row.Cells), len(r.Columns))
		}
		cells := make([]feel.CompiledExpr, len(row.Cells))
		for ci, cell := range row.Cells {
			ce, err := Compile(cell, env, funcs)
			if err != nil {
				return nil, fmt.Errorf("relation row %d cell %d: %w", ri+1, ci+1, err)
			}
			cells[ci] = ce
		}
		rows[ri] = cells
	}
	cols := append([]string(nil), r.Columns...)
	return func(s *feel.Scope) (value.Value, error) {
		out := make([]value.Value, len(rows))
		for ri, cells := range rows {
			ctx := value.NewContext()
			for ci, ce := range cells {
				v, err := ce(s)
				if err != nil {
					return nil, err
				}
				ctx.Put(cols[ci], v)
			}
			out[ri] = ctx
		}
		return value.NewList(out...), nil
	}, nil
}
