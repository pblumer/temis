<p align="center">
  <img src="docs/readme-hero.svg" alt="Temis — deterministische DMN-1.5/FEEL-Engine, erreichbar als eingebettete Go-Library, über REST, gRPC und MCP für AI-Agenten; im Kern wertet eine Decision-Table mit Hit Policy U aus und liefert typisierte, nachvollziehbare Entscheidungen" width="900">
</p>

<h1 align="center">Temis — Deterministische DMN-Engine: Go-Library, REST, gRPC & MCP für AI-Agenten</h1>

<p align="center">
  <strong>Schnelle DMN-1.5-Engine in Go</strong> · <strong>volles FEEL</strong> · <strong>Library</strong> · <strong>HTTP/gRPC</strong> · <strong>Modeler</strong> · <strong>MCP für Agenten</strong>
</p>

<p align="center">
  <a href="https://github.com/pblumer/temis"><img alt="Repository" src="https://img.shields.io/badge/GitHub-pblumer%2Ftemis-24292f?logo=github"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.24+-00ADD8?logo=go&logoColor=white">
  <img alt="DMN" src="https://img.shields.io/badge/DMN-1.5-7C3AED">
  <img alt="FEEL" src="https://img.shields.io/badge/FEEL-full-EC4899">
  <a href="#dmn-tck-konformität"><img alt="DMN-TCK" src="https://img.shields.io/badge/DMN--TCK-98.1%25-brightgreen"></a>
  <img alt="API" src="https://img.shields.io/badge/API-HTTP%20%2B%20gRPC-10B981">
  <img alt="Agents" src="https://img.shields.io/badge/Agent--First-MCP-F59E0B">
</p>

---

> **Kurz gesagt:** Temis nimmt standardisierte DMN-Modelle aus dem Modeler, kompiliert sie deterministisch
> und wertet sie als embeddable Go-Library oder als sofort startbaren Service aus — inklusive Trace,
> Audit, Git-Workflows und Agenten-Schnittstelle.

<div align="center">

| 🧠 Entscheidungen | ⚡ Engine | 🎛️ Betrieb | 🤖 Agenten |
|---|---|---|---|
| DMN 1.5, Decision Tables, DRG, BKM, Decision Services | Decimal-genaues FEEL, Compiler, Limits, Cache | `temisd` mit Web-UI, OpenAPI, Auth, clio-Audit | MCP stdio/HTTP, Explain-Traces, typisierte Schemas |

</div>

## Warum Temis?

- **Produktiv einbettbar:** `package dmn` ist als v1 zugesagt; `internal/` bleibt frei beweglich.
- **Zero-config Demo, harte Produktion:** `temisd` startet ohne Pflichtparameter mit Modeler, Swagger UI,
  Beispielen und MCP — lässt sich aber per `TEMIS_*`-Env vollständig härten.
- **FEEL ohne Float-Überraschungen:** Zahlen laufen decimal-genau; Temporaltypen, Listen, Contexts,
  Ranges, Filter, Projektionen und Built-ins sind breit abgedeckt.
- **Review-fähige Modelle:** DMN-XML bleibt Standardformat; Git-backed Lesen, Schreiben, Branches und PRs
  machen Regeländerungen nachvollziehbar.
- **Auditierbare Entscheidungen:** optionale clio-Anbindung, Replay/Re-Audit und Explain-Traces zeigen,
  warum ein Ergebnis entstanden ist.

## 30-Sekunden-Eindruck

```sh
temisd
# Browser: http://localhost:8080/      # Modeler
# Browser: http://localhost:8080/docs  # OpenAPI-Testkonsole
```

```go
eng := dmn.New()
defs, diags, _ := eng.Compile(ctx, xmlBytes)
dec, _ := defs.Decision("Dish")
res, _ := dec.Evaluate(ctx, dmn.Input{"Season": "Winter", "Guest Count": 8})
fmt.Println(res.Outputs["Dish"]) // → "Roastbeef"
```

## DMN-TCK-Konformität

Temis wird gegen das **offizielle DMN Technology Compatibility Kit**
([github.com/dmn-tck/tck](https://github.com/dmn-tck/tck)) geprüft — die vom DMN-Standard
getragene Referenz-Testsuite. Über **Compliance Level 2 + 3** bestehen aktuell:

<div align="center">

### **3430 / 3495 Cases — 98,1 %** ✅

</div>

Das ist kein Selbstläufer-Wert, sondern **nachprüfbar und regressionsgeschützt**:

- **Gepinnter Korpus:** Der TCK wird an Commit `0dbcaf9` bezogen (nicht vendored), damit
  die Zahl reproduzierbar bleibt.
- **Im CI verankert:** Die Lane `tck` (`.github/workflows/ci.yml`) klont den gepinnten
  Korpus und lässt `internal/tck.TestOfficialTCKConformance` laufen.
- **Ratchet-Floor:** Der Gate erzwingt einen Mindestwert (aktuell **98,1 %**), der nur nach
  oben wandert — eine Regression bricht den Build.
- **Selbst nachvollziehen:** `make tck-conformance` holt den Korpus und misst lokal.

Die verbleibenden ~2 % sind kategorisiert und dokumentiert (u. a. externe Java-Funktionen
ohne JVM, Decision-Service-Randfälle) — siehe **[`docs/tck-exceptions.md`](docs/tck-exceptions.md)**.

## Performance

Eine kompilierte Entscheidung ist unveränderlich und nebenläufigkeitssicher:
einmal kompilieren, millionenfach auswerten.

<div align="center">

### **> 1,6 Mio Entscheidungen/s** auf einer 4-Kern-VM ⚡

</div>

| Szenario (4 vCPU, `GOGC=400`) | Durchsatz | Latenz (1 Kern, warm) |
|---|---:|---:|
| Decision-Table (String/Enum) | ≈ 1,62 Mio/s | ≈ 1,9 µs |
| Decision-Table (numerisch, Intervalle) | ≈ 1,22 Mio/s | ≈ 2,9 µs |

Gemessen auf einer bescheidenen, geteilten Cloud-VM (Intel® Xeon® @ 2,8 GHz) —
also eine Untergrenze; bessere Hardware skaliert linear. Jede Zahl ist mit einem
eingecheckten Benchmark reproduzierbar (`go test -bench=BenchmarkThroughput ./dmn/`),
Methodik und ehrliche Einordnung in **[`docs/55-benchmarks.md`](docs/55-benchmarks.md)**.

**1:1 gegen Drools** (`kie-dmn-core`, identische DMN-Dateien, gleiche VM, fünf
Feature-Typen): pro Auswertung ist Temis in jedem Szenario schneller — **1,2×–3,0×**
(ein Kern), am stärksten bei Decision-Tables. Reproduzierbares Harness mit
vollen Zahlen und ehrlicher Einordnung in
**[`benchmarks/comparison/`](benchmarks/comparison/README.md)**.

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
| WP-54 | Entscheidungs-Logbuch: opt-in clio-Audit-Sink in `temisd` (ADR-0023) | ✅ |
| WP-55 | Entscheidungs-Logbuch: Re-Audit-/Replay-Tool `temis-reaudit` (ADR-0023) | ✅ |
| WP-121 | Command-Consumer: Entscheidungen per clio-Event auslösen (`temis-clio-worker`, ADR-0033) | ✅ |
| WP-70 | Git-gestützte Modelle: Lesen/Browsen (`vcs` + GitHub-Provider) | ✅ |
| WP-71 | Git-gestützte Modelle: Schreiben (`vcs.Writer`, Commit/Branch/PR) | ✅ |
| WP-72 | Git-Modelle über HTTP (`/v1/git/*`, Token pro Request) | ✅ |
| WP-73 | Git-Modelle über MCP (`git_list_models`/`git_load_model`/`git_propose`) | ✅ |
| WP-80 | Modellierungs-Assistent: LLM-Chat im Modeler (`assist`, `POST /v1/chat`) | ✅ |

> **MVP erreicht (WP-01–11); Beta abgeschlossen.** Über die oben gelisteten Pakete hinaus
> sind inzwischen u. a. **WP-23–26** (Boxed Context/Invocation/Function, BKM, DRG-Verkettung,
> Decision Services), **WP-27** (alle Hit Policies inkl. PRIORITY/OUTPUT ORDER), **WP-30/31**
> (Typsystem, `instance of`, Item-Definition-Constraints), **WP-34/35** (Ressourcenlimits,
> LRU-Modell-Cache), **WP-40** (TCK-Runner), **WP-42** (Performance-Budget-Gate),
> **WP-43** (API-Stabilisierung: `package dmn` als **v1**, SemVer + Deprecation-Policy,
> Golden-Surface-Test) und **WP-44** (Fuzzing über jede untrusted-Input-Schicht) fertig.
> Die öffentliche `dmn/`-API ist damit **als v1 zugesagt** (ADR-0019); `internal/` bleibt frei.
> **WP-41** (offizielle TCK-Konformität) hat sein Ziel erreicht: **98,1 %** der Level-2/3-Cases,
> CI-verankert mit Ratchet-Floor (siehe [Abschnitt oben](#dmn-tck-konformität) und
> `docs/tck-exceptions.md`). Offen u. a. das erste getaggte Release. Voller Live-Status:
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

**Nullkonfiguration — einfach starten:** `temisd` ist bewusst „batteries-included". Ein
nackter Start ohne Flags oder Env-Variablen bringt sofort einen **voll ausgestatteten**
Server: DMN-Modeler unter `/`, Swagger-UI unter `/docs`, mitgelieferte Beispielmodelle,
Modell-Listing (`GET /v1/models`), den **MCP-Endpunkt** (`POST /mcp`) und den
**Modellier-Assistenten** (`POST /v1/chat`). Keine Rückfragen, keine Pflicht-Parameter.

```sh
temisd                                 # alles an, http://localhost:8080/
```

Der Assistent läuft ohne serverseitigen Schlüssel im **BYOK-Modus** (der Endpunkt ist live
und antwortet, sobald ein Aufrufer seinen eigenen Provider-Key per `X-LLM-Token`-Header
mitschickt); setzt man `TEMIS_LLM_TOKEN`, nutzt der Server diesen Schlüssel.

**Opt-out für Profis — nur über Umgebungsvariablen.** Jeder Default entstammt einer
`TEMIS_*`-Variable, sodass sich jedes Feature ohne ein einziges Flag abschalten lässt
(ideal für Container). Ein explizit gesetztes Flag hat immer Vorrang vor der Env-Variable.

| Env-Variable | Default | Wirkung |
|---|---|---|
| `TEMIS_ADDR` | `:8080` | Listen-Adresse (`host:port`) |
| `TEMIS_KEYS_FILE` | *(leer)* | JSON-Datei mit scoped `kid.secret`-API-Keys für `/v1`, `/mcp`, gRPC (leer = keine; ADR-0028) |
| `TEMIS_KEYS_DIR` | *(leer)* | Verzeichnis für den persistenten Keystore + Lifecycle-API (`POST /v1/keys …`); Keys überleben Neustart (leer = Key-Verwaltung aus; WP-103) |
| `TEMIS_BOOTSTRAP_ADMIN_KEY` | *(leer)* | Bootstrap-Admin-Secret; erzeugt einen `admin`-Key, dessen `kid` beim Start geloggt wird (Secret nie) |
| `TEMIS_API_TOKEN` | *(leer)* | **DEPRECATED** Legacy-Admin-Token für `/v1` (leer = keiner); ersetzt durch `TEMIS_KEYS_FILE` |
| `TEMIS_PUBLIC_EVALUATE` | `false` | Öffnet den `evaluate`-Scope für anonyme Aufrufer trotz konfigurierter Keys — jede Auswertung (HTTP/gRPC/MCP) braucht keinen Token, `write`/`admin`/`assist`/`git`/`flow` weiterhin schon (ADR-0035) |
| `TEMIS_PUBLIC_MODELS` | *(leer)* | Komma-Liste von `modelId`s **oder** Modellnamen, deren Auswertung anonym offen ist (public decisions) — alles andere bleibt hinter Key (leer = keine; ADR-0035) |
| `TEMIS_EXAMPLES` | `true` | Beispielmodelle vorladen |
| `TEMIS_MODELS_DIR` | *(leer)* | Modelle in dieses Verzeichnis persistieren + beim Start laden (leer = nur In-Memory) |
| `TEMIS_MCP` | `true` | MCP-Endpunkt `POST /mcp` |
| `TEMIS_LIST_MODELS` | `true` | `GET /v1/models` (false → `404`, Decisions privat) |
| `TEMIS_ASSIST` | `true` | Modellier-Assistent `POST /v1/chat` |
| `TEMIS_LLM_TOKEN` | *(leer)* | Serverseitiger LLM-Key (leer = BYOK-only) |
| `TEMIS_LLM_PROVIDER` | `anthropic` | LLM-Provider (`anthropic`/`openai`) |
| `TEMIS_LLM_ALLOW_BYOK` | `true` | Aufrufer-Key per `X-LLM-Token` zulassen |
| `TEMIS_CLIO_TOKEN` | *(leer)* | clio-Audit-Sink **anschalten** (`kid.secret`; leer = aus, kein Datenabfluss) |
| `TEMIS_CLIO_URL` | `https://clio.blumer.cloud` | Ziel-clio (nur aktiv, wenn ein Token gesetzt ist) |
| `TEMIS_CLIO_ACTIVE_PROBE` | `false` | `GET /v1/status` pingt clios Health-Endpunkt aktiv (statt passiver Last-Write-Health) |
| `TEMIS_CACHE_SIZE` | `0` | LRU-Cache-Größe (0 = Default, negativ = unbegrenzt) |

```sh
# Beispiel: gehärteter Betrieb ganz ohne Flags
TEMIS_API_TOKEN=geheim TEMIS_MCP=false TEMIS_ASSIST=false TEMIS_LIST_MODELS=false temisd
```

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

Kern-Endpunkte: `POST /v1/models`, `GET /v1/models`, `GET /v1/models/{id}`,
`GET /v1/models/{id}/xml`, `POST /v1/models/{id}/evaluate`,
`POST /v1/models/{id}/evaluate-graph`, `POST /v1/evaluate`, `GET /v1/status`,
`GET /healthz`/`/readyz`.
Dazu die Modeler-Endpunkte (ADR-0016), die den eingebauten DMN-Modeler bedienen
(Graph, Typen, Decision-Tables, Literal-Expressions, BKM, Save). Vollständig — Pfade
und Schemas — in `service/openapi.yaml` und `docs/40-api-contract.md` §2; ein Test
hält Routen und OpenAPI in synch. Fehler als RFC-7807 `application/problem+json`.

**Gecachte Modelle auflisten:** `GET /v1/models` liefert alle aktuell im Cache
liegenden Modelle (id, Decisions, Inputs). Wer nicht möchte, dass jemand die
hinterlegten Decisions einsehen kann, schaltet den Endpunkt mit
`-list-models=false` ab — er antwortet dann mit `404`, als gäbe es ihn nicht.

**Modelle über Neustarts hinweg behalten (`-models-dir`, ADR-0027):** Der Modell-Cache
lebt normalerweise nur im RAM — nach einem Neustart sind selbst hochgeladene und im
Modeler gebaute Modelle weg (die gebündelten Beispiele kommen per `go:embed` zurück).
Setzt man `-models-dir` (oder `TEMIS_MODELS_DIR`) auf ein Verzeichnis, persistiert `temisd`
jedes hochgeladene/editierte Modell **content-adressiert als rohes DMN-XML** (`<sha256>.dmn`)
und lädt es beim Start wieder in den Cache. Reine Standardbibliothek, kein neuer Dependency;
per Default aus (dann byte-identisch rein in-memory). Ideal im Container mit einem
gemounteten Volume:

```sh
temisd -models-dir /data/models
# oder: docker run -v temis-models:/data/models -e TEMIS_MODELS_DIR=/data/models …
```

Nur das rohe XML liegt auf der Platte — Kompilat, Index und Diagnostik werden beim Laden
deterministisch neu erzeugt, können also nie vom Engine-Verhalten abdriften. Ein aus dem
beschränkten LRU-Cache verdrängtes, aber persistiertes Modell wird bei Bedarf on-demand
von der Platte rekompiliert. Für **versionierte** Modelle mit Review/PR bleibt die
Git-Anbindung (`/v1/git/*`, ADR-0022) die richtige Wahl.

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

**Import-Cockpit (Testfälle stapelweise durchlaufen lassen):** Neben **Design** und **Operate**
bietet der Modeler einen dritten Modus **Import** — einen Testfall-Stapellauf als *Fließband*. Man
lädt eine **Vorlage** (**CSV** oder **JSON**) herunter, die exakt zu den Eingaben des Modells passt,
füllt sie mit Testdaten (von Hand, in der Tabellenkalkulation oder von einem **KI-Agenten**) und
importiert sie per Datei-Auswahl oder Drag & Drop. Optionale `→Decision`-Spalten machen aus einer
Zeile eine **Pass/Fail-Erwartung**. „Durchlaufen lassen" schickt jeden Datensatz **animiert** von
links (*Eingang*) durch die *Evaluation* (dieselbe Engine wie Operate) nach rechts in den *clio
Store* — mit berechneten Ergebnissen und Pass/Fail-Badges. Reines Frontend, kein neuer Endpunkt.

**Regelset über einen Datensatz + Auswertung „welcher Datensatz welche Regel verletzt" (ADR-0034):**
Der typische Fall: ein **ganzes Regelset** über einen grossen Datensatz laufen lassen — etwa 70 000
Server — und am Schluss wissen, **welcher Server welche Regel nicht bestanden hat**. Das Regelset ist
ein DMN-Modell; das gebündelte Beispiel **`server_compliance`** nutzt eine **`COLLECT`**-Tabelle,
deren Regeln je einen Server-Check prüfen (Patch-Alter, TLS-Version, freier Speicher, Firewall,
Root-SSH) und die **verletzten Regel-IDs als Liste** ausgeben. Man streamt den Datensatz als
**Produktivlauf** (`record: true`) über `POST /v1/models/{id}/evaluate-graph-batch` (ein einzelner
Request bleibt an das 8-MiB-Body-Limit gebunden, ~50 000 reiche Zeilen — grössere Flotten in Blöcken).
Pro Fall entsteht ein revisionssicheres **Quality-Event** auf der Entität. Die **Auswertung** liest
diese Events über **einen** geteilten Kern (`package quality`) auf **drei Kanälen**:

```sh
# CLI: Report aus clio, Text oder JSON; -fail-on-violation macht es CI-gattbar
go run ./cmd/temis-quality-report -clio-url http://127.0.0.1:3000 -clio-token kid_ro.secret
# → Rangliste je Regel + jede verletzende Entität mit ihren Regeln; „55 000 failed …"

# HTTP: der Server fragt clio selbst ab (Token bleibt serverseitig), Scope `audit`
curl -H 'Authorization: Bearer kid.audit' localhost:8080/v1/quality/report | jq
```

Im **Import-Cockpit** öffnet der Button **„Bericht ▾"** dasselbe als Panel (Tabelle „Entität ×
verletzte Regeln" plus Regel-Rangliste) — der Browser sieht nie einen clio-Token. Ohne konfigurierte
clio antwortet der Endpunkt klar mit `409 CLIO_NOT_CONFIGURED`.

**Flow Studio & Designer (Decision-Flows via UI, ADR-0026/0032):** Über den Modellen (L1) liegt
eine eigene **FLOWS**-Sektion (L2a) in der Sidebar. Ein registrierter Flow wird per Klick im
**Flow Studio** geöffnet: seine Steps als auto-gelayouteter **Cross-Model-Graph**, ein Run-Panel
aus den deklarierten Inputs, und nach dem Auswerten *illuminiert* der Canvas — jede Leitung zeigt
den Wert, der über sie floss, und ein **Entscheidungspfad** listet die gefeuerten Regeln. Mit dem
**Flow-Designer** (das **+** in der FLOWS-Sektion, oder **„✎ Bearbeiten"** an einem offenen Flow)
lassen sich Flows auch **erstellen und designen**: ein strukturierter Inspector editiert Name,
deklarierte Inputs, **Steps** (Modell- + Decision-Picker, FEEL-Input-Verdrahtung mit
**Auto-Wiring** aus dem Decision-Schema) und das Output-Mapping, während eine **Live-Graph-Preview**
den DRG beim Tippen neu zeichnet. **„Testen"** wertet den Entwurf inline aus (ohne Registrierung),
**„Prüfen"** validiert gegen die geladenen Modelle, **„Registrieren & Öffnen"** legt ihn im Katalog
ab, **„Export"** lädt den `*.flow.json`-Deskriptor herunter. Der Server-Flow-Store bleibt der
flüchtige Dev-Pfad; **Git bleibt die dauerhafte Quelle** (`flows/`, ADR-0032) — der Export ist der
Weg dorthin. Reines Frontend über die bestehenden `/v1/flows`-Endpunkte, kein neuer Endpunkt.

**Interaktive API-Doku (Swagger UI):** Der Server liefert zusätzlich eine dynamische
OpenAPI-Testseite unter `GET /docs` (lädt das eingebettete Spec von
`GET /openapi.yaml`) — Endpunkte direkt im Browser ausprobieren.

```sh
go run ./cmd/temisd -addr :8080
# Browser: http://localhost:8080/docs
```

**Scoped API-Keys (ADR-0028):** Mit `-keys-file <datei>` (oder `TEMIS_KEYS_FILE`)
schützt `temisd` `/v1`, `/mcp` und gRPC über `kid.secret`-Keys im Modell von
[clio](https://github.com/pblumer/clio). Der Bearer ist `Authorization: Bearer <kid>.<secret>`;
die Keystore hält **nur** `sha256(secret)` (Klartext nie), verglichen in Konstantzeit.
Jede Route braucht einen **Scope** — `evaluate`, `models:read`, `models:write`, `git`,
`assist`, `flow`, `admin` (Super-Scope). Fehlender/ungültiger/abgelaufener/widerrufener
Key → `401` (`code: UNAUTHORIZED`, `WWW-Authenticate: Bearer`); gültiger Key ohne den
Scope → `403` (`code: FORBIDDEN`). `/docs`, `/openapi.yaml` und die Health-Probes bleiben
offen. Ohne Keys **und** ohne Legacy-Token bleibt die API offen (heutiger Default).

Der `keys-file` ist JSON; je Key wird bevorzugt der Hex-`secretHash` hinterlegt
(so berührt kein Klartext die Platte); alternativ `secret` (wird beim Laden gehasht):

```json
{ "keys": [
  { "kid": "ci01",  "secretHash": "<hex sha256(secret)>", "scopes": ["models:write"], "owner": "CI" },
  { "kid": "agent", "secret": "s3cret",                    "scopes": ["evaluate"] }
] }
```

Ein **Bootstrap-Admin-Key** entsteht aus `TEMIS_BOOTSTRAP_ADMIN_KEY` (das Secret); der
daraus abgeleitete `kid` wird beim Start geloggt, das Secret nie. Der Bearer ist dann
`<geloggter-kid>.<secret>`.

```sh
printf '%s' "s3cret" | sha256sum   # Hash für secretHash erzeugen
go run ./cmd/temisd -addr :8080 -keys-file keys.json
curl -H 'Authorization: Bearer agent.s3cret' \
     -d '{"decision":"Dish","input":{"Season":"Winter","Guest Count":8}}' \
     -H 'Content-Type: application/json' localhost:8080/v1/models/<id>/evaluate
```

**Keys zur Laufzeit verwalten (WP-103, Scope `admin`):** Mit `-keys-dir <dir>`
(`$TEMIS_KEYS_DIR`) hängt der Keystore am Dateisystem-Store (ADR-0027, atomarer
JSON-Write, reine stdlib — kein bbolt) und eine Admin-API legt Keys an, listet,
rotiert und widerruft sie. Das Secret erscheint **einmalig** beim Anlegen/Rotieren
(als `secret`/`bearer`), danach wird nur der `sha256` gehalten. Nur so erzeugte
(*managed*) Keys sind rotier-/widerrufbar; statische Keys → `409`. Die Keys
überleben einen Neustart. Praktisch mit einem Bootstrap-Admin kombinieren:

```sh
TEMIS_BOOTSTRAP_ADMIN_KEY=adminsecret go run ./cmd/temisd -keys-dir ./keystore
# → Log: bootstrap admin key registered: kid=boot-xxxxxxxxxxxx
ADMIN='boot-xxxxxxxxxxxx.adminsecret'

# Key anlegen (Secret nur hier sichtbar)
curl -H "Authorization: Bearer $ADMIN" -H 'Content-Type: application/json' \
     -d '{"scopes":["evaluate"],"owner":"agent-1"}' localhost:8080/v1/keys
# → {"kid":"k_…","secret":"…","bearer":"k_….…","scopes":["evaluate"]}

curl -H "Authorization: Bearer $ADMIN" localhost:8080/v1/keys                 # listen (ohne Secrets)
curl -H "Authorization: Bearer $ADMIN" -X POST localhost:8080/v1/keys/k_…/rotate  # rotieren
curl -H "Authorization: Bearer $ADMIN" -X POST localhost:8080/v1/keys/k_…/revoke  # widerrufen
```

**Absichern beim ersten Start (Trust-on-first-use, WP-107):** Ohne Bootstrap-Secret
starten — nur mit `-keys-dir` — und den ersten Admin-Key **über die Oberfläche**
anlegen. Solange **kein** Key existiert, ist die API offen (die Lifecycle-API ist
erreichbar, beim Start laut geloggt); der **erste angelegte Key kippt den Server zur
Laufzeit auf abgesichert** (`enabled()` hängt an „mindestens ein Key") und wird
persistiert, übersteht also den Neustart.

```sh
go run ./cmd/temisd -keys-dir ./keystore           # offen; /v1/keys ist erreichbar
# Browser → Modeler → Sidebar „Zugriff" → „🔒 Admin-Key anlegen & absichern"
# Der Modeler übernimmt den neuen Bearer sofort als Session, zeigt das Secret
# einmalig und lädt neu — ab jetzt ist der Server abgesichert.
```

Der Bootstrap-Button erzwingt bewusst `admin`-Scope (sonst würde man den Server ohne
Admin-Key aussperren). Achtung: bis der erste Key existiert, ist die API offen —
diesen Schritt im vertrauenswürdigen Netz bzw. direkt nach dem Deploy machen, und
für Klartext-Transport TLS davorschalten (`-tls-cert`/`-tls-key` oder Reverse-Proxy).

**Lockout-Recovery — Offline-CLI (WP-104):** Ist kein nutzbarer Admin-Key mehr da,
verwaltet `temisd keys …` denselben Keystore **bei gestopptem Server** direkt am
Verzeichnis. Ein so erzeugter Key wird beim nächsten Start akzeptiert:

```sh
temisd keys create -keys-dir ./keystore -scopes admin -owner recovery
temisd keys list   -keys-dir ./keystore          # ohne Secrets
temisd keys rotate -keys-dir ./keystore k_abc123 # entwertet das alte Secret
temisd keys revoke -keys-dir ./keystore k_abc123
```

**Prefix-Scopes (WP-105):** Ein Scope lässt sich auf einen Ressourcen-Prefix
einschränken — `evaluate:/orders/*` oder eine auf eine `modelId` gepinnte
`models:read:sha256:…`. Der Grant greift nur, wenn die Request-Ressource (`{id}` =
modelId/flowId) mit dem Prefix beginnt; ressourcenlose Routen (Listing, stateless
`/v1/evaluate`, gRPC, MCP) erfüllt nur ein **unbeschränkter** Grant. **Authorship:**
bei aktiver Auth stempelt der clio-Audit-Sink die `kid` als CloudEvents-Extension
`clioauthkid` auf jedes Decision-/Flow-Event (`docs/80`). Abgelaufene Keys
(`expiresAt`) werden abgewiesen (`401`).

**DEPRECATED Legacy-Token:** `-token <token>` (oder `TEMIS_API_TOKEN`) läuft weiter als
**Legacy-Admin-Key** — der ganze Token als `Authorization: Bearer <token>` deckt alle
Routen (Admin), byte-identisch zum bisherigen Verhalten. Für neue Deployments `-keys-file`
verwenden.

```sh
go run ./cmd/temisd -addr :8080 -token gehenix   # DEPRECATED, deckt alles als admin
curl -H 'Authorization: Bearer gehenix' \
     --data-binary @dmn/testdata/models/dish_15.dmn \
     -H 'Content-Type: application/xml' localhost:8080/v1/models
```

**Public decisions (ADR-0035):** Sonst ist Auth binär — sobald ein Key existiert, verlangt
*jede* Route einen Token. Für „diese Entscheidung darf jeder auswerten, alles andere bleibt
zu" öffnet man gezielt nur den `evaluate`-Scope, ohne die schreibenden/kostenverursachenden
Routen (`models:write`/`admin`/`assist`) freizugeben:

- **Pro Modell** — `-public-models "<modelId|Name>,…"` (oder `TEMIS_PUBLIC_MODELS`): nur die
  gelisteten Modelle sind anonym auswertbar. Ein Eintrag matcht per content-adressierter
  `modelId` **oder** per Anzeigename (so bleibt ein neu gespeichertes Modell per Name public).
  Gilt für die id-adressierten Routen (`/v1/models/{id}/evaluate`, `…/evaluate-graph`).
- **Global** — `-public-evaluate` (oder `TEMIS_PUBLIC_EVALUATE=true`): der ganze `evaluate`-Scope
  ist anonym offen, inkl. dem stateless `POST /v1/evaluate`, über HTTP, gRPC und MCP.

Beides ist opt-in und wird beim Start laut geloggt. Rate-Limiting (`-rate-limit`) greift auch
für anonyme Aufrufer; ein trotzdem mitgeschickter gültiger Key stempelt weiterhin seine
Authorship (`clioauthkid`) ins Audit-Log.

```sh
# Nur das Modell "Dish" ist öffentlich auswertbar, alles andere braucht einen Key:
go run ./cmd/temisd -keys-file keys.json -public-models Dish
curl --data '{"decision":"Dish","input":{"Season":"Winter","Guest Count":4}}' \
     -H 'Content-Type: application/json' \
     localhost:8080/v1/models/<modelId>/evaluate      # ohne Authorization-Header → 200
```

**Zur Laufzeit umschalten (WP-107):** Neben der Startup-Config lässt sich der Pro-Modell-Schalter
**ohne Redeploy** umlegen — im Modeler über den **„🔒 Privat / 🌐 Öffentlich"-Toggle** in der
Toolbar (pro geöffnetem Modell, nur für `admin`) oder das Zugriff-Panel, bzw. per API
`POST /v1/access/public/models` `{"model":"<id|Name>","public":true|false}` (Scope `admin`). Mit
`-keys-dir` werden Laufzeit-Umschaltungen in `public.json` **persistiert** (überstehen Neustart);
ohne bleiben sie im Speicher. `-public-models`-Einträge sind fix (nur per Neustart änderbar).

**Betriebs-Observability (`GET /v1/status`, ehrliches `/readyz`, ADR-0030):** temis
ist *observierbar*, überwacht sich aber nicht selbst. `GET /v1/status` zeigt den Zustand
der **Umsysteme** (clio/LLM/Git) und die Last der Engine — für clio `writesOk`/
`writesFailed`/`idempotentSkips`, `lastOk`/`lastError` und `reachable`, dazu Version,
Uptime und Cache-Zähler. Der Output ist **secret-frei** (kein Token/Key erscheint je) und
liegt hinter dem `audit`-Scope (ADR-0028; `admin`-Keys lesen ebenfalls, offen ohne Auth-Konfig). clio-Erreichbarkeit kommt per Default
**passiv** aus echten Writes; `-clio-active-probe` (oder `TEMIS_CLIO_ACTIVE_PROBE`) schaltet
einen aktiven Health-Ping zu. `/healthz` ist reine Liveness (Prozess lebt); `/readyz` meldet
jetzt **echte** Readiness — `503`, wenn eine harte Startbedingung fehlt (z. B. ein
fail-closed `-clio-strict` clio unerreichbar ist); ein **best-effort**-clio-Ausfall lässt
`/readyz` bewusst bei `200`. Dashboards/Alerting bleiben die externe Ops-Schicht.

```sh
curl localhost:8080/v1/status | jq .clio
# → {"enabled":true,"mode":"best-effort","writesOk":128,"writesFailed":0,"reachable":true,…}
```

**Revisionssicheres Entscheidungs-Logbuch (clio):** `temisd` protokolliert jede
Einzel-Decision-Auswertung als manipulationssicheres CloudEvent im Schwesterprojekt
**[clio](https://github.com/pblumer/clio)** (append-only, hash-verkettet) — Eingabe,
Ausgabe, optionale Spur und content-addressed `modelId`. Der Sink zeigt standardmäßig
auf die gehostete clio (`https://clio.blumer.cloud`), bleibt aber **aus, bis ein
`TEMIS_CLIO_TOKEN` (`kid.secret`) gesetzt ist** — ohne Token verlässt keine Decision-Daten
den Prozess (byte-identischer Default). Anschalten ist damit ein einziger Schritt: Token
setzen (oder `-clio-url` auf die eigene clio zeigen). Die Kopplung läuft nur über clios
HTTP-API, ohne Go-Import (ADR-0023, ADR-0011). Idempotent per clio-Precondition;
`-clio-strict` macht den Sink fail-closed (`502`), sonst best-effort. Voller Vertrag &
Betrieb: `docs/80-clio-decision-log.md`.

```sh
# Gehostete clio (Default-URL) — nur der Token schaltet an:
TEMIS_CLIO_TOKEN=kid_ci01.geheim temisd

# Oder die eigene clio:
temisd -clio-url http://127.0.0.1:3000 -clio-token kid_ci01.geheim -clio-subject-key "Order ID"
# entsprechend per Env: TEMIS_CLIO_URL / TEMIS_CLIO_TOKEN / TEMIS_CLIO_SOURCE
```

**Nachrechnen (`temis-reaudit`):** Weil temis deterministisch ist, lässt sich das Logbuch
**verifizieren** — `temis-reaudit` liest die Events aus clio, rechnet jede Entscheidung
`input`@`modelId` erneut nach und vergleicht mit der protokollierten Ausgabe. Das ergänzt
clios `verify` (Hash-Kette/Signatur = *unverändert*) um den *Regelkonformitäts*-Beweis;
Exit-Code 0/1 macht es skriptbar.

```sh
go run ./cmd/temis-reaudit \
  -clio-url http://127.0.0.1:3000 -clio-token kid_ro.secret -models ./models
# → re-audited 127 decision event(s) against 9 model(s): 127 reproduced — OK ✓
```

**Auslösen per Event (`temis-clio-worker`, ADR-0033):** Die **Gegenrichtung** — ein in clio
geschriebenes **Command-Event** `com.temis.decision.requested.v1` löst eine Auswertung aus
(Einzel-Decision, ganzer Graph oder Decision-Flow/DRG), und das Ergebnis fliesst korreliert
(`requestId`, gleicher Subject) als bestehendes `evaluated.v1` zurück ins Logbuch. So wird clio
zur **entkoppelnden Naht**: ein Umsystem schreibt nur ein Event und muss temis nicht kennen. Der
Consumer ist **zustandslos** (clio hält den Zustand) und bleibt damit Decisioning, nicht Prozess
(Grenze aus ADR-0025). Vertrag & Betrieb: `docs/80-clio-decision-log.md` §6.

```sh
go run ./cmd/temis-clio-worker \
  -clio-url http://127.0.0.1:3000 -clio-token kid_worker.secret -models ./models
# beobachtet Command-Events (observe), wertet aus, schreibt evaluated.v1 idempotent zurück
```

**gRPC (`dmn.v1.DmnEngine`):** Derselbe Server bietet die Engine zusätzlich als
**gRPC**-Dienst an — über **ConnectRPC** (ADR-0020), auf **demselben Port** wie REST,
mit geteilter Engine und geteiltem Modell-Cache. RPCs: `Compile`, `Evaluate` (per
`model_id` oder inline `xml`, mit `explain`/`strict`) und `EvaluateBatch` (bidirektionaler
Stream fürs Pipelining). Es spricht gRPC, gRPC-Web und das Connect-Protokoll; Klartext-
HTTP/2 (h2c) ist aktiv, sodass voller gRPC auch ohne TLS läuft. Der optionale Bearer-Token
gilt per Interceptor für jeden RPC. Contract: `proto/dmn/v1/engine.proto`, `docs/40-api-contract.md §3`.
Generierter Go-Code ist committet (`internal/gen/dmnv1/`); `make proto` regeneriert ihn.

### Git-gestützte Modelle (`/v1/git/*`, ADR-0022)

DMN-Modelle können **versioniert aus einem Git-Repository** gelesen, ausgewertet und
bearbeitet werden — Branch/Commit/PR inklusive. Als SaaS zuerst über **GitHub**,
grundsätzlich über jeden Remote (Provider-Interface, `package vcs`). Der GitHub-Token wird
**pro Request** im Header `X-Git-Token` mitgegeben und nie serverseitig gespeichert (getrennt
vom optionalen temisd-Bearer-Token).

```sh
# Modelle eines Repos auf einem Branch auflisten (nur *.dmn)
curl -H 'X-Git-Token: ghp_…' \
  'localhost:8080/v1/git/models?owner=pblumer&repo=temis&ref=main&dir=models'

# Ein Modell aus dem Repo laden → liefert eine modelId (danach wie jedes Cache-Modell nutzbar)
curl -X POST localhost:8080/v1/git/load -H 'Content-Type: application/json' -H 'X-Git-Token: ghp_…' \
  -d '{"owner":"pblumer","repo":"temis","ref":"main","path":"models/dish.dmn"}'

# Editiertes Modell als Pull Request vorschlagen (Branch → Commit → PR; kompiliert vorab)
curl -X POST localhost:8080/v1/git/propose -H 'Content-Type: application/json' -H 'X-Git-Token: ghp_…' \
  -d '{"owner":"pblumer","repo":"temis","base":"main","branch":"edit-dish",
       "path":"models/dish.dmn","title":"Update dish","xml":"<definitions …/>"}'
```

Endpunkte: `GET /v1/git/branches|commits|models`, `POST /v1/git/load|save|propose`. Fehler als
RFC-7807 (`GIT_NOT_FOUND`/`GIT_UNAUTHORIZED`/`GIT_CONFLICT`/`GIT_UPSTREAM_ERROR`). `save`/`propose`
kompilieren das Modell **vor** dem Schreiben — ein kaputtes DMN landet nie im Repo. GitHub
Enterprise via `service.WithGitHubBaseURL`. Dieselben Operationen stehen KI-Agenten über die
MCP-Tools **`git_list_models`**, **`git_load_model`** und **`git_propose`** zur Verfügung
(Token pro Call als `gitToken`-Argument).

### Für KI-Agenten (`temis-mcp`, MCP über stdio & HTTP)

temis ist bewusst als **Verifikationswerkzeug für KI-Agenten** ausgelegt (ADR-0013):
Statt eine regelbasierte Entscheidung selbst zu „raten", delegiert ein Agent sie an
temis und bekommt eine **deterministische, reproduzierbare** Antwort zurück. `temis-mcp`
bietet die Engine dafür als natives Werkzeug über das **Model Context Protocol**
(JSON-RPC 2.0 über stdio) an — abhängigkeitsfrei, reine Standardbibliothek.

```sh
go run ./cmd/temis-mcp        # spricht MCP über stdin/stdout (Logs auf stderr)
```

Kern-Tools: **`list_models`** (Cache auflisten — je Modell mit Name, Decisions und Inputs),
**`get_model_xml`** (das rohe DMN/FEEL eines gecachten Modells zurücklesen, nicht nur
auswerten), **`load_model`** (DMN-XML kompilieren + content-addressed cachen, idempotent),
**`describe_decision`** (Decision + erwartete Inputs beschreiben), **`evaluate`**
(auswerten per `modelId` oder stateless per `xml`) sowie die Typ-Werkzeuge
**`list_types`**/**`save_type`**/**`delete_type`** (eigene Item-Definitionen lesen,
einfache Typen anlegen/ändern und entfernen — jede Änderung liefert eine neue modelId).
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

In `temisd` schützt `/mcp` derselbe scoped Keystore wie die `/v1`-Endpunkte
(ADR-0028): jedes Tool verlangt seinen Scope (`evaluate`→`evaluate`,
`list_models`/`get_model_xml`/`load_model`/`describe_decision`/`list_types`→`models:read`,
`save_type`/`delete_type`→`models:write`, `git_*`→`git`,
`*_flow`→`flow`), gültiger Key ohne Scope → `403`. Das eigenständige `temis-mcp`
bleibt für reines stdio/lokales Einbetten erhalten (dort weiterhin optionaler
`-token` nur über HTTP).

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

### Modellierungs-Assistent im Modeler (`POST /v1/chat`, opt-in)

Während die MCP-/Agent-Schnittstelle temis von einem **externen** Agenten aufrufen lässt,
dreht der eingebaute **Modellierungs-Assistent** die Richtung um (ADR-0024): temis ruft
selbst einen LLM und lässt ihn beim **Bauen** von Decisions helfen — FEEL erklären,
Decision-Tables vorschlagen und auf Wunsch direkt anlegen/ändern. Der Assistent ist ein
Agent-First-Bürger: er **prüft** seine eigenen Vorschläge mit `evaluate` gegen die echte
Engine, statt zu raten.

Anbieter-agnostisch über ein schmales Provider-Interface (`package assist`, reine
Standardbibliothek, kein SDK — konsistent mit ADR-0014): **Anthropic** (Messages API) oder
**OpenAI** (Chat Completions). Der server-seitige Agent-Loop teilt den **Modell-Cache** mit
Modeler, `/v1` und MCP (ein Adressraum), und über `load_model`/`save_decision_table`
Erstelltes erscheint sofort im Modeler.

Der Endpunkt ist **per Default aus** und wird über `temisd`-Flags aktiviert:

```sh
# Anthropic, Token server-seitig (Browser sieht ihn nie):
go run ./cmd/temisd -llm-provider anthropic -llm-token "$ANTHROPIC_API_KEY"

# OpenAI, mit Modell-Override:
go run ./cmd/temisd -llm-provider openai -llm-token "$OPENAI_API_KEY" -llm-model gpt-4o
# Browser: http://localhost:8080/ → Toolbar „✦ Assistent"
```

Flags (Env-Defaults in Klammern): `-llm-provider` (`$TEMIS_LLM_PROVIDER`), `-llm-token`
(`$TEMIS_LLM_TOKEN`), `-llm-model` (`$TEMIS_LLM_MODEL`), `-llm-base-url`
(`$TEMIS_LLM_BASE_URL`, z. B. ein Proxy oder OpenAI-kompatibler Endpunkt) und
`-llm-allow-byok` (Default an). Mit **Bring-your-own-key** trägt ein Nutzer im Modeler einen
eigenen Schlüssel ein, der pro Anfrage als `X-LLM-Token`-Header vorrangig genutzt und **nie**
serverseitig gespeichert wird. `/v1/chat` wird vom selben optionalen `-token` bewacht wie die
übrigen `/v1`-Endpunkte.

> **Datenschutz:** Anders als die rein lokale Engine **sendet** der aktive Assistent
> Modellkontext (Decisions, FEEL, Beispiel-Eingaben) an den gewählten Anbieter. Deshalb
> opt-in und per Default aus (ADR-0024).

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

Voraussetzung: **Go ≥ 1.24**.

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
| `docs/90-decision-organization.md` | Decision-Organisation im Großen (Schichten, Ownership, Repo-Layout; ADR-0027) |
| `docs/adr/` | Architecture Decision Records |

## Mitwirken

Die Implementierung erfolgt durch einen KI-Coding-Agenten entlang der Arbeitspakete. Wer Code
beiträgt, liest zuerst `docs/00-overview.md`, `docs/10-architecture.md` und
`docs/60-ai-agent-guide.md`, wählt das nächste offene Arbeitspaket aus `docs/20-roadmap.md`,
schreibt Tests zuerst und hält `make verify` grün. Der Einstieg steht in
[CONTRIBUTING.md](CONTRIBUTING.md).

## Sicherheit

Schwachstellen bitte vertraulich melden — siehe [SECURITY.md](SECURITY.md). Für den
produktiven Betrieb ist die Grundhaltung dort dokumentiert (Auth/TLS sind opt-in).

## Lizenz

Siehe [LICENSE](LICENSE).
