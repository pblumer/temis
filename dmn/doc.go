// Package dmn is the single public entry point of the Temis DMN engine.
//
// It exposes the two-phase API described in docs/40-api-contract.md: an
// [Engine] compiles DMN models once into immutable, thread-safe
// CompiledDecisions, which are then evaluated cheaply and repeatedly against
// an input context.
//
// Everything under internal/ is private and may change freely; the service/
// and cmd/ packages access the engine exclusively through this package.
//
// Typical use:
//
//	eng := dmn.New()
//	defs, diags, err := eng.Compile(ctx, xmlBytes)
//	if err != nil || diags.HasErrors() { /* handle */ }
//	dec, err := defs.Decision("Dish")
//	res, err := dec.Evaluate(ctx, dmn.Input{"Season": "Winter", "Guest Count": 8})
//	fmt.Println(res.Outputs["Dish"])
//
// Evaluating a decision automatically evaluates the decisions it requires and
// feeds their results in by name, so the caller supplies only the leaf input
// data; a required result passed in directly is used as given. Result.Decisions
// reports every decision evaluated.
//
// A [Definitions.Service] returns a compiled decision service, whose Evaluate
// runs its output decisions (and any encapsulated decisions) while treating its
// input decisions as caller-supplied boundaries.
//
// # Stability
//
// The exported surface of this package is the SemVer-stable v1 contract
// (ADR-0011, ADR-0019). Additive changes ship in a minor release; a breaking
// change requires a major version. A symbol scheduled for removal is first
// marked // Deprecated: and dropped no earlier than the next major. Everything
// under internal/ is exempt and may change at any time. See
// docs/40-api-contract.md §4; the surface is frozen by a golden test
// (apisurface_test.go).
package dmn
