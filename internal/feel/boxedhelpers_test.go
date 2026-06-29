package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// constExpr is a CompiledExpr yielding v, for driving the boxed helpers directly.
func constExpr(v value.Value) CompiledExpr {
	return func(*Scope) (value.Value, error) { return v, nil }
}

func nums(ns ...int64) value.Value {
	vs := make([]value.Value, len(ns))
	for i, n := range ns {
		vs[i] = value.NumberFromInt64(n)
	}
	return value.NewList(vs...)
}

func TestForOne(t *testing.T) {
	// for x in [1,2,3] return x * 10
	coll := constExpr(nums(1, 2, 3))
	body := func(s *Scope) (value.Value, error) {
		return value.Mul(s.at(0), value.NumberFromInt64(10)), nil
	}
	got, err := ForOne(coll, body)(NewEnv().NewScope(nil))
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[10, 20, 30]" {
		t.Errorf("ForOne = %s, want [10, 20, 30]", got)
	}
}

func TestQuantifyOne(t *testing.T) {
	gt := func(n int64) CompiledExpr {
		return func(s *Scope) (value.Value, error) {
			cmp, _ := value.Compare(s.at(0), value.NumberFromInt64(n))
			return value.BoolOf(cmp > 0), nil
		}
	}
	coll := constExpr(nums(1, 2, 3))
	if got, _ := QuantifyOne(false, coll, gt(0))(NewEnv().NewScope(nil)); got != value.True {
		t.Errorf("every >0 = %s, want true", got)
	}
	if got, _ := QuantifyOne(false, coll, gt(2))(NewEnv().NewScope(nil)); got != value.False {
		t.Errorf("every >2 = %s, want false", got)
	}
	if got, _ := QuantifyOne(true, coll, gt(2))(NewEnv().NewScope(nil)); got != value.True {
		t.Errorf("some >2 = %s, want true", got)
	}
	if got, _ := QuantifyOne(true, coll, gt(5))(NewEnv().NewScope(nil)); got != value.False {
		t.Errorf("some >5 = %s, want false", got)
	}
	// A non-boolean predicate makes the result unknown (null).
	nullPred := constExpr(value.Null)
	if got, _ := QuantifyOne(true, coll, nullPred)(NewEnv().NewScope(nil)); !value.IsNull(got) {
		t.Errorf("some null = %s, want null", got)
	}
	if got, _ := QuantifyOne(false, coll, nullPred)(NewEnv().NewScope(nil)); !value.IsNull(got) {
		t.Errorf("every null = %s, want null", got)
	}
}

func TestBoxedFilterHelper(t *testing.T) {
	coll := constExpr(nums(1, 2, 3, 4))

	// Boolean predicate over the implicit item.
	f, err := BoxedFilter(coll, "item > 2", NewEnv(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f(NewEnv().NewScope(nil)); got.String() != "[3, 4]" {
		t.Errorf("filter item>2 = %s, want [3, 4]", got)
	}

	// Numeric predicate selects by 1-based index.
	f, err = BoxedFilter(coll, "1", NewEnv(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f(NewEnv().NewScope(nil)); got.String() != "1" {
		t.Errorf("filter [1] = %s, want 1", got)
	}

	// Malformed predicate is a compile error.
	if _, err := BoxedFilter(coll, "item >", NewEnv(), nil); err == nil {
		t.Error("malformed match should be a compile error")
	}
}

func TestIfThenElseCondError(t *testing.T) {
	boom := func(*Scope) (value.Value, error) { return nil, errBoom }
	_, err := IfThenElse(boom, constExpr(value.Null), constExpr(value.Null))(NewEnv().NewScope(nil))
	if err == nil {
		t.Error("a condition error should propagate")
	}
}

var errBoom = errString("boom")

type errString string

func (e errString) Error() string { return string(e) }
