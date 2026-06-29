package dmn_test

import "testing"

// budget is one performance budget: a benchmark plus the ceilings a regression
// must not cross (docs/50-testing-strategy.md §6). allocs/op is the primary,
// machine-independent guard; ns/op is a generous ceiling that only trips on a
// catastrophic or complexity regression, not on shared-runner timing noise.
type budget struct {
	name      string
	bench     func(*testing.B)
	maxAllocs int64
	maxNs     int64
}

// TestPerformanceBudget is the WP-42 CI gate: it runs the hot-path benchmarks
// and fails when one exceeds its budget. It is skipped under the race detector
// (which distorts both metrics); the race-free `make budget` target enforces it.
func TestPerformanceBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("performance budget runs without the race detector; use `make budget`")
	}

	budgets := []budget{
		// Warm evaluation is the critical path: low single-digit µs, stable allocs.
		{"EvaluateMidTable", BenchmarkEvaluateMidTable, 60, 80_000},
		{"EvaluateArithmetic", BenchmarkEvaluateArithmetic, 40, 60_000},
		// A 10-deep DRG chain stays roughly linear (~13 allocs per decision).
		{"EvaluateDRGChain10", BenchmarkEvaluateDRGChain10, 130, 150_000},
		// Compilation is one-off and uncritical; the ceiling only catches blow-ups.
		{"CompileMidTable", BenchmarkCompileMidTable, 5_000, 5_000_000},
	}

	for _, bg := range budgets {
		res := testing.Benchmark(bg.bench)
		allocs, ns := res.AllocsPerOp(), res.NsPerOp()
		t.Logf("%s: %d ns/op, %d allocs/op (budget: %d ns, %d allocs)",
			bg.name, ns, allocs, bg.maxNs, bg.maxAllocs)
		if allocs > bg.maxAllocs {
			t.Errorf("%s: %d allocs/op exceeds budget %d — a hot-path allocation regression", bg.name, allocs, bg.maxAllocs)
		}
		if ns > bg.maxNs {
			t.Errorf("%s: %d ns/op exceeds budget %d — a performance regression", bg.name, ns, bg.maxNs)
		}
	}
}
