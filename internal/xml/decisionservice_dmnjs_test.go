package xml_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func TestDMNJSDecisionServiceFixtureExecutes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "models", "decisionservice_dmnjs_15.dmn"))
	if err != nil {
		t.Fatal(err)
	}

	defs, diags, err := dmn.New().Compile(context.Background(), data)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("unexpected compile diagnostics: %+v", diags)
	}

	t.Run("Approval", func(t *testing.T) {
		svc, err := defs.Service("Approval")
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.Evaluate(context.Background(), dmn.Input{"Applicant Age": 20})
		if err != nil {
			t.Fatal(err)
		}

		if got := res.Outputs["Routing"]; got != "ACCEPT" {
			t.Errorf("Routing = %v, want ACCEPT", got)
		}

		want := map[string]any{
			"Eligibility": "ELIGIBLE",
			"Routing":     "ACCEPT",
		}
		if !reflect.DeepEqual(res.Decisions, want) {
			t.Errorf("Decisions = %#v, want %#v", res.Decisions, want)
		}
	})

	t.Run("Routing Only", func(t *testing.T) {
		svc, err := defs.Service("Routing Only")
		if err != nil {
			t.Fatal(err)
		}

		res, err := svc.Evaluate(context.Background(), dmn.Input{"Eligibility": "ELIGIBLE"})
		if err != nil {
			t.Fatal(err)
		}

		if got := res.Outputs["Routing"]; got != "ACCEPT" {
			t.Errorf("Routing = %v, want ACCEPT", got)
		}
		if _, ok := res.Decisions["Eligibility"]; ok {
			t.Errorf("input decision should not be evaluated: %#v", res.Decisions)
		}
	})
}
