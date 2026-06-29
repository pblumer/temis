package dmn_test

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/dmn"
)

// A minimal DMN 1.5 model: one input (a number) and one decision whose literal
// FEEL expression doubles it. Real models are authored in a DMN editor and
// loaded as standard DMN XML; this one is inlined to keep the example
// self-contained.
const doubleModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/"
             namespace="http://temis.example/double" name="Double" id="def_double">
  <inputData id="id_n" name="N">
    <variable name="N" typeRef="number"/>
  </inputData>
  <decision id="id_double" name="Double">
    <variable name="Double" typeRef="number"/>
    <informationRequirement>
      <requiredInput href="#id_n"/>
    </informationRequirement>
    <literalExpression><text>N * 2</text></literalExpression>
  </decision>
</definitions>`

// Example shows the two-phase library flow: an Engine compiles a model once into
// reusable, thread-safe Definitions, from which a decision is fetched by name
// and evaluated against an input context.
func Example() {
	eng := dmn.New()

	defs, diags, err := eng.Compile(context.Background(), []byte(doubleModel))
	if err != nil {
		panic(err) // malformed XML — a hard error
	}
	if diags.HasErrors() {
		panic(diags) // per-decision compile problems
	}

	dec, err := defs.Decision("Double")
	if err != nil {
		panic(err)
	}

	res, err := dec.Evaluate(context.Background(), dmn.Input{"N": 21})
	if err != nil {
		panic(err)
	}
	fmt.Println(res.Outputs["Double"])
	// Output: 42
}

// ExampleCompiledDecision_Evaluate reuses one compiled decision across several
// inputs. Compilation happens once; each Evaluate is cheap and concurrency-safe.
func ExampleCompiledDecision_Evaluate() {
	defs, _, err := dmn.New().Compile(context.Background(), []byte(doubleModel))
	if err != nil {
		panic(err)
	}
	dec, err := defs.Decision("Double")
	if err != nil {
		panic(err)
	}

	for _, n := range []int{1, 10, 100} {
		res, err := dec.Evaluate(context.Background(), dmn.Input{"N": n})
		if err != nil {
			panic(err)
		}
		fmt.Printf("%d -> %v\n", n, res.Outputs["Double"])
	}
	// Output:
	// 1 -> 2
	// 10 -> 20
	// 100 -> 200
}

// ExampleWithLimits bounds the resources any single evaluation may consume,
// turning hostile input into a clean error instead of a hang or out-of-memory.
func ExampleWithLimits() {
	eng := dmn.New(dmn.WithLimits(dmn.Limits{
		MaxCallDepth:  64,
		MaxIterations: 100_000,
		MaxListSize:   100_000,
	}))
	defs, _, err := eng.Compile(context.Background(), []byte(doubleModel))
	if err != nil {
		panic(err)
	}
	dec, _ := defs.Decision("Double")
	res, _ := dec.Evaluate(context.Background(), dmn.Input{"N": 5})
	fmt.Println(res.Outputs["Double"])
	// Output: 10
}
