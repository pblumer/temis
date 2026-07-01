package dmn

import (
	"strings"
	"testing"
	"time"

	"github.com/pblumer/temis/internal/feel"
	"github.com/pblumer/temis/internal/model"
	"github.com/pblumer/temis/internal/value"
)

// --- convert.go -----------------------------------------------------------

// TestToValueAllScalarKinds exercises every Go input type toValue maps, including
// the unsigned and float branches the round-trip table did not reach.
func TestToValueAllScalarKinds(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string // fromValue(toValue(in)) rendered for scalars
	}{
		{"int8", int8(-3), "-3"},
		{"int16", int16(300), "300"},
		{"int32", int32(-70000), "-70000"},
		{"uint8", uint8(255), "255"},
		{"uint16", uint16(65535), "65535"},
		{"uint32", uint32(4000000000), "4000000000"},
		{"uint", uint(7), "7"},
		{"uint64", uint64(18446744073709551615), "18446744073709551615"},
		{"float32", float32(1.5), "1.5"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, err := toValue(c.in)
			if err != nil {
				t.Fatalf("toValue(%v): %v", c.in, err)
			}
			if got := fromValue(v); got != c.want {
				t.Errorf("round-trip %v = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestToValuePassthroughAndTime covers the value.Value passthrough and time.Time
// branches.
func TestToValuePassthroughAndTime(t *testing.T) {
	v, err := toValue(value.Str("hi"))
	if err != nil {
		t.Fatal(err)
	}
	if fromValue(v) != "hi" {
		t.Errorf("passthrough = %v, want hi", fromValue(v))
	}

	tm := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	tv, err := toValue(tm)
	if err != nil {
		t.Fatal(err)
	}
	if value.IsNull(tv) {
		t.Error("time.Time converted to null")
	}
}

// TestToValueNestedErrorPropagation covers the error branches in the list and
// context cases, where a nested unsupported value fails.
func TestToValueNestedErrorPropagation(t *testing.T) {
	if _, err := toValue([]any{struct{}{}}); err == nil {
		t.Error("expected error for unsupported list element")
	}
	if _, err := toValue(map[string]any{"k": struct{}{}}); err == nil {
		t.Error("expected error for unsupported context value")
	}
}

// TestInputToValuesError covers the error path of inputToValues.
func TestInputToValuesError(t *testing.T) {
	if _, err := inputToValues(Input{"x": struct{}{}}); err == nil {
		t.Error("expected error for unsupported input value")
	}
	if _, err := inputToValues(Input{"x": 1}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestFromValueListAndContext covers the list and context recursion in fromValue.
func TestFromValueListAndContext(t *testing.T) {
	lst := value.NewList(value.Str("a"), value.NumberFromInt64(2))
	got := fromValue(lst)
	gotList, ok := got.([]any)
	if !ok || len(gotList) != 2 || gotList[0] != "a" || gotList[1] != "2" {
		t.Errorf("fromValue(list) = %#v", got)
	}

	ctx := value.NewContext()
	ctx.Put("k", value.BoolOf(true))
	gotCtx := fromValue(ctx)
	m, ok := gotCtx.(map[string]any)
	if !ok || m["k"] != true {
		t.Errorf("fromValue(context) = %#v", gotCtx)
	}
}

// TestFromValueTemporalFallback covers the default branch (temporal canonical form).
func TestFromValueTemporalFallback(t *testing.T) {
	dt := value.NewDateTime(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	if s, ok := fromValue(dt).(string); !ok || s == "" {
		t.Errorf("fromValue(datetime) = %#v, want non-empty string", fromValue(dt))
	}
}

// --- schema.go ------------------------------------------------------------

// TestInputErrorMessages covers both the single-problem and multi-problem
// rendering of InputError.Error.
func TestInputErrorMessages(t *testing.T) {
	one := &InputError{Problems: []InputProblem{{Message: "bad"}}}
	if got := one.Error(); got != "dmn: invalid input: bad" {
		t.Errorf("single = %q", got)
	}
	two := &InputError{Problems: []InputProblem{{Message: "a"}, {Message: "b"}}}
	got := two.Error()
	if !strings.Contains(got, "2 input problems") || !strings.Contains(got, "a; b") {
		t.Errorf("multi = %q", got)
	}
}

// TestCanonicalTypeAllBranches exercises every recognised type word plus the
// namespace-prefixed and unknown paths.
func TestCanonicalTypeAllBranches(t *testing.T) {
	cases := map[string]string{
		"number":                 "number",
		"feel:string":            "string",
		"boolean":                "boolean",
		"date":                   "date",
		"time":                   "time",
		"dateTime":               "date and time",
		"date and time":          "date and time",
		"dayTimeDuration":        "duration",
		"yearMonthDuration":      "duration",
		"daysAndTimeDuration":    "duration",
		"yearsAndMonthsDuration": "duration",
		"duration":               "duration",
		"Person":                 "",
		"":                       "",
	}
	for in, want := range cases {
		if got := canonicalType(in); got != want {
			t.Errorf("canonicalType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestSchemaTypeName covers the item-definition fallback and the unknown path.
func TestSchemaTypeName(t *testing.T) {
	items := map[string]*feel.Type{"Person": nil}
	items["Person"] = feel.ContextOf(map[string]*feel.Type{})
	if got := schemaTypeName("number", items); got != "number" {
		t.Errorf("builtin = %q", got)
	}
	if got := schemaTypeName("ns:Person", items); got != "Person" {
		t.Errorf("item = %q", got)
	}
	if got := schemaTypeName("Unknown", items); got != "" {
		t.Errorf("unknown = %q, want empty", got)
	}
}

// TestGoKindAllBranches covers every kind goKind reports, including the default.
func TestGoKindAllBranches(t *testing.T) {
	cases := []struct {
		v    any
		want string
	}{
		{nil, "null"},
		{true, "boolean"},
		{"s", "string"},
		{42, "number"},
		{int64(1), "number"},
		{uint(1), "number"},
		{float64(1.5), "number"},
		{time.Now(), "date and time"},
		{[]any{1}, "list"},
		{map[string]any{"a": 1}, "context"},
	}
	for _, c := range cases {
		if got := goKind(c.v); got != c.want {
			t.Errorf("goKind(%T) = %q, want %q", c.v, got, c.want)
		}
	}
	// default branch: an unsupported type renders as its Go type name.
	if got := goKind(struct{}{}); !strings.Contains(got, "struct") {
		t.Errorf("goKind(struct) = %q", got)
	}
}

// TestTypeMismatchBranches covers every expected-type arm of typeMismatch.
func TestTypeMismatchBranches(t *testing.T) {
	cases := []struct {
		expected string
		v        any
		bad      bool
	}{
		{"", 1, false},                // no constraint
		{"number", nil, false},        // null never clashes
		{"number", 5, false},          // ok
		{"number", "5", true},         // clash
		{"string", "x", false},        // ok
		{"boolean", true, false},      // ok
		{"date", "2026-01-01", false}, // temporal accepts string
		{"date", 5, true},             // temporal clash
		{"time", "12:00:00", false},
		{"duration", "P1D", false},
		{"date and time", "2026-01-01T00:00:00", false},
		{"date and time", time.Now(), false}, // accepts the date-and-time kind
		{"date and time", 5, true},
		{"customType", 5, false}, // default arm: no constraint
	}
	for _, c := range cases {
		_, _, bad := typeMismatch(c.expected, c.v)
		if bad != c.bad {
			t.Errorf("typeMismatch(%q, %v) bad=%v, want %v", c.expected, c.v, bad, c.bad)
		}
	}
}

// TestQuoteNames covers the empty and populated forms.
func TestQuoteNames(t *testing.T) {
	if got := quoteNames(nil); got != "(none)" {
		t.Errorf("empty = %q", got)
	}
	got := quoteNames([]InputField{{Name: "A"}, {Name: "B"}})
	if got != `"A", "B"` {
		t.Errorf("names = %q", got)
	}
}

// TestMergeDistinct covers the dedup helper directly.
func TestMergeDistinct(t *testing.T) {
	got := mergeDistinct([]string{"a", "b"}, []string{"b", "c"})
	want := []string{"a", "b", "c"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("mergeDistinct = %v, want %v", got, want)
	}
}

// TestSuggestValues covers the closed enumeration, partial-literal and
// cell-only branches plus the empty fallback.
func TestSuggestValues(t *testing.T) {
	if vs, closed := suggestValues(`"a","b"`, nil); !closed || len(vs) != 2 {
		t.Errorf("closed enum: %v closed=%v", vs, closed)
	}
	// Partial: a literal plus a range -> not complete, merges with cells.
	if vs, closed := suggestValues(`"a",[1..5]`, []string{"z"}); closed || len(vs) != 2 {
		t.Errorf("partial: %v closed=%v", vs, closed)
	}
	// No constraint, only cell literals.
	if vs, closed := suggestValues("", []string{"x"}); closed || len(vs) != 1 {
		t.Errorf("cell-only: %v closed=%v", vs, closed)
	}
	// Nothing to suggest.
	if vs, closed := suggestValues("", nil); closed || vs != nil {
		t.Errorf("empty: %v closed=%v", vs, closed)
	}
	// A constraint with no literal parts (a pure range) yields no values.
	if vs, _ := suggestValues("[1..5]", nil); vs != nil {
		t.Errorf("range-only constraint should yield no values, got %v", vs)
	}
}

// TestLiteralsInAndValue covers the literal-extraction helpers, including the
// quoted-with-internal-quote rejection and a non-literal part.
func TestLiteralsInAndValue(t *testing.T) {
	vs, complete := literalsIn(`"a", 3, > 5`)
	if complete || len(vs) != 2 {
		t.Errorf("literalsIn = %v complete=%v", vs, complete)
	}
	if _, complete := literalsIn(""); complete {
		t.Error("empty literalsIn should not be complete")
	}

	if v, ok := literalValue(`"x"`); !ok || v != "x" {
		t.Errorf("string literal = %q ok=%v", v, ok)
	}
	if v, ok := literalValue("3.5"); !ok || v != "3.5" {
		t.Errorf("number literal = %q ok=%v", v, ok)
	}
	if _, ok := literalValue(`"a"b"`); ok {
		t.Error("string with embedded quote should not be a literal value")
	}
	if _, ok := literalValue("> 5"); ok {
		t.Error("comparison should not be a literal value")
	}
}

// --- table.go -------------------------------------------------------------

// TestHitPolicyXMLAllBranches covers every hit-policy spelling.
func TestHitPolicyXMLAllBranches(t *testing.T) {
	cases := map[string]string{
		"U": "UNIQUE", "unique": "UNIQUE",
		"A": "ANY", "any": "ANY",
		"P": "PRIORITY", "priority": "PRIORITY",
		"F": "FIRST", "first": "FIRST",
		"R": "RULE ORDER", "rule_order": "RULE ORDER", "rule order": "RULE ORDER",
		"O": "OUTPUT ORDER", "output_order": "OUTPUT ORDER",
		"C": "COLLECT", "collect": "COLLECT",
		"": "", "weird": "",
	}
	for in, want := range cases {
		if got := hitPolicyXML(in); got != want {
			t.Errorf("hitPolicyXML(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestAggregationFor covers the Collect-only gating and each aggregation word.
func TestAggregationFor(t *testing.T) {
	for _, agg := range []string{"SUM", "count", "Min", "MAX"} {
		if got := aggregationFor(TableEdit{HitPolicy: "C", Aggregation: agg}); got != strings.ToUpper(agg) {
			t.Errorf("aggregationFor(%q) = %q", agg, got)
		}
	}
	if got := aggregationFor(TableEdit{HitPolicy: "C", Aggregation: "bogus"}); got != "" {
		t.Errorf("bogus aggregation = %q, want empty", got)
	}
	// Not Collect: aggregation is dropped.
	if got := aggregationFor(TableEdit{HitPolicy: "U", Aggregation: "SUM"}); got != "" {
		t.Errorf("non-collect aggregation = %q, want empty", got)
	}
}

// TestFitPadTruncatePassthrough covers all three fit behaviours.
func TestFitPadTruncatePassthrough(t *testing.T) {
	if got := fit([]string{"a"}, -1, "-"); len(got) != 1 {
		t.Errorf("passthrough = %v", got)
	}
	if got := fit([]string{"a"}, 3, "-"); strings.Join(got, ",") != "a,-,-" {
		t.Errorf("pad = %v", got)
	}
	if got := fit([]string{"a", "b", "c"}, 2, "-"); strings.Join(got, ",") != "a,b" {
		t.Errorf("truncate = %v", got)
	}
}

// TestDropTrailingEmpty covers the trailing-empty trimming.
func TestDropTrailingEmpty(t *testing.T) {
	if got := dropTrailingEmpty([]string{"a", "", ""}); len(got) != 1 || got[0] != "a" {
		t.Errorf("dropTrailingEmpty = %v", got)
	}
	if got := dropTrailingEmpty([]string{"", ""}); len(got) != 0 {
		t.Errorf("all-empty = %v", got)
	}
	if got := dropTrailingEmpty([]string{"a", "b"}); len(got) != 2 {
		t.Errorf("no-trailing = %v", got)
	}
}

// --- diagnostics.go -------------------------------------------------------

// TestSeverityString covers every Severity rendering, including the default.
func TestSeverityString(t *testing.T) {
	cases := map[Severity]string{
		SevError:     "error",
		SevWarning:   "warning",
		SevInfo:      "info",
		Severity(99): "unknown",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Severity(%d).String() = %q, want %q", s, got, want)
		}
	}
}

// TestFromModelSeverity covers every model-severity mapping including the default.
func TestFromModelSeverity(t *testing.T) {
	if fromModelSeverity(model.SeverityError) != SevError {
		t.Error("error mapping")
	}
	if fromModelSeverity(model.SeverityWarning) != SevWarning {
		t.Error("warning mapping")
	}
	// A severity outside the known set falls through to info.
	if fromModelSeverity(model.Severity(99)) != SevInfo {
		t.Error("default mapping should be info")
	}
}

// --- evalerror.go / definitions.go ---------------------------------------

// TestEvalErrorError covers both the with- and without-decision rendering.
func TestEvalErrorError(t *testing.T) {
	with := &EvalError{Code: "X", DecisionID: "d1", Message: "boom"}
	if got := with.Error(); got != `dmn: X: decision "d1": boom` {
		t.Errorf("with decision = %q", got)
	}
	without := &EvalError{Code: "X", Message: "boom"}
	if got := without.Error(); got != "dmn: X: boom" {
		t.Errorf("without decision = %q", got)
	}
}

// --- engine.go ------------------------------------------------------------

// TestDecisionLabelFallsBackToID covers the unnamed-decision branch of
// decisionLabel, and label for compiled decisions.
func TestDecisionLabelFallsBackToID(t *testing.T) {
	if got := decisionLabel(&model.Decision{ID: "d1"}); got != "d1" {
		t.Errorf("unnamed decisionLabel = %q, want d1", got)
	}
	if got := decisionLabel(&model.Decision{ID: "d1", Name: "Named"}); got != "Named" {
		t.Errorf("named decisionLabel = %q", got)
	}
	if got := label(&CompiledDecision{id: "c1"}); got != "c1" {
		t.Errorf("unnamed label = %q", got)
	}
	if got := label(&CompiledDecision{id: "c1", name: "N"}); got != "N" {
		t.Errorf("named label = %q", got)
	}
	if got := serviceLabel(&model.DecisionService{ID: "s1"}); got != "s1" {
		t.Errorf("unnamed serviceLabel = %q", got)
	}
	if got := serviceLabel(&model.DecisionService{ID: "s1", Name: "Svc"}); got != "Svc" {
		t.Errorf("named serviceLabel = %q", got)
	}
}

// --- constraint.go --------------------------------------------------------

// TestStructurallyConforms covers the context/list/scalar arms.
func TestStructurallyConforms(t *testing.T) {
	ctxType := feel.ContextOf(map[string]*feel.Type{})
	listType := feel.ListOf(nil)

	if !structurallyConforms(ctxType, value.NewContext()) {
		t.Error("context conforms to context type")
	}
	if structurallyConforms(ctxType, value.Str("x")) {
		t.Error("string should not conform to context type")
	}
	if !structurallyConforms(listType, value.NewList()) {
		t.Error("list conforms to list type")
	}
	if structurallyConforms(listType, value.Str("x")) {
		t.Error("string should not conform to list type")
	}
	// A scalar type imposes nothing structurally (default arm): a number type
	// accepts any value here, scalar conformance being left to the canonical check.
	numType, _ := feel.BuiltinType("number")
	if numType != nil && !structurallyConforms(numType, value.Str("x")) {
		t.Error("scalar/default type should impose no structural constraint")
	}
}

// TestInputConstraintCheck exercises the check method across its problem arms.
func TestInputConstraintCheck(t *testing.T) {
	// Null is treated as absent.
	c := &inputConstraint{}
	if p := c.check("x", value.Null); p != nil {
		t.Errorf("null should be nil, got %+v", p)
	}

	// Structural type mismatch (expects context, gets string).
	ctxType := feel.ContextOf(map[string]*feel.Type{})
	cs := &inputConstraint{typ: ctxType}
	if p := cs.check("x", value.Str("nope")); p == nil || p.Code != "TYPE_MISMATCH" {
		t.Errorf("want structural TYPE_MISMATCH, got %+v", p)
	}
	// A conforming context passes the structural check.
	if p := cs.check("x", value.NewContext()); p != nil {
		t.Errorf("conforming context should pass, got %+v", p)
	}

	// Allowed-values matcher: a value outside the set is VALUE_NOT_ALLOWED.
	m, err := feel.CompileUnaryTest(`"a","b"`, unaryEnv)
	if err != nil {
		t.Fatal(err)
	}
	ca := &inputConstraint{allowedText: `"a","b"`, matcher: m}
	if p := ca.check("x", value.Str("c")); p == nil || p.Code != "VALUE_NOT_ALLOWED" {
		t.Errorf("want VALUE_NOT_ALLOWED, got %+v", p)
	}
	if p := ca.check("x", value.Str("a")); p != nil {
		t.Errorf("allowed value should pass, got %+v", p)
	}
}

// --- typecheck.go ---------------------------------------------------------

// TestResolveType covers the empty, builtin and item-definition arms.
func TestResolveType(t *testing.T) {
	items := map[string]*feel.Type{"Person": feel.ContextOf(map[string]*feel.Type{})}
	if resolveType("", items) != nil {
		t.Error("empty ref should be nil/Any")
	}
	if resolveType("number", items) == nil {
		t.Error("builtin should resolve")
	}
	if resolveType("Person", items) == nil {
		t.Error("item definition should resolve")
	}
	if resolveType("Nope", items) != nil {
		t.Error("unknown should be nil/Any")
	}
}
