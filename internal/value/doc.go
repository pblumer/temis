// Package value is the FEEL/DMN runtime value model: the Value interface and its
// concrete kinds (null, boolean, number, string, the temporal types, list,
// context, range and function), together with equality, ordering and arithmetic
// that follow FEEL semantics — most importantly decimal numbers (never float64,
// ADR-0007) and pervasive null propagation.
//
// It is a foundational package imported by the FEEL compiler, the boxed
// expression and decision-table engines and the public API. It lives apart from
// package feel so that value names such as Number and Kind do not collide with
// the lexer's token kinds.
package value
