package dmn_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

// TestGoldenModels evaluates real DMN models end to end through the public API
// (XML → Compile → Decision → Evaluate) and pins their outputs as golden
// values, so a regression in any layer of the pipeline fails here.
//
// The suite intentionally spans the supported surface: UNIQUE/FIRST/COLLECT hit
// policies, single- and multi-output tables (the latter returning a context),
// COLLECT SUM aggregation, literal expressions with decimal arithmetic and
// string built-ins, and a decision that consumes another decision's result
// supplied directly (DRG chaining is covered separately in graph_test.go).
//
// Numbers come back as their exact decimal string (ADR-0007), so e.g. an output
// cell of 0.10 is the value "0.1" once reduced to canonical form.
func TestGoldenModels(t *testing.T) {
	type tc struct {
		decision string
		in       dmn.Input
		want     any
	}
	suites := []struct {
		file  string
		cases []tc
	}{
		{"dish_15.dmn", []tc{
			{"Dish", dmn.Input{"Season": "Fall", "Guest Count": 4}, "Spareribs"},
			{"Dish", dmn.Input{"Season": "Winter", "Guest Count": 4}, "Roastbeef"},
			{"Dish", dmn.Input{"Season": "Spring", "Guest Count": 6}, "Steak"},
			{"Dish", dmn.Input{"Season": "Summer", "Guest Count": 10}, "Stew"},
		}},
		{"discount_14.dmn", []tc{
			{"Discount", dmn.Input{"Customer Type": "Business", "Order Total": 1500}, "0.15"},
			{"Discount", dmn.Input{"Customer Type": "Business", "Order Total": 500}, "0.1"},
			{"Discount", dmn.Input{"Customer Type": "Private", "Order Total": 9999}, "0.05"},
		}},
		{"routing_13.dmn", []tc{
			{"Eligibility", dmn.Input{"Applicant Age": 20}, "ELIGIBLE"},
			{"Eligibility", dmn.Input{"Applicant Age": 16}, "INELIGIBLE"},
			// Routing depends on the Eligibility decision; supplied directly here.
			{"Routing", dmn.Input{"Eligibility": "ELIGIBLE"}, "ACCEPT"},
			{"Routing", dmn.Input{"Eligibility": "INELIGIBLE"}, "DECLINE"},
		}},
		{"loan_15.dmn", []tc{
			{"Loan Approval", dmn.Input{"Credit Score": 800, "Annual Income": 60000},
				map[string]any{"Decision": "Approved", "Interest Rate": "0.035"}},
			{"Loan Approval", dmn.Input{"Credit Score": 720, "Annual Income": 60000},
				map[string]any{"Decision": "Approved", "Interest Rate": "0.05"}},
			{"Loan Approval", dmn.Input{"Credit Score": 550, "Annual Income": 60000},
				map[string]any{"Decision": "Declined", "Interest Rate": "0"}},
			{"Loan Approval", dmn.Input{"Credit Score": 680, "Annual Income": 20000},
				map[string]any{"Decision": "Declined", "Interest Rate": "0"}},
			{"Loan Approval", dmn.Input{"Credit Score": 680, "Annual Income": 60000},
				map[string]any{"Decision": "Review", "Interest Rate": "0.08"}},
		}},
		{"risk_15.dmn", []tc{
			{"Risk Score", dmn.Input{"Has Debt": true, "Is New Customer": true}, "55"},
			{"Risk Score", dmn.Input{"Has Debt": true, "Is New Customer": false}, "35"},
			{"Risk Score", dmn.Input{"Has Debt": false, "Is New Customer": false}, "5"},
		}},
		{"pricing_15.dmn", []tc{
			{"Net Total", dmn.Input{"Unit Price": 19.99, "Quantity": 3}, "59.97"},
			{"Net Total", dmn.Input{"Unit Price": 0.1, "Quantity": 3}, "0.3"}, // exact decimal
			{"Label", dmn.Input{"Product Name": "Widget"}, "WID"},
		}},
	}

	for _, s := range suites {
		t.Run(s.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", "models", s.file))
			if err != nil {
				t.Fatalf("read model: %v", err)
			}
			defs, diags, err := dmn.New().Compile(context.Background(), data)
			if err != nil {
				t.Fatalf("compile: %v", err)
			}
			if diags.HasErrors() {
				t.Fatalf("unexpected compile diagnostics: %+v", diags)
			}

			for _, c := range s.cases {
				name := fmt.Sprintf("%s/%v", c.decision, c.in)
				t.Run(name, func(t *testing.T) {
					dec, err := defs.Decision(c.decision)
					if err != nil {
						t.Fatalf("decision %q: %v", c.decision, err)
					}
					res, err := dec.Evaluate(context.Background(), c.in)
					if err != nil {
						t.Fatalf("evaluate: %v", err)
					}
					got := res.Outputs[c.decision]
					if !reflect.DeepEqual(got, c.want) {
						t.Errorf("%s(%v) = %#v, want %#v", c.decision, c.in, got, c.want)
					}
				})
			}
		})
	}
}
