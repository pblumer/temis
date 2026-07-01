package xml

import (
	"encoding/xml"
	"testing"
)

// hasLocal reports whether tok is a start element with the given local name.
func hasLocal(tok xml.Token, local string) bool {
	se, ok := tok.(xml.StartElement)
	return ok && se.Name.Local == local
}

func TestUpsertShape(t *testing.T) {
	t.Run("nil raw", func(t *testing.T) {
		if UpsertShape(nil, "id_a", 1, 2, 3, 4) {
			t.Error("UpsertShape(nil) = true")
		}
	})
	t.Run("updates existing shape", func(t *testing.T) {
		r := rawFromString(t, diSample)
		if !UpsertShape(r, "id_a", 11, 22, 33, 44) {
			t.Fatal("UpsertShape = false")
		}
		di := ParseDI(r)
		var a *DIShape
		for i := range di.Shapes {
			if di.Shapes[i].Ref == "id_a" {
				a = &di.Shapes[i]
			}
		}
		if a == nil || a.X != 11 || a.Y != 22 || a.Width != 33 || a.Height != 44 {
			t.Errorf("updated shape wrong: %+v", a)
		}
	})
	t.Run("appends new shape", func(t *testing.T) {
		r := rawFromString(t, diSample)
		before := len(ParseDI(r).Shapes)
		if !UpsertShape(r, "id_new", 500, 600, 70, 80) {
			t.Fatal("UpsertShape = false")
		}
		di := ParseDI(r)
		if len(di.Shapes) != before+1 {
			t.Fatalf("shape count = %d, want %d", len(di.Shapes), before+1)
		}
		var n *DIShape
		for i := range di.Shapes {
			if di.Shapes[i].Ref == "id_new" {
				n = &di.Shapes[i]
			}
		}
		if n == nil || n.X != 500 || n.Width != 70 {
			t.Errorf("appended shape wrong: %+v", n)
		}
	})
	t.Run("no shape template returns false", func(t *testing.T) {
		// DMNDI with no DMNShape -> no template available.
		r := rawFromString(t, `<DMNDI><DMNDiagram id="d"></DMNDiagram></DMNDI>`)
		if UpsertShape(r, "id_new", 1, 2, 3, 4) {
			t.Error("UpsertShape(no template) = true")
		}
	})
}

func TestRemoveDIRefs(t *testing.T) {
	t.Run("nil and empty", func(t *testing.T) {
		RemoveDIRefs(nil, []string{"x"}) // must not panic
		r := rawFromString(t, diSample)
		n := len(r.Tokens)
		RemoveDIRefs(r, nil)
		if len(r.Tokens) != n {
			t.Error("RemoveDIRefs(empty refs) changed tokens")
		}
	})
	t.Run("drops shapes and edges", func(t *testing.T) {
		r := rawFromString(t, diSample)
		RemoveDIRefs(r, []string{"id_a", "ir_x"})
		di := ParseDI(r)
		if di == nil || len(di.Shapes) != 1 || di.Shapes[0].Ref != "id_b" {
			t.Fatalf("remaining shapes wrong: %+v", di)
		}
		// the edge ir_x must be gone: no DMNEdge tokens left
		for _, tok := range r.Tokens {
			if hasLocal(tok, "DMNEdge") {
				t.Error("DMNEdge not removed")
			}
		}
	})
}

func TestShapeTemplate(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		r := rawFromString(t, diSample)
		shape, bounds, ok := shapeTemplate(r)
		if !ok || shape.Local != "DMNShape" || bounds.Local != "Bounds" {
			t.Errorf("shapeTemplate = %v %v %v", shape, bounds, ok)
		}
	})
	t.Run("not found", func(t *testing.T) {
		r := rawFromString(t, `<DMNDI><DMNDiagram id="d"></DMNDiagram></DMNDI>`)
		if _, _, ok := shapeTemplate(r); ok {
			t.Error("shapeTemplate(no shape) ok = true")
		}
	})
	t.Run("first shape without bounds closes then second matches", func(t *testing.T) {
		// The first DMNShape has no Bounds and closes (exercising the EndElement
		// branch that resets inShape); the second provides the template.
		r := rawFromString(t, `<DMNDI><DMNShape dmnElementRef="a"></DMNShape><DMNShape dmnElementRef="b"><Bounds x="1"/></DMNShape></DMNDI>`)
		shape, bounds, ok := shapeTemplate(r)
		if !ok || shape.Local != "DMNShape" || bounds.Local != "Bounds" {
			t.Errorf("shapeTemplate = %v %v %v", shape, bounds, ok)
		}
	})
	t.Run("bounds outside shape ignored", func(t *testing.T) {
		// A stray <Bounds> not inside a DMNShape must not be picked up.
		r := rawFromString(t, `<DMNDI><Bounds x="1"/><DMNShape dmnElementRef="a"><Bounds x="2"/></DMNShape></DMNDI>`)
		shape, bounds, ok := shapeTemplate(r)
		if !ok || shape.Local != "DMNShape" || bounds.Local != "Bounds" {
			t.Errorf("shapeTemplate = %v %v %v", shape, bounds, ok)
		}
	})
}

func TestLastShapeEndIndex(t *testing.T) {
	r := rawFromString(t, diSample)
	if idx := lastShapeEndIndex(r); idx < 0 {
		t.Error("lastShapeEndIndex < 0")
	}
	r2 := rawFromString(t, `<DMNDI></DMNDI>`)
	if idx := lastShapeEndIndex(r2); idx != -1 {
		t.Errorf("lastShapeEndIndex(no shape) = %d, want -1", idx)
	}
}

func TestSetBoundsAll(t *testing.T) {
	r := rawFromString(t, diSample)
	if !setShapeBounds(r, "id_a", 1, 2, 3, 4) {
		t.Fatal("setShapeBounds = false")
	}
	if setShapeBounds(r, "missing", 1, 2, 3, 4) {
		t.Error("setShapeBounds(missing) = true")
	}
}
