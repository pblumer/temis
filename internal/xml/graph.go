package xml

import (
	"encoding/xml"
	"strings"
)

// ReqEdge is a desired requirement edge for ReconcileRequirements: a dependency
// from Source (the required/upstream element) to Target (the requiring element),
// matching the DMN arrow direction. Kind is "informationRequirement" or
// "knowledgeRequirement".
type ReqEdge struct {
	Kind   string
	Source string
	Target string
}

// ElementIDs returns the ids of every inputData, decision and BKM in document
// order — the addressable DRG nodes.
func (d *Definitions) ElementIDs() []string {
	ids := make([]string, 0, len(d.InputData)+len(d.Decisions)+len(d.BKMs))
	for _, in := range d.InputData {
		ids = append(ids, in.ID)
	}
	for _, dec := range d.Decisions {
		ids = append(ids, dec.ID)
	}
	for _, b := range d.BKMs {
		ids = append(ids, b.ID)
	}
	return ids
}

// ElementType reports whether id is an "inputData", "decision",
// "businessKnowledgeModel" or "" (unknown).
func (d *Definitions) ElementType(id string) string {
	for _, in := range d.InputData {
		if in.ID == id {
			return "inputData"
		}
	}
	for _, dec := range d.Decisions {
		if dec.ID == id {
			return "decision"
		}
	}
	for _, b := range d.BKMs {
		if b.ID == id {
			return "businessKnowledgeModel"
		}
	}
	return ""
}

// RemoveElement removes the inputData/decision/BKM with id and every requirement
// (in any decision or BKM) that references it. It returns the ids of the removed
// requirement elements (so their DMNDI edges can be dropped) and whether the
// element existed. The element's own logic, if any, is discarded with it.
func (d *Definitions) RemoveElement(id string) (reqIDs []string, ok bool) {
	switch d.ElementType(id) {
	case "inputData":
		d.InputData = removeInputData(d.InputData, id)
	case "decision":
		d.Decisions = removeDecision(d.Decisions, id)
	case "businessKnowledgeModel":
		d.BKMs = removeBKM(d.BKMs, id)
	default:
		return nil, false
	}
	for di := range d.Decisions {
		dec := &d.Decisions[di]
		dec.InformationRequirts, reqIDs = dropInfoReqs(dec.InformationRequirts, id, reqIDs)
		dec.KnowledgeRequirts, reqIDs = dropKnowReqs(dec.KnowledgeRequirts, id, reqIDs)
	}
	for bi := range d.BKMs {
		d.BKMs[bi].KnowledgeRequirts, reqIDs = dropKnowReqs(d.BKMs[bi].KnowledgeRequirts, id, reqIDs)
	}
	return reqIDs, true
}

// UpsertInputData creates the inputData with id, or updates its name and type if
// it already exists. An empty typeRef leaves/clears the declared type.
func (d *Definitions) UpsertInputData(id, name, typeRef string) {
	if indexInputData(d.InputData, id) < 0 {
		d.InputData = append(d.InputData, InputData{ID: id, Name: name})
	} else {
		d.SetElementName(id, name)
	}
	d.SetInputType(id, typeRef)
}

// UpsertDecision creates the decision with id (no logic yet — an undecided
// decision), or updates its name if it already exists.
func (d *Definitions) UpsertDecision(id, name string) {
	if i := indexDecision(d.Decisions, id); i < 0 {
		d.Decisions = append(d.Decisions, Decision{ID: id, Name: name})
	} else {
		d.Decisions[i].Name = name
	}
}

// UpsertBKM creates the business knowledge model with id, or updates its name.
func (d *Definitions) UpsertBKM(id, name string) {
	if i := indexBKM(d.BKMs, id); i < 0 {
		d.BKMs = append(d.BKMs, BKM{ID: id, Name: name})
	} else {
		d.BKMs[i].Name = name
	}
}

// ReconcileRequirements rewrites each decision's and BKM's requirement edges to
// match the desired set, reusing the existing requirement element (and thus its
// id and any DMNDI edge) when an edge is unchanged and minting a new one when it
// is added. typeOf maps a source id to its element type, to pick requiredInput vs
// requiredDecision. It returns the ids of the requirement elements that were
// removed, so their DMNDI edges can be dropped.
func (d *Definitions) ReconcileRequirements(edges []ReqEdge, typeOf map[string]string) (removed []string) {
	// Group desired edges by target, preserving their order and de-duplicating, so
	// the rebuilt requirement lists (and the XML) are deterministic.
	want := map[string][]ReqEdge{}
	seen := map[string]bool{}
	for _, e := range edges {
		key := e.Target + "\x00" + e.Source
		if seen[key] {
			continue
		}
		seen[key] = true
		want[e.Target] = append(want[e.Target], e)
	}

	for di := range d.Decisions {
		dec := &d.Decisions[di]
		newInfo, newKnow, gone := reconcileTarget(dec.ID, dec.InformationRequirts, dec.KnowledgeRequirts, want[dec.ID], typeOf)
		dec.InformationRequirts, dec.KnowledgeRequirts = newInfo, newKnow
		removed = append(removed, gone...)
	}
	for bi := range d.BKMs {
		b := &d.BKMs[bi]
		// A BKM only carries knowledge requirements.
		_, newKnow, gone := reconcileTarget(b.ID, nil, b.KnowledgeRequirts, want[b.ID], typeOf)
		b.KnowledgeRequirts = newKnow
		removed = append(removed, gone...)
	}
	return removed
}

// reconcileTarget rebuilds one target's requirement lists from the desired edges
// (in order), preserving existing requirement elements where the edge still
// exists and minting new ones otherwise.
func reconcileTarget(target string, info []InformationRequirt, know []KnowledgeRequirt, want []ReqEdge, typeOf map[string]string) (newInfo []InformationRequirt, newKnow []KnowledgeRequirt, removed []string) {
	infoBySrc := map[string]InformationRequirt{}
	for _, ir := range info {
		if src := infoReqSource(ir); src != "" {
			infoBySrc[src] = ir
		}
	}
	knowBySrc := map[string]KnowledgeRequirt{}
	for _, kr := range know {
		if src := hrefID(refHref(kr.RequiredKnowledge)); src != "" {
			knowBySrc[src] = kr
		}
	}

	keptInfo := map[string]bool{}
	keptKnow := map[string]bool{}
	for _, e := range want {
		src := e.Source
		switch e.Kind {
		case "knowledgeRequirement":
			if kr, ok := knowBySrc[src]; ok {
				newKnow = append(newKnow, kr)
			} else {
				newKnow = append(newKnow, KnowledgeRequirt{ID: reqID("kr", src, target), RequiredKnowledge: &Ref{Href: "#" + src}})
			}
			keptKnow[src] = true
		default: // informationRequirement
			if ir, ok := infoBySrc[src]; ok {
				newInfo = append(newInfo, ir)
			} else {
				ir := InformationRequirt{ID: reqID("ir", src, target)}
				if typeOf[src] == "inputData" {
					ir.RequiredInput = &Ref{Href: "#" + src}
				} else {
					ir.RequiredDecision = &Ref{Href: "#" + src}
				}
				newInfo = append(newInfo, ir)
			}
			keptInfo[src] = true
		}
	}

	for src, ir := range infoBySrc {
		if !keptInfo[src] {
			removed = append(removed, ir.ID)
		}
	}
	for src, kr := range knowBySrc {
		if !keptKnow[src] {
			removed = append(removed, kr.ID)
		}
	}
	return newInfo, newKnow, removed
}

// --- helpers ---

func infoReqSource(ir InformationRequirt) string {
	if s := hrefID(refHref(ir.RequiredInput)); s != "" {
		return s
	}
	return hrefID(refHref(ir.RequiredDecision))
}

func refHref(r *Ref) string {
	if r == nil {
		return ""
	}
	return r.Href
}

// hrefID strips the leading fragment of an href to its local id ("#x" -> "x").
func hrefID(href string) string {
	href = strings.TrimSpace(href)
	if i := strings.LastIndex(href, "#"); i >= 0 {
		return href[i+1:]
	}
	return href
}

// reqID mints a stable requirement id from its kind prefix, source and target, so
// repeated reconciliations of the same graph are deterministic.
func reqID(prefix, source, target string) string {
	return prefix + "_" + source + "_" + target
}

func dropInfoReqs(in []InformationRequirt, id string, acc []string) ([]InformationRequirt, []string) {
	out := in[:0:0]
	for _, ir := range in {
		if infoReqSource(ir) == id {
			acc = append(acc, ir.ID)
			continue
		}
		out = append(out, ir)
	}
	return out, acc
}

func dropKnowReqs(in []KnowledgeRequirt, id string, acc []string) ([]KnowledgeRequirt, []string) {
	out := in[:0:0]
	for _, kr := range in {
		if hrefID(refHref(kr.RequiredKnowledge)) == id {
			acc = append(acc, kr.ID)
			continue
		}
		out = append(out, kr)
	}
	return out, acc
}

func indexInputData(in []InputData, id string) int {
	for i := range in {
		if in[i].ID == id {
			return i
		}
	}
	return -1
}

func indexDecision(in []Decision, id string) int {
	for i := range in {
		if in[i].ID == id {
			return i
		}
	}
	return -1
}

func indexBKM(in []BKM, id string) int {
	for i := range in {
		if in[i].ID == id {
			return i
		}
	}
	return -1
}

func removeInputData(in []InputData, id string) []InputData {
	if i := indexInputData(in, id); i >= 0 {
		return append(in[:i], in[i+1:]...)
	}
	return in
}

func removeDecision(in []Decision, id string) []Decision {
	if i := indexDecision(in, id); i >= 0 {
		return append(in[:i], in[i+1:]...)
	}
	return in
}

func removeBKM(in []BKM, id string) []BKM {
	if i := indexBKM(in, id); i >= 0 {
		return append(in[:i], in[i+1:]...)
	}
	return in
}

// CreateDecisionTable gives a logic-less decision a fresh decision table: one
// input column per information requirement (the required element's name as the
// input expression, carrying its declared type) and a single output named after
// the decision (or its variable), hit policy UNIQUE, no rules yet. It reports
// false when the decision is unknown or already carries logic.
func (d *Definitions) CreateDecisionTable(decisionID string) bool {
	i := indexDecision(d.Decisions, decisionID)
	if i < 0 {
		return false
	}
	dec := &d.Decisions[i]
	if dec.present() {
		return false
	}

	dt := &DecisionTable{HitPolicy: "UNIQUE"}
	for _, ir := range dec.InformationRequirts {
		name, typeRef := d.elementNameType(infoReqSource(ir))
		if name == "" {
			continue
		}
		dt.Inputs = append(dt.Inputs, Input{Label: name, InputExpression: InputExpression{Text: name, TypeRef: typeRef}})
	}

	outName, outType := dec.Name, ""
	if dec.Variable != nil {
		if dec.Variable.Name != "" {
			outName = dec.Variable.Name
		}
		outType = dec.Variable.TypeRef
	}
	dt.Outputs = append(dt.Outputs, Output{Name: outName, TypeRef: outType})

	dec.DecisionTable = dt
	return true
}

// present reports whether any boxed-expression child is set — i.e. the holder
// already carries logic.
func (e Expression) present() bool {
	return e.LiteralExpression != nil || e.DecisionTable != nil || e.Context != nil ||
		e.Invocation != nil || e.FunctionDefinition != nil || e.List != nil ||
		e.Relation != nil || e.Conditional != nil || e.For != nil || e.Every != nil ||
		e.Some != nil || e.Filter != nil
}

// elementNameType resolves an inputData or decision id to its FEEL identifier and
// declared variable type ("" when none). The name is the element's variable name
// when declared, else its display @name — so an authored input expression (a
// decision-table column) references the element the way FEEL binds it, not by a
// free-form display label.
func (d *Definitions) elementNameType(id string) (name, typeRef string) {
	for _, in := range d.InputData {
		if in.ID == id {
			if in.Variable != nil {
				typeRef = in.Variable.TypeRef
			}
			return refElementName(in.Name, in.Variable), typeRef
		}
	}
	for _, dec := range d.Decisions {
		if dec.ID == id {
			if dec.Variable != nil {
				typeRef = dec.Variable.TypeRef
			}
			return refElementName(dec.Name, dec.Variable), typeRef
		}
	}
	return "", ""
}

// refElementName is the FEEL identifier for an element: its variable name when
// declared, else its display name. Mirrors model.RefName on the raw XML side.
func refElementName(displayName string, v *Variable) string {
	if v != nil {
		if n := strings.TrimSpace(v.Name); n != "" {
			return n
		}
	}
	return displayName
}

// --- DMNDI shape surgery ---

// UpsertShape sets the bounds of the DMNShape bound to id, or appends a new
// DMNShape (with a <Bounds>) after the last existing one, reusing the document's
// shape/bounds namespaces. It reports false only when there is no DMNDI or no
// existing shape to use as a namespace template (so the caller leaves layout to
// the client).
func UpsertShape(r *Raw, id string, x, y, w, h float64) bool {
	if r == nil {
		return false
	}
	if setShapeBounds(r, id, x, y, w, h) {
		return true
	}
	shapeName, boundsName, ok := shapeTemplate(r)
	if !ok {
		return false
	}
	idx := lastShapeEndIndex(r)
	if idx < 0 {
		return false
	}
	toks := newShapeTokens(shapeName, boundsName, id, x, y, w, h)
	out := make([]xml.Token, 0, len(r.Tokens)+len(toks))
	out = append(out, r.Tokens[:idx+1]...)
	out = append(out, toks...)
	out = append(out, r.Tokens[idx+1:]...)
	r.Tokens = out
	return true
}

// RemoveDIRefs drops every DMNShape and DMNEdge whose dmnElementRef is in refs,
// keeping the diagram consistent after elements or requirements are removed.
func RemoveDIRefs(r *Raw, refs []string) {
	if r == nil || len(refs) == 0 {
		return
	}
	drop := make(map[string]bool, len(refs))
	for _, id := range refs {
		drop[id] = true
	}
	out := r.Tokens[:0:0]
	skipDepth := 0
	for _, tok := range r.Tokens {
		if skipDepth > 0 {
			switch tok.(type) {
			case xml.StartElement:
				skipDepth++
			case xml.EndElement:
				skipDepth--
			}
			continue
		}
		if se, ok := tok.(xml.StartElement); ok {
			if (se.Name.Local == "DMNShape" || se.Name.Local == "DMNEdge") && drop[attrLocal(se, "dmnElementRef")] {
				skipDepth = 1
				continue
			}
		}
		out = append(out, tok)
	}
	r.Tokens = out
}

// shapeTemplate returns the element names of an existing DMNShape and its Bounds
// child, so a new shape matches the document's prefixes/namespaces.
func shapeTemplate(r *Raw) (shape, bounds xml.Name, ok bool) {
	inShape := false
	for _, tok := range r.Tokens {
		se, k := tok.(xml.StartElement)
		if !k {
			if ee, k := tok.(xml.EndElement); k && ee.Name.Local == "DMNShape" {
				inShape = false
			}
			continue
		}
		switch se.Name.Local {
		case "DMNShape":
			shape = se.Name
			inShape = true
		case "Bounds":
			if inShape {
				return shape, se.Name, true
			}
		}
	}
	return xml.Name{}, xml.Name{}, false
}

func lastShapeEndIndex(r *Raw) int {
	idx := -1
	for i, tok := range r.Tokens {
		if ee, ok := tok.(xml.EndElement); ok && ee.Name.Local == "DMNShape" {
			idx = i
		}
	}
	return idx
}

func newShapeTokens(shape, bounds xml.Name, id string, x, y, w, h float64) []xml.Token {
	shapeStart := xml.StartElement{Name: shape, Attr: []xml.Attr{
		{Name: xml.Name{Local: "id"}, Value: "shape_" + id},
		{Name: xml.Name{Local: "dmnElementRef"}, Value: id},
	}}
	boundsStart := xml.StartElement{Name: bounds, Attr: []xml.Attr{
		{Name: xml.Name{Local: "x"}, Value: formatCoord(x)},
		{Name: xml.Name{Local: "y"}, Value: formatCoord(y)},
		{Name: xml.Name{Local: "width"}, Value: formatCoord(w)},
		{Name: xml.Name{Local: "height"}, Value: formatCoord(h)},
	}}
	return []xml.Token{shapeStart, boundsStart, boundsStart.End(), shapeStart.End()}
}

// setShapeBounds updates the x/y/width/height of an existing DMNShape's Bounds,
// reporting whether the shape was found.
func setShapeBounds(r *Raw, id string, x, y, w, h float64) bool {
	inShape := false
	for i, tok := range r.Tokens {
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "DMNShape":
				inShape = attrLocal(t, "dmnElementRef") == id
			case "Bounds":
				if inShape {
					r.Tokens[i] = setBoundsAll(t, x, y, w, h)
					return true
				}
			}
		case xml.EndElement:
			if t.Name.Local == "DMNShape" {
				inShape = false
			}
		}
	}
	return false
}

func setBoundsAll(se xml.StartElement, x, y, w, h float64) xml.StartElement {
	out := xml.StartElement{Name: se.Name, Attr: make([]xml.Attr, len(se.Attr))}
	copy(out.Attr, se.Attr)
	for i := range out.Attr {
		switch out.Attr[i].Name.Local {
		case "x":
			out.Attr[i].Value = formatCoord(x)
		case "y":
			out.Attr[i].Value = formatCoord(y)
		case "width":
			out.Attr[i].Value = formatCoord(w)
		case "height":
			out.Attr[i].Value = formatCoord(h)
		}
	}
	return out
}
