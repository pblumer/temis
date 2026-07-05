package builtins

import (
	"sort"

	"github.com/pblumer/temis/internal/value"
)

func registerListMore(r *Registry) {
	// all/any(list): three-valued conjunction/disjunction over a boolean list.
	r.Register(variadic("all", 1, func(args []value.Value) (value.Value, error) {
		return allAny(listOf(args), true), nil
	}))
	r.Register(variadic("any", 1, func(args []value.Value) (value.Value, error) {
		return allAny(listOf(args), false), nil
	}))

	// sublist(list, start position, length?): 1-indexed; negative start counts
	// from the end; an omitted length runs to the end.
	r.Register(fixed("sublist", []string{"list", "start position", "length"}, 2, 3, sublist))

	// append(list, item...): list with the items added at the end.
	r.Register(variadic("append", 1, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		out := append(append([]value.Value{}, l.Elements...), args[1:]...)
		return value.NewList(out...), nil
	}))

	// concatenate(list...): the lists joined end to end.
	r.Register(variadic("concatenate", 1, func(args []value.Value) (value.Value, error) {
		out := []value.Value{}
		for _, a := range args {
			l, ok := a.(value.List)
			if !ok {
				return value.Null, nil
			}
			out = append(out, l.Elements...)
		}
		return value.NewList(out...), nil
	}))

	// insert before(list, position, newItem): newItem inserted before position.
	r.Register(fixed("insert before", []string{"list", "position", "newItem"}, 3, 3, insertBefore))

	// remove(list, position): list without the element at position (1-indexed).
	r.Register(fixed("remove", []string{"list", "position"}, 2, 2, remove))

	// reverse(list): elements in reverse order.
	r.Register(fixed("reverse", []string{"list"}, 1, 1, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		n := len(l.Elements)
		out := make([]value.Value, n)
		for i, e := range l.Elements {
			out[n-1-i] = e
		}
		return value.NewList(out...), nil
	}))

	// index of(list, match): 1-based positions of elements equal to match.
	r.Register(fixed("index of", []string{"list", "match"}, 2, 2, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		out := []value.Value{}
		for i, e := range l.Elements {
			if value.Equal(e, args[1]) == value.True {
				out = append(out, value.NumberFromInt64(int64(i+1)))
			}
		}
		return value.NewList(out...), nil
	}))

	// union(list...): the lists concatenated with duplicates removed.
	r.Register(variadic("union", 1, func(args []value.Value) (value.Value, error) {
		out := []value.Value{}
		for _, a := range args {
			l, ok := a.(value.List)
			if !ok {
				return value.Null, nil
			}
			out = append(out, l.Elements...)
		}
		return value.NewList(distinct(out)...), nil
	}))

	// distinct values(list): elements with duplicates removed, order preserved.
	r.Register(fixed("distinct values", []string{"list"}, 1, 1, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		return value.NewList(distinct(l.Elements)...), nil
	}))

	// flatten(list): nested lists flattened into a single list.
	r.Register(fixed("flatten", []string{"list"}, 1, 1, func(args []value.Value) (value.Value, error) {
		l, ok := args[0].(value.List)
		if !ok {
			return value.Null, nil
		}
		return value.NewList(flatten(l.Elements, nil)...), nil
	}))

	// product(list): product of the numeric elements; empty or non-number → null.
	r.Register(variadic("product", 1, func(args []value.Value) (value.Value, error) {
		elems := listOf(args)
		if len(elems) == 0 {
			return value.Null, nil
		}
		total := value.Value(value.NumberFromInt64(1))
		for _, e := range elems {
			if _, ok := asNumber(e); !ok {
				return value.Null, nil
			}
			total = value.Mul(total, e)
		}
		return total, nil
	}))

	// median(list): middle value of the sorted numbers; even count averages the
	// two middle values.
	r.Register(variadic("median", 1, median))

	// stddev(list): sample standard deviation of the numbers; needs ≥ 2 values.
	r.Register(variadic("stddev", 1, stddev))

	// mode(list): the most frequent value(s), ascending; empty list → empty list.
	r.Register(variadic("mode", 1, mode))
}

// allAny implements the three-valued all/any reduction. For conj=true (all):
// any false ⇒ false, else any null/non-boolean ⇒ null, else true. For conj=false
// (any): any true ⇒ true, else any null/non-boolean ⇒ null, else false.
func allAny(elems []value.Value, conj bool) value.Value {
	sawUnknown := false
	for _, e := range elems {
		b, ok := e.(value.Bool)
		if !ok {
			sawUnknown = true
			continue
		}
		if conj && !bool(b) {
			return value.False
		}
		if !conj && bool(b) {
			return value.True
		}
	}
	if sawUnknown {
		return value.Null
	}
	return value.BoolOf(conj)
}

func sublist(args []value.Value) (value.Value, error) {
	l, ok := args[0].(value.List)
	if !ok {
		return value.Null, nil
	}
	start, ok := asInt(args[1])
	if !ok || start == 0 {
		return value.Null, nil
	}
	n := len(l.Elements)
	var begin int
	if start < 0 {
		begin = n + start
	} else {
		begin = start - 1
	}
	if begin < 0 || begin > n {
		return value.Null, nil
	}
	end := n
	if len(args) >= 3 && !value.IsNull(args[2]) {
		length, ok := asInt(args[2])
		if !ok || length < 0 {
			return value.Null, nil
		}
		end = begin + length
		if end > n {
			end = n
		}
	}
	return value.NewList(append([]value.Value{}, l.Elements[begin:end]...)...), nil
}

func insertBefore(args []value.Value) (value.Value, error) {
	l, ok := args[0].(value.List)
	if !ok {
		return value.Null, nil
	}
	pos, ok := asInt(args[1])
	if !ok || pos < 1 || pos > len(l.Elements)+1 {
		return value.Null, nil
	}
	out := make([]value.Value, 0, len(l.Elements)+1)
	out = append(out, l.Elements[:pos-1]...)
	out = append(out, args[2])
	out = append(out, l.Elements[pos-1:]...)
	return value.NewList(out...), nil
}

func remove(args []value.Value) (value.Value, error) {
	l, ok := args[0].(value.List)
	if !ok {
		return value.Null, nil
	}
	pos, ok := asInt(args[1])
	if !ok || pos < 1 || pos > len(l.Elements) {
		return value.Null, nil
	}
	out := make([]value.Value, 0, len(l.Elements)-1)
	out = append(out, l.Elements[:pos-1]...)
	out = append(out, l.Elements[pos:]...)
	return value.NewList(out...), nil
}

// distinct returns the elements with later duplicates (by FEEL equality) removed.
func distinct(elems []value.Value) []value.Value {
	out := []value.Value{}
	for _, e := range elems {
		dup := false
		for _, seen := range out {
			if value.Equal(seen, e) == value.True {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, e)
		}
	}
	return out
}

// flatten recursively appends elements, descending into nested lists.
func flatten(elems []value.Value, out []value.Value) []value.Value {
	for _, e := range elems {
		if l, ok := e.(value.List); ok {
			out = flatten(l.Elements, out)
		} else {
			out = append(out, e)
		}
	}
	return out
}

// sortedNumbers returns the elements as ascending numbers, or ok=false if any
// element is not a number or the list is empty.
func sortedNumbers(elems []value.Value) ([]value.Number, bool) {
	if len(elems) == 0 {
		return nil, false
	}
	nums := make([]value.Number, len(elems))
	for i, e := range elems {
		n, ok := asNumber(e)
		if !ok {
			return nil, false
		}
		nums[i] = n
	}
	sort.SliceStable(nums, func(i, j int) bool { return nums[i].Cmp(nums[j]) < 0 })
	return nums, true
}

func median(args []value.Value) (value.Value, error) {
	nums, ok := sortedNumbers(listOf(args))
	if !ok {
		return value.Null, nil
	}
	n := len(nums)
	if n%2 == 1 {
		return nums[n/2], nil
	}
	sum := value.Add(nums[n/2-1], nums[n/2])
	return value.Div(sum, value.NumberFromInt64(2)), nil
}

func stddev(args []value.Value) (value.Value, error) {
	elems := listOf(args)
	nums, ok := sortedNumbers(elems)
	if !ok || len(nums) < 2 {
		return value.Null, nil
	}
	// mean
	sum := value.Value(value.NumberFromInt64(0))
	for _, x := range nums {
		sum = value.Add(sum, x)
	}
	mean := value.Div(sum, value.NumberFromInt64(int64(len(nums))))
	// sum of squared deviations
	sqSum := value.Value(value.NumberFromInt64(0))
	for _, x := range nums {
		d := value.Sub(x, mean)
		sqSum = value.Add(sqSum, value.Mul(d, d))
	}
	variance := value.Div(sqSum, value.NumberFromInt64(int64(len(nums)-1)))
	v, ok := variance.(value.Number)
	if !ok {
		return value.Null, nil
	}
	return numOrNull(v.Sqrt()), nil
}

func mode(args []value.Value) (value.Value, error) {
	// mode(null) is null (the argument is not a list); mode of an empty list is [].
	if len(args) == 1 && value.IsNull(args[0]) {
		return value.Null, nil
	}
	elems := listOf(args)
	if len(elems) == 0 {
		return value.NewList(), nil
	}
	// count occurrences keeping first-seen order of distinct values.
	type bucket struct {
		v     value.Value
		count int
	}
	buckets := []*bucket{}
	for _, e := range elems {
		found := false
		for _, b := range buckets {
			if value.Equal(b.v, e) == value.True {
				b.count++
				found = true
				break
			}
		}
		if !found {
			buckets = append(buckets, &bucket{v: e, count: 1})
		}
	}
	maxCount := 0
	for _, b := range buckets {
		if b.count > maxCount {
			maxCount = b.count
		}
	}
	out := []value.Value{}
	for _, b := range buckets {
		if b.count == maxCount {
			out = append(out, b.v)
		}
	}
	// FEEL mode returns the values in ascending order where comparable.
	sort.SliceStable(out, func(i, j int) bool {
		c, ok := value.Compare(out[i], out[j])
		return ok && c < 0
	})
	return value.NewList(out...), nil
}
