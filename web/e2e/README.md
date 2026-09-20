# End-to-End-Tests (Modeler-Frontend)

Playwright-Specs, die den **vollen Stack wie ein echter Nutzer** ausüben: ein
echtes Chromium fährt gegen den eingebetteten Modeler, der von `temisd`
ausgeliefert wird — inklusive der WASM-FEEL-Engine und der gebauten `dist/`.
Kein Mock, kein Stub: was hier grün ist, funktioniert im Browser.

Verbindliche Teststrategie: [`docs/50-testing-strategy.md`](../../docs/50-testing-strategy.md) §4.

## Wie fahre ich sie?

```bash
# Aus dem Repo-Root — baut Frontend (WASM + dist) und läuft die Suite:
make web-e2e

# Direkt (setzt voraus, dass `make web` schon lief):
cd web && npm run test:e2e            # == npx playwright test

# Einzelne Spec / einzelner Test:
cd web && npx playwright test operate
cd web && npx playwright test -g "run history is keyboard-navigable"

# Sichtbarer Browser / Debugging:
cd web && npx playwright test --headed
cd web && npx playwright test --ui
```

### Architektur des Laufs (`playwright.config.ts`)

- **`webServer`** startet `go run ./cmd/temisd -addr 127.0.0.1:8099 -examples=true -mcp=false`.
  Playwright wartet, bis der Server antwortet, und fährt ihn danach herunter.
  Voraussetzung: `web/dist` ist gebaut (`make web`) — der Server bettet es ein.
- **Browser:** von Playwright verwaltetes Chromium, oder ein vorinstalliertes über
  `PLAYWRIGHT_CHROMIUM_PATH` (praktisch in Sandboxes). Port via `TEMIS_E2E_PORT`.
- **CI:** Lane `web-e2e` in `.github/workflows/ci.yml` — `retries: 1`,
  `trace: on-first-retry`. `forbidOnly` verhindert vergessenes `test.only`.
- **Seed-Daten:** `-examples=true` lädt die gebündelten Beispielmodelle (u. a.
  „Discount"), gegen die viele Specs auswerten.

> **Nicht Teil von `make verify`.** Der E2E-Lauf braucht Browser + Node-Build und
> läuft als eigene CI-Lane (`web-e2e`), nicht im Go-Gate. Lokal separat fahren.

## Was wird getestet? (Katalog)

Jede Spec trägt oben einen Intent-Kommentar; hier die Übersicht. Zahl = Testfälle.

### Auswertung, Cockpit & Live-Graph
| Spec | # | Deckt ab |
|---|---|---|
| `operate.spec.ts` | 1 | Operate-Cockpit: tastaturnavigierbare Run-History, Overlays, Decision-Table-Popover mit Hit-Regel, Decision-Path-Trace |
| `flow.spec.ts` | 1 | Flows-View: Flow-Graph durchsuchen und mit Ergebnis pro Schritt auswerten |
| `flow-editor.spec.ts` | 1 | Flow Designer: Flow visuell bauen, testen und registrieren |
| `flow-layout.spec.ts` | 1 | Autolayout-Regression (dagre): geteilte Quellen ⇒ kollisionsfreie Layer |
| `live-graph.spec.ts` | 1 | Nach Auswertung leuchten die Requirement-Kanten mit ihrem Fluss |
| `input-pills.spec.ts` | 1 | On-Canvas-Eingabe-Pillen: Leaf-Input ändern ⇒ Graph wertet neu aus |
| `juice.spec.ts` | 1 | „Juice": Auswertung streamt die Wires; ⚡-Toggle schaltet Effekte |
| `table-wired-input.spec.ts` | 1 | Verdrahteter Input ohne Spalte wird beim Öffnen der Tabelle sichtbar |

### Modellierung & Editor
| Spec | # | Deckt ab |
|---|---|---|
| `palette.spec.ts` | 4 | DMN-Palette: Elemente aus der Toolbar erzeugen; Ghost-Click-Regression |
| `create-table.spec.ts` | 1 | Frisch abgelegter Decision eine Tabelle geben — ohne manuelles Speichern |
| `bkm.spec.ts` | 1 | BKM ablegen und Funktion editieren — ohne vorheriges Speichern |
| `dblclick-gesture.spec.ts` | 4 | Doppelklick öffnet überall den Inhalt eines Elements (nie Inline-Edit) |
| `edge-style.spec.ts` | 1 | Kanten-Context-Pad: Requirement-Kante eckig/gerundet/… umschalten |
| `orientation.spec.ts` | 1 | DRD-Orientierungs-Toggle: Inputs speisen von unten oder von oben |
| `highlight.spec.ts` | 5 | FEEL-Syntax-Highlighting: Funktionen, Variablen, Keywords, Strings, Zahlen |
| `completion.spec.ts` | 9 | Code-Completion: In-Scope-Variablen + Built-in-Katalog der Engine |
| `json-editor.spec.ts` | 1 | JSON-Editor: Wert zurückschreiben, formatieren, validieren |

### Verwaltung, Import & Betriebsanzeigen
| Spec | # | Deckt ab |
|---|---|---|
| `model-admin.spec.ts` | 1 | Modell anlegen, umbenennen, löschen (Sidebar) |
| `model-search.spec.ts` | 1 | Live-Textfilter über die Modell-Liste |
| `testimport.spec.ts` | 8 | Import-Cockpit: Samples laufen das Band hinunter in den Clio-Store |
| `clio-status.spec.ts` | 1 | Clio-Badge (ADR-0030) spiegelt den realen Sink-Zustand |
| `resizable.spec.ts` | 2 | Sidebar/Panel-Divider: Größe ändern und persistieren |

### Robustheit / Sicherheits-Regressionen
| Spec | # | Deckt ab |
|---|---|---|
| `empty-server.spec.ts` | 1 | Audit H3: leerer Server bootet die Shell ohne Early-Return |
| `escaping.spec.ts` | 1 | Audit H1: Werte in quotierten Attributen injizieren kein Markup (XSS) |

## Konventionen für neue Specs

- **Intent-Kommentar** oben ins File: *was* die Spec absichert und *warum* (bei
  Regressionen den Audit-/WP-/ADR-Bezug nennen — siehe `escaping`, `operate`).
- Gegen die gebündelten Beispielmodelle auswerten (`-examples=true`) statt eigene
  Fixtures zu erfinden, wo möglich.
- Nach echtem, nutzersichtbarem Zustand assertieren (Text, ARIA, Sichtbarkeit),
  nicht gegen Interna.
- Diese Tabelle mitpflegen, wenn eine Spec dazukommt oder ihren Zweck ändert.
