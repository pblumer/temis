package value

import (
	"strings"
	"time"
)

// Equal implements FEEL `=` semantics. It returns a Bool: two nulls are equal,
// a null and a non-null are not, and values of different kinds are not equal.
// Equality never yields null.
func Equal(a, b Value) Value {
	an, bn := IsNull(a), IsNull(b)
	if an || bn {
		return BoolOf(an && bn)
	}
	if a.Kind() != b.Kind() {
		return False
	}
	switch a.Kind() {
	case KindBool:
		return BoolOf(a.(Bool) == b.(Bool))
	case KindNumber:
		return BoolOf(a.(Number).Cmp(b.(Number)) == 0)
	case KindString:
		return BoolOf(a.(Str) == b.(Str))
	case KindDate:
		return BoolOf(a.(Date).t.Equal(b.(Date).t))
	case KindTime:
		return BoolOf(a.(Time).t.Equal(b.(Time).t))
	case KindDateTime:
		return BoolOf(a.(DateTime).t.Equal(b.(DateTime).t))
	case KindDaysTimeDuration:
		return BoolOf(a.(DaysTimeDuration).d == b.(DaysTimeDuration).d)
	case KindYearsMonthsDuration:
		return BoolOf(a.(YearsMonthsDuration).months == b.(YearsMonthsDuration).months)
	case KindList:
		return listEqual(a.(List), b.(List))
	case KindContext:
		return contextEqual(a.(*Context), b.(*Context))
	case KindRange:
		return rangeEqual(a.(Range), b.(Range))
	default:
		return False
	}
}

func listEqual(a, b List) Value {
	if len(a.Elements) != len(b.Elements) {
		return False
	}
	for i := range a.Elements {
		if Equal(a.Elements[i], b.Elements[i]) != True {
			return False
		}
	}
	return True
}

func contextEqual(a, b *Context) Value {
	if a.Len() != b.Len() {
		return False
	}
	for _, k := range a.keys {
		av := a.values[k]
		bv, ok := b.values[k]
		if !ok || Equal(av, bv) != True {
			return False
		}
	}
	return True
}

func rangeEqual(a, b Range) Value {
	if a.LowClosed != b.LowClosed || a.HighClosed != b.HighClosed {
		return False
	}
	return BoolOf(Equal(a.Low, b.Low) == True && Equal(a.High, b.High) == True)
}

// Compare orders two values for the relational operators (<, <=, >, >=). It
// returns -1, 0 or +1 and ok=true when the values are comparable; ok is false
// when either is null or the kinds have no defined ordering, in which case the
// relational operator yields null.
func Compare(a, b Value) (int, bool) {
	if IsNull(a) || IsNull(b) || a.Kind() != b.Kind() {
		return 0, false
	}
	switch a.Kind() {
	case KindNumber:
		return a.(Number).Cmp(b.(Number)), true
	case KindString:
		return strings.Compare(string(a.(Str)), string(b.(Str))), true
	case KindDate:
		return cmpTime(a.(Date).t, b.(Date).t), true
	case KindTime:
		return cmpTime(a.(Time).t, b.(Time).t), true
	case KindDateTime:
		return cmpTime(a.(DateTime).t, b.(DateTime).t), true
	case KindDaysTimeDuration:
		return cmpInt64(int64(a.(DaysTimeDuration).d), int64(b.(DaysTimeDuration).d)), true
	case KindYearsMonthsDuration:
		return cmpInt64(a.(YearsMonthsDuration).months, b.(YearsMonthsDuration).months), true
	default:
		return 0, false // booleans, lists, contexts, ranges and functions are unordered
	}
}

func cmpTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
