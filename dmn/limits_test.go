package dmn_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pblumer/temis/dmn"
)

// readModel reads a testdata model file's bytes.
func readModel(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "models", file))
	if err != nil {
		t.Fatalf("read model %s: %v", file, err)
	}
	return data
}

// assertLimitExceeded checks that err is an *EvalError with CodeLimitExceeded.
func assertLimitExceeded(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("want a limit error, got nil")
	}
	var ee *dmn.EvalError
	if !errors.As(err, &ee) || ee.Code != dmn.CodeLimitExceeded {
		t.Fatalf("want EvalError{LIMIT_EXCEEDED}, got %T: %v", err, err)
	}
}

const bigForModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" namespace="http://temis.example/big" name="Big" id="def_big">
  <decision id="d_big" name="Big">
    <variable name="Big"/>
    <literalExpression><text>for i in 1..2000 return i</text></literalExpression>
  </decision>
</definitions>`

// TestIterationLimit covers WP-34: a comprehension exceeding the iteration
// budget fails with CodeLimitExceeded instead of running to completion.
func TestIterationLimit(t *testing.T) {
	eng := dmn.New(dmn.WithLimits(dmn.Limits{MaxIterations: 100}))
	defs, diags, err := eng.Compile(context.Background(), []byte(bigForModel))
	if err != nil || diags.HasErrors() {
		t.Fatalf("compile: %v %+v", err, diags)
	}
	dec, _ := defs.Decision("Big")
	_, err = dec.Evaluate(context.Background(), nil)
	assertLimitExceeded(t, err)
}

// TestListSizeLimit covers WP-34: a comprehension producing too large a list
// fails with CodeLimitExceeded.
func TestListSizeLimit(t *testing.T) {
	eng := dmn.New(dmn.WithLimits(dmn.Limits{MaxListSize: 100}))
	defs, _, err := eng.Compile(context.Background(), []byte(bigForModel))
	if err != nil {
		t.Fatal(err)
	}
	dec, _ := defs.Decision("Big")
	_, err = dec.Evaluate(context.Background(), nil)
	assertLimitExceeded(t, err)
}

// TestRecursionLimitConfigurable covers WP-34: the recursion depth limit is
// configurable and enforced (a deep recursive BKM trips it).
func TestRecursionLimitConfigurable(t *testing.T) {
	eng := dmn.New(dmn.WithLimits(dmn.Limits{MaxCallDepth: 10}))
	defs := compileWith(t, eng, "recursion_15.dmn")
	dec, _ := defs.Decision("Factorial")
	_, err := dec.Evaluate(context.Background(), dmn.Input{"N": 100})
	assertLimitExceeded(t, err)
}

// TestDefaultLimitsAllowNormalModels confirms the defaults do not interfere with
// ordinary evaluation.
func TestDefaultLimitsAllowNormalModels(t *testing.T) {
	defs := compileModel(t, "recursion_15.dmn")
	dec, _ := defs.Decision("Factorial")
	res, err := dec.Evaluate(context.Background(), dmn.Input{"N": 6})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outputs["Factorial"] != "720" {
		t.Errorf("Factorial(6) = %v, want 720", res.Outputs["Factorial"])
	}

	// The big comprehension runs fine under default limits.
	big := compileInline(t, bigForModel)
	bd, _ := big.Decision("Big")
	if _, err := bd.Evaluate(context.Background(), nil); err != nil {
		t.Errorf("big-for under default limits: %v", err)
	}
}

// TestCompileContextCancelled covers the compile-time context check.
func TestCompileContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := dmn.New().Compile(ctx, []byte(bigForModel)); err == nil {
		t.Error("compile with a cancelled context should error")
	}
}

// TestCompileTimeoutOptionAccepted confirms a configured compile timeout is
// applied without breaking a normal compile.
func TestCompileTimeoutOptionAccepted(t *testing.T) {
	eng := dmn.New(dmn.WithLimits(dmn.Limits{CompileTimeout: time.Minute}))
	if _, _, err := eng.Compile(context.Background(), []byte(bigForModel)); err != nil {
		t.Errorf("compile with a generous timeout: %v", err)
	}
}

// compileWith compiles a testdata model with a specific engine.
func compileWith(t *testing.T, eng *dmn.Engine, file string) *dmn.Definitions {
	t.Helper()
	data := readModel(t, file)
	defs, diags, err := eng.Compile(context.Background(), data)
	if err != nil {
		t.Fatalf("compile %s: %v", file, err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics for %s: %+v", file, diags)
	}
	return defs
}
