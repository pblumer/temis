// Package boxed compiles DMN boxed expressions (decision tables, contexts,
// invocations, lists, relations, functions/BKM and the 1.4+/1.5 conditional,
// iterator and filter forms) into FEEL closures, reusing internal/feel.
//
// Each boxed expression compiles down to the same CompiledExpr signature that
// backs FEEL, keeping evaluation uniform and fast. Compile is the dispatch entry
// point; it threads a map of user-defined functions (business knowledge models,
// boxed function definitions) so any expression can call or reference them.
//
// Implemented: decision tables with hit policies U/A/F/R/C (WP-09); boxed
// contexts with a result cell, invocations of a business knowledge model and
// boxed function definitions (WP-23/WP-24); boxed lists and relations (WP-25);
// and the 1.4+/1.5 conditional, iterator (for/every/some) and filter forms
// (WP-26). The iterator and filter forms reuse internal/feel's iteration and
// filtering semantics via exported helpers, so boxed and literal FEEL stay
// behaviourally identical.
package boxed
