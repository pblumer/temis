# ADR-0010: DMNDI-Round-trip über verbatim erfassten Token-Stream

- **Status:** accepted
- **Datum:** 2026-06-11
- **Kontext-WP:** WP-02

## Kontext
WP-02 verlangt, dass eine geladene DMN-Datei verlustfrei wieder serialisiert
werden kann und der `DMNDI`-Diagramm-Interchange-Block dabei erhalten bleibt
(dmn-js muss die Datei danach wieder öffnen können). Go's `encoding/xml`
behandelt XML-Namespaces beim Marshalling nicht verlustfrei: Präfixe und
`xmlns`-Deklarationen werden umgeschrieben, `XMLName.Space` wird beim Encoden
sogar ignoriert. Ein naiver Struct-Round-trip des stark namespace-behafteten
`DMNDI` (Namespaces `dmndi`, `dc`, `di`) würde dessen Inhalt beschädigen.

## Optionen
1. **DMNDI vollständig in Structs modellieren** — viel Code (Shape, Bounds, Edge,
   Waypoints, Styles), und beim Encode bleibt das Namespace-Problem bestehen.
   Für die Ausführung ist DMNDI irrelevant, nur für den Editor-Round-trip nötig.
2. **DMNDI als Token-Stream verbatim erfassen und replayen** — beim Decoden den
   `<DMNDI>`-Teilbaum (inkl. Start/Ende) als kopierte `xml.Token`-Folge sichern,
   beim Encoden über `EncodeToken` wieder ausgeben. Geringer Aufwand, Inhalt
   bleibt strukturell und in allen Werten erhalten; Präfixe dürfen sich ändern.
3. **Byte-genaue Extraktion aus dem Quelltext** — exakt, aber fragil beim
   Wieder-Einfügen in die `encoding/xml`-Ausgabe.

## Entscheidung
Option 2. Der Typ `internal/xml.Raw` erfasst den `DMNDI`-Teilbaum als
Token-Stream und gibt ihn beim Encoden unverändert wieder aus. Der
Model-Default-Namespace der `<definitions>`-Wurzel wird über ein explizit
gesetztes `xmlns`-Attribut beim Encode rekonstruiert.

Round-trip-Treue ist **semantisch** definiert, nicht byte-identisch: gleiche
Element-Struktur, Attributwerte und Texte; Namespace-Präfixe dürfen abweichen.
Das deckt die Anforderung „DMNDI bleibt erhalten" aus
`docs/50-testing-strategy.md` ab.

## Konsequenzen
- **Positiv:** minimaler Code, robust gegen `encoding/xml`-Namespace-Eigenheiten,
  garantiert keinen DMNDI-Datenverlust.
- **Negativ:** DMNDI ist (noch) nicht typisiert auswertbar — für reine Ausführung
  unnötig, für spätere Editor-/Layout-Features ggf. nachzurüsten.
- **Folgeaufgabe:** Der Round-trip-Test vergleicht eine präfix-unabhängige
  Projektion des DMNDI-Token-Streams (siehe `internal/xml/roundtrip_test.go`).
