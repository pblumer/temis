package boxed

import (
	"testing"

	"github.com/pblumer/feel"
	"github.com/pblumer/feel/value"
	"github.com/pblumer/temis/internal/model"
)

func TestBoxedList(t *testing.T) {
	l := &model.ListExpr{Items: []model.Expression{lit("1"), lit("2"), lit("x")}}
	got, err := evalExpr(t, l, feel.NewEnv("x"), nil, map[string]value.Value{"x": value.MustNumber("3")})
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[1, 2, 3]" {
		t.Errorf("list = %s, want [1, 2, 3]", got)
	}
}

func TestBoxedListItemError(t *testing.T) {
	l := &model.ListExpr{Items: []model.Expression{lit("1 +")}}
	if _, err := Compile(l, feel.NewEnv(), nil); err == nil {
		t.Error("malformed list item should be a compile error")
	}
}

func TestBoxedRelation(t *testing.T) {
	rel := &model.RelationExpr{
		Columns: []string{"a", "b"},
		Rows: []model.RelationRow{
			{Cells: []model.Expression{lit("1"), lit("2")}},
			{Cells: []model.Expression{lit("3"), lit("4")}},
		},
	}
	got, err := evalExpr(t, rel, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	list, ok := got.(value.List)
	if !ok || len(list.Elements) != 2 {
		t.Fatalf("relation value = %s, want a 2-element list", got)
	}
	row0 := list.Elements[0].(*value.Context)
	if a, _ := row0.Get("a"); a.String() != "1" {
		t.Errorf("row0.a = %s, want 1", a)
	}
}

func TestBoxedRelationRowArityMismatch(t *testing.T) {
	rel := &model.RelationExpr{
		Columns: []string{"a", "b"},
		Rows:    []model.RelationRow{{Cells: []model.Expression{lit("1")}}},
	}
	if _, err := Compile(rel, feel.NewEnv(), nil); err == nil {
		t.Error("row with fewer cells than columns should be a compile error")
	}
}

func TestBoxedRelationCellError(t *testing.T) {
	rel := &model.RelationExpr{
		Columns: []string{"a"},
		Rows:    []model.RelationRow{{Cells: []model.Expression{lit("1 +")}}},
	}
	if _, err := Compile(rel, feel.NewEnv(), nil); err == nil {
		t.Error("malformed relation cell should be a compile error")
	}
}

func TestBoxedConditional(t *testing.T) {
	cond := &model.Conditional{If: lit("x > 5"), Then: lit(`"hi"`), Else: lit(`"lo"`)}
	env := feel.NewEnv("x")
	for in, want := range map[string]string{"10": "hi", "1": "lo"} {
		got, err := evalExpr(t, cond, env, nil, map[string]value.Value{"x": value.MustNumber(in)})
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != want {
			t.Errorf("conditional(x=%s) = %s, want %s", in, got, want)
		}
	}
}

func TestBoxedConditionalMissingElseIsNull(t *testing.T) {
	cond := &model.Conditional{If: lit("false"), Then: lit("1")}
	got, err := evalExpr(t, cond, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !value.IsNull(got) {
		t.Errorf("missing else = %s, want null", got)
	}
}

func TestBoxedConditionalErrors(t *testing.T) {
	cases := map[string]*model.Conditional{
		"bad if":   {If: lit("1 +"), Then: lit("1"), Else: lit("2")},
		"bad then": {If: lit("true"), Then: lit("1 +"), Else: lit("2")},
		"bad else": {If: lit("true"), Then: lit("1"), Else: lit("2 *")},
	}
	for name, c := range cases {
		if _, err := Compile(c, feel.NewEnv(), nil); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
}

func TestBoxedFor(t *testing.T) {
	f := &model.ForExpr{IteratorVariable: "x", In: lit("[1, 2, 3]"), Return: lit("x * 10")}
	got, err := evalExpr(t, f, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[10, 20, 30]" {
		t.Errorf("for = %s, want [10, 20, 30]", got)
	}
}

func TestBoxedForErrors(t *testing.T) {
	cases := map[string]*model.ForExpr{
		"no var":     {In: lit("[1]"), Return: lit("1")},
		"bad in":     {IteratorVariable: "x", In: lit("[1"), Return: lit("x")},
		"bad return": {IteratorVariable: "x", In: lit("[1]"), Return: lit("y")},
	}
	for name, f := range cases {
		if _, err := Compile(f, feel.NewEnv(), nil); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
}

func TestBoxedQuantified(t *testing.T) {
	every := &model.Quantified{Kind: "every", IteratorVariable: "x", In: lit("[1, 2, 3]"), Satisfies: lit("x > 0")}
	if got, _ := evalExpr(t, every, feel.NewEnv(), nil, nil); got != value.True {
		t.Errorf("every x>0 = %s, want true", got)
	}
	some := &model.Quantified{Kind: "some", IteratorVariable: "x", In: lit("[1, 2, 3]"), Satisfies: lit("x > 2")}
	if got, _ := evalExpr(t, some, feel.NewEnv(), nil, nil); got != value.True {
		t.Errorf("some x>2 = %s, want true", got)
	}
	none := &model.Quantified{Kind: "some", IteratorVariable: "x", In: lit("[1, 2]"), Satisfies: lit("x > 5")}
	if got, _ := evalExpr(t, none, feel.NewEnv(), nil, nil); got != value.False {
		t.Errorf("some x>5 = %s, want false", got)
	}
}

func TestBoxedQuantifiedErrors(t *testing.T) {
	cases := map[string]*model.Quantified{
		"no var":        {Kind: "every", In: lit("[1]"), Satisfies: lit("true")},
		"bad in":        {Kind: "every", IteratorVariable: "x", In: lit("[1"), Satisfies: lit("true")},
		"bad satisfies": {Kind: "some", IteratorVariable: "x", In: lit("[1]"), Satisfies: lit("z")},
	}
	for name, q := range cases {
		if _, err := Compile(q, feel.NewEnv(), nil); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
}

func TestBoxedFilter(t *testing.T) {
	f := &model.FilterExpr{In: lit("[1, 2, 3, 4]"), Match: lit("item > 2")}
	got, err := evalExpr(t, f, feel.NewEnv(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "[3, 4]" {
		t.Errorf("filter = %s, want [3, 4]", got)
	}
}

func TestBoxedFilterErrors(t *testing.T) {
	cases := map[string]*model.FilterExpr{
		"bad in":            {In: lit("[1"), Match: lit("item > 0")},
		"bad match":         {In: lit("[1]"), Match: lit("item >")},
		"non-literal match": {In: lit("[1]"), Match: &model.ListExpr{}},
	}
	for name, f := range cases {
		if _, err := Compile(f, feel.NewEnv(), nil); err == nil {
			t.Errorf("%s: expected a compile error", name)
		}
	}
}
