package boxed

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/value"
)

// parsePriorityList parses an output clause's list of allowed values (priority
// order, highest first) into ordered values, used by the Priority and Output
// Order hit policies. An empty specification yields no list (no priority).
func parsePriorityList(text string) ([]value.Value, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	ce, err := feel.CompileString("["+text+"]", feel.NewEnv())
	if err != nil {
		return nil, err
	}
	v, err := ce(feel.NewEnv().NewScope(nil))
	if err != nil {
		return nil, err
	}
	list, ok := v.(value.List)
	if !ok {
		return nil, fmt.Errorf("output values did not evaluate to a list")
	}
	return list.Elements, nil
}

// prioritized implements the Priority (asList=false) and Output Order
// (asList=true) hit policies: matching rules are ordered by output priority —
// the position of each output value in its allowed-values list, compared across
// outputs in column order (ties keep table order). Priority returns the single
// highest-priority output; Output Order returns all outputs in priority order.
func (ct *compiledTable) prioritized(s *feel.Scope, matched []int, asList bool, tt *TableTrace) (value.Value, error) {
	if len(matched) == 0 {
		if asList {
			return value.NewList(), nil
		}
		return value.Null, nil
	}

	type scored struct {
		cells []value.Value
		key   []int
		order int // original table position, for a stable tiebreak
	}
	scoredRules := make([]scored, len(matched))
	for i, ri := range matched {
		cells, err := ct.ruleCells(s, ri, tt)
		if err != nil {
			return nil, err
		}
		scoredRules[i] = scored{cells: cells, key: ct.priorityKey(cells), order: ri}
	}

	sort.SliceStable(scoredRules, func(a, b int) bool {
		return lessKey(scoredRules[a].key, scoredRules[b].key)
	})

	if !asList {
		return ct.outputValue(scoredRules[0].cells), nil
	}
	out := make([]value.Value, len(scoredRules))
	for i, sr := range scoredRules {
		out[i] = ct.outputValue(sr.cells)
	}
	return value.NewList(out...), nil
}

// priorityKey scores a rule's output cells: per output, the index of the cell's
// value in that output's priority list (lower = higher priority), or the list
// length when the value is absent or no list is defined.
func (ct *compiledTable) priorityKey(cells []value.Value) []int {
	key := make([]int, len(cells))
	for i, c := range cells {
		list := ct.priorities[i]
		idx := len(list)
		for j, pv := range list {
			if value.Equal(c, pv) == value.True {
				idx = j
				break
			}
		}
		key[i] = idx
	}
	return key
}

// lessKey reports whether priority key a outranks b (lexicographic, lower wins).
func lessKey(a, b []int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
