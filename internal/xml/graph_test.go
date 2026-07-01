package xml

import (
	"reflect"
	"sort"
	"testing"
)

func sampleDefs() *Definitions {
	return &Definitions{
		InputData: []InputData{{ID: "in1", Name: "Input1", Variable: &Variable{TypeRef: "string"}}},
		Decisions: []Decision{{ID: "dec1", Name: "Dec1"}},
		BKMs:      []BKM{{ID: "bkm1", Name: "Bkm1"}},
	}
}

func TestElementIDs(t *testing.T) {
	d := sampleDefs()
	got := d.ElementIDs()
	want := []string{"in1", "dec1", "bkm1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ElementIDs = %v, want %v", got, want)
	}
}

func TestElementType(t *testing.T) {
	d := sampleDefs()
	cases := map[string]string{
		"in1":  "inputData",
		"dec1": "decision",
		"bkm1": "businessKnowledgeModel",
		"x":    "",
	}
	for id, want := range cases {
		if got := d.ElementType(id); got != want {
			t.Errorf("ElementType(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestRemoveElement(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		d := sampleDefs()
		ids, ok := d.RemoveElement("nope")
		if ok || ids != nil {
			t.Errorf("RemoveElement(nope) = %v, %v", ids, ok)
		}
	})
	t.Run("inputData with requirements", func(t *testing.T) {
		d := &Definitions{
			InputData: []InputData{{ID: "in1"}},
			Decisions: []Decision{{ID: "dec1", InformationRequirts: []InformationRequirt{
				{ID: "ir1", RequiredInput: &Ref{Href: "#in1"}},
			}}},
		}
		ids, ok := d.RemoveElement("in1")
		if !ok {
			t.Fatal("RemoveElement = false")
		}
		if len(d.InputData) != 0 {
			t.Error("inputData not removed")
		}
		if !reflect.DeepEqual(ids, []string{"ir1"}) {
			t.Errorf("removed reqIDs = %v, want [ir1]", ids)
		}
		if len(d.Decisions[0].InformationRequirts) != 0 {
			t.Error("requirement not dropped")
		}
	})
	t.Run("decision", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1"}, {ID: "dec2", InformationRequirts: []InformationRequirt{
			{ID: "ir1", RequiredDecision: &Ref{Href: "#dec1"}},
		}}}}
		ids, ok := d.RemoveElement("dec1")
		if !ok {
			t.Fatal("RemoveElement = false")
		}
		if !reflect.DeepEqual(ids, []string{"ir1"}) {
			t.Errorf("removed reqIDs = %v", ids)
		}
	})
	t.Run("bkm with knowledge requirements", func(t *testing.T) {
		d := &Definitions{
			BKMs: []BKM{
				{ID: "bkm1"},
				{ID: "bkm2", KnowledgeRequirts: []KnowledgeRequirt{{ID: "kr1", RequiredKnowledge: &Ref{Href: "#bkm1"}}}},
			},
			Decisions: []Decision{{ID: "dec1", KnowledgeRequirts: []KnowledgeRequirt{
				{ID: "kr2", RequiredKnowledge: &Ref{Href: "#bkm1"}},
			}}},
		}
		ids, ok := d.RemoveElement("bkm1")
		if !ok {
			t.Fatal("RemoveElement = false")
		}
		sort.Strings(ids)
		if !reflect.DeepEqual(ids, []string{"kr1", "kr2"}) {
			t.Errorf("removed reqIDs = %v, want [kr1 kr2]", ids)
		}
	})
}

func TestUpsertInputData(t *testing.T) {
	d := &Definitions{}
	d.UpsertInputData("in1", "Name1", "string")
	if len(d.InputData) != 1 || d.InputData[0].Name != "Name1" {
		t.Fatalf("create failed: %+v", d.InputData)
	}
	if d.InputData[0].Variable == nil || d.InputData[0].Variable.TypeRef != "string" {
		t.Errorf("type not set: %+v", d.InputData[0].Variable)
	}
	// update existing
	d.UpsertInputData("in1", "Renamed", "")
	if len(d.InputData) != 1 || d.InputData[0].Name != "Renamed" {
		t.Errorf("update failed: %+v", d.InputData)
	}
}

func TestUpsertDecision(t *testing.T) {
	d := &Definitions{}
	d.UpsertDecision("dec1", "D1")
	if len(d.Decisions) != 1 || d.Decisions[0].Name != "D1" {
		t.Fatalf("create failed: %+v", d.Decisions)
	}
	d.UpsertDecision("dec1", "D1-renamed")
	if len(d.Decisions) != 1 || d.Decisions[0].Name != "D1-renamed" {
		t.Errorf("update failed: %+v", d.Decisions)
	}
}

func TestUpsertBKM(t *testing.T) {
	d := &Definitions{}
	d.UpsertBKM("bkm1", "B1")
	if len(d.BKMs) != 1 || d.BKMs[0].Name != "B1" {
		t.Fatalf("create failed: %+v", d.BKMs)
	}
	d.UpsertBKM("bkm1", "B1-renamed")
	if len(d.BKMs) != 1 || d.BKMs[0].Name != "B1-renamed" {
		t.Errorf("update failed: %+v", d.BKMs)
	}
}

func TestReconcileRequirements(t *testing.T) {
	t.Run("add reuse and remove", func(t *testing.T) {
		d := &Definitions{
			Decisions: []Decision{{ID: "dec1", InformationRequirts: []InformationRequirt{
				// existing edge from in1 -> reused
				{ID: "ir_keep", RequiredInput: &Ref{Href: "#in1"}},
				// existing edge from dec0 -> will be removed (not in want)
				{ID: "ir_gone", RequiredDecision: &Ref{Href: "#dec0"}},
			}}},
			BKMs: []BKM{{ID: "bkm1"}},
		}
		typeOf := map[string]string{"in1": "inputData", "dec2": "decision", "bkm9": "businessKnowledgeModel"}
		edges := []ReqEdge{
			{Kind: "informationRequirement", Source: "in1", Target: "dec1"},  // reused
			{Kind: "informationRequirement", Source: "dec2", Target: "dec1"}, // new requiredDecision
			{Kind: "informationRequirement", Source: "in1", Target: "dec1"},  // duplicate, deduped
			{Kind: "knowledgeRequirement", Source: "bkm9", Target: "dec1"},   // new knowledge req on decision
			{Kind: "knowledgeRequirement", Source: "bkm9", Target: "bkm1"},   // new knowledge req on bkm
		}
		removed := d.ReconcileRequirements(edges, typeOf)
		if !reflect.DeepEqual(removed, []string{"ir_gone"}) {
			t.Errorf("removed = %v, want [ir_gone]", removed)
		}
		ir := d.Decisions[0].InformationRequirts
		if len(ir) != 2 {
			t.Fatalf("info reqs = %+v", ir)
		}
		if ir[0].ID != "ir_keep" {
			t.Errorf("existing edge not reused: %+v", ir[0])
		}
		if ir[1].RequiredDecision == nil || ir[1].RequiredDecision.Href != "#dec2" {
			t.Errorf("new requiredDecision wrong: %+v", ir[1])
		}
		kr := d.Decisions[0].KnowledgeRequirts
		if len(kr) != 1 || kr[0].RequiredKnowledge.Href != "#bkm9" {
			t.Errorf("decision knowledge req wrong: %+v", kr)
		}
		if len(d.BKMs[0].KnowledgeRequirts) != 1 {
			t.Errorf("bkm knowledge req wrong: %+v", d.BKMs[0].KnowledgeRequirts)
		}
	})
	t.Run("new info requirement from inputData", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1"}}}
		typeOf := map[string]string{"in1": "inputData"}
		edges := []ReqEdge{{Kind: "informationRequirement", Source: "in1", Target: "dec1"}}
		d.ReconcileRequirements(edges, typeOf)
		ir := d.Decisions[0].InformationRequirts
		if len(ir) != 1 || ir[0].RequiredInput == nil || ir[0].RequiredInput.Href != "#in1" {
			t.Errorf("requiredInput not set: %+v", ir)
		}
		if ir[0].RequiredDecision != nil {
			t.Errorf("requiredDecision should be nil: %+v", ir[0])
		}
	})
	t.Run("reuse existing knowledge requirement", func(t *testing.T) {
		d := &Definitions{BKMs: []BKM{{ID: "bkm1", KnowledgeRequirts: []KnowledgeRequirt{
			{ID: "kr_keep", RequiredKnowledge: &Ref{Href: "#bkm2"}},
		}}}}
		edges := []ReqEdge{{Kind: "knowledgeRequirement", Source: "bkm2", Target: "bkm1"}}
		removed := d.ReconcileRequirements(edges, nil)
		if len(removed) != 0 {
			t.Errorf("removed = %v, want none", removed)
		}
		if d.BKMs[0].KnowledgeRequirts[0].ID != "kr_keep" {
			t.Error("existing knowledge req not reused")
		}
	})
}

func TestCreateDecisionTable(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.CreateDecisionTable("nope") {
			t.Error("CreateDecisionTable(missing) = true")
		}
	})
	t.Run("already has logic", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{LiteralExpression: &LiteralExpression{}}}}}
		if d.CreateDecisionTable("dec1") {
			t.Error("CreateDecisionTable(has logic) = true")
		}
	})
	t.Run("builds inputs from requirements and output from variable", func(t *testing.T) {
		d := &Definitions{
			InputData: []InputData{{ID: "in1", Name: "Age", Variable: &Variable{TypeRef: "number"}}},
			Decisions: []Decision{
				{ID: "dep", Name: "Dep", Variable: &Variable{TypeRef: "string"}},
				{ID: "dec1", Name: "Dec1", Variable: &Variable{Name: "Result", TypeRef: "boolean"},
					InformationRequirts: []InformationRequirt{
						{RequiredInput: &Ref{Href: "#in1"}},
						{RequiredDecision: &Ref{Href: "#dep"}},
						{RequiredInput: &Ref{Href: "#unknown"}}, // name="" -> skipped
					}},
			},
		}
		if !d.CreateDecisionTable("dec1") {
			t.Fatal("CreateDecisionTable = false")
		}
		dt := d.Decisions[1].DecisionTable
		if dt == nil || dt.HitPolicy != "UNIQUE" {
			t.Fatalf("dt wrong: %+v", dt)
		}
		if len(dt.Inputs) != 2 {
			t.Fatalf("inputs = %+v", dt.Inputs)
		}
		if dt.Inputs[0].Label != "Age" || dt.Inputs[0].InputExpression.TypeRef != "number" {
			t.Errorf("input 0 wrong: %+v", dt.Inputs[0])
		}
		if dt.Inputs[1].InputExpression.Text != "Dep" {
			t.Errorf("input 1 wrong: %+v", dt.Inputs[1])
		}
		if len(dt.Outputs) != 1 || dt.Outputs[0].Name != "Result" || dt.Outputs[0].TypeRef != "boolean" {
			t.Errorf("output wrong: %+v", dt.Outputs)
		}
	})
	t.Run("output falls back to decision name", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Name: "Plain"}}}
		if !d.CreateDecisionTable("dec1") {
			t.Fatal("CreateDecisionTable = false")
		}
		if d.Decisions[0].DecisionTable.Outputs[0].Name != "Plain" {
			t.Errorf("output name = %q, want Plain", d.Decisions[0].DecisionTable.Outputs[0].Name)
		}
	})
}

func TestPresent(t *testing.T) {
	if (Expression{}).present() {
		t.Error("empty Expression.present() = true")
	}
	cases := []Expression{
		{LiteralExpression: &LiteralExpression{}},
		{DecisionTable: &DecisionTable{}},
		{Context: &Context{}},
		{Invocation: &Invocation{}},
		{FunctionDefinition: &FunctionDefinition{}},
		{List: &List{}},
		{Relation: &Relation{}},
		{Conditional: &Conditional{}},
		{For: &Iterator{}},
		{Every: &Iterator{}},
		{Some: &Iterator{}},
		{Filter: &Filter{}},
	}
	for i, e := range cases {
		if !e.present() {
			t.Errorf("case %d present() = false", i)
		}
	}
}

func TestHrefID(t *testing.T) {
	cases := map[string]string{
		"#x":      "x",
		"  #y  ":  "y",
		"foo#z":   "z",
		"plainid": "plainid",
	}
	for in, want := range cases {
		if got := hrefID(in); got != want {
			t.Errorf("hrefID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRefHref(t *testing.T) {
	if refHref(nil) != "" {
		t.Error("refHref(nil) != empty")
	}
	if refHref(&Ref{Href: "#x"}) != "#x" {
		t.Error("refHref wrong")
	}
}

func TestReqID(t *testing.T) {
	if got := reqID("ir", "src", "tgt"); got != "ir_src_tgt" {
		t.Errorf("reqID = %q", got)
	}
}

// TestRemoveHelpersNotFound covers the "id absent, return slice unchanged" path
// of the remove* helpers (RemoveElement never reaches it because it checks
// ElementType first).
func TestRemoveHelpersNotFound(t *testing.T) {
	in := []InputData{{ID: "a"}}
	if got := removeInputData(in, "missing"); len(got) != 1 {
		t.Errorf("removeInputData(missing) = %+v", got)
	}
	dec := []Decision{{ID: "a"}}
	if got := removeDecision(dec, "missing"); len(got) != 1 {
		t.Errorf("removeDecision(missing) = %+v", got)
	}
	bkm := []BKM{{ID: "a"}}
	if got := removeBKM(bkm, "missing"); len(got) != 1 {
		t.Errorf("removeBKM(missing) = %+v", got)
	}
}

// TestDropReqsMixed covers both branches of dropInfoReqs/dropKnowReqs: one
// requirement matching id (dropped) and one not (kept).
func TestDropReqsMixed(t *testing.T) {
	info := []InformationRequirt{
		{ID: "drop", RequiredInput: &Ref{Href: "#x"}},
		{ID: "keep", RequiredInput: &Ref{Href: "#y"}},
	}
	out, acc := dropInfoReqs(info, "x", nil)
	if len(out) != 1 || out[0].ID != "keep" {
		t.Errorf("dropInfoReqs kept = %+v", out)
	}
	if len(acc) != 1 || acc[0] != "drop" {
		t.Errorf("dropInfoReqs acc = %v", acc)
	}

	know := []KnowledgeRequirt{
		{ID: "kdrop", RequiredKnowledge: &Ref{Href: "#x"}},
		{ID: "kkeep", RequiredKnowledge: &Ref{Href: "#y"}},
	}
	kout, kacc := dropKnowReqs(know, "x", nil)
	if len(kout) != 1 || kout[0].ID != "kkeep" {
		t.Errorf("dropKnowReqs kept = %+v", kout)
	}
	if len(kacc) != 1 || kacc[0] != "kdrop" {
		t.Errorf("dropKnowReqs acc = %v", kacc)
	}
}

// TestReconcileTargetMixedKnowledge drives the knowledgeRequirement removal
// branch in reconcileTarget (an existing knowledge edge dropped).
func TestReconcileTargetMixedKnowledge(t *testing.T) {
	d := &Definitions{Decisions: []Decision{{ID: "dec1", KnowledgeRequirts: []KnowledgeRequirt{
		{ID: "kr_gone", RequiredKnowledge: &Ref{Href: "#bkmA"}},
	}}}}
	// no edges -> kr_gone removed
	removed := d.ReconcileRequirements(nil, nil)
	if len(removed) != 1 || removed[0] != "kr_gone" {
		t.Errorf("removed = %v, want [kr_gone]", removed)
	}
}
