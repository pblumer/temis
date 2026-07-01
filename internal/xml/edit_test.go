package xml

import "testing"

func TestSetElementName(t *testing.T) {
	d := &Definitions{
		InputData: []InputData{{ID: "in1", Name: "old"}},
		Decisions: []Decision{{ID: "dec1", Name: "old"}},
		BKMs:      []BKM{{ID: "bkm1", Name: "old"}},
	}
	for _, id := range []string{"in1", "dec1", "bkm1"} {
		if !d.SetElementName(id, "new") {
			t.Errorf("SetElementName(%q) = false, want true", id)
		}
	}
	if d.InputData[0].Name != "new" || d.Decisions[0].Name != "new" || d.BKMs[0].Name != "new" {
		t.Errorf("names not updated: %+v", d)
	}
	if d.SetElementName("missing", "x") {
		t.Error("SetElementName(missing) = true, want false")
	}
}

func TestSetInputType(t *testing.T) {
	t.Run("creates variable", func(t *testing.T) {
		d := &Definitions{InputData: []InputData{{ID: "in1", Name: "N"}}}
		if !d.SetInputType("in1", " string ") {
			t.Fatal("SetInputType = false")
		}
		if d.InputData[0].Variable == nil || d.InputData[0].Variable.TypeRef != "string" {
			t.Errorf("variable not set: %+v", d.InputData[0].Variable)
		}
		if d.InputData[0].Variable.Name != "N" {
			t.Errorf("variable name = %q, want N", d.InputData[0].Variable.Name)
		}
	})
	t.Run("clear with no variable", func(t *testing.T) {
		d := &Definitions{InputData: []InputData{{ID: "in1"}}}
		if !d.SetInputType("in1", "  ") {
			t.Fatal("SetInputType = false")
		}
		if d.InputData[0].Variable != nil {
			t.Error("variable should remain nil")
		}
	})
	t.Run("updates existing variable", func(t *testing.T) {
		d := &Definitions{InputData: []InputData{{ID: "in1", Variable: &Variable{Name: "N", TypeRef: "number"}}}}
		if !d.SetInputType("in1", "string") {
			t.Fatal("SetInputType = false")
		}
		if d.InputData[0].Variable.TypeRef != "string" {
			t.Errorf("typeRef = %q, want string", d.InputData[0].Variable.TypeRef)
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.SetInputType("missing", "string") {
			t.Error("SetInputType(missing) = true, want false")
		}
	})
}

func TestUpdateDecisionTable(t *testing.T) {
	newRules := []Rule{{ID: "r1", InputEntries: []string{"x"}}}
	newInputs := []Input{{ID: "i2"}}
	newOutputs := []Output{{ID: "o2"}}

	t.Run("replace rules and policy keep columns", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{
			DecisionTable: &DecisionTable{HitPolicy: "UNIQUE", Inputs: []Input{{ID: "i1"}}, Rules: []Rule{{ID: "old"}}},
		}}}}
		if !d.UpdateDecisionTable("dec1", "COLLECT", "SUM", newInputs, newOutputs, newRules, false) {
			t.Fatal("UpdateDecisionTable = false")
		}
		dt := d.Decisions[0].DecisionTable
		if dt.HitPolicy != "COLLECT" || dt.Aggregation != "SUM" {
			t.Errorf("policy not updated: %+v", dt)
		}
		if len(dt.Inputs) != 1 || dt.Inputs[0].ID != "i1" {
			t.Errorf("columns should be kept: %+v", dt.Inputs)
		}
		if len(dt.Rules) != 1 || dt.Rules[0].ID != "r1" {
			t.Errorf("rules not replaced: %+v", dt.Rules)
		}
	})
	t.Run("replace columns and empty hitPolicy", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{
			DecisionTable: &DecisionTable{HitPolicy: "UNIQUE", Inputs: []Input{{ID: "i1"}}},
		}}}}
		if !d.UpdateDecisionTable("dec1", "", "", newInputs, newOutputs, newRules, true) {
			t.Fatal("UpdateDecisionTable = false")
		}
		dt := d.Decisions[0].DecisionTable
		if dt.HitPolicy != "UNIQUE" {
			t.Errorf("hitPolicy should be unchanged: %q", dt.HitPolicy)
		}
		if len(dt.Inputs) != 1 || dt.Inputs[0].ID != "i2" {
			t.Errorf("columns not replaced: %+v", dt.Inputs)
		}
		if len(dt.Outputs) != 1 || dt.Outputs[0].ID != "o2" {
			t.Errorf("outputs not replaced: %+v", dt.Outputs)
		}
	})
	t.Run("no decision table", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1"}}}
		if d.UpdateDecisionTable("dec1", "", "", nil, nil, nil, false) {
			t.Error("UpdateDecisionTable(no dt) = true, want false")
		}
	})
	t.Run("missing decision", func(t *testing.T) {
		d := &Definitions{}
		if d.UpdateDecisionTable("missing", "", "", nil, nil, nil, false) {
			t.Error("UpdateDecisionTable(missing) = true, want false")
		}
	})
}

func TestUpsertItemDefinition(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		d := &Definitions{}
		if !d.UpsertItemDefinition(" Color ", " string ", true, " a, b ") {
			t.Fatal("UpsertItemDefinition = false")
		}
		it := d.ItemDefs[0]
		if it.Name != "Color" || it.TypeRef != "string" || !it.IsCollection {
			t.Errorf("create wrong: %+v", it)
		}
		if it.AllowedValues == nil || it.AllowedValues.Value != "a, b" {
			t.Errorf("allowed values wrong: %+v", it.AllowedValues)
		}
	})
	t.Run("update simple clears allowed values", func(t *testing.T) {
		d := &Definitions{ItemDefs: []ItemDef{{Name: "Color", TypeRef: "string", AllowedValues: &Text{Value: "x"}}}}
		if !d.UpsertItemDefinition("Color", "number", false, "  ") {
			t.Fatal("UpsertItemDefinition = false")
		}
		it := d.ItemDefs[0]
		if it.TypeRef != "number" || it.IsCollection {
			t.Errorf("update wrong: %+v", it)
		}
		if it.AllowedValues != nil {
			t.Errorf("allowed values should be cleared: %+v", it.AllowedValues)
		}
	})
	t.Run("refuse structured", func(t *testing.T) {
		d := &Definitions{ItemDefs: []ItemDef{{Name: "Person", Components: []ItemDef{{Name: "age"}}}}}
		if d.UpsertItemDefinition("Person", "string", false, "") {
			t.Error("UpsertItemDefinition(structured) = true, want false")
		}
	})
	t.Run("refuse empty name", func(t *testing.T) {
		d := &Definitions{}
		if d.UpsertItemDefinition("   ", "string", false, "") {
			t.Error("UpsertItemDefinition(empty) = true, want false")
		}
	})
}

func TestRemoveItemDefinition(t *testing.T) {
	d := &Definitions{ItemDefs: []ItemDef{{Name: "A"}, {Name: "B"}}}
	if !d.RemoveItemDefinition("A") {
		t.Fatal("RemoveItemDefinition(A) = false")
	}
	if len(d.ItemDefs) != 1 || d.ItemDefs[0].Name != "B" {
		t.Errorf("remove wrong: %+v", d.ItemDefs)
	}
	if d.RemoveItemDefinition("missing") {
		t.Error("RemoveItemDefinition(missing) = true, want false")
	}
}

func TestSetLiteralExpression(t *testing.T) {
	t.Run("create on undecided", func(t *testing.T) {
		// decoy first decision exercises the id-mismatch continue branch
		d := &Definitions{Decisions: []Decision{{ID: "decoy"}, {ID: "dec1"}}}
		if !d.SetLiteralExpression("dec1", "1 + 1", "number") {
			t.Fatal("SetLiteralExpression = false")
		}
		le := d.Decisions[1].LiteralExpression
		if le == nil || le.Text != "1 + 1" || le.TypeRef != "number" {
			t.Errorf("literal expr wrong: %+v", le)
		}
	})
	t.Run("update existing literal", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{LiteralExpression: &LiteralExpression{Text: "old"}}}}}
		if !d.SetLiteralExpression("dec1", "new", "") {
			t.Fatal("SetLiteralExpression = false")
		}
		if d.Decisions[0].LiteralExpression.Text != "new" {
			t.Errorf("text not updated: %q", d.Decisions[0].LiteralExpression.Text)
		}
	})
	t.Run("refuse other logic", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{DecisionTable: &DecisionTable{}}}}}
		if d.SetLiteralExpression("dec1", "x", "") {
			t.Error("SetLiteralExpression(other logic) = true, want false")
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.SetLiteralExpression("missing", "x", "") {
			t.Error("SetLiteralExpression(missing) = true, want false")
		}
	})
}

func TestSetBoxedContext(t *testing.T) {
	entries := []ContextEntry{{Variable: &Variable{Name: "e1"}}}
	t.Run("set on undecided", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "decoy"}, {ID: "dec1"}}}
		if !d.SetBoxedContext("dec1", entries) {
			t.Fatal("SetBoxedContext = false")
		}
		if d.Decisions[1].Context == nil || len(d.Decisions[1].Context.Entries) != 1 {
			t.Errorf("context wrong: %+v", d.Decisions[1].Context)
		}
	})
	t.Run("replace existing context", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{Context: &Context{}}}}}
		if !d.SetBoxedContext("dec1", entries) {
			t.Fatal("SetBoxedContext = false")
		}
		if len(d.Decisions[0].Context.Entries) != 1 {
			t.Error("context not replaced")
		}
	})
	t.Run("refuse other logic", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{LiteralExpression: &LiteralExpression{}}}}}
		if d.SetBoxedContext("dec1", entries) {
			t.Error("SetBoxedContext(other logic) = true, want false")
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.SetBoxedContext("missing", entries) {
			t.Error("SetBoxedContext(missing) = true, want false")
		}
	})
}

func TestCreateBoxedContext(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1"}}}
		if !d.CreateBoxedContext("dec1") {
			t.Fatal("CreateBoxedContext = false")
		}
		ctx := d.Decisions[0].Context
		if ctx == nil || len(ctx.Entries) != 1 {
			t.Fatalf("context wrong: %+v", ctx)
		}
		if ctx.Entries[0].Variable.Name != "Eintrag 1" || ctx.Entries[0].LiteralExpression.Text != "0" {
			t.Errorf("entry wrong: %+v", ctx.Entries[0])
		}
	})
	t.Run("refuse existing logic", func(t *testing.T) {
		d := &Definitions{Decisions: []Decision{{ID: "dec1", Expression: Expression{LiteralExpression: &LiteralExpression{}}}}}
		if d.CreateBoxedContext("dec1") {
			t.Error("CreateBoxedContext(has logic) = true, want false")
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.CreateBoxedContext("missing") {
			t.Error("CreateBoxedContext(missing) = true, want false")
		}
	})
}

func TestSetBKMFunction(t *testing.T) {
	params := []FormalParameter{{Name: "p", TypeRef: "number"}}
	t.Run("set on empty", func(t *testing.T) {
		d := &Definitions{BKMs: []BKM{{ID: "decoy"}, {ID: "bkm1"}}}
		if !d.SetBKMFunction("bkm1", params, "p + 1", "number") {
			t.Fatal("SetBKMFunction = false")
		}
		fn := d.BKMs[1].EncapsulatedLogic
		if fn == nil || fn.Kind != "FEEL" || len(fn.Parameters) != 1 {
			t.Fatalf("fn wrong: %+v", fn)
		}
		if fn.LiteralExpression == nil || fn.LiteralExpression.Text != "p + 1" {
			t.Errorf("body wrong: %+v", fn.LiteralExpression)
		}
	})
	t.Run("replace literal-bodied fn", func(t *testing.T) {
		d := &Definitions{BKMs: []BKM{{ID: "bkm1", EncapsulatedLogic: &FunctionDefinition{
			Expression: Expression{LiteralExpression: &LiteralExpression{Text: "old"}},
		}}}}
		if !d.SetBKMFunction("bkm1", params, "new", "") {
			t.Fatal("SetBKMFunction = false")
		}
		if d.BKMs[0].EncapsulatedLogic.LiteralExpression.Text != "new" {
			t.Error("body not replaced")
		}
	})
	t.Run("refuse non-literal body", func(t *testing.T) {
		d := &Definitions{BKMs: []BKM{{ID: "bkm1", EncapsulatedLogic: &FunctionDefinition{
			Expression: Expression{DecisionTable: &DecisionTable{}},
		}}}}
		if d.SetBKMFunction("bkm1", params, "x", "") {
			t.Error("SetBKMFunction(non-literal) = true, want false")
		}
	})
	t.Run("missing", func(t *testing.T) {
		d := &Definitions{}
		if d.SetBKMFunction("missing", params, "x", "") {
			t.Error("SetBKMFunction(missing) = true, want false")
		}
	})
}

func TestTextOrNil(t *testing.T) {
	if textOrNil("  ") != nil {
		t.Error("textOrNil(blank) should be nil")
	}
	if v := textOrNil("  hi  "); v == nil || v.Value != "hi" {
		t.Errorf("textOrNil(hi) = %+v", v)
	}
}

func TestFormatCoord(t *testing.T) {
	if got := formatCoord(180); got != "180" {
		t.Errorf("formatCoord(180) = %q, want 180", got)
	}
	if got := formatCoord(12.5); got != "12.5" {
		t.Errorf("formatCoord(12.5) = %q, want 12.5", got)
	}
}
