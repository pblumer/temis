// Package comparison holds the Temis side of the 1:1 Drools comparison. It
// evaluates the SAME model files (../models/*.dmn) the Drools JMH harness runs,
// with the same inputs, so the two engines are measured on identical work.
//
// Reproduce:
//
//	go test -run=^$ -bench='Latency'    -benchmem ./     # single-core latency + allocs
//	go test -run=^$ -bench='Throughput' -benchtime=2s ./ # parallel dec/s
//
// See ../README.md for the full methodology and the Drools side.
package comparison

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// scenario is one benchmarked model: which decision, the inputs, and the exact
// expected output. The want-check is the parity guard — the identical assertion
// the Drools harness applies — so neither engine can silently drift.
type scenario struct {
	file     string
	decision string
	in       dmn.Input
	want     any
}

var scenarios = map[string]scenario{
	"StringTable":  {"string-table.dmn", "Menu", dmn.Input{"Season": "Winter", "Region": "R8"}, "m8"},
	"NumericTable": {"numeric-table.dmn", "Grade", dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0}, "g5"},
	"Arithmetic":   {"arithmetic.dmn", "R", dmn.Input{"A": 6, "B": 7}, "21.5"},
	"DrgChain":     {"drg-chain.dmn", "D10", dmn.Input{"Seed": 0}, "10"},
	"CollectTable": {"collect-table.dmn", "Tags", dmn.Input{"Score": 5}, []any{"low", "mid", "spot"}},
}

func (sc scenario) compile(b *testing.B) *dmn.CompiledDecision {
	b.Helper()
	xml, err := os.ReadFile(filepath.Join("..", "models", sc.file))
	if err != nil {
		b.Fatal(err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil || diags.HasErrors() {
		b.Fatalf("compile %s: %v %+v", sc.file, err, diags)
	}
	dec, err := defs.Decision(sc.decision)
	if err != nil {
		b.Fatal(err)
	}
	// Parity guard: fail fast if the output is not exactly what Drools must also
	// produce for this scenario.
	res, err := dec.Evaluate(context.Background(), sc.in)
	if err != nil {
		b.Fatal(err)
	}
	if got := res.Outputs[sc.decision]; !reflect.DeepEqual(got, sc.want) {
		b.Fatalf("parity %s: %s = %#v, want %#v", sc.file, sc.decision, got, sc.want)
	}
	return dec
}

func latency(b *testing.B, name string) {
	sc := scenarios[name]
	dec := sc.compile(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, sc.in); err != nil {
			b.Fatal(err)
		}
	}
}

func throughput(b *testing.B, name string) {
	sc := scenarios[name]
	dec := sc.compile(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := dec.Evaluate(ctx, sc.in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()
	if s := b.Elapsed().Seconds(); s > 0 {
		b.ReportMetric(float64(b.N)/s, "dec/s")
	}
}

func BenchmarkLatencyStringTable(b *testing.B)  { latency(b, "StringTable") }
func BenchmarkLatencyNumericTable(b *testing.B) { latency(b, "NumericTable") }
func BenchmarkLatencyArithmetic(b *testing.B)   { latency(b, "Arithmetic") }
func BenchmarkLatencyDrgChain(b *testing.B)     { latency(b, "DrgChain") }
func BenchmarkLatencyCollectTable(b *testing.B) { latency(b, "CollectTable") }

func BenchmarkThroughputStringTable(b *testing.B)  { throughput(b, "StringTable") }
func BenchmarkThroughputNumericTable(b *testing.B) { throughput(b, "NumericTable") }
func BenchmarkThroughputArithmetic(b *testing.B)   { throughput(b, "Arithmetic") }
func BenchmarkThroughputDrgChain(b *testing.B)     { throughput(b, "DrgChain") }
func BenchmarkThroughputCollectTable(b *testing.B) { throughput(b, "CollectTable") }
