package boxed

import (
	"fmt"
	"strings"

	"github.com/pblumer/feel"
	"github.com/pblumer/temis/internal/model"
)

// Compile lowers a boxed expression into a FEEL closure over env. The funcs map
// supplies the user-defined functions (business knowledge models, function
// definitions) in scope, so an expression may call or reference them by name.
//
// It dispatches on the concrete expression form: a literal expression and a
// decision table compile as before (WP-06/WP-09); a context, invocation and
// function definition are the boxed forms added in WP-23/WP-24.
func Compile(expr model.Expression, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	switch e := expr.(type) {
	case *model.LiteralExpression:
		return feel.CompileStringWith(e.Text, env, funcs)
	case *model.DecisionTable:
		return CompileTable(e, env, funcs)
	case *model.ContextExpr:
		return compileContext(e, env, funcs)
	case *model.Invocation:
		return compileInvocation(e, env, funcs)
	case *model.FunctionDef:
		return compileFunctionDef(e, env, funcs)
	case *model.ListExpr:
		return compileList(e, env, funcs)
	case *model.RelationExpr:
		return compileRelation(e, env, funcs)
	case *model.Conditional:
		return compileConditional(e, env, funcs)
	case *model.ForExpr:
		return compileFor(e, env, funcs)
	case *model.Quantified:
		return compileQuantified(e, env, funcs)
	case *model.FilterExpr:
		return compileFilter(e, env, funcs)
	case nil:
		return nil, fmt.Errorf("no executable logic")
	default:
		return nil, fmt.Errorf("unsupported boxed expression %T", expr)
	}
}

// compileFunctionDef compiles a boxed function definition into a closure that
// yields a first-class function value. Only FEEL bodies are executable.
func compileFunctionDef(fn *model.FunctionDef, env *feel.Env, funcs map[string]*feel.Func) (feel.CompiledExpr, error) {
	if fn.Kind != "" && !strings.EqualFold(fn.Kind, "FEEL") {
		return nil, fmt.Errorf("function kind %q is not executable (only FEEL)", fn.Kind)
	}
	bodyEnv := env
	params := make([]string, len(fn.Parameters))
	for i, p := range fn.Parameters {
		params[i] = p.Name
		bodyEnv = bodyEnv.Append(p.Name)
	}
	body, err := Compile(fn.Body, bodyEnv, funcs)
	if err != nil {
		return nil, fmt.Errorf("function body: %w", err)
	}
	return feel.FuncValue(params, body), nil
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
