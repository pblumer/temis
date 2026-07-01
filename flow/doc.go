// Package flow evaluates a decision-flow descriptor: a stateless, deterministic
// composition of several DMN decisions across model boundaries (the L2a layer of
// ADR-0025, specified by ADR-0026).
//
// A flow is an external JSON artifact (not DMN XML) that wires several models —
// each pinned by its content-addressed modelId — into one evaluation. Steps form
// a directed acyclic graph: a step's inputs are drawn from the flow's inputs and
// from earlier steps' outputs. Because every model is pinned and evaluation is
// stateless, the whole flow is a pure function of its inputs and thus
// reproducible and re-auditable (ADR-0023).
//
// The package builds only on the stable dmn public API (no internal imports), so
// the frozen dmn v1 surface is untouched and flow carries its own SemVer track.
// A Resolver turns a modelId into a compiled model, letting the caller back the
// flow with a cache, a git source (package vcs) or an in-memory map.
//
// Scope (WP-90): step-input mappings are references — a flow-input name or a
// "stepID.output" reference into an earlier step's result. Full FEEL expressions
// in mappings are a deliberate follow-up: they need a public FEEL-evaluation
// primitive in package dmn, which is its own additive-surface decision.
package flow
