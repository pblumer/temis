# Roadmap & Arbeitspakete

> **Format für KI-Agenten:** Jedes Arbeitspaket (WP) hat: ID, Abhängigkeiten, klar
> testbares Akzeptanzkriterium (AK), Status. Bearbeite immer das oberste offene WP,
> dessen Abhängigkeiten `done` sind. Status-Werte: `todo | in-progress | done | blocked`.
> Schreibe Tests zuerst (siehe `60-ai-agent-guide.md`).

---

## Etappe MVP — „Lädt echte dmn-js-Dateien und entscheidet"

**Ziel:** Eine reale, in dmn-js erstellte Datei mit Decision Table + einfachen
FEEL-Ausdrücken laden, kompilieren und korrekt evaluieren — als Go-Library.

### MVP-Definition of Done (gesamt)
- `dmn.Engine.Compile(xml)` lädt eine dmn-js-Beispieldatei ohne Fehler.
- Single Decision Table mit Hit Policies **U, A, F, R, C** + numerische/String-Aggregation.
- FEEL-Kern: Literale, arithmetik, Vergleiche, `and/or/not`, Unary Tests
  (`<,>,<=,>=`, Intervalle `[..]`, Aufzählungen `a,b,c`, `-` als „egal").
- DRD-Verkettung **nicht** zwingend (eine Decision genügt), aber Multi-Input-Context vorhanden.
- ≥ 90 % Testabdeckung der beteiligten Pakete; `make verify` grün.

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-01 ✅ | Projektgerüst | – | **done** — `go build ./...`, `Makefile` mit `verify/test/bench/tck`, CI-Skript, lint-Konfig. `make verify` läuft (leer) grün. |
| WP-02 ✅ | DMN-XML-Decoding (1.5, tolerant 1.3/1.4) | WP-01 | **done** — Structs gemäß DMN-XSD; lädt DMN 1.3/1.4/1.5 namespace-tolerant verlustfrei in `internal/model`. `DMNDI`-Round-trip erhalten (ADR-0010). Golden-File-, Round-trip- & Fuzz-Tests. |
| WP-03 ✅ | FEEL Lexer | WP-01 | **done** — Tokenisiert alle FEEL-Lexeme (Zahlen inkl. Exponent, Strings mit Escapes/`\uXXXX`, Namensfragmente, `@`-Temporalliterale, Operatoren). Fehler als `Error`-Token statt Panic. Fuzz-Property-Test grün. |
| WP-04 ✅ | FEEL Parser → AST | WP-03 | **done** — Precedence-Climbing-Parser für die volle Ausdrucksgrammatik (Operatorpräzedenz inkl. rechtsassoz. `**`, `if/then/else`, `for/some/every`, Pfad/Filter, Funktionsaufruf positional+named, Listen/Contexts/Ranges, `between/in/instance of`, Funktionsdefinition). Mehrwort-Namen greedy + optionaler Namens-Oracle. Fehler mit Position; Fuzz-Test (kein Panic). |
| WP-05 ✅ | FEEL-Werte & Number-Decimal | WP-01 | **done** — `internal/value`: `Value`-Modell inkl. Decimal-Number (`apd/v3`, 34 Stellen half-even; `0.1+0.2=0.3`), date/time/date-time, zwei Dauer-Typen, List, geordneter Context, Range, Function. Gleichheit/Ordnung + Dauer-/Datum-Arithmetik mit `null`-Propagation. Fuzz auf Number-/Duration-Parsing. (Zeitzonen-Feinheiten/`@`-Vollgrammatik → WP-22.) |
| WP-06 ✅ | FEEL-Compiler (AST→Closure) Kern | WP-04, WP-05 | **done** — `Compile(AST,*Env)→CompiledExpr` (reine Go-Closure). Literale/Variablen(Slot-Index)/Pfad, Arithmetik, Vergleiche, `between`/`in`, dreiwertige Boolesche Logik, `if`. `null`-Propagation; Compile-Fehler mit Position. Benchmark: Eval ~270 ns/op, 3 allocs/op. (Aufrufe→WP-07; for/some/every/Filter/Funktionsdef→WP-20.) |
| WP-07 ✅ | Built-ins Kern | WP-06 | **done** — `internal/feel/builtins`: datengetriebene Registry, compile-zeit gebunden; aktiviert `CallExpr`. ~20 Built-ins (not; count/sum/min/max/mean/list contains; substring/string length/upper-lower case/contains/starts-ends with; number/string/date; floor/ceiling/abs). Positional + named Args, Aritätsprüfung. Je Built-in Normal- & `null`-Test. Voll → WP-21. |
| WP-08 ✅ | Unary Tests | WP-06 | **done** — `CompileUnaryTest` + `Matches`: kompiliert Eingabezellen zu booleschem `CompiledExpr` über die implizite Variable `?`. Werte (impl. Gleichheit), Intervalle (offen/geschlossen), Aufzählungen, `-`/leer (immer Match), `not(...)`, führende Operatoren `< <= > >=` (auch gegen andere Inputs), explizites `?`. Dünne Schicht über dem WP-04-Parser. Match-Funktion getestet. |
| WP-09 ✅ | Decision Table Compiler + Hit Policies U/A/F/R/C | WP-07, WP-08 | **done** — `internal/boxed.CompileTable` → `CompiledExpr`. U/A/F/R/C inkl. Collect-Aggregation SUM/MIN/MAX/COUNT; Einzel-/Mehrfach-Output (Context), sammelnde Policies → Liste. DMN-konform: no-match→null, U-Mehrfach/A-divergent→Fehler. Tabellengetriebene Tests + **End-to-End** mit `dish_15.dmn` (XML→Modell→Compile→Evaluate). (P/O/C+… → WP-27.) |
| WP-10 ✅ | Public Library-API (Compile/Evaluate) | WP-09 | **done** — `package dmn`: `Engine`/`New`, `Compile(ctx,xml)→Definitions,Diagnostics`, `Definitions.Decision(idOrName)`/`Index()`, `CompiledDecision.Evaluate(ctx,Input)→Result`. Go⇄FEEL-Mapping (FEEL-Number→exakter Dezimal-String, ADR-0007). Compile-Fehler je Decision als `Diagnostic` (Position), malformed XML als `error`. End-to-End: `dish_15.dmn` → Ergebnis über die öffentliche API. (DRG-Verkettung→WP-28, Decision Services→WP-29, Options/Limits→WP-22/34.) |
| WP-11 ✅ | MVP-Beispiele & Golden-Tests | WP-10 | **done** — 6 DMN-Modelle unter `dmn/testdata/models/` (dish, discount, routing, loan, risk, pricing) als E2E-Golden-Suite über die öffentliche API: deckt U/F/C-Hit-Policies, Einzel-/Mehrfach-Output (Context), Collect-SUM, Literal-Decimal-Arithmetik + String-Built-ins und decision-as-input ab. Number-Outputs als exakter Dezimal-String gepinnt. |

---

## Etappe Beta — „Vollständiges DMN 1.5"

**Ziel:** Voller FEEL, **alle** Boxed Expressions, vollständige DRD-Verkettung,
Decision Services, plus Service-Wrapper (HTTP + gRPC).

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-20 ✅ | FEEL vollständig | WP-06 | **done** — `for`-Comprehensions (multi-Iterator/kartesisch, geschachtelte Domains, Range-Domains `1..3` asc/desc), `some`/`every … satisfies` (dreiwertig), Filter `list[cond]`: Index `list[n]` (1-basiert, negativ vom Ende), Boolean-Prädikat über impliziter Variable `item` **und** direktem Kontext-Key-Zugriff (`people[age > 18]`, geschachtelt), Pfad-Projektion über Listen `list.feld`. Bare-Range im Iterationsdomain im Parser ergänzt; `function(…)`→WP-24, `instance of`→WP-30. (Range-**Built-ins** before/after/… → WP-21.) |
| WP-21 ✅ | FEEL Built-ins vollständig (nicht-temporal) | WP-07, WP-20 | **done** — alle **nicht-temporalen** Built-ins der DMN-1.5-Tabelle: **string** (matches/replace/split regex via RE2, string join, substring before/after), **numeric** (decimal, round up/down/half up/half down mit Scale, modulo, sqrt, log, exp, even, odd; floor/ceiling mit optionalem Scale), **list** (all/any, sublist, append, concatenate, insert before, remove, reverse, index of, union, distinct values, flatten, product, median, stddev, mode), **context** (get value inkl. Pfad-Liste, get entries, context put, context merge, context), **range** (before/after, meets/met by, overlaps/overlaps before/after, finishes/finished by, includes/during, starts/started by, coincides — Point- & Range-Overloads), **sort**(list, precedes?). Zahl-Helfer in `internal/value` (kein `apd`-Import in `builtins`). Je Built-in Normal- + `null`-Test; E2E durch Parser→Compiler (CompileString reicht jetzt die Built-in-Registry als Namens-Oracle durch, damit Keyword-Fragmente wie `index of` assemblieren). Regex-Flavor: Go RE2 statt XPath (dokumentierte Grenze). **Temporale Funktions-Built-ins (`now`/`today`/`date and time(…)`/`time(…)`/`duration(…)`/Komponenten-Extraktoren) → WP-22**, wo deren Wert-Semantik (Zeitzonen, `@`-Literale, Dauer-Arithmetik) liegt. |
| WP-22 ✅ | Date/Time/Duration vollständig **+ temporale Built-ins** | WP-05, WP-21 | **done** — temporale Built-ins der DMN-1.5-Tabelle: `date`(string/`y,m,d`/aus date-time), `time`(string/`h,m,s`/`+offset`/aus date-time), `date and time`(string/`date,time`), `duration`, `years and months duration(from,to)`, `now`/`today` (injizierbare Uhr, deterministisch testbar), `day of week`/`month of year` (Namen), `day of year`/`week of year` (ISO). **Komponentenzugriff** per Pfad: `date.year/month/day/weekday`, `time.hour/minute/second/time offset/timezone`, `duration.years/months/days/hours/minutes/seconds` (über `value.Member`). `@`-Literale (date/time/date-time/duration), Zonen (Offset `±HH:MM`, `Z`, `@Area/City`), Dauer-/Datums-Arithmetik & Vergleiche bestehen (WP-05). E2E durch Parser→Compiler inkl. Mehrwort-Namen mit `and`-Keyword. **Offene Feinheit:** Bruchteil-Komponenten in Dauer-Literalen (`PT1.5H`) noch nicht akzeptiert. |
| WP-23 ✅ | Boxed Context & Invocation | WP-20 | **done** — Unified-Expression-Decoding (`internal/xml`/`model`: `<context>`, `<invocation>`, `<functionDefinition>`, BKM-`<encapsulatedLogic>` über eingebettetes `Expression`-Element; `Decision.Logic()`). `boxed.Compile` dispatcht auf die Boxed-Form. **Boxed Context** (`internal/boxed/context.go`): Einträge sequenziell ausgewertet (spätere sehen frühere Variablen über `Env.Append`/`Scope.Extend`); Result-Cell (letzter namenloser Eintrag) → dessen Wert, sonst Context `{name: value}`. **Invocation** (`invocation.go`): Bindung an BKM-Formalparameter **per Name** (fehlende → null), Fallback positional bei Funktionswert-Callee. E2E `bkm_invocation_15.dmn`/`boxed_context_15.dmn` über die öffentliche API. |
| WP-24 ✅ | Boxed Function & BKM | WP-23 | **done** — First-class FEEL-Funktionswerte: `function(…)`-Literale (`feel.compileFunctionDef`/`FuncValue`) mit **Closure** über den umgebenden Scope; BKMs als globale, namensaufgelöste `feel.Func` (Registrierung vor Body-Compile → **Rekursion** & wechselseitige Rekursion via Pointer-Indirektion). CallExpr ruft Builtins, benannte Funktionen (positional/named Args) und Funktionswerte. **Rekursions-Tiefenlimit** (`DefaultMaxCallDepth`, ADR-0008-Anzahlung) über per-Evaluation `evalState` im Scope → Laufzeitfehler statt Stack-Overflow. E2E `recursion_15.dmn` (Fakultät). (Konfigurierbare Limits → WP-34; non-FEEL-`kind` nicht ausführbar.) |
| WP-25 ✅ | Boxed List & Relation | WP-20 | **done** — Geordnetes, heterogenes XML-Decoding (`internal/xml/boxed.go`: `UnmarshalXML` für `<list>`/`<row>`, da Pointer-Felder die Reihenfolge über Typen verlören). `boxed.compileList` → FEEL-Liste in Reihenfolge; `compileRelation` → Liste von Kontexten (je Zeile, nach Spaltennamen), mit Zeilen-Arity-Prüfung. E2E `boxed_collections_15.dmn` (Numbers, People). |
| WP-26 ✅ | Conditional / Iterator / Filter (1.4/1.5) | WP-20 | **done** — Boxed `<conditional>` (else optional → null), `<for>`/`<every>`/`<some>` (benannte `iteratorVariable`) und `<filter>` (`<in>`/`<match>`). Wiederverwendung der FEEL-Semantik über exportierte Helfer `IfThenElse`/`ForOne`/`QuantifyOne`/`BoxedFilter` (dreiwertige Quantoren, Domain = Liste/Range/Einzelwert, Filter-Index- vs. Prädikat-Modus, impliziter `item` + Kontext-Key-Zugriff). E2E `boxed_collections_15.dmn` (Grade, Doubled, AllPositive, AnyBig, BigNumbers, Adults). (Filter-`match` als Literal-Ausdruck; verschachtelte Boxed-Matches → später.) |
| WP-27 ✅ | Hit Policies vollständig | WP-09 | **done** — **P** (Priority) und **O** (Output Order) ergänzt (`internal/boxed/priority.go`): Priorität = Position des Output-Werts in der Output-Werteliste (`<outputValues>`), spaltenweise lexikografisch verglichen, Gleichstand → Tabellenreihenfolge. P → höchstrangiger Treffer, O → alle Treffer als Liste in Prioritätsreihenfolge; nicht-gelistete Werte rangieren zuletzt. Multi-Output unterstützt. **C+/C</C>/C#** liefen bereits über das `aggregation`-Attribut (SUM/MIN/MAX/COUNT, WP-09). E2E `hitpolicy_15.dmn`. |
| WP-28 ✅ | DRG-Verkettung & Eval-Plan | WP-10 | **done** — `dmn/graph.go`: Required-Decision-Referenzen → Kanten zwischen `CompiledDecision`s; **Zyklus-Erkennung** per DFS (3-Färbung) als `DECISION_CYCLE`-Diagnostic beim Compile. `Evaluate` wertet die benötigten Entscheidungen rekursiv & **memoisiert** aus (Diamond → einmal), speist Ergebnisse per Name als Zwischenvariablen ein; Laufzeit-Zyklus-Guard. Ein direkt im Input gelieferter Decision-Wert überschreibt (kein Recompute) → Rückwärtskompatibilität. `Result.Decisions` listet alle ausgewerteten Entscheidungen. E2E `routing_13.dmn` (Chaining) + Diamond/Cycle-Modelle. (Decision Services → WP-29.) |
| WP-29 ✅ | Decision Services | WP-28 | **done** — `<decisionService>` (XML/Modell: output/encapsulated/input-Decisions + inputData). Öffentliche API `Definitions.Service(idOrName)` → `CompiledService.Evaluate`. Gemeinsamer, memoisierter `evaluator` (`dmn/eval.go`, aus WP-28 extrahiert): wertet Output- (+ benötigte encapsulated) Decisions aus; **Input-Decisions sind Grenzen** — vom Aufrufer geliefert, nie berechnet (unbeliefert → null, kein Chaining dahinter). `Result.Outputs` je Output-Decision, `Result.Decisions` = tatsächlich ausgewertete. E2E `decisionservice_15.dmn` (encapsulated vs. input-decision). |
| WP-30 | Typecheck-Phase | WP-20, WP-28 | Statische Typprüfung wo möglich (Item Definitions), Diagnostics mit Position. |
| WP-31 | Item Definitions / Typsystem-Bindung | WP-30 | Benutzerdefinierte Typen, Listen-/Struct-Typen, Constraints (allowed values). |
| WP-32 ✅ | HTTP-Service-Wrapper | WP-10 | **done** — `service.Server` über `*dmn.Engine` (nur öffentliche `dmn/`-API, kein `internal/`). Endpunkte: `POST /v1/models` (XML → kompilieren+cachen, content-addressed `sha256:`-ID, idempotent), `GET /v1/models/{id}` (Index), `POST /v1/models/{id}/evaluate` (`{decision,input}` → Outputs/Decisions/Diagnostics), `POST /v1/evaluate` (stateless: XML+Input in einem Request), `GET /healthz`/`/readyz`. Go-1.22-Mux (kein externer Router), Request-Body-Limit, RFC-7807 `application/problem+json` mit stabilen Codes (`MALFORMED_XML`/`MODEL_NOT_FOUND`/`DECISION_NOT_FOUND`/`INVALID_REQUEST`/`EVALUATION_FAILED`). `cmd/temisd` startet den Server (`-addr`). `service/openapi.yaml`. httptest-Suite über alle Endpunkte; manuell mit `dish_15.dmn` per `curl` verifiziert. (In-Memory-Cache = WP-35-Vorstufe; Hot-Reload/Eviction → WP-35. Limits-Konfiguration → WP-34.) |
| WP-33 | gRPC-Service-Wrapper | WP-10 | `dmn.proto`, Evaluate/Compile-RPCs, Streaming für Batch. |
| WP-34 | Sicherheits-/Ressourcenlimits | WP-32 | Limits (Rekursion, Iteration, Listengröße, Compile-Timeout) konfigurierbar & erzwungen. Fuzz-getestet. |
| WP-35 | Model-Cache im Service | WP-32 | Kompilierte Modelle werden gecacht (ID/Hash); Hot-Reload bei Änderung. |

---

## Etappe Agent-First — „Verlässliches Verifikationswerkzeug für KI-Agenten" (ADR-0013)

**Ziel:** temis so zugänglich machen, dass ein KI-Agent regelbasierte Entscheidungen
**delegieren**, das Ergebnis **begründen** und seine Eingaben **absichern** kann —
deterministisch, nachvollziehbar, agenten-nativ. Alle drei WPs sind dünne Adapter bzw.
Erweiterungen über `package dmn` (ADR-0011); kein `internal/`-Zugriff von außen.

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-50 ✅ | MCP-Server (`temis-mcp`) | WP-32 | **done** — `package mcp` + `cmd/temis-mcp`: dünner Adapter über `package dmn` (kein `internal/`-Import), JSON-RPC 2.0 über **stdio** mit **reiner Standardbibliothek** (kein MCP-SDK, Entscheidung dokumentiert in **ADR-0014** — null neue Deps, kein Go-Bump). Vier Tools: `list_models`, `load_model` (content-addressed, idempotent), `describe_decision`, `evaluate` (per `modelId` oder stateless per `xml`). `initialize`-Handshake (echot Protokollversion), `tools/list`, `tools/call`, Notifications ohne Antwort. Tabellengetriebene Tests über die Tool-Oberfläche (`dish_15.dmn` → `Roastbeef`), `make verify` grün, end-to-end per stdio verifiziert. HTTP/SSE-Transport und ein optionales SDK bleiben nachrüstbar (dann eigenes ADR). |
| WP-51 ✅ | Entscheidungsspur in `Result` | WP-09, WP-10 | **done** — **opt-in** `Evaluate(…, dmn.WithTrace())` füllt `Result.Trace`: je Decision Table Hit Policy + Aggregation, Eingabespalten mit ausgewerteten Werten, jede Regel mit Bedingungsergebnissen (bis zur ersten verfehlten, Short-Circuit) und Outputs der beitragenden Regeln, plus `Matched`-Indizes. Aus der **echten** Auswertung abgeleitet (Trace-Senke über `feel.Scope`, Aufzeichnung in `internal/boxed`), kein Rationalisat. Default-Pfad unverändert allokationsarm (nur nil-Typassertion). `Trace`-Typen mit JSON-Tags; HTTP (`"explain": true` → `"trace"`) und MCP (`evaluate`-Arg `explain`) reichen sie durch. In `docs/40-api-contract.md` §1.3 + `openapi.yaml` spezifiziert. Tabellengetriebene Tests (U via dish, Collect-SUM via risk, Literal=keine Tables, Default=kein Trace) über Library, Service und MCP; `make verify` grün. |
| WP-52 ✅ | Agent-Schema & strenge Eingabevalidierung | WP-10, WP-51 | **done** — beim Kompilieren je Decision ein typisiertes `[]InputField` (Name, FEEL-Typ, Required), Typ aus InputData-`typeRef` bzw. Decision-Table-Input-Clause abgeleitet. `CompiledDecision.InputSchema()`/`Definitions.InputSchema(idOrName)` zur Selbstbeschreibung; `ValidateInput(in)` liefert `[]InputProblem` mit Codes `TYPE_MISMATCH`/`UNKNOWN_INPUT`/`MISSING_INPUT` und präziser Message („input \"Guest Count\" expects number, got string"). `WithStrictInput()` lässt `Evaluate` vorab validieren und mit `*InputError{Problems}` scheitern statt still zu null/Nichttreffer zu coercen. Adapter: MCP `describe_decision` → typisiertes Schema, `evaluate`-Arg `strict`; HTTP Modell-Antwort trägt `schema`, `evaluate` akzeptiert `strict` → `422 INVALID_INPUT` mit `problems`. Tabellengetriebene Tests je Fehlerklasse über Library, Service, MCP; `make verify` grün, end-to-end per stdio verifiziert. (Custom Item Definitions → WP-31.) |
| WP-53 ✅ | Remote-MCP über HTTP (`temis-mcp -http`) | WP-50 | **done** — `temis-mcp -http host:port` bietet MCP über **Streamable HTTP** (reine stdlib, ADR-0015), damit der Server netzwerk-/Reverse-Proxy-routebar ist statt nur lokal per stdio. `POST /mcp` (eine JSON-RPC-Nachricht → `application/json`-Antwort; Notification → `202`), `GET /mcp` → `405` (kein SSE-Stream nötig), `GET /healthz`. Wiederverwendung derselben `handleMessage`-Dispatch wie stdio; optionaler Bearer-Token (`-token`/`TEMIS_API_TOKEN`, konstantzeit), nur für HTTP. httptest-Suite (initialize/evaluate/notification/405/healthz/token) + live `curl`-E2E; `make verify` grün. Kein neuer Dependency, kein Go-Bump. (SSE/Sampling/Resources → erneuter ADR-0014-Revisit bei Bedarf.) |

---

## Etappe 1.0 — „TCK-konform, schnell, stabil"

**Ziel:** Nachgewiesene Konformität, erfülltes Performance-Budget, eingefrorene API, Doku.

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-40 | TCK-Runner | WP-21, WP-28 | Liest offizielle TCK-`.dmn` + Testdefinitionen, führt Cases aus, Report grün/rot. |
| WP-41 | TCK-Konformität | WP-40, alle Beta-WP | **Zielquote:** ≥ 95 % der anwendbaren TCK-Cases grün; jede Ausnahme dokumentiert mit Begründung. |
| WP-42 | Performance-Budget | WP-10 | Benchmark-Suite erfüllt Budgets aus `50-testing-strategy.md`; Regressionen brechen CI. |
| WP-43 | API-Stabilisierung | WP-10, WP-32 | Public API als `v1` markiert, `// Deprecated`-Policy, semver. Keine breaking changes danach ohne Major. |
| WP-44 | Fuzzing & Robustheit | WP-34 | `go test -fuzz` für Lexer/Parser/XML ohne Crash über definierte Laufzeit. |
| WP-45 | Doku & Beispiele | WP-43 | GoDoc vollständig, README, Quickstart (Lib + Service), dmn-js-Integrationsanleitung. |
| WP-46 | Release-Pipeline | WP-43 | Versionierte Releases, Container-Image für `temisd`, Changelog. |

---

## dmn-js-Integration (querschnittlich, ab MVP relevant)

> **Überholt durch ADR-0016 (2026-06-29).** Die hier beschriebene Linie „dmn-js unverändert
> einbetten" ist abgelöst: temis baut einen **eigenen Modeler** auf einem Fork des MIT-Kerns
> (diagram-js/table-js/dmn-moddle) — für 1.5-Authoring, ohne bpmn.io-Logo-Pflicht, mit
> FEEL-Validierung gegen die echte Engine. Siehe **Etappe Eigener Modeler (WP-60–67)** unten.
> Die **Round-trip-Pflicht** (Standard-DMN-XML als Schnittstelle, WP-02) bleibt unverändert
> gültig; F-01 ist als historischer Stand `done` dokumentiert.

dmn-js erzeugt und liest **Standard-DMN-XML**. Es gibt nichts „proprietär" zu adaptieren —
die Engine muss exakt dieses XML lesen/schreiben können (das ist WP-02). dmn-js wird
**unverändert** eingebettet und angepasst — **nie geforkt** (ADR-0012; bpmn.io-Logo bleibt
sichtbar). Der Editor ist in die bestehende `/ui`-Seite (`service/ui.go`) integriert und lädt
dmn-js per CDN — wie die Swagger-UI unter `/docs`; **keine zweite Toolchain**.

- **F-01** (Beta, optional) ✅: **Editor in `/ui`** (`service/ui.go`). Bettet dmn-js per CDN ein
  (read-only `dmn-navigated-viewer`, bearbeitbar `dmn-modeler`). Fluss: Datei hochladen/XML
  einfügen → read-only Ansicht → „Bearbeiten" → editierbar → „Auf Server deployen"
  (`POST /v1/models`) → Decision wählen → `POST /v1/models/{id}/evaluate` (`explain:true`).
  **Ergebnis als Graph:** die durchlaufenen Decisions werden im DRD markiert und mit ihrem
  Wert beschriftet (angefragte Decision hervorgehoben, Zwischenergebnisse separat) — nutzt die
  DRG-Verkettung (WP-28) und die `decisions`-Map. „Neu" erstellt ein leeres Modell.
  **Kein** Produktziel der Engine. End-to-End per Headless-Browser verifiziert. Grenze: dmn-js
  rendert DMN 1.3 (1.4/1.5 wertet die Engine aus, zeichnet der Editor ggf. nicht).
- **F-02** (optional, nach F-01): Einsteiger-UX-Module — reduzierte Palette,
  Decision-Table-Vorlagen, Inline-FEEL-Hilfe und ein **Diagnostics-Overlay**, das temis-
  Diagnostics (`line/col`) auf die betroffenen Tabellenzellen mappt. Optional: dmn-js-Bundles
  per `go:embed` für ein offline lauffähiges `/ui`.
- Round-trip-Pflicht: Eine in dmn-js gespeicherte Datei muss von der Engine ladbar sein
  **und** eine von der Engine (un)veränderte Datei muss in dmn-js wieder öffnen.

---

## Etappe Eigener Modeler — „eigener Stack, DMN 1.5, kein bpmn-io" (ADR-0016)

**Ziel:** Ein eigener, eingebetteter DMN-Modeler (DRD-Canvas + Decision-Table- +
Boxed-Expression-Editor) auf einem **Fork des MIT-Kerns** (diagram-js/table-js/dmn-moddle),
vollständig gelöst von dmn-js. Treiber: **1.5-Authoring** inkl. Boxed Expressions, verlustfreier
Round-Trip, kein bpmn.io-Logo, und **FEEL-Validierung gegen die echte temis-Engine**. temis
bleibt XML-/FEEL-/1.5-Autorität.

**Vorarbeit ✅ (ADR-0016-Gates):** Lizenz-Audit (Kern = MIT) bestätigt; **FEEL-WASM-Spike**
(`cmd/feel-wasm`, `web/feel-spike`) gebaut & headless verifiziert — beweist Live-Validierung
im Browser, offline.

**Etappen-Definition of Done:** `/ui` nutzt ausschließlich den eigenen, per `go:embed`
ausgelieferten Modeler (kein CDN, offline); ein DMN-**1.5**-Modell (inkl. eines Boxed-1.5-
Konstrukts) lässt sich visuell anlegen/bearbeiten und durchläuft Laden→Speichern **verlustfrei**;
FEEL-Zellen werden live gegen temis validiert; alter Fluss (Upload→bearbeiten→deployen→auswerten)
E2E-headless grün.

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-60 ✅ | Frontend-Toolchain & Repo-Struktur | – | **done (Skelett)** — `web/` (Vite + TypeScript, framework-frei vorerst); Build → `web/dist/` (committet, damit `go build` ohne Node läuft); `go:embed` via `package web`; `temisd` serviert das Frontend **offline** unter **`GET /app/`** (kein CDN), `/ui` bleibt bis WP-67 unberührt. `make web`/`web-check`; CI-`web`-Lane (typecheck + build + dist-Drift-Check). End-to-End headless verifiziert (TS→Bundle→embed→Browser-Render, keine Konsolenfehler). Framework-Wahl bewusst aufgeschoben. |
| WP-61 ✅ | MIT-Kern vendoren, dmn-js-Wrapper weglassen | WP-60 | **done (Kern-Canvas)** — `diagram-js@15.18.0` (MIT) + seine MIT/ISC/Apache-Runtime-Deps (min-dom/min-dash/tiny-svg/didi/object-refs/path-intersection/preact/…) als npm-Dependency gebündelt & via `go:embed` ausgeliefert (offline, kein CDN); **dmn-js entfällt komplett**. `/app/` rendert einen diagram-js-Canvas (SVG, Default-Renderer) — headless verifiziert: 2 Shapes + 1 Connection, **kein bpmn.io-Logo** (`a[href*=bpmn.io]`=0, `.bjs-powered-by`=0), keine Konsolenfehler. Lizenz-Attribution in `web/THIRD_PARTY_NOTICES.md`. (Source-Level-Fork von diagram-js erst, wenn Interna für DMN/1.5 zu patchen sind; table-js/dmn-moddle kommen mit WP-62-JS/WP-64.) |
| WP-62 | Client-Modell + DMN-1.5-XML-Round-Trip | WP-61 | dmn-moddle-Fork um **1.5-Namespace** + neue Deskriptoren (conditional/filter/iterator/…) erweitert; Client liest/schreibt DMN 1.5; temis validiert/normalisiert. AK: ein 1.5-Modell inkl. Boxed-1.5-Konstrukt überlebt Laden→Speichern **verlustfrei** (Go-Round-Trip-Test wie WP-02 **+** Frontend-Test). **Risiko-WP — zuerst absichern.** **Teil-Fortschritt (Spike):** temis-Seite verifiziert & gefixt — boxed `<list>`/`<relation>` und Decision-Table-`<rule>` round-trippten XML-strukturell **nicht** treu (Go-Feldnamen `<ID>`/`<Items>`/`<Cells>`; kollabierte `<inputEntry>`; leere `<annotationEntry>`); behoben via gezielter `MarshalXML`, abgesichert durch `TestRoundTripXMLFidelity` + Boxed-1.5-Fixtures. **Offen:** JS-Seite (dmn-moddle-Fork auf 1.5). |
| WP-63 | Command-Stack / Undo-Redo (Fundament) | WP-61 | Jede Modell-Mutation als **reversibles Command**; Undo/Redo. AK: erzeugen/verschieben/löschen rückgängig **und** wiederholbar. Fundament, **nicht** nachgerüstet. |
| WP-64 | Decision-Table-Editor | WP-62, WP-63 | Editierbares Grid (Spalten/Regeln add/reorder, Hit Policy, Ein-/Mehrfach-Output); FEEL-Zellen **live gegen temis-FEEL** validiert (baut auf dem WASM-Spike auf). AK: Tabelle bearbeiten → korrektes 1.5-XML; ungültige Zelle mit `line:col` markiert; E2E headless. |
| WP-65 ◑ | DRD-Canvas (Modeler) | WP-62, WP-63 | Renderer für Decision/InputData/BKM/KnowledgeSource + die 4 Requirement-Typen, DMN-Modellierungsregeln, Palette/Context-Pad, Connection-Routing/Docking. AK: DRD zeichnen/bearbeiten → gültiges Modell **+ DMNDI**. Iterativ: Render/Selektion/Move → Connect/Rules → Palette/Context-Pad → Routing/Snapping. **Teil-Fortschritt (Render):** eigene diagram-js-Renderer (`web/src/dmn-renderer.ts`) zeichnen Decision (Rechteck), InputData (Oval), BusinessKnowledgeModel (geknickte Ecken) + Requirement-Kanten (Information solid / Knowledge dashed, mit Pfeilspitze) — headless verifiziert (5 Shapes, 4 Kanten, kein dmn-js/Logo). **Offen:** echtes Modell-Laden + DMNDI-Layout, Selektion/Move/Connect, Rules, Palette/Context-Pad. |
| WP-66 | Boxed-Expression-Editor (1.5) | WP-64 | context/list/invocation/function/conditional/filter/iterator als rekursive Forms. AK: ein Boxed-1.5-Konstrukt **visuell anlegbar** und round-trip-fest. |
| WP-67 | `/ui`-Migration & F-01/F-02-Ablösung | WP-64, WP-65 | CDN-dmn-js in `service/ui.go` → **eigener eingebetteter Modeler**; Deploy/Evaluate + Diagnostics-/Trace-Overlay (`line:col`) angebunden. AK: `/ui` nutzt nur noch den eigenen Modeler, **offline, kein CDN**; Fluss Upload→bearbeiten→deployen→auswerten E2E-headless grün. |

> **BPMN-Synergie (später, eigenes ADR):** `bpmn-js` sitzt auf demselben MIT-Kern. Der hier
> entstehende Fork (Canvas/Command-Stack/Palette aus WP-61/63/65) ist das Fundament für einen
> **eigenen BPMN-Editor** in der künftigen BPMN-Workflow-Engine — gleiche Toolchain, gemeinsame
> FEEL-Integration; DMN ist die erste fachliche Schicht, BPMN die zweite.
