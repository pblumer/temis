// Package xml decodes DMN XML (1.5, tolerant towards 1.3/1.4) into the
// version-neutral domain model in internal/model.
//
// Decoding is namespace-tolerant and forward-compatible: unknown elements
// produce diagnostics rather than hard failures, and the DMNDI diagram layout
// is preserved so that files remain round-trippable for editors such as dmn-js.
//
// This package is currently a scaffold (WP-01); decoding is implemented in WP-02.
package xml
