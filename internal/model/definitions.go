// Package model holds the version-neutral DMN domain model and the mapping from
// the decoded XML structs (internal/xml) into it.
//
// The model is the semantic projection consumed by the compiler and graph
// packages; diagram interchange (DMNDI) is not represented here because it is
// irrelevant to execution and is preserved at the XML layer instead.
package model

import "strconv"

// Version identifies the DMN specification version a model was authored against.
type Version int

// Supported DMN versions. Temis targets 1.5 and reads 1.3/1.4 (ADR-0002).
const (
	VersionUnknown Version = iota
	Version13
	Version14
	Version15
)

// String returns the dotted version, e.g. "1.5".
func (v Version) String() string {
	switch v {
	case Version13:
		return "1.3"
	case Version14:
		return "1.4"
	case Version15:
		return "1.5"
	default:
		return "unknown"
	}
}

// MarshalJSON renders the version as its dotted string for readable golden files.
func (v Version) MarshalJSON() ([]byte, error) {
	return []byte(strconv.Quote(v.String())), nil
}

// Definitions is the root of a DMN model: a set of decisions and the elements
// they depend on, all sharing one target namespace.
type Definitions struct {
	ID         string
	Name       string
	Namespace  string
	DMNVersion Version
	HasDMNDI   bool

	ItemDefinitions []*ItemDefinition  `json:",omitempty"`
	InputData       []*InputData       `json:",omitempty"`
	BKMs            []*BKM             `json:",omitempty"`
	Decisions       []*Decision        `json:",omitempty"`
	Services        []*DecisionService `json:",omitempty"`

	// Diagram carries the DMNDI element bounds when the model has a diagram, for
	// tooling that draws the DRG with the authored layout. Nil when absent.
	Diagram *Diagram `json:",omitempty"`
}
