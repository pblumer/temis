//go:build js && wasm

// Command feel-wasm compiles temis' FEEL front-end to WebAssembly and exposes
// it to the browser as two synchronous validation functions. It is the
// Gate-2 spike for ADR-0016: it proves that the *real* engine — the same
// parser/compiler that later evaluates the model — can validate FEEL cells
// live in an in-browser editor, offline, with no server round-trip.
//
//	window.temisFeelValidate(expr, inputNamesCsv)       // output / literal expression
//	window.temisFeelValidateUnary(test, inputNamesCsv)  // decision-table input cell (unary test)
//
// Each returns { ok: bool } on success or { ok: false, line, col, message }
// with the engine's 1-based line/col diagnostic on failure.
package main

import (
	"strings"
	"syscall/js"

	"github.com/pblumer/temis/internal/feel"
)

// splitNames turns "Season, Guest Count" into ["Season", "Guest Count"], the
// decision's input variables that a cell expression may reference.
func splitNames(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// diag maps an engine error to the JS result object. ParseError and
// CompileError both carry a 1-based source position; anything else degrades to
// 0:0 with its message.
func diag(err error) map[string]any {
	if err == nil {
		return map[string]any{"ok": true}
	}
	// ParseError/CompileError carry a structured position plus a bare message;
	// hand back the position separately so the page renders "line:col" once
	// (err.Error() would prefix it again).
	line, col, msg := 0, 0, err.Error()
	switch e := err.(type) {
	case *feel.ParseError:
		line, col, msg = e.Line, e.Col, e.Msg
	case *feel.CompileError:
		line, col, msg = e.Line, e.Col, e.Msg
	}
	return map[string]any{"ok": false, "line": line, "col": col, "message": msg}
}

// validateOutput checks a full FEEL expression (output/literal cell) against
// the given input names. CompileString runs parse + compile, so it catches both
// syntax errors and unknown-variable references.
func validateOutput(_ js.Value, args []js.Value) any {
	env := feel.NewEnv(splitNames(args[1].String())...)
	_, err := feel.CompileString(args[0].String(), env)
	return diag(err)
}

// validateUnary checks a decision-table input cell (a FEEL unary test, e.g.
// "> 10", "[1..5]", "\"Winter\", \"Spring\""). A unary test is implicitly a
// predicate over the cell's input value, so the env must define feel.InputVar
// ("?") — the decision-table compiler binds it per input column at runtime.
func validateUnary(_ js.Value, args []js.Value) any {
	env := feel.NewEnv(append([]string{feel.InputVar}, splitNames(args[1].String())...)...)
	_, err := feel.CompileUnaryTest(args[0].String(), env)
	return diag(err)
}

// nameOracle is a NameSet that knows exactly one name, so the parser can
// assemble it (including multi-word names and names containing FEEL keywords).
type nameOracle string

func (o nameOracle) Has(name string) bool { return name == string(o) }

// validateName checks whether a string is a valid, unambiguous FEEL variable
// name — i.e. the engine, told this name exists, parses the string back to
// exactly that single name reference. This permits spaces and keyword-words
// (e.g. "Profit and Loss") while rejecting FEEL operator characters (e.g.
// "test-rule" parses as test − rule, not a name). It is the engine's own rule,
// not a regex.
func validateName(_ js.Value, args []js.Value) any {
	name := strings.TrimSpace(args[0].String())
	if name == "" {
		return map[string]any{"ok": false, "message": "Name darf nicht leer sein"}
	}
	expr, err := feel.ParseWithNames(name, nameOracle(name))
	if err != nil {
		return map[string]any{"ok": false, "message": "kein gültiger FEEL-Name"}
	}
	ref, ok := expr.(*feel.NameRef)
	if !ok || ref.Name != name {
		return map[string]any{"ok": false, "message": "kein eindeutiger FEEL-Name — Sonderzeichen wie - / . + * sind nicht erlaubt"}
	}
	return map[string]any{"ok": true}
}

func main() {
	js.Global().Set("temisFeelValidate", js.FuncOf(validateOutput))
	js.Global().Set("temisFeelValidateUnary", js.FuncOf(validateUnary))
	js.Global().Set("temisFeelValidateName", js.FuncOf(validateName))
	select {} // keep the Go runtime alive so the exported funcs stay callable
}
