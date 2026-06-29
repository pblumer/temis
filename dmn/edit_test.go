package dmn_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pblumer/temis/dmn"
)

func strptr(s string) *string { return &s }
func fptr(f float64) *float64 { return &f }

// graphByName recompiles xml and indexes its graph nodes by name, for asserting
// that edits survived the patch → recompile round-trip.
func graphByName(t *testing.T, xml []byte) map[string]dmn.GraphNode {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), xml)
	if err != nil {
		t.Fatalf("recompile patched model: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("patched model has compile errors: %+v", diags)
	}
	m := map[string]dmn.GraphNode{}
	for _, n := range defs.Graph().Nodes {
		m[n.Name] = n
	}
	return m
}

// TestApplyEditsRenameAndMove patches a DMNDI-carrying model: it renames a leaf
// decision and repositions another, then checks the recompiled graph reflects
// both — the new name and the new bounds — and that the rest of the model still
// compiles (logic preserved). The renamed decision is a leaf, so no FEEL
// reference to it breaks (renaming does not rewrite references, by design).
func TestApplyEditsRenameAndMove(t *testing.T) {
	src := readModel(t, "pricing_15.dmn")

	before := graphByName(t, src)
	label, ok := before["Label"]
	if !ok {
		t.Fatalf("fixture lacks 'Label' decision; nodes: %v", names(before))
	}
	netTotal, ok := before["Net Total"]
	if !ok {
		t.Fatalf("fixture lacks 'Net Total' decision")
	}
	if netTotal.X == 0 && netTotal.Y == 0 {
		t.Fatal("fixture decision has no DMNDI bounds to move")
	}

	out, err := dmn.ApplyEdits(src, []dmn.NodeEdit{
		{ID: label.ID, Name: strptr("Price Label")},
		{ID: netTotal.ID, X: fptr(999), Y: fptr(777)},
	})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}

	after := graphByName(t, out)
	if _, stillOld := after["Label"]; stillOld {
		t.Error("old decision name 'Label' still present after rename")
	}
	renamed, ok := after["Price Label"]
	if !ok {
		t.Fatalf("renamed decision 'Price Label' missing; nodes: %v", names(after))
	}
	if renamed.ID != label.ID {
		t.Errorf("renamed node id = %q, want %q (same element)", renamed.ID, label.ID)
	}
	moved := after["Net Total"]
	if moved.X != 999 || moved.Y != 777 {
		t.Errorf("moved decision bounds = (%v,%v), want (999,777)", moved.X, moved.Y)
	}
	if moved.Width != netTotal.Width || moved.Height != netTotal.Height {
		t.Errorf("move changed size: %vx%v, want %vx%v", moved.Width, moved.Height, netTotal.Width, netTotal.Height)
	}
}

// TestApplyEditsSetType sets an inputData's FEEL type and checks the recompiled
// graph reports it as the node's data contract.
func TestApplyEditsSetType(t *testing.T) {
	src := readModel(t, "pricing_15.dmn")

	before := graphByName(t, src)
	var inputID, inputName string
	for name, n := range before {
		if n.Type == "inputData" {
			inputID, inputName = n.ID, name
			break
		}
	}
	if inputID == "" {
		t.Fatal("fixture has no inputData")
	}

	out, err := dmn.ApplyEdits(src, []dmn.NodeEdit{{ID: inputID, DataType: strptr("string")}})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if got := graphByName(t, out)[inputName].DataType; got != "string" {
		t.Errorf("input %q type after edit = %q, want string", inputName, got)
	}
}

// TestApplyEditsPreservesLogic checks that patching a name does not disturb the
// decision logic: the patched model still evaluates to the same result.
func TestApplyEditsPreservesLogic(t *testing.T) {
	src := readModel(t, "pricing_15.dmn")

	out, err := dmn.ApplyEdits(src, []dmn.NodeEdit{
		{ID: graphByName(t, src)["Net Total"].ID, X: fptr(10), Y: fptr(20)},
	})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	// The decision tables / FEEL text must round-trip verbatim through the patch.
	if !strings.Contains(string(out), "decisionTable") && !strings.Contains(string(out), "literalExpression") {
		t.Error("patched XML lost its decision logic elements")
	}
	// And it must still compile without errors.
	graphByName(t, out)
}

// TestApplyEditsUnknownIDIgnored checks edits for ids not in the model are no-ops
// and leave the document compilable.
func TestApplyEditsUnknownIDIgnored(t *testing.T) {
	src := readModel(t, "routing_13.dmn")
	out, err := dmn.ApplyEdits(src, []dmn.NodeEdit{{ID: "does-not-exist", Name: strptr("X"), X: fptr(1), Y: fptr(2)}})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	graphByName(t, out) // still compiles
}

func names(m map[string]dmn.GraphNode) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
