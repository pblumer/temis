# ADR-0016: Eigener DMN-Modeler durch Fork des MIT-Kerns, Loslösung von dmn-js

- **Status:** proposed
- **Datum:** 2026-06-29
- **Kontext-WP:** F-02 (Folge-WPs: Modell/1.5-XML im Client, FEEL-Editor-Spike, DRD-Canvas)
- **Ersetzt:** ADR-0006; löst die in ADR-0012 gewählte dmn-js-CDN-Integration ab

## Kontext

ADR-0006 und ADR-0012 legen fest: Editor/Viewer ist **dmn-js unverändert** (per CDN in
`service/ui.go` eingebettet), Schnittstelle ist Standard-DMN-XML, kein eigener Editor-Code.
Diese Linie stößt an eine harte Grenze:

- **Kein 1.5-Authoring.** dmn-js (dmn-moddle) liest/schreibt **DMN 1.3**. temis zielt aber
  auf **1.5** (ADR-0002) inklusive aller **Boxed Expressions** (ADR-0003) — `conditional`,
  `filter`, `for`/`every`/`some`, `context`, `invocation`, `function`. Diese 1.5-Konstrukte
  lassen sich in dmn-js **nicht visuell anlegen**, und beim Speichern eines 1.5-Modells
  **verwirft** dmn-js, was es nicht kennt (verlustbehafteter Round-Trip). ADR-0012 hält das
  als „Grenze" fest — genau diese Grenze wollen wir auflösen.
- **Fremd-Roadmap.** Wann (und ob) dmn-js 1.5 unterstützt, liegt außerhalb unserer Kontrolle.
- **Branding-Klausel.** dmn-js steht unter der **bpmn.io-Lizenz**: das bpmn.io-Logo muss
  sichtbar bleiben, der rendernde Code darf nicht entfernt werden. Für ein Produkt ist das
  ein dauerhafter Klotz (ADR-0012 akzeptierte das nur als Übergang).
- **Verschenkter Trumpf.** temis besitzt einen vollständigen **FEEL**-Stack (Lexer/Parser/
  Compiler + typisiertes Eingabe-Schema, ADR-0003/WP-52). dmn-js kennt FEEL nur oberflächlich.
  Ein eigener Editor kann Zellen **live gegen die echte Engine** validieren/typisieren — der
  konsequente Ausbau der Verifikations-Story (ADR-0013) ins Authoring.

**Entscheidende Unterscheidung gegenüber ADR-0012:** Jenes ADR verwarf das Forken von
**dmn-js** (zu Recht — Lizenz/Logo, Wartungslast). Der teure Unterbau ist aber nicht dmn-js,
sondern dessen **MIT-lizenzierte Primitive**: `diagram-js` (SVG-Canvas, Command-Stack,
Rules, Palette, Context-Pad …), `table-js` (Grid), `moddle`/`moddle-xml`/`dmn-moddle`
(XML ⇄ Objektmodell). „Von dmn-js lösen" heißt hier: den **bpmn.io-lizenzierten dmn-js-
Wrapper wegwerfen** (mitsamt Logo-Klausel) und den **MIT-Kern forken**.

> **Annahme, die die Entscheidung trägt:** `diagram-js`, `table-js`, `moddle`/`dmn-moddle`,
> `min-dom`, `tiny-svg`, `didi` sind MIT-lizenziert; die Logo-Pflicht lebt ausschließlich im
> dmn-js-Paket. Diese Annahme ist **vor Acceptance per Lizenz-Audit zu bestätigen** (siehe
> Konsequenzen) — fällt sie, wird neu bewertet (Option 2 oder 3).

## Optionen

1. **Status quo — dmn-js unverändert einbetten (ADR-0006/0012).** Kein 1.5-Authoring,
   verlustbehafteter 1.5-Round-Trip, Logo-Pflicht, Bindung an Fremd-Roadmap, FEEL nur
   oberflächlich. **Verworfen** (genau die Gründe für dieses ADR).
2. **Nur `dmn-moddle` forken, 1.5 ergänzen, dmn-js-UI behalten.** Billig fürs XML, aber die
   **UI bleibt bpmn.io** (Logo) und die neuen Boxed-Typen sind weiterhin **nicht editierbar**.
   Halbe Sache — löst weder Branding noch Authoring. **Verworfen.**
3. **Greenfield-Modeler von null.** Maximale Kontrolle, null Code-Abstammung — aber
   diagram-js' Canvas/Command-Stack/Routing komplett neu: **Größenordnung Personenjahr**,
   Jahre an Edge-Cases nachgebaut. **Verworfen** (Kosten/Risiko unverhältnismäßig).
4. **MIT-Kern forken, dmn-js-Wrapper ersetzen, 1.5 + FEEL selbst einziehen.** `diagram-js`
   + `table-js` + `dmn-moddle` forken; darauf eine **eigene dünne DMN-Schicht** (Renderer,
   Modellierungsregeln, Palette, Context-Pad); `dmn-moddle`-Fork um **1.5-Deskriptoren**
   erweitern; FEEL-Zellen **gegen temis** validieren. Kein bpmn.io-Logo (lebt im verworfenen
   dmn-js-Layer). Aufwand **~1–3 Personenmonate** statt Personenjahr. **Gewählt.**

## Entscheidung

**Option 4.** temis bekommt einen **eigenen DMN-Modeler inkl. DRD-Canvas**, aufgebaut auf
einem Fork des MIT-Kerns, vollständig gelöst vom Projekt dmn-js.

- **Forken statt neu schreiben:** `diagram-js` (Canvas/Interaktion/Command-Stack),
  `table-js` (Decision-Table-Grid), `dmn-moddle`/`moddle`/`moddle-xml` (XML-Modell). Der
  **dmn-js-Wrapper entfällt** — und mit ihm die bpmn.io-Logo-Pflicht.
- **Eigene DMN-Schicht** auf den Forks: Renderer für Decision/InputData/BKM/KnowledgeSource
  + die vier Requirement-Typen, DMN-Modellierungsregeln, Palette/Context-Pad, Boxed-
  Expression-Editor (1.5) und der Decision-Table-Editor.
- **1.5 ist Pflicht:** Der `dmn-moddle`-Fork wird um den **1.5-Namespace** und die neuen
  Element-Deskriptoren ergänzt; der Modeler schreibt **DMN 1.5**. DRD-/Tabellen-Struktur ist
  versionsstabil, der Delta ist v. a. Namespace + neue Boxed-Typen.
- **temis bleibt XML-/FEEL-/1.5-Autorität.** Das Client-Modell serialisiert über temis bzw.
  wird gegen temis validiert (Round-Trip-Hoheit liegt im Backend, ADR-0010/WP-02). FEEL-
  Zellen werden **live gegen die echte Engine** geprüft (per WASM im Browser oder per API) —
  das ist der eigentliche Mehrwert der Loslösung.
- **Architektur-Leitplanke:** Command-Stack/Undo-Redo ist **Fundament, nicht Nachrüstung**;
  jede Mutation ist von Beginn an ein reversibles Command.
- **Build-Reihenfolge:** (1) Client-Modell + 1.5-XML-Round-Trip → (2) Command-Stack →
  (3) Decision-Table-Editor (höchster Nutzwert, validiert FEEL-Integration früh) →
  (4) DRD-Canvas iterativ (Render/Selektion/Move → Connect/Rules → Palette/Context-Pad →
  Routing/Snapping). FEEL-Integration als Querschnitt von Tag 1.

**Acceptance-Gate:** Dieses ADR bleibt `proposed`, bis (a) das **Lizenz-Audit** die MIT-
Annahme bestätigt und (b) ein **FEEL-Validierungs-Spike** (Tabellen-Zelle live gegen temis)
das Backend-Zusammenspiel belegt. Erst dann `accepted`.

## Konsequenzen

**Positiv**
- **1.5-Hoheit & verlustfreier Round-Trip** — Boxed Expressions visuell editierbar, kein
  stilles Verwerfen mehr.
- **Kein bpmn.io-Logo/-Branding** (lebt nur im verworfenen dmn-js-Wrapper; MIT-Forks tragen
  es nicht).
- **FEEL-Differenzierer:** „der einzige DMN-Editor, der gegen die Engine validiert, die das
  Modell danach ausführt" — direkter Ausbau von ADR-0013.
- **Kein Personenjahr:** Der teure Canvas/Command-Stack kommt aus dem MIT-Fork; eigener
  Aufwand konzentriert sich auf DMN-Schicht, 1.5 und FEEL.
- **BPMN-Synergie (strategisch).** `bpmn-js` und `dmn-js` teilen denselben MIT-Kern
  (`diagram-js`, `moddle`, `didi`, `min-dom`, `tiny-svg`) — BPMN unterscheidet sich nur in
  der **fachlichen Schicht** (`bpmn-moddle` + BPMN-Renderer/Regeln). Der hier gewählte Fork
  des Kerns ist damit zugleich das Fundament für einen **eigenen BPMN-Editor** in einer
  künftigen BPMN-Workflow-Engine: derselbe Canvas/Command-Stack/Palette-Unterbau, dieselbe
  Toolchain, gemeinsame FEEL-Integration. Die DMN-Schicht ist die erste fachliche Schicht auf
  diesem Kern, BPMN eine zweite — beide aus einem Haus. Diese Synergie ist ein zusätzliches
  Argument für Option 4 (und gegen Greenfield, das den Vorteil verschenken würde); ein
  separates BPMN-ADR konkretisiert das später.

**Negativ / Spannungen**
- **Fork-Wartungslast & kein Upstream** — Sicherheits-/Bugfixes von diagram-js müssen aktiv
  nachgezogen werden (bewusst akzeptiert für Kontrolle + 1.5).
- **Frontend-Toolchain kehrt zurück.** Eine eigene JS/TS-Build- und CI-Lane (npm/Vite o. Ä.)
  steht im Widerspruch zu ADR-0012 („keine zweite Toolchain"). Diese Konsequenz wird bewusst
  in Kauf genommen; die Assets werden per `go:embed` ausgeliefert, damit `temisd`/`/ui`
  weiterhin **ein** Binary ohne CDN bleibt (löst zugleich den Offline-/„no CDN"-Anspruch).
- **Code-Abstammung** mit bpmn-io bleibt über die MIT-Forks bestehen (MIT erlaubt das voll;
  falls eine Policy auch das ausschließt, ist Option 3 die Rückfallebene).

**Folgeaufgaben**
- **Lizenz-Audit** diagram-js/table-js/dmn-moddle/moddle/min-dom/tiny-svg/didi (Acceptance-
  Gate); Ergebnis hier nachtragen.
- **FEEL-Validierungs-Spike** (1–2 Tage): Tabellen-Zelle live gegen temis-FEEL.
- Neue Roadmap-WPs: Client-Modell + 1.5-`dmn-moddle`-Deskriptoren, Command-Stack, Decision-
  Table-Editor, DRD-Canvas, FEEL-Integration, `go:embed`-Auslieferung.
- **Migration `service/ui.go`:** CDN-dmn-js → eigener, eingebetteter Modeler.
- **ADR-0006** auf `superseded by ADR-0016` setzen; **ADR-0012** Integration (dmn-js per CDN)
  ist damit überholt — Status entsprechend markieren.
