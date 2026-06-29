package boxed

import (
	"fmt"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// compileContext compiles a boxed context. Entries are evaluated in order, each
// named entry's value becoming visible to the entries that follow (so a later
// entry may build on an earlier one). A trailing entry without a name is the
// result cell: its value is the context's value. With no result cell the value
// is a context keyed by the entry names.
func compileContext(c *model.ContextExpr, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	type entry struct {
		name string
		expr feel.CompiledExpr
	}
	entries := make([]entry, 0, len(c.Entries))
	hasResult := false
	cur := env
	for i, e := range c.Entries {
		if e.Name == "" {
			if i != len(c.Entries)-1 {
				return nil, fmt.Errorf("context result cell must be the last entry")
			}
			hasResult = true
		}
		ce, err := Compile(e.Value, cur, funcs)
		if err != nil {
			return nil, fmt.Errorf("context entry %d (%q): %w", i+1, e.Name, err)
		}
		entries = append(entries, entry{name: e.Name, expr: ce})
		if e.Name != "" {
			cur = cur.Append(e.Name)
		}
	}

	return func(s *feel.Scope) (value.Value, error) {
		scope := s
		ctx := value.NewContext()
		result := value.Value(value.Null)
		for _, en := range entries {
			v, err := en.expr(scope)
			if err != nil {
				return nil, err
			}
			if en.name == "" {
				result = v
				continue
			}
			ctx.Put(en.name, v)
			scope = scope.Extend(v)
		}
		if hasResult {
			return result, nil
		}
		return ctx, nil
	}, nil
}
