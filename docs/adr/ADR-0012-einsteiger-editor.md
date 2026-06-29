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
2. **Separates Frontend mit eigener npm-/Vite-Toolchain** (`web/`) — dmn-js per npm, eigener
   Build, eigene CI-Lane. Funktioniert, war aber in der Praxis umständlich: separater
   Dev-Server, zweite Toolchain, doppelte UI neben der bestehenden `/ui`-Seite. **Verworfen.**
3. **In den bestehenden Service `/ui` integriert, dmn-js per CDN unverändert geladen** — die
   schon vorhandene `service/ui.go`-Seite (Go-Raw-String, keine zweite Toolchain) bettet
   dmn-js wie die Swagger-UI unter `/docs` von `cdn.jsdelivr.net` ein. Kein npm-Build, kein
   separater Server: `temisd` startet, `/ui` ist der Editor. **Gewählt.**

## Entscheidung
Option 3. F-01 bleibt **kein Produktziel der Engine**, wird aber von „Demo-Viewer" zum
**Editor in `/ui`** ausgebaut:

- **Kein separates Frontend, keine zweite Toolchain.** Der Editor ist Teil der bestehenden
  `service/ui.go`-Seite. dmn-js wird per **CDN** (`cdn.jsdelivr.net`, UMD-Bundles) geladen —
  exakt das Muster, das `/docs` (Swagger UI) schon nutzt. Das Go-Modul bleibt ohne
  vendored Frontend-Assets (ADR-0006, ADR-0011); es gibt keinen JS-Build-Schritt.
- **dmn-js unverändert**, inkl. sichtbarem bpmn.io-Logo; nie Fork/Patch. Read-only =
  `dmn-navigated-viewer`, bearbeitbar = `dmn-modeler` (beide UMD-Bundles, global `window.DmnJS`,
  sequenziell geladen).
- **Bedienfluss:** Datei hochladen (oder XML einfügen) → **read-only** in dmn-js gerendert →
  Schalter **„Bearbeiten"** öffnet dasselbe Modell im editierbaren Modeler → **„Auf Server
  deployen"** (`POST /v1/models`) macht Decisions/Inputs auswertbar → Auswerten
  (`POST /v1/models/{id}/evaluate` bzw. `/v1/evaluate`).
- **Schnittstelle bleibt Standard-DMN-XML** (ADR-0006); Round-trip durch WP-02 garantiert.
- **Grenze:** dmn-js rendert **DMN 1.3** (was es selbst schreibt). Modelle in 1.4/1.5 werden
  von der Engine ausgewertet, aber ggf. nicht im Editor gezeichnet — akzeptiert, da in dmn-js
  erstellte Dateien 1.3 sind.

## Konsequenzen
- **Positiv:** Ein einziger Einstieg (`temisd` → `/ui`), keine zweite Toolchain/CI-Lane, kein
  Build; konsistent mit `/docs`. dmn-js-Updates per CDN-Versionspin. End-to-End verifiziert
  (Upload → read-only → bearbeiten → deployen → auswerten) per Headless-Browser.
- **Negativ:** `/ui` lädt dmn-js zur Laufzeit vom CDN (wie `/docs`), also online beim ersten
  Aufruf; das bpmn.io-Logo ist verpflichtend sichtbar (akzeptiert). Offline-Betrieb müsste
  die Bundles später per `go:embed` ausliefern (Folgeaufgabe, falls nötig).
- **Folgeaufgaben:** F-02 — Einsteiger-Module (reduzierte Palette, Tabellen-Vorlagen,
  Inline-FEEL-Hilfe) und das **Diagnostics-Overlay**, das temis-`line/col`-Diagnostics auf
  Tabellenzellen mappt. Optional: dmn-js-Bundles per `go:embed` für Offline-`/ui`.

> **Hinweis:** Diese Entscheidung ersetzt die zuvor (in derselben ADR) gewählte Variante
> „separates `web/`-Frontend". Der Kern bleibt unverändert: **dmn-js einbetten, nicht forken.**
