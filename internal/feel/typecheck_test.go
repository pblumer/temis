package feel

import "testing"

// checkSrc parses src and type-checks it against env, returning the findings.
func checkSrc(t *testing.T, src string, env *TypeEnv) []TypeError {
	t.Helper()
	expr, err := Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	return Typecheck(expr, env)
}

func TestTypecheckClean(t *testing.T) {
	env := NewTypeEnv().Set("age", TNumber).Set("name", TString).Set("ok", TBoolean)
	// Each of these is well-typed (or involves an unknown), so must be silent.
	for _, src := range []string{
		`age + 1`,
		`age >= 18`,
		`if age >= 18 then "adult" else "minor"`,
		`ok and age > 0`,
		`name = "x"`,
		`age + unknownVar`, // unknown operand → Any → no flag
		`[1, 2, 3]`,
		`upper case(name)`,
		`some x in [1, 2, 3] satisfies x > age`,
		`for x in [1, 2] return x * age`,
		`5 instance of number`,
		`{a: 1, b: a + 1}`,
	} {
		if errs := checkSrc(t, src, env); len(errs) != 0 {
			t.Errorf("%q: unexpected findings %v", src, errs)
		}
	}
}

func TestTypecheckFindings(t *testing.T) {
	env := NewTypeEnv().Set("age", TNumber).Set("name", TString).Set("ok", TBoolean)
	cases := []struct {
		src  string
		want string // substring expected in the message
	}{
		{`age + name`, "cannot apply"},     // number + string
		{`age < name`, "cannot compare"},   // number vs string
		{`ok and name`, "needs a boolean"}, // string in `and`
		{`if name then 1 else 2`, "must be boolean"},
		{`-name`, "number or duration"}, // negate a string
		{`name * 2`, "cannot apply"},    // string in arithmetic
	}
	for _, c := range cases {
		errs := checkSrc(t, c.src, env)
		if len(errs) == 0 {
			t.Errorf("%q: expected a finding, got none", c.src)
			continue
		}
		if !contains(errs, c.want) {
			t.Errorf("%q: findings %v, want one containing %q", c.src, errs, c.want)
		}
	}
}

func TestTypecheckPositions(t *testing.T) {
	// The finding should point at the offending operator/operand, not 1:1.
	errs := checkSrc(t, `age + name`, NewTypeEnv().Set("age", TNumber).Set("name", TString))
	if len(errs) == 0 || errs[0].Line != 1 || errs[0].Col == 0 {
		t.Errorf("expected a positioned finding, got %v", errs)
	}
}

func TestTypecheckContextFieldPath(t *testing.T) {
	// A path into a known context field carries that field's type through.
	env := NewTypeEnv().Set("p", ContextOf(map[string]*Type{"age": TNumber}))
	if errs := checkSrc(t, `p.age + 1`, env); len(errs) != 0 {
		t.Errorf("p.age + 1: unexpected findings %v", errs)
	}
	if errs := checkSrc(t, `p.age and true`, env); len(errs) == 0 {
		t.Error("p.age and true: expected a boolean finding (age is number)")
	}
}

func contains(errs []TypeError, sub string) bool {
	for _, e := range errs {
		if len(sub) == 0 || (len(e.Msg) >= len(sub) && indexOfStr(e.Msg, sub) >= 0) {
			return true
		}
	}
	return false
}

func indexOfStr(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestInstanceOfTypes(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`5 instance of number`, "true"},
		{`5 instance of string`, "false"},
		{`"x" instance of string`, "true"},
		{`true instance of boolean`, "true"},
		{`5 instance of Any`, "true"},
		{`null instance of Any`, "true"},
		{`[1] instance of list`, "true"},
		{`@"P1D" instance of duration`, "true"},
		{`@"P1D" instance of days and time duration`, "true"},
	}
	for _, c := range cases {
		ce, err := CompileString(c.src, NewEnv())
		if err != nil {
			t.Fatalf("compile %q: %v", c.src, err)
		}
		v, err := ce(NewEnv().NewScope(nil))
		if err != nil {
			t.Fatalf("eval %q: %v", c.src, err)
		}
		if v.String() != c.want {
			t.Errorf("%q = %s, want %s", c.src, v, c.want)
		}
	}
}

func TestInstanceOfUnknownTypeIsCompileError(t *testing.T) {
	if _, err := CompileString(`x instance of Frobnicate`, NewEnv("x")); err == nil {
		t.Error("instance of an unknown type should be a compile error")
	}
}

func TestTypecheckMoreBranches(t *testing.T) {
	env := NewTypeEnv().Set("age", TNumber).Set("name", TString).Set("when", TDate)
	// Clean cases across more node kinds.
	for _, src := range []string{
		`age between 1 and 10`,
		`age in (1, 2, 3)`,
		`when instance of date`,
		`-age`,
		`name.length`, // path on a non-context → Any, no flag
		`@"2024-01-01" instance of date`,
	} {
		if errs := checkSrc(t, src, env); len(errs) != 0 {
			t.Errorf("%q: unexpected findings %v", src, errs)
		}
	}
	// Mismatches in between/comparison.
	if errs := checkSrc(t, `age between name and 10`, env); len(errs) == 0 {
		t.Error("age between name and 10: expected a finding")
	}
}

func TestTypecheckString(t *testing.T) {
	env := NewTypeEnv().Set("age", TNumber)
	if errs := TypecheckString(`age + 1`, env, nil); len(errs) != 0 {
		t.Errorf("clean: %v", errs)
	}
	if errs := TypecheckString(`age and true`, env, nil); len(errs) == 0 {
		t.Error("age and true: expected a boolean finding")
	}
	// A syntax error yields no findings (the compile path reports it).
	if errs := TypecheckString(`1 +`, env, nil); len(errs) != 0 {
		t.Errorf("syntax error should yield no findings, got %v", errs)
	}
}

func TestTypeErrorErrorString(t *testing.T) {
	e := TypeError{Msg: "boom", Line: 2, Col: 3}
	if e.Error() != "2:3: boom" {
		t.Errorf("Error() = %q", e.Error())
	}
}

func TestBuiltinType(t *testing.T) {
	for _, name := range []string{"number", "feel:string", "date and time", "dateTime", "boolean", "list", "context"} {
		if _, ok := BuiltinType(name); !ok {
			t.Errorf("BuiltinType(%q) = not found, want a builtin", name)
		}
	}
	if _, ok := BuiltinType("MyType"); ok {
		t.Error("BuiltinType(MyType) should not resolve")
	}
}
