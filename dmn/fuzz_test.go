package dmn_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// FuzzCompile drives the full public surface — Compile, then Decision/Evaluate
// for every decision the model exposes — over arbitrary bytes (WP-44,
// docs/50-testing-strategy.md §3). It is the end-to-end counterpart to the
// component fuzzers (internal/xml.FuzzDecode here, plus the FEEL front-end
// fuzzers that now live in github.com/pblumer/feel — ADR-0039): malformed input
// must yield an error or diagnostics,
// never a panic, and tight Limits guarantee evaluation returns rather than
// hanging or exhausting memory on hostile input. Seeded from the DMN fixtures.
func FuzzCompile(f *testing.F) {
	entries, err := os.ReadDir(filepath.Join("testdata", "models"))
	if err != nil {
		f.Fatalf("read fixtures dir: %v", err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".dmn") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join("testdata", "models", e.Name())); err == nil {
			f.Add(data)
		}
	}

	eng := dmn.New(dmn.WithLimits(dmn.Limits{
		MaxCallDepth:  16,
		MaxIterations: 5000,
		MaxListSize:   5000,
	}))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		defs, _, err := eng.Compile(ctx, data)
		if err != nil {
			return // malformed document — a clean error, not a panic
		}
		if defs == nil {
			t.Fatalf("Compile returned nil Definitions and nil error")
		}
		// Exercise evaluation of every executable decision. Missing inputs and
		// type errors surface as errors; the tight limits bound any iteration so
		// even a hostile comprehension returns instead of hanging.
		for _, name := range defs.Index().Decisions {
			dec, derr := defs.Decision(name)
			if derr != nil {
				continue
			}
			_, _ = dec.Evaluate(ctx, dmn.Input{})
		}
	})
}
