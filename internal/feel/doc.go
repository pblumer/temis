// Package feel implements the FEEL expression language: the
// lexer, parser, AST, type system, type checker and the compiler that lowers
// expressions into reusable Go closures (CompiledExpr).
//
// The compiler follows the two-phase principle: expensive work happens once at
// compile time, while the resulting closure evaluates with minimal allocation
// on the hot path. Numbers are decimal (never float64); see ADR-0007.
//
// This package is currently a scaffold (WP-01); the lexer starts at WP-03.
package feel
