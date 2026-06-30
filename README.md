# Temis — DMN-Engine (Go)

**Repository:** https://github.com/pblumer/temis

> **GitHub-Beschreibung (About):**
> Fast, embeddable DMN 1.5 decision engine in Go with full FEEL support — usable as a library or HTTP/gRPC service.
>
> **Topics:** `dmn` · `dmn-engine` · `feel` · `decision-engine` · `business-rules` · `golang` · `dmn-js` · `decision-tables` · `rules-engine`

**Temis** ist eine schnelle, eingebettete **DMN-1.5**-Engine in Go mit vollem **FEEL**-Support,
betreibbar als **Library** und **HTTP/gRPC-Service**. Modelle werden im **dmn-js**-Editor erstellt
und als Standard-DMN-XML geladen.

> Der Name spielt auf *Themis* an, die griechische Göttin der Ordnung, Gerechtigkeit und
> des Urteils — passend zu einer Engine, die Entscheidungen trifft. Schreibweise bewusst
> ohne „h": eindeutige Aussprache, sauberer als Binary-/Modulname.

## Status

**Aktiv in Entwicklung.** Das Fundament der Engine steht; das MVP (lauffähige Library, die
reale dmn-js-Dateien auswertet) wird entlang der Arbeitspakete in `docs/20-roadmap.md` gebaut.
Jedes Arbeitspaket landet als eigener, CI-grüner Pull Request (`make verify`: fmt, vet, lint,
`-race`, Benchmarks).

| Arbeitspaket | Inhalt | Stand |
|---|---|---|
| WP-01 | Projektgerüst, Makefile, CI | ✅ |
| WP-02 | DMN-XML-Decoding (1.5, tolerant 1.3/1.4) → Modell, `DMNDI`-Round-trip | ✅ |
| WP-03 | FEEL-Lexer | ✅ |
| WP-04 | FEEL-Parser → AST | ✅ |
| WP-05 | FEEL-Wertemodell, Number als Decimal (`apd`), Temporaltypen | ✅ |
| WP-06 | FEEL-Compiler-Kern (AST → Closure, Slot-Index-Variablen) | ✅ |
| WP-07 | FEEL-Built-ins (Kern) | ✅ |
| WP-08 | Unary Tests | ✅ |
| WP-09 | Decision-Table-Compiler + Hit Policies U/A/F/R/C | ✅ |
| WP-10 | Öffentliche Library-API (`dmn.Engine`, Compile/Evaluate) | ✅ |
| WP-11 | MVP-Beispiele & Golden-Tests | ✅ |
| WP-20 | FEEL vollständig (`for`/`some`/`every`, Filter, Pfad-Projektion) | ✅ |
| WP-21 | FEEL-Built-ins vollständig (nicht-temporal: string/numeric/list/context/range/sort) | ✅ |
| WP-22 | Date/Time/Duration + temporale Built-ins, Komponentenzugriff, `@`-Literale | ✅ |
| WP-32 | HTTP-Service (`temisd`): `/v1/models`, `/v1/evaluate`, OpenAPI | ✅ |
| WP-50 | Agent-First: MCP-Server (`temis-mcp`) über stdio | ✅ |
| WP-51 | Agent-First: Entscheidungsspur (`Result.Trace`, `explain`) | ✅ |
| WP-52 | Agent-First: typisiertes Eingabe-Schema & strikte Validierung | ✅ |
| WP-53 | Agent-First: Remote-MCP über HTTP (`temis-mcp -http`) | ✅ |
| WP-70 | Git-gestützte Modelle: Lesen/Browsen (`vcs` + GitHub-Provider) | ✅ |
| WP-71 | Git-gestützte Modelle: Schreiben (`vcs.Writer`, Commit/Branch/PR) | ✅ |

> **MVP erreicht (WP-01–11); Beta abgeschlossen.** Über die oben gelisteten Pakete hinaus
> sind inzwischen u. a. **WP-23–26** (Boxed Context/Invocation/Function, BKM, DRG-Verkettung,
> Decision Services), **WP-27** (alle Hit Policies inkl. PRIORITY/OUTPUT ORDER), **WP-30/31**
> (Typsystem, `instance of`, Item-Definition-Constraints), **WP-34/35** (Ressourcenlimits,
> LRU-Modell-Cache), **WP-40** (TCK-Runner), **WP-42** (Performance-Budget-Gate),
> **WP-43** (API-Stabilisierung: `package dmn` als **v1**, SemVer + Deprecation-Policy,
> Golden-Surface-Test) und **WP-44** (Fuzzing über jede untrusted-Input-Schicht) fertig.
> Die öffentliche `dmn/`-API ist damit **als v1 zugesagt** (ADR-0019); `internal/` bleibt frei.
> Offen u. a.: **WP-33** (gRPC) und **WP-41** (offizielles TCK-Korpus). Voller Live-Status:
> `docs/20-roadmap.md`.

### Was heute funktioniert

- **DMN-XML laden:** namespace-tolerantes Decoding (1.3/1.4/1.5) in ein versionsneutrales
  Modell; das `DMNDI`-Diagramm-Layout übersteht einen Round-trip.
- **FEEL auswerten (intern):** Lexer → Parser → Compiler liefert eine `CompiledExpr`-Closure.
  Unterstützt u. a. Arithmetik (Decimal, `0.1 + 0.2 = 0.3`), Vergleiche, dreiwertige
  Boolesche Logik, `if`, `between`/`in`, Listen/Contexts/Ranges, Pfadzugriff, `@`-Temporal­literale
  und Funktionsaufrufe gegen die Built-in-Registry — alles mit FEEL-`null`-Propagation.
- **FEEL-Comprehensions & Filter:** `for … return` (mehrere Iteratoren, kartesisch, Range-Domains
  `1..3`), `some`/`every … satisfies`, Filter `list[prädikat]` (inkl. Kontext-Keys wie
  `people[age > 18]`), Index-Zugriff `list[n]` und Pfad-Projektion `list.feld`.
- **FEEL-Built-ins (nicht-temporal vollständig):** string (inkl. `matches`/`replace`/`split`,
  `string join`, `substring before/after`), numeric (`decimal`, `round …`, `modulo`, `sqrt`,
  `log`, `exp`, `even`/`odd`), list (`all`/`any`, `sublist`, `append`, `concatenate`,
  `insert before`, `remove`, `reverse`, `index of`, `union`, `distinct values`, `flatten`,
  `product`, `median`, `stddev`, `mode`), context (`get value`, `get entries`, `context put`,
  `context merge`, `context`), range-Relationen (`before`, `overlaps`, `includes`, `during`, …)
  und `sort`.
- **Date/Time/Duration:** Konstruktoren `date`/`time`/`date and time`/`duration`/
  `years and months duration`, `now`/`today` (injizierbare Uhr), Kalender-Funktionen
  (`day of week`, `month of year`, `day of year`, `week of year`) sowie Komponentenzugriff
  per Pfad (`date("2024-02-29").year`, `duration("P1Y6M").months`, `…​.time offset`). Zonen
  als Offset, `Z` oder `@Area/City`; `@`-Literale für alle vier Temporaltypen.
- **Decision Tables ausführen:** Unary Tests in den Eingabezellen, Hit Policies **U/A/F/R/C**
  (inkl. Collect-Aggregation SUM/MIN/MAX/COUNT), Einzel-/Mehrfach-Output.
- **Library-API (`dmn`):** `Engine.Compile(ctx, xml)` → `Definitions`, daraus `Decision(idOrName)`
  → `CompiledDecision.Evaluate(ctx, Input)` → `Result`. Go⇄FEEL-Typ-Mapping; FEEL-Numbers
  werden verlustfrei als exakter Dezimal-String zurückgegeben.

```go
eng := dmn.New()
defs, diags, _ := eng.Compile(ctx, xmlBytes)
dec, _ := defs.Decision("Dish")
res, _ := dec.Evaluate(ctx, dmn.Input{"Season": "Winter", "Guest Count": 8})
fmt.Println(res.Outputs["Dish"]) // → "Roastbeef"
```

### Als HTTP-Service (`temisd`)

```sh
go run ./cmd/temisd -addr :8080        # Server starten

# Modell hochladen (→ liefert eine content-addressed modelId)
curl --data-binary @dmn/testdata/models/dish_15.dmn \
     -H 'Content-Type: application/xml' localhost:8080/v1/models

# Stateless kompilieren + auswerten in einem Request
curl -X POST localhost:8080/v1/evaluate -H 'Content-Type: application/json' -d "{
  \"xml\": $(jq -Rs . < dmn/testdata/models/dish_15.dmn),
  \"decision\": \"Dish\",
  \"input\": {\"Season\": \"Winter\", \"Guest Count\": 8}
}"
# → {"outputs":{"Dish":"Roastbeef"}, ...}
```

Endpunkte: `POST /v1/models`, `GET /v1/models`, `GET /v1/models/{id}`,
`POST /v1/models/{id}/evaluate`, `POST /v1/evaluate`, `GET /healthz`/`/readyz`.
Vollständig in `service/openapi.yaml` und `docs/40-api-contract.md` §2. Fehler als
RFC-7807 `application/problem+json`.

**Gecachte Modelle auflisten:** `GET /v1/models` liefert alle aktuell im Cache
liegenden Modelle (id, Decisions, Inputs). Wer nicht möchte, dass jemand die
hinterlegten Decisions einsehen kann, schaltet den Endpunkt mit
`-list-models=false` ab — er antwortet dann mit `404`, als gäbe es ihn nicht.

**Web-UI (eigener DMN-Modeler):** Der Server liefert unter `GET /` einen
eigenständigen, abhängigkeitsfreien DMN-Modeler (ADR-0016, kein dmn-js, kein CDN,
offline): DRD-Canvas mit eigenen Renderern, Knoten verschieben/umbenennen/typisieren
(FEEL-validiert), **Decision-Tables ansehen & editieren** (Zellen, Regeln, FEEL-Validierung),
Datei öffnen, **Auswerten** sowie **Speichern** zurück ins DMN-XML — alles über die
`/v1`-Endpunkte. Ein optionaler Bearer-Token kann gesetzt werden. Die Alt-Pfade `/ui`
und `/app/` leiten dauerhaft auf `/` um.

```sh
go run ./cmd/temisd -addr :8080
# Browser: http://localhost:8080/
```

**Interaktive API-Doku (Swagger UI):** Der Server liefert zusätzlich eine dynamische
OpenAPI-Testseite unter `GET /docs` (lädt das eingebettete Spec von
`GET /openapi.yaml`) — Endpunkte direkt im Browser ausprobieren.

```sh
go run ./cmd/temisd -addr :8080
# Browser: http://localhost:8080/docs
```

**Optionaler Token-Schutz:** Mit `-token <token>` (oder `TEMIS_API_TOKEN`) verlangen
die `/v1`-Endpunkte `Authorization: Bearer <token>` (sonst `401`,
`code: UNAUTHORIZED`); `/docs`, `/openapi.yaml` und die Health-Probes bleiben offen.
In Swagger UI den Token über **Authorize** eintragen.

```sh
go run ./cmd/temisd -addr :8080 -token gehenix
curl -H 'Authorization: Bearer gehenix' \
     --data-binary @dmn/testdata/models/dish_15.dmn \
     -H 'Content-Type: application/xml' localhost:8080/v1/models
```

**gRPC (`dmn.v1.DmnEngine`):** Derselbe Server bietet die Engine zusätzlich als
**gRPC**-Dienst an — über **ConnectRPC** (ADR-0020), auf **demselben Port** wie REST,
mit geteilter Engine und geteiltem Modell-Cache. RPCs: `Compile`, `Evaluate` (per
`model_id` oder inline `xml`, mit `explain`/`strict`) und `EvaluateBatch` (bidirektionaler
Stream fürs Pipelining). Es spricht gRPC, gRPC-Web und das Connect-Protokoll; Klartext-
HTTP/2 (h2c) ist aktiv, sodass voller gRPC auch ohne TLS läuft. Der optionale Bearer-Token
gilt per Interceptor für jeden RPC. Contract: `proto/dmn/v1/engine.proto`, `docs/40-api-contract.md §3`.
Generierter Go-Code ist committet (`internal/gen/dmnv1/`); `make proto` regeneriert ihn.

### Für KI-Agenten (`temis-mcp`, MCP über stdio & HTTP)

temis ist bewusst als **Verifikationswerkzeug für KI-Agenten** ausgelegt (ADR-0013):
Statt eine regelbasierte Entscheidung selbst zu „raten", delegiert ein Agent sie an
temis und bekommt eine **deterministische, reproduzierbare** Antwort zurück. `temis-mcp`
bietet die Engine dafür als natives Werkzeug über das **Model Context Protocol**
(JSON-RPC 2.0 über stdio) an — abhängigkeitsfrei, reine Standardbibliothek.

```sh
go run ./cmd/temis-mcp        # spricht MCP über stdin/stdout (Logs auf stderr)
```

Vier Tools: **`list_models`** (Cache auflisten), **`load_model`** (DMN-XML kompilieren +
content-addressed cachen, idempotent), **`describe_decision`** (Decision + erwartete
Inputs beschreiben) und **`evaluate`** (auswerten per `modelId` oder stateless per `xml`).
Ein Agent-Runtime (z. B. Claude) startet das Binary als Subprozess; Beispiel-Eintrag:

```jsonc
// in der MCP-Client-Konfiguration
{ "command": "go", "args": ["run", "./cmd/temis-mcp"] }   // oder das gebaute Binary
```

**Remote/HTTP (hinter einem Reverse Proxy routebar).** Statt als lokaler Subprozess
kann `temis-mcp` MCP auch über **Streamable HTTP** anbieten (ADR-0015) — derselbe
Server, anderer Transport, weiterhin reine Standardbibliothek:

```sh
go run ./cmd/temis-mcp -http :8081               # POST /mcp, GET /healthz
go run ./cmd/temis-mcp -http :8081 -token geheim # optionaler Bearer-Token (nur HTTP)
```

`POST /mcp` nimmt je eine JSON-RPC-Nachricht und antwortet mit `application/json`
(Notifications → `202`); `GET /mcp` → `405` (kein SSE-Stream); `GET /healthz` für
Load-Balancer-Probes. Damit ist temis als geteilter MCP-Dienst hinter Traefik o. ä.
erreichbar.

**Ko-lokalisiert in `temisd` (ein Prozess, ein Cache).** Statt eines separaten
Prozesses bedient auch `temisd` denselben MCP-Endpoint — auf **demselben Modell-Cache**
wie Modeler und `/v1`-API (ADR-0021). Dann sieht ein Agent die vorgeladenen Beispiele
und die im Modeler bearbeiteten Modelle, und über MCP geladene Modelle erscheinen im
Modeler — eine `modelId` über alle Oberflächen.

```sh
go run ./cmd/temisd                 # /, /v1/... UND POST /mcp auf einem geteilten Cache
go run ./cmd/temisd -mcp=false      # MCP-Endpoint abschalten
```

`/mcp` wird vom selben optionalen `-token` bewacht wie die `/v1`-Endpunkte. Das
eigenständige `temis-mcp` bleibt für reines stdio/lokales Einbetten erhalten.

**Entscheidungsspur (warum?).** Auswerten lässt sich opt-in erklären: `evaluate` mit
`explain: true` (bzw. `dmn.WithTrace()` in der Library) liefert zusätzlich eine
`trace` — welche Regel(n) gefeuert haben, welche Bedingungen erfüllt/verfehlt waren und
welche Outputs beigetragen haben. So *begründet* ein Agent eine Entscheidung, statt sie
nur abzulesen. Die Spur stammt aus der echten Auswertung; der Default-Pfad ohne `explain`
bleibt unverändert schnell.

**Typisiertes Eingabe-Schema & strenge Validierung (kein stilles Falschergebnis).**
Jede Decision beschreibt ihre erwarteten Inputs samt FEEL-Typ selbst (`describe_decision`
über MCP, `schema` in der HTTP-Modell-Antwort, `CompiledDecision.InputSchema()` in der
Library). Mit `strict: true` (bzw. `dmn.WithStrictInput()`) prüft die Engine die Eingabe
vorab und liefert **präzise, maschinenlesbare** Fehler — „input \"Guest Count\" expects
number, got string" (`TYPE_MISMATCH`), unbekannte (`UNKNOWN_INPUT`) oder fehlende
(`MISSING_INPUT`) Felder — statt eine falsch getippte Eingabe still zu `null`/Nichttreffer
zu machen. So weiß ein Agent *vor* dem Vertrauen ins Ergebnis, dass seine Eingabe stimmt.

> Damit sind alle drei Agent-First-Säulen aus ADR-0013 umgesetzt (WP-50/51/52). Weiter
> geht die DMN-Abdeckung mit u. a. **WP-27** (restliche Hit Policies) und **WP-28**
> (DRG-Verkettung).

## Releases & Container

Releases werden über einen **SemVer-Tag** geschnitten; die Pipeline
(`.github/workflows/release.yml`) baucht daraus versionierte Binaries (`temisd` und
`temis-mcp` für linux/macOS/windows × amd64/arm64, Version per `-ldflags` eingebrannt),
einen **GitHub-Release** mit Notizen aus dem passenden `CHANGELOG.md`-Abschnitt und ein
**Container-Image für `temisd`** auf GHCR.

```sh
git tag v1.2.3 && git push origin v1.2.3        # löst die Release-Pipeline aus
```

Image direkt nutzen (sobald ein Release existiert):

```sh
docker run --rm -p 8080:8080 ghcr.io/pblumer/temis/temisd:latest
# Browser: http://localhost:8080/
```

Lokal bauen — der Build brennt die Version ein:

```sh
docker build --build-arg VERSION=v1.2.3 -t temisd:v1.2.3 .
temisd -version    # → temisd v1.2.3
```

Das Image basiert auf `distroless/static` (kein Shell, non-root); `temisd` bettet UI,
OpenAPI-Spec und Beispielmodelle per `go:embed` ein, läuft also ohne weitere Assets.
Änderungen sammeln sich unter `[Unreleased]` in [`CHANGELOG.md`](CHANGELOG.md).

## Entwicklung

Voraussetzung: **Go ≥ 1.23**.

```sh
go test ./...      # alle Tests
make verify        # fmt-check, vet, lint, test -race, bench-smoke, tck  (CI-Gate)
make help          # alle Make-Targets
```

### Projektstruktur (Auszug)

```
dmn/                 # öffentliche API (Engine, Compile/Evaluate — WP-10)
service/             # HTTP-Service-Adapter (temisd, WP-32)
mcp/                 # MCP-Server-Adapter für KI-Agenten (temis-mcp, WP-50)
vcs/                 # DMN-Modelle aus Git lesen (Provider-Interface, WP-70)
  github/            #   erster Provider: GitHub-REST über reine stdlib (ADR-0022)
internal/
  xml/               # DMN-XML ⇄ Modell (namespace-tolerant)
  model/             # versionsneutrales Domänenmodell
  value/             # FEEL-Wertemodell (Decimal-Number, Temporaltypen, …)
  feel/              # FEEL: Lexer, Parser/AST, Compiler, builtins/
  …                  # boxed/, drg/, tck/ folgen gemäß Roadmap
docs/                # Planung, Architektur, ADRs (Single Source of Truth)
```

## Dokumentation

| Datei | Inhalt |
|---|---|
| `docs/00-overview.md` | Projekt-Charter, harte Entscheidungen, Glossar |
| `docs/10-architecture.md` | Paketstruktur, Compile/Evaluate-Pipeline, interne Schnittstellen |
| `docs/20-roadmap.md` | MVP / Beta / 1.0 mit Arbeitspaketen & Akzeptanzkriterien **(Live-Status)** |
| `docs/30-feel-spec.md` | FEEL-Bauplan (Grammatik, Typen, Built-ins) |
| `docs/40-api-contract.md` | stabile Go- + HTTP/gRPC-API (SemVer-/Deprecation-Policy) |
| `docs/50-testing-strategy.md` | Test-Pyramide, Fuzzing, TCK, Benchmarks |
| `docs/60-ai-agent-guide.md` | Arbeitsregeln für KI-Coding-Agenten |
| `docs/70-integration-guide.md` | Quickstart (Library + Service) & DMN-Editor-Integration |
| `docs/80-clio-decision-log.md` | Revisionssicheres Entscheidungs-Logbuch via clio (ADR-0023) |
| `docs/adr/` | Architecture Decision Records |

## Mitwirken

Die Implementierung erfolgt durch einen KI-Coding-Agenten entlang der Arbeitspakete. Wer Code
beiträgt, liest zuerst `docs/00-overview.md`, `docs/10-architecture.md` und
`docs/60-ai-agent-guide.md`, wählt das nächste offene Arbeitspaket aus `docs/20-roadmap.md`,
schreibt Tests zuerst und hält `make verify` grün.

## Lizenz

Siehe [LICENSE](LICENSE).
