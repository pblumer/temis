package dmn_test

import (
	"context"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// Throughput benchmarks report sustained decisions/second, the headline number
// for real deployments where a compiled decision is evaluated repeatedly from
// many goroutines. A CompiledDecision is immutable and safe for concurrent use
// (docs/10-architecture.md §5.2), so RunParallel measures aggregate throughput
// across GOMAXPROCS cores; the reported "dec/s" metric is total evaluations
// divided by wall-clock time. Reproduce with:
//
//	go test -run=^$ -bench=BenchmarkThroughput -benchtime=2s ./dmn/
//
// See docs/55-benchmarks.md for methodology and published figures.

// throughput evaluates dec concurrently across all cores and reports dec/s.
func throughput(b *testing.B, dec *dmn.CompiledDecision, in dmn.Input) {
	b.Helper()
	ctx := context.Background()
	// Warm one evaluation so a compile-time lazy path can't skew the first op.
	if _, err := dec.Evaluate(ctx, in); err != nil {
		b.Fatal(err)
	}
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
	if secs := b.Elapsed().Seconds(); secs > 0 {
		b.ReportMetric(float64(b.N)/secs, "dec/s")
	}
}

// singleCoreThroughput evaluates dec on one goroutine and reports dec/s, so the
// per-core figure can be read alongside the parallel one.
func singleCoreThroughput(b *testing.B, dec *dmn.CompiledDecision, in dmn.Input) {
	b.Helper()
	ctx := context.Background()
	if _, err := dec.Evaluate(ctx, in); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := dec.Evaluate(ctx, in); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	if secs := b.Elapsed().Seconds(); secs > 0 {
		b.ReportMetric(float64(b.N)/secs, "dec/s")
	}
}

func BenchmarkThroughputStringTable(b *testing.B) {
	dec := mustCompileDecision(b, stringTableModel(10), "Menu")
	throughput(b, dec, dmn.Input{"Season": "Winter", "Region": "R8"})
}

func BenchmarkThroughputStringTableSingleCore(b *testing.B) {
	dec := mustCompileDecision(b, stringTableModel(10), "Menu")
	singleCoreThroughput(b, dec, dmn.Input{"Season": "Winter", "Region": "R8"})
}

func BenchmarkThroughputMidTable(b *testing.B) {
	dec := mustCompileDecision(b, midTableModel(10), "Grade")
	throughput(b, dec, dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0})
}

func BenchmarkThroughputMidTableSingleCore(b *testing.B) {
	dec := mustCompileDecision(b, midTableModel(10), "Grade")
	singleCoreThroughput(b, dec, dmn.Input{"A": 55, "B": 0, "C": 0, "D": 0})
}

func BenchmarkThroughputArithmetic(b *testing.B) {
	dec := mustCompileDecision(b, []byte(arithmeticModel), "R")
	throughput(b, dec, dmn.Input{"A": 6, "B": 7})
}
