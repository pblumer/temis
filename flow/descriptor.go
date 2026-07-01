package flow

// Descriptor is the on-disk / on-wire form of a decision flow (ADR-0026). It is
// parsed and validated by Compile. The JSON media type is
// application/vnd.temis.flow+json and the file extension is ".flow.json".
type Descriptor struct {
	// Flow is the flow's identifier (stable name), used in audit events.
	Flow string `json:"flow"`
	// Version is the descriptor's own version, bumped on breaking shape changes.
	Version string `json:"version,omitempty"`
	// Inputs optionally declares the flow's own input schema. When present, the
	// compiler can validate that mapping references to flow inputs are known.
	Inputs []InputDecl `json:"inputs,omitempty"`
	// Steps are the composed decisions. Their reference graph — not their array
	// order — defines the evaluation order (a DAG; cycles are rejected).
	Steps []Step `json:"steps"`
	// Output assembles the flow's result: result key → reference. When empty, the
	// flow's outputs are the last evaluated step's outputs.
	Output map[string]string `json:"output,omitempty"`
}

// InputDecl declares one flow input: its name and optional FEEL type.
type InputDecl struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// Step evaluates one decision (or decision service) of one pinned model and
// exposes its outputs under the step's ID.
type Step struct {
	// ID names the step; its outputs are addressable as "ID.<output name>". Must
	// be unique within the flow and must not contain a '.'.
	ID string `json:"id"`
	// Model is the content-addressed modelId resolved via the Resolver.
	Model string `json:"model"`
	// Decision is the decision or decision-service name/id to evaluate.
	Decision string `json:"decision"`
	// In wires the target decision's inputs: input name → reference. A reference
	// is either a flow-input name or "stepID.output" of an earlier step.
	In map[string]string `json:"in,omitempty"`
}
