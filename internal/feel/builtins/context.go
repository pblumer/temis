package builtins

import "github.com/pblumer/temis/internal/value"

func registerContext(r *Registry) {
	// get value(context, key): the entry for key, or null when absent. key may be
	// a list of keys to navigate nested contexts (DMN 1.5).
	r.Register(fixed("get value", []string{"m", "key"}, 2, 2, func(args []value.Value) (value.Value, error) {
		ctx, ok := args[0].(*value.Context)
		if !ok {
			return value.Null, nil
		}
		switch k := args[1].(type) {
		case value.Str:
			if v, ok := ctx.Get(string(k)); ok {
				return v, nil
			}
			return value.Null, nil
		case value.List:
			cur := value.Value(ctx)
			for _, step := range k.Elements {
				c, ok := cur.(*value.Context)
				if !ok {
					return value.Null, nil
				}
				key, ok := step.(value.Str)
				if !ok {
					return value.Null, nil
				}
				v, ok := c.Get(string(key))
				if !ok {
					return value.Null, nil
				}
				cur = v
			}
			return cur, nil
		default:
			return value.Null, nil
		}
	}))

	// get entries(context): the entries as a list of {key, value} contexts.
	r.Register(fixed("get entries", []string{"m"}, 1, 1, func(args []value.Value) (value.Value, error) {
		ctx, ok := args[0].(*value.Context)
		if !ok {
			return value.Null, nil
		}
		out := make([]value.Value, 0, ctx.Len())
		for _, k := range ctx.Keys() {
			v, _ := ctx.Get(k)
			entry := value.NewContext().Put("key", value.Str(k)).Put("value", v)
			out = append(out, entry)
		}
		return value.NewList(out...), nil
	}))

	// context put(context, key, value): a copy of context with key set to value.
	r.Register(fixed("context put", []string{"context", "key", "value"}, 3, 3, func(args []value.Value) (value.Value, error) {
		ctx, ok := args[0].(*value.Context)
		if !ok {
			return value.Null, nil
		}
		key, ok := args[1].(value.Str)
		if !ok {
			return value.Null, nil
		}
		return cloneContext(ctx).Put(string(key), args[2]), nil
	}))

	// context merge(contexts): the contexts merged left to right; later entries win.
	r.Register(variadic("context merge", 1, func(args []value.Value) (value.Value, error) {
		ctxs := listOf(args)
		out := value.NewContext()
		for _, c := range ctxs {
			ctx, ok := c.(*value.Context)
			if !ok {
				return value.Null, nil
			}
			for _, k := range ctx.Keys() {
				v, _ := ctx.Get(k)
				out.Put(k, v)
			}
		}
		return out, nil
	}))

	// context(entries): build a context from a list of {key, value} entry contexts.
	r.Register(fixed("context", []string{"entries"}, 1, 1, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		out := value.NewContext()
		for _, e := range l.Elements {
			entry, ok := e.(*value.Context)
			if !ok {
				return value.Null, nil
			}
			key, ok := entry.Get("key")
			if !ok {
				return value.Null, nil
			}
			ks, ok := key.(value.Str)
			if !ok {
				return value.Null, nil
			}
			val, ok := entry.Get("value")
			if !ok {
				return value.Null, nil
			}
			out.Put(string(ks), val)
		}
		return out, nil
	}))
}

// cloneContext returns a shallow copy so builtins never mutate their input.
func cloneContext(c *value.Context) *value.Context {
	out := value.NewContext()
	for _, k := range c.Keys() {
		v, _ := c.Get(k)
		out.Put(k, v)
	}
	return out
}
