// Package comparison holds the Temis side of the 1:1 Drools comparison. It
// evaluates the SAME model files (../models/*.dmn) the Drools JMH harness runs,
// with the same inputs, so the two engines are measured on identical work.
//
// Reproduce:
//
//	go test -run=^$ -bench=. -benchmem ./          # single-core latency + allocs
//	go test -run=^$ -bench=Throughput -benchtime=2s ./
//
// See ../README.md for the full methodology and the Drools side.
package comparison

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func load(b *testing.B, file, decision string) *dmn.CompiledDecision {
	b.Helper()
	xml, err := os.ReadFile(filepath.Join("..", "models", file))
	if err != nil {
		b.Fatal(err)
	}
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil || diags.HasErrors() {
		b.Fatalf("compile %s: %v %+v", file, err, diags)
	}
	dec, err := defs.Decision(decision)
	if err != nil {
		b.Fatal(err)
	}
	return dec
}

// parity fails the benchmark if Temis does not produce the expected output, the
// same guard the Drools harness applies, so neither side can silently drift.
func parity(b *testing.B, dec *dmn.CompiledDecision, in dmn.Input, key, want string) {
	b.Helper()
	res, err := dec.Evaluate(context.Background(), in)
	if err != nil {
		b.Fatal(err)
	}
	if got := res.Outputs[key]; got != want {
		b.Fatalf("parity: %s = %v, want %v", key, got, want)
	}
}

func BenchmarkStringTable(b *testing.B) {
	dec := load(b, "string-table.dmn", "Menu")
	in := dmn.Input{"Season": "Winter", "Region": "R8"}
	parity(b, dec, in, "Menu", "m8")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNumericTable(b *testing.B) {
	dec := load(b, "numeric-table.dmn", "Grade")
	in := dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0}
	parity(b, dec, in, "Grade", "g5")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStringTableThroughput(b *testing.B) {
	dec := load(b, "string-table.dmn", "Menu")
	in := dmn.Input{"Season": "Winter", "Region": "R8"}
	parity(b, dec, in, "Menu", "m8")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := dec.Evaluate(ctx, in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()
	if s := b.Elapsed().Seconds(); s > 0 {
		b.ReportMetric(float64(b.N)/s, "dec/s")
	}
}

func BenchmarkNumericTableThroughput(b *testing.B) {
	dec := load(b, "numeric-table.dmn", "Grade")
	in := dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0}
	parity(b, dec, in, "Grade", "g5")
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := dec.Evaluate(ctx, in); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.StopTimer()
	if s := b.Elapsed().Seconds(); s > 0 {
		b.ReportMetric(float64(b.N)/s, "dec/s")
	}
}
