# Integrations- & Quickstart-Leitfaden

> **Zielgruppe:** Wer temis **nutzt** (nicht baut). Drei Wege rein: als **Go-Library**,
> als **HTTP-Service** (`temisd`) und über einen **DMN-Editor** (dmn-js / eigener Modeler)
> per Standard-DMN-XML. Die stabile API ist in `docs/40-api-contract.md` fixiert; hier
> stehen lauffähige Einstiege.

---

## 1. Quickstart: Go-Library

temis ist **library-first** (ADR-0005/0011): die Engine ist reines Go, ohne Transport,
ohne Server. Eine einzige öffentliche Stelle — `package dmn` — exportiert die zweiphasige
API: einmal `Compile`, beliebig oft `Evaluate`.

```go
package main

import (
	"context"
	"fmt"

	"github.com/pblumer/temis/dmn"
)

func main() {
	xml := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="https://www.omg.org/spec/DMN/20230324/MODEL/"
             namespace="http://example/double" name="Double" id="def">
  <inputData id="id_n" name="N"><variable name="N" typeRef="number"/></inputData>
  <decision id="id_d" name="Double">
    <variable name="Double" typeRef="number"/>
    <informationRequirement><requiredInput href="#id_n"/></informationRequirement>
    <literalExpression><text>N * 2</text></literalExpression>
  </decision>
</definitions>`)

	eng := dmn.New() // re-entrant, share across goroutines

	defs, diags, err := eng.Compile(context.Background(), xml)
	if err != nil {
		panic(err) // malformed XML
	}
	if diags.HasErrors() {
		panic(diags) // per-decision compile problems
	}

	dec, err := defs.Decision("Double")
	if err != nil {
		panic(err)
	}

	res, err := dec.Evaluate(context.Background(), dmn.Input{"N": 21})
	if err != nil {
		panic(err)
	}
	fmt.Println(res.Outputs["Double"]) // 42
}
```

**Kernpunkte**

- **Zweiphasig:** `Compile` ist teuer und einmalig; das Ergebnis (`Definitions`,
  `CompiledDecision`) ist **immutable & thread-safe** und wird beliebig oft, billig,
  nebenläufig ausgewertet.
- **DRG-Verkettung:** `Evaluate` einer Decision wertet automatisch alle benötigten
  Decisions vorher aus und reicht ihre Ergebnisse namentlich ein; der Aufrufer liefert nur
  die Blatt-Inputs. `Result.Decisions` listet jede ausgewertete Decision.
- **Limits:** `dmn.New(dmn.WithLimits(dmn.Limits{…}))` begrenzt Rekursion/Iteration/
  Listengröße und einen Compile-Timeout — feindlicher Input wird ein sauberer Fehler statt
  Hang/OOM (ADR-0008).
- **Striktes Eingabe-Schema:** `CompiledDecision.InputSchema()` beschreibt die erwarteten
  Inputs samt FEEL-Typ; `dec.Evaluate(ctx, in, dmn.WithStrictInput())` validiert vorab und
  liefert präzise Fehler (`TYPE_MISMATCH`/`UNKNOWN_INPUT`/`MISSING_INPUT`) statt still `null`.
- **Erklärbarkeit:** `dmn.WithTrace()` liefert zusätzlich `Result.Trace` (welche Regeln
  feuerten, welche Bedingungen erfüllt/verfehlt waren).

Lauffähige, godoc-sichtbare Beispiele: `dmn/example_test.go` (`go doc github.com/pblumer/temis/dmn`).

---

## 2. Quickstart: HTTP-Service (`temisd`)

Derselbe Engine-Kern, als dünner Netzwerk-Adapter (ADR-0011). Kein Engine-Verhalten lebt
im Service — er übersetzt nur HTTP ⇄ `package dmn`.

```sh
go run ./cmd/temisd -addr :8080
```

**Stateless auswerten** (Modell + Eingabe in einem Request):

```sh
curl -X POST localhost:8080/v1/evaluate -H 'Content-Type: application/json' -d "{
  \"xml\": $(jq -Rs . < dmn/testdata/models/dish_15.dmn),
  \"decision\": \"Dish\",
  \"input\": {\"Season\": \"Winter\", \"Guest Count\": 8}
}"
# → {"outputs":{"Dish":"Roastbeef"}, ...}
```

**Stateful** (einmal hochladen, per content-addressed `modelId` wiederverwerten):

```sh
ID=$(curl -s --data-binary @dmn/testdata/models/dish_15.dmn \
      -H 'Content-Type: application/xml' localhost:8080/v1/models | jq -r .id)

curl -X POST localhost:8080/v1/models/$ID/evaluate \
     -H 'Content-Type: application/json' \
     -d '{"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}'
```

**Endpunkte:** `POST /v1/models`, `GET /v1/models`, `GET /v1/models/{id}`,
`POST /v1/models/{id}/evaluate`, `POST /v1/evaluate`, `GET /healthz`/`/readyz`.
Spec: `service/openapi.yaml` (interaktiv unter `GET /docs`), Vertrag in
`docs/40-api-contract.md §2`. Fehler als RFC-7807 `application/problem+json`.

**Betriebsschalter:** `-token` (Bearer-Auth für `/v1`), `-list-models=false`
(`GET /v1/models` → 404), `-cache-size` (LRU-Modell-Cache), `-max-call-depth`/
`-max-iterations`/`-max-list-size`/`-compile-timeout` (Limits, siehe oben).

---

## 3. DMN-Editor-Integration (dmn-js / eigener Modeler)

**Vertrag = Standard-DMN-XML.** temis liest und schreibt ausschließlich
**Standard-DMN-XML** (1.5; tolerant 1.3/1.4). Es gibt nichts Proprietäres zu adaptieren:
ein beliebiger DMN-Editor, der konformes XML erzeugt — voran **dmn-js** — ist out of the
box kompatibel. Pflicht ist der **verlustfreie Round-Trip** (WP-02, ADR-0010): eine im
Editor gespeicherte Datei lädt temis, und eine von temis (un)veränderte Datei öffnet der
Editor wieder — inklusive `DMNDI`-Diagramm-Layout.

### Der Integrationsfluss

```
 ┌────────────┐   DMN-XML    ┌──────────────┐  /v1/models   ┌──────────┐
 │ DMN-Editor │ ───────────▶ │  dein Backend │ ────────────▶ │  temisd  │
 │ (dmn-js)   │ ◀─────────── │  / Frontend   │ ◀──────────── │ (Engine) │
 └────────────┘   gerendert  └──────────────┘   outputs +    └──────────┘
                  Ergebnis                      trace/diags
```

1. **Authoring:** Editor erzeugt/bearbeitet das DRD und die Decision-Tables → exportiert
   DMN-XML (`modeler.saveXML()`).
2. **Deploy:** XML an `POST /v1/models` (oder direkt `POST /v1/evaluate` stateless).
3. **Evaluate:** Eingabewerte sammeln (das `schema` aus der Modell-Antwort beschreibt
   Decisions + erwartete Inputs), an `…/evaluate` mit `explain: true` für die Spur.
4. **Render zurück:** Die durchlaufenen Decisions (`result.decisions`) im DRD markieren und
   mit ihrem Wert beschriften; ungültige FEEL-Zellen über `diagnostics[].line/col` auf die
   betroffene Tabellenzelle mappen.

### Eingebettetes Beispiel: der DMN-Modeler an `/`

`temisd` liefert unter `GET /` einen lauffähigen, **eigenen DMN-Modeler** (ADR-0016, kein
dmn-js, kein CDN, offline), der genau diesen Fluss demonstriert: Datei öffnen
(`POST /v1/models`) → DRD-Canvas (eigene Renderer) bearbeiten — Knoten verschieben/umbenennen/
typisieren, **Decision-Tables ansehen & editieren** (Zellen/Regeln, live FEEL-validiert) →
**Speichern** zurück ins DMN-XML (`/save`, `/decisions/{d}/table`) → Decision auswerten
(`/evaluate`). Das Frontend lebt in `web/` (Vite/TypeScript) und wird per `go:embed`
ausgeliefert; es nutzt ausschließlich die `/v1`-Endpunkte. Die Alt-Pfade `/ui` und `/app/`
leiten dauerhaft auf `/` um.

```sh
go run ./cmd/temisd -addr :8080
# Browser: http://localhost:8080/
```

### Round-Trip absichern

Wer einen eigenen Editor anbindet, sollte den Round-Trip testen wie temis selbst
(`TestRoundTripXMLFidelity`, `docs/50-testing-strategy.md §4`): Datei laden → unverändert
serialisieren → muss gültiges DMN-XML mit erhaltenem `DMNDI` bleiben.

> **Ausblick (ADR-0016).** Mittelfristig löst temis die CDN-dmn-js-Einbettung durch einen
> **eigenen, eingebetteten Modeler** auf einem Fork des MIT-Kerns (diagram-js/table-js/
> dmn-moddle) ab — für 1.5-Authoring inkl. Boxed Expressions, offline (`go:embed`, kein CDN),
> ohne bpmn.io-Logo und mit Live-FEEL-Validierung gegen die echte Engine (Etappe „Eigener
> Modeler", WP-60–67 in `docs/20-roadmap.md`). Der **XML-Round-Trip-Vertrag bleibt
> unverändert** — die Integration über Standard-DMN-XML ist davon unberührt.

---

## 4. Für KI-Agenten (MCP)

Ein vierter Weg, speziell für Agenten als Laufzeit-Konsumenten: `temis-mcp` bietet die
Engine über das **Model Context Protocol** an (stdio oder HTTP). Tools: `list_models`,
`load_model`, `describe_decision`, `evaluate`. Details und Begründung (ADR-0013) im README
(„Für KI-Agenten") und `docs/60-ai-agent-guide.md`.
