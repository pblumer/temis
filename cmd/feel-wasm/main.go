//go:build js && wasm

// Command feel-wasm compiles temis' FEEL front-end to WebAssembly and exposes
// it to the browser as two synchronous validation functions. It is the
// Gate-2 spike for ADR-0016: it proves that the *real* engine — the same
// parser/compiler that later evaluates the model — can validate FEEL cells
// live in an in-browser editor, offline, with no server round-trip.
//
//	window.temisFeelValidate(expr, inputNamesCsv, funcNamesCsv)       // output / literal expression
//	window.temisFeelValidateUnary(test, inputNamesCsv, funcNamesCsv)  // decision-table input cell (unary test)
//
// funcNamesCsv is an optional trailing argument: the names of the model's
// user-defined functions (its BKMs), comma-separated like the input names, so a
// call to one — a BKM's own recursive call included — validates as a known
// function rather than an unknown name. It may be omitted or empty when the model
// defines no functions.
//
// Each returns { ok: bool } on success or { ok: false, line, col, message }
// with the engine's 1-based line/col diagnostic on failure.
//
// It also exposes the engine's built-in catalog so the editor can offer code
// completion (variables come from the page, functions from here):
//
//	window.temisFeelBuiltins()  // [{ name, params: [...], variadic }, …]
package main

import (
	"sort"
	"strings"
	"syscall/js"

	"github.com/pblumer/feel"
	"github.com/pblumer/feel/builtins"
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

// modelFuncs turns the model's user-defined function names (its BKMs), passed as
// a comma-separated list like the input names, into the engine's function map so
// a call to one validates as a known function instead of an unknown name. This
// is the path that makes a BKM's own recursive call resolve: the body is checked
// with its own name in scope, exactly as compileBKMs does at evaluation time.
//
// Only the name matters here: it lets the parser assemble a multi-word BKM name
// as one reference (the name oracle) and the compiler bind the call to a known
// function. Argument arity is not an error either way (a mismatch evaluates to
// null, as it does at runtime), so the signature is left empty — the modeler
// carries the parameter names for its own completion hints, off this hot path.
// A missing or blank argument yields no functions (the pre-existing behaviour).
func modelFuncs(arg js.Value) map[string]*feel.Func {
	if arg.Type() != js.TypeString {
		return nil
	}
	names := splitNames(arg.String())
	if len(names) == 0 {
		return nil
	}
	funcs := make(map[string]*feel.Func, len(names))
	for _, n := range names {
		funcs[n] = &feel.Func{Name: n}
	}
	return funcs
}

// argAt returns args[i] or a JS undefined when the call passed fewer arguments,
// so an optional trailing argument (the model functions) stays backward
// compatible with callers that omit it.
func argAt(args []js.Value, i int) js.Value {
	if i < len(args) {
		return args[i]
	}
	return js.Undefined()
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
// the given input names, with the model's user-defined functions (BKMs, args[2])
// in scope so calls to them — a BKM's own recursion included — resolve as known
// functions. CompileStringWith runs parse + compile, so it catches both syntax
// errors and unknown-variable references.
func validateOutput(_ js.Value, args []js.Value) any {
	env := feel.NewEnv(splitNames(args[1].String())...)
	_, err := feel.CompileStringWith(args[0].String(), env, modelFuncs(argAt(args, 2)))
	return diag(err)
}

// validateUnary checks a decision-table input cell (a FEEL unary test, e.g.
// "> 10", "[1..5]", "\"Winter\", \"Spring\""). A unary test is implicitly a
// predicate over the cell's input value, so the env must define feel.InputVar
// ("?") — the decision-table compiler binds it per input column at runtime.
func validateUnary(_ js.Value, args []js.Value) any {
	env := feel.NewEnv(append([]string{feel.InputVar}, splitNames(args[1].String())...)...)
	_, err := feel.CompileUnaryTestWith(args[0].String(), env, modelFuncs(argAt(args, 2)))
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

// builtinsCatalog returns the FEEL built-in functions as a JS array of
// { name, params: [...], variadic } objects, so the in-browser editor can offer
// function completions with signature hints straight from the engine's own
// registry — never a hand-maintained list that could drift from the runtime.
func builtinsCatalog(_ js.Value, _ []js.Value) any {
	reg := builtins.Default()
	names := reg.Names()
	sort.Strings(names)
	out := make([]any, 0, len(names))
	for _, n := range names {
		b, ok := reg.Lookup(n)
		if !ok {
			continue
		}
		params := make([]any, len(b.Params))
		for i, p := range b.Params {
			params[i] = p
		}
		out = append(out, map[string]any{"name": b.Name, "params": params, "variadic": b.Variadic()})
	}
	return out
}

func main() {
	js.Global().Set("temisFeelValidate", js.FuncOf(validateOutput))
	js.Global().Set("temisFeelValidateUnary", js.FuncOf(validateUnary))
	js.Global().Set("temisFeelValidateName", js.FuncOf(validateName))
	js.Global().Set("temisFeelBuiltins", js.FuncOf(builtinsCatalog))
	select {} // keep the Go runtime alive so the exported funcs stay callable
}
