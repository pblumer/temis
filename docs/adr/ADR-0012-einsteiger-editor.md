# ADR-0012: F-01 wird zum Einsteiger-Editor (separates Frontend, dmn-js unverändert)

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** F-01

## Kontext
ADR-0006 legt fest: dmn-js ist Editor/Viewer, Schnittstelle ist Standard-DMN-XML, im
Engine-Repo entsteht **kein** Frontend-Code. F-01 war bisher als reine Demo-Webseite
(Modell laden → auswerten) skizziert; umgesetzt ist davon der **Playground** in
`service/ui.go` — ein *Evaluator*, kein Editor.

Offene Frage aus der Praxis: Soll temis Einsteigern eine **einfache Oberfläche zum
Erstellen** von DMN-Diagrammen anbieten? Naheliegender Reflex ist, dmn-js zu **forken**
und „zu entschlacken". Das ist aus drei Gründen falsch:

- **Lizenz:** dmn-js steht unter der bpmn.io-Lizenz — Quelle offen, kommerzielle Nutzung
  frei, **aber** das bpmn.io-Logo muss sichtbar bleiben und der Code, der es rendert, darf
  nicht entfernt/verändert werden. Ein „Vereinfachungs"-Fork läuft genau in diese Klausel.
- **Architektur:** dmn-js ist auf Erweiterung über Module (diagram-js / table-js /
  dmn-moddle, DI-Container) ausgelegt. Palette, Context-Pad, Renderer etc. ersetzt/ergänzt
  man über eigene Module — ohne die Codebasis anzufassen. Ein Fork erbt nur Wartungslast und
  verliert Upstream-Updates.
- **Mehrwert:** Einsteigerfreundlichkeit entsteht nicht aus einem anderen Editor, sondern
  aus dem Drumherum — und dort hat temis ein Alleinstellungsmerkmal: die Engine liefert
  bereits **Diagnostics mit `line/col`** und eine **Live-Auswertungs-API** (`/v1/...`).

## Optionen
1. **dmn-js forken & anpassen** — hoher Aufwand, Wartungslast, Lizenzrisiko (Logo), kein
   Mehrwert gegenüber Modul-Erweiterung. **Verworfen.**
2. **Editor-Code ins Engine-Repo legen** — verletzt ADR-0006 (kein Frontend im Go-Repo),
   mischt Go- und JS-Toolchains. **Verworfen.**
3. **Separates Frontend, dmn-js unverändert eingebettet** — F-01 wird vom Demo-Viewer zum
   Einsteiger-Editor erweitert: dmn-js per npm, Einsteiger-UX über **eigene Module**
   (reduzierte Palette, Tabellen-Vorlagen, Inline-Hilfe), verdrahtet gegen die bestehende
   temis-HTTP-API für Live-Auswertung und inline gemappte Diagnostics. **Gewählt.**

## Entscheidung
Option 3. F-01 bleibt **kein Produktziel der Engine**, wird aber von „Demo" zu
„Einsteiger-Editor" konkretisiert:

- **Eigenständiges Frontend** (Verzeichnis `web/`, eigener npm-Build, eigene CI-Lane),
  getrennt vom Go-Modul. Das Go-Modul bleibt frontend-frei (ADR-0006, ADR-0011).
- **dmn-js unverändert** (npm-Dependency), inkl. sichtbarem bpmn.io-Logo. Einsteiger-UX
  ausschließlich über **additive Module**, nie durch Fork/Patch.
- **Schnittstelle bleibt Standard-DMN-XML** (ADR-0006). Round-trip ist durch WP-02
  garantiert; der Editor spricht die bestehenden Endpunkte `POST /v1/models`,
  `POST /v1/models/{id}/evaluate`, `POST /v1/evaluate`.
- **Einsteiger-Mehrwert = Drumherum:** Live-Auswertung beim Editieren, temis-Diagnostics
  (`line/col`) inline an Tabellenzellen, mitgelieferte Vorlagen/Beispiele.

## Konsequenzen
- **Positiv:** Klare Einsteiger-Oberfläche ohne Lizenzrisiko und ohne Fork-Wartung; Engine-
  Repo bleibt sauber; temis-Diagnostics werden sichtbar nutzbar; Upstream-dmn-js-Updates
  fließen per `npm update` ein.
- **Negativ:** Zusätzliche JS-Toolchain/CI-Lane für `web/`; das bpmn.io-Logo ist
  verpflichtend sichtbar (akzeptiert).
- **Folgeaufgaben:** F-01 in `20-roadmap.md` entsprechend fassen; `web/`-Gerüst (dmn-js +
  Live-Eval gegen `/v1`) aufsetzen; spätere Einsteiger-Module (Palette, Vorlagen,
  Diagnostics-Overlay) als eigene F-Pakete nachziehen.
