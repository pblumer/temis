// Package boxed compiles DMN boxed expressions (decision tables, contexts,
// invocations, lists, relations, functions/BKM and the 1.4+/1.5 conditional,
// iterator and filter forms) into FEEL closures, reusing internal/feel.
//
// Each boxed expression compiles down to the same CompiledExpr signature that
// backs FEEL, keeping evaluation uniform and fast.
//
// This package is currently a scaffold (WP-01); decision tables start at WP-08.
package boxed
