package feel

import (
	"testing"

	"github.com/pblumer/temis/internal/value"
)

// TestFuncParamAndResultCoercion covers WP-41.21: a formal-parameter type coerces
// (or rejects) arguments, and a declared result type coerces the body's value.
func TestFuncParamAndResultCoercion(t *testing.T) {
	num := func(s string) value.Value { return value.MustNumber(s) }

	// A function returning a singleton list, declared to return a number: the list
	// unwraps to its element (CoerceToType).
	wrap := &Func{
		Name:       "wrap",
		Params:     []string{"x"},
		Body:       func(s *Scope) (value.Value, error) { return value.NewList(s.at(0)), nil },
		ResultType: TNumber,
	}
	env := NewEnv()
	scope := env.NewScope(nil)
	got, err := wrap.call(scope, []value.Value{num("10")})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "10" {
		t.Errorf("wrap(10) with number result type = %s, want 10 (singleton unwrapped)", got.String())
	}

	// A non-conforming argument makes the whole call null (the function is not
	// invoked), rather than binding a nulled parameter.
	needNum := &Func{
		Name:       "needNum",
		Params:     []string{"x"},
		Body:       func(s *Scope) (value.Value, error) { return value.True, nil },
		ParamTypes: []*Type{TNumber},
	}
	got, err = needNum.call(scope, []value.Value{value.Str("foo")})
	if err != nil {
		t.Fatal(err)
	}
	if !value.IsNull(got) {
		t.Errorf(`needNum("foo") = %s, want null (arg does not conform)`, got.String())
	}
	// A conforming argument invokes normally.
	if got, _ := needNum.call(scope, []value.Value{num("1")}); got != value.True {
		t.Errorf("needNum(1) = %s, want true", got.String())
	}
}
