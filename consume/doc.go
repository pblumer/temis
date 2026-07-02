// Package consume turns a clio command event into a temis evaluation and result
// events written back to clio (ADR-0033) — the reverse direction of the audit
// sink (ADR-0023), which only pushes results out. A command event
// (com.temis.decision.requested.v1) names what to decide — a single decision, a
// whole model graph, or a decision-flow (DRG) — and its input; consume evaluates
// it over the public dmn/flow API and produces the same versioned result events
// temisd's sink writes (com.temis.decision.evaluated.v1 /
// com.temis.flow.evaluated.v1), filed under the command's subject and correlated
// back to the command by requestId. A failed evaluation produces a
// com.temis.decision.failed.v1 event so a command is always answered.
//
// Like package audit it is a read-only-core consumer over the public dmn package
// (ADR-0011): it imports no internal/ package, holds no durable state, and never
// waits, times or retries. clio owns all state — the command log, the result log
// and the "already answered?" query — so the core here stays a pure
// event→evaluate→event transform. That is what keeps temis on the decisioning
// side of the ADR-0025 boundary: it computes an answer now; it does not
// orchestrate a process. The reactive glue (the observe loop, the clio HTTP
// calls, the idempotent write-back) lives in cmd/temis-clio-worker.
package consume
