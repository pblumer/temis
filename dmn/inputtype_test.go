package dmn_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pblumer/feel/value"
	"github.com/pblumer/temis/dmn"
)

// Der deklarierte Eingabetyp entscheidet, wie ein Wert an der Aussengrenze
// ankommt (ADR-0040).
//
// Vorher galt beides zugleich, und beides war falsch: `ValidateInput` liess jeden
// String für ein `date` durch — auch "nonsense" und "" —, und `toValue` machte
// daraus eine FEEL-Zeichenkette, worauf jede Datumsregel ins Leere lief und die
// Catch-all-Zeile antwortete. Umgekehrt wurde der einzige Wert, der korrekt
// auswertete (eine echte `value.Date`, seit ADR-0039 öffentlich konstruierbar),
// als Typkonflikt abgelehnt, mit `got value.Date` — kein FEEL-Typname.
//
// Das Modell führt genau den Fall: eine `date`-typisierte Spalte, deren erste
// Regel `< date("2026-01-01")` prüft. Trifft sie nicht, gewinnt die Catch-all-
// Zeile — das ist die stille falsche Antwort, gegen die WP-52 angetreten ist.
const dateModel = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="dt40" name="Fristen" namespace="http://temis/adr0040">
  <inputData id="i40" name="Stichtag"><variable name="Stichtag" typeRef="date"/></inputData>
  <decision id="Frist" name="Frist">
    <variable name="Frist" typeRef="string"/>
    <informationRequirement><requiredInput href="#i40"/></informationRequirement>
    <decisionTable id="t40" hitPolicy="FIRST">
      <input id="ic40"><inputExpression id="ie40" typeRef="date"><text>Stichtag</text></inputExpression></input>
      <output id="o40" typeRef="string"/>
      <rule id="r40a"><inputEntry id="e40a"><text>&lt; date("2026-01-01")</text></inputEntry><outputEntry id="v40a"><text>"alt"</text></outputEntry></rule>
      <rule id="r40b"><inputEntry id="e40b"><text>-</text></inputEntry><outputEntry id="v40b"><text>"neu"</text></outputEntry></rule>
    </decisionTable>
  </decision>
</definitions>`

// declaredTypeDecision kompiliert ein Modell aus dem Quelltext. Das Gegenstueck zu
// compileDecision in trace_test.go, das aus testdata liest; hier steht das Modell
// neben seinen Erwartungen, weil genau seine Spalte der Gegenstand ist.
func declaredTypeDecision(t *testing.T, src, name string) *dmn.CompiledDecision {
	t.Helper()
	defs, diags, err := dmn.New().Compile(context.Background(), []byte(src))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if diags.HasErrors() {
		t.Fatalf("compile diagnostics: %v", diags)
	}
	dec, err := defs.Decision(name)
	if err != nil {
		t.Fatalf("decision %q: %v", name, err)
	}
	return dec
}

func TestADR0040DeclaredDateIsEvaluatedAsADate(t *testing.T) {
	dec := declaredTypeDecision(t, dateModel, "Frist")

	for _, tc := range []struct {
		name string
		in   any
		want string
	}{
		{"ISO-Text, wie ihn ein JSON-Aufrufer sendet", "2025-06-01", "alt"},
		{"ISO-Text nach dem Stichtag", "2027-06-01", "neu"},
		{"bereits ein FEEL-Datum", value.NewDate(2025, time.June, 1), "alt"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := dec.Evaluate(context.Background(), dmn.Input{"Stichtag": tc.in})
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got := res.Outputs["Frist"]; got != tc.want {
				t.Errorf("Frist = %v, want %v — die Datumsregel hat nicht gegriffen", got, tc.want)
			}
		})
	}
}

// Was streng validiert durchkommt, muss auch korrekt auswerten — und umgekehrt.
// Genau diese Äquivalenz war gebrochen.
func TestADR0040ValidationAndEvaluationAgree(t *testing.T) {
	dec := declaredTypeDecision(t, dateModel, "Frist")

	for _, tc := range []struct {
		name     string
		in       any
		wantCode string // "" = konform
		wantGot  string
	}{
		{"gültiger ISO-Text", "2025-06-01", "", ""},
		{"Text, der kein Datum ist", "nonsense", "TYPE_MISMATCH", "string"},
		{"leerer Text", "", "TYPE_MISMATCH", "string"},
		{"Datum mit Uhrzeit als Text", "2025-06-01T10:00:00", "TYPE_MISMATCH", "string"},
		{"gebietsabhängige Schreibweise", "01.06.2025", "TYPE_MISMATCH", "string"},
		{"Zahl", 42, "TYPE_MISMATCH", "number"},
		{"Wahrheitswert", true, "TYPE_MISMATCH", "boolean"},
		{"time.Time ist eine date and time", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), "TYPE_MISMATCH", "date and time"},
		{"echtes FEEL-Datum", value.NewDate(2025, time.June, 1), "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probs := dec.ValidateInput(dmn.Input{"Stichtag": tc.in})
			if tc.wantCode == "" {
				if len(probs) != 0 {
					t.Fatalf("ValidateInput = %+v, want keine Beanstandung", probs)
				}
				return
			}
			if len(probs) != 1 {
				t.Fatalf("ValidateInput = %+v, want genau eine Beanstandung", probs)
			}
			if probs[0].Code != tc.wantCode {
				t.Errorf("Code = %q, want %q", probs[0].Code, tc.wantCode)
			}
			if probs[0].Got != tc.wantGot {
				t.Errorf("Got = %q, want %q — der Ist-Typ muss ein FEEL-Typname sein", probs[0].Got, tc.wantGot)
			}
			if probs[0].Expected != "date" {
				t.Errorf("Expected = %q, want \"date\"", probs[0].Expected)
			}
		})
	}
}

// WithStrictInput lässt genau das durch, was danach richtig rechnet.
func TestADR0040StrictInputAcceptsWhatEvaluatesCorrectly(t *testing.T) {
	dec := declaredTypeDecision(t, dateModel, "Frist")

	res, err := dec.Evaluate(context.Background(), dmn.Input{"Stichtag": "2025-06-01"}, dmn.WithStrictInput())
	if err != nil {
		t.Fatalf("Evaluate(strict) mit gültigem Datum: %v", err)
	}
	if got := res.Outputs["Frist"]; got != "alt" {
		t.Errorf("Frist = %v, want alt", got)
	}

	_, err = dec.Evaluate(context.Background(), dmn.Input{"Stichtag": "nonsense"}, dmn.WithStrictInput())
	if err == nil {
		t.Fatal("Evaluate(strict) mit Unsinn: kein Fehler, want *InputError")
	}
	if !strings.Contains(err.Error(), "expects date") {
		t.Errorf("Fehler = %q, want einen Hinweis auf den erwarteten Typ", err)
	}
}

// Ohne deklarierten Typ ändert sich nichts: ein Custom-Typ (Item Definition,
// WP-31) trägt kein `Type`, und der Wert geht unverändert durch.
func TestADR0040UndeclaredTypeIsUntouched(t *testing.T) {
	const undeclared = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/" id="du40" name="Frei" namespace="http://temis/adr0040b">
  <inputData id="iu40" name="Wert"/>
  <decision id="Echo" name="Echo">
    <informationRequirement><requiredInput href="#iu40"/></informationRequirement>
    <literalExpression id="lu40"><text>Wert</text></literalExpression>
  </decision>
</definitions>`
	dec := declaredTypeDecision(t, undeclared, "Echo")

	res, err := dec.Evaluate(context.Background(), dmn.Input{"Wert": "2025-06-01"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := res.Outputs["Echo"]; got != "2025-06-01" {
		t.Errorf("Echo = %v, want die unveränderte Zeichenkette", got)
	}
	if probs := dec.ValidateInput(dmn.Input{"Wert": "nonsense"}); len(probs) != 0 {
		t.Errorf("ValidateInput = %+v, want keine Beanstandung ohne deklarierten Typ", probs)
	}
}
