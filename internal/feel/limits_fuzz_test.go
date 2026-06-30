package feel

import "testing"

// FuzzBoundedEvaluation feeds arbitrary FEEL source through compilation and, when
// it compiles, evaluates it under tight resource limits. It asserts the engine
// never panics and — because the limits bound recursion, iteration and list
// growth (WP-34) — always returns rather than hanging or exhausting memory, even
// for hostile input such as a comprehension over a vast range.
func FuzzBoundedEvaluation(f *testing.F) {
	seeds := []string{
		"for i in 1..1000000000 return i",
		"for a in 1..100000 return (for b in 1..100000 return a * b)",
		"some x in 1..1000000000 satisfies x > 5",
		"every x in 1..1000000000 satisfies x > 0",
		"[1, 2, 3][item > 1]",
		"1 + 1",
		`upper case("abc")`,
		"if true then 1 else 2",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	lim := Limits{MaxCallDepth: 16, MaxIterations: 5000, MaxListSize: 5000}
	f.Fuzz(func(t *testing.T, src string) {
		ce, err := CompileString(src, NewEnv())
		if err != nil {
			return // not a valid expression in the empty environment
		}
		// Must not panic; the tight limits guarantee this returns.
		_, _ = ce(NewEnv().NewScopeWithLimits(nil, lim))
	})
}
