package xml

import (
	"encoding/xml"
	"strings"
	"testing"
)

// rawFromString captures the first element of s as a Raw token stream.
func rawFromString(t *testing.T, s string) *Raw {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(s))
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("tokenize: %v", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			var r Raw
			if err := r.UnmarshalXML(dec, se); err != nil {
				t.Fatalf("UnmarshalXML: %v", err)
			}
			return &r
		}
	}
}

const diSample = `<dmndi:DMNDI xmlns:dmndi="d" xmlns:dc="c">` +
	`<dmndi:DMNDiagram id="diag">` +
	`<dmndi:DMNShape id="s1" dmnElementRef="id_a"><dc:Bounds x="10" y="20" width="100" height="50"/></dmndi:DMNShape>` +
	`<dmndi:DMNShape id="s2" dmnElementRef="id_b"><dc:Bounds x="200" y="20" width="100" height="50"/></dmndi:DMNShape>` +
	`<dmndi:DMNEdge id="e1" dmnElementRef="ir_x"><di:waypoint x="1" y="2"/></dmndi:DMNEdge>` +
	`</dmndi:DMNDiagram></dmndi:DMNDI>`

func TestBuildDI(t *testing.T) {
	t.Run("nil for no shapes", func(t *testing.T) {
		r, err := BuildDI(nil)
		if err != nil {
			t.Fatal(err)
		}
		if r != nil {
			t.Errorf("BuildDI(nil) = %+v, want nil", r)
		}
	})
	t.Run("builds shapes parseable by ParseDI", func(t *testing.T) {
		shapes := []DIShape{
			{Ref: "id_a", X: 10, Y: 20, Width: 100, Height: 50},
			{Ref: `a&b<c>"d"`, X: 5.5, Y: 0, Width: 80, Height: 40},
		}
		r, err := BuildDI(shapes)
		if err != nil {
			t.Fatal(err)
		}
		if r == nil {
			t.Fatal("BuildDI = nil")
		}
		di := ParseDI(r)
		if di == nil || len(di.Shapes) != 2 {
			t.Fatalf("ParseDI round-trip = %+v", di)
		}
		if di.Shapes[0].Ref != "id_a" || di.Shapes[0].X != 10 || di.Shapes[0].Width != 100 {
			t.Errorf("shape 0 wrong: %+v", di.Shapes[0])
		}
		if di.Shapes[1].Ref != `a&b<c>"d"` {
			t.Errorf("escaped ref not round-tripped: %q", di.Shapes[1].Ref)
		}
	})
}

func TestFtoa(t *testing.T) {
	if got := ftoa(10); got != "10" {
		t.Errorf("ftoa(10) = %q", got)
	}
	if got := ftoa(5.5); got != "5.5" {
		t.Errorf("ftoa(5.5) = %q", got)
	}
}

func TestEscapeAttr(t *testing.T) {
	if got := escapeAttr(`a&b<c>"d"`); got != "a&amp;b&lt;c&gt;&quot;d&quot;" {
		t.Errorf("escapeAttr = %q", got)
	}
}

func TestParseDI(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if ParseDI(nil) != nil {
			t.Error("ParseDI(nil) != nil")
		}
	})
	t.Run("no usable shapes", func(t *testing.T) {
		// Bounds with zero width/height, and a shape with no ref -> filtered out.
		r := rawFromString(t, `<DMNDI><DMNShape dmnElementRef="x"><Bounds x="1" y="2" width="0" height="0"/></DMNShape></DMNDI>`)
		if ParseDI(r) != nil {
			t.Error("ParseDI(no usable shapes) != nil")
		}
	})
	t.Run("extracts bounds", func(t *testing.T) {
		di := ParseDI(rawFromString(t, diSample))
		if di == nil || len(di.Shapes) != 2 {
			t.Fatalf("ParseDI = %+v", di)
		}
		if di.Shapes[1].Ref != "id_b" || di.Shapes[1].X != 200 {
			t.Errorf("shape 1 wrong: %+v", di.Shapes[1])
		}
	})
}

func TestAttrLocal(t *testing.T) {
	se := xml.StartElement{Attr: []xml.Attr{{Name: xml.Name{Local: "x"}, Value: "v"}}}
	if got := attrLocal(se, "x"); got != "v" {
		t.Errorf("attrLocal(x) = %q", got)
	}
	if got := attrLocal(se, "missing"); got != "" {
		t.Errorf("attrLocal(missing) = %q, want empty", got)
	}
}

func TestAtof(t *testing.T) {
	if got := atof("3.5"); got != 3.5 {
		t.Errorf("atof(3.5) = %v", got)
	}
	if got := atof("notanumber"); got != 0 {
		t.Errorf("atof(bad) = %v, want 0", got)
	}
}

func TestMoveShape(t *testing.T) {
	t.Run("nil raw", func(t *testing.T) {
		if MoveShape(nil, "id_a", 1, 2) {
			t.Error("MoveShape(nil) = true")
		}
	})
	t.Run("moves matching shape", func(t *testing.T) {
		r := rawFromString(t, diSample)
		if !MoveShape(r, "id_b", 333, 444) {
			t.Fatal("MoveShape = false")
		}
		di := ParseDI(r)
		var b *DIShape
		for i := range di.Shapes {
			if di.Shapes[i].Ref == "id_b" {
				b = &di.Shapes[i]
			}
		}
		if b == nil || b.X != 333 || b.Y != 444 {
			t.Errorf("moved shape wrong: %+v", b)
		}
		// width/height preserved
		if b.Width != 100 || b.Height != 50 {
			t.Errorf("dimensions changed: %+v", b)
		}
	})
	t.Run("unknown id", func(t *testing.T) {
		r := rawFromString(t, diSample)
		if MoveShape(r, "nope", 1, 2) {
			t.Error("MoveShape(unknown) = true")
		}
	})
}

func TestSetBoundsXY(t *testing.T) {
	se := xml.StartElement{Name: xml.Name{Local: "Bounds"}, Attr: []xml.Attr{
		{Name: xml.Name{Local: "x"}, Value: "1"},
		{Name: xml.Name{Local: "y"}, Value: "2"},
		{Name: xml.Name{Local: "width"}, Value: "10"},
	}}
	out := setBoundsXY(se, 7, 8)
	got := map[string]string{}
	for _, a := range out.Attr {
		got[a.Name.Local] = a.Value
	}
	if got["x"] != "7" || got["y"] != "8" || got["width"] != "10" {
		t.Errorf("setBoundsXY = %+v", got)
	}
	// original untouched
	if se.Attr[0].Value != "1" {
		t.Error("setBoundsXY mutated original")
	}
}
