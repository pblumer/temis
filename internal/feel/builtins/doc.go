// Package builtins provides the FEEL built-in functions, organised one file
// per category (conversion, boolean, string, list, numeric, date/time, range,
// temporal, sort and context functions).
//
// Built-ins are plain Go functions registered in a static lookup table and
// bound at compile time, avoiding string dispatch on the hot path.
//
// This package is currently a scaffold (WP-01); built-ins start at WP-07.
package builtins
