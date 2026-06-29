package model

// Bounds is an element's diagram position: top-left corner and size, in DMNDI
// coordinates (y grows downward, matching SVG).
type Bounds struct {
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// Diagram is the subset of DMNDI the tooling needs: element bounds keyed by the
// local DMN element id. It is populated from the preserved DMNDI subtree when
// the model carries one (HasDMNDI); nil otherwise.
type Diagram struct {
	Shapes map[string]Bounds
}
