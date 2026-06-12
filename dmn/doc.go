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
// Decision-graph chaining, decision services and the remaining hit policies
// arrive in later work packages; see docs/20-roadmap.md.
package dmn
