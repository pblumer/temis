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
// This package is currently a scaffold (WP-01); the API is introduced in WP-10.
package dmn
