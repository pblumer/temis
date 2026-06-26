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
| WP-23 | Boxed Context & Invocation | WP-20 | Context mit Result-Cell; Invocation ruft BKM mit benannten Parametern. |
| WP-24 | Boxed Function & BKM | WP-23 | Definition + Aufruf, Closures über Kontext, Rekursion mit Tiefenlimit. |
| WP-25 | Boxed List & Relation | WP-20 | List-/Relation-Auswertung zu FEEL-Listen/Kontextlisten. |
| WP-26 | Conditional / Iterator / Filter (1.4/1.5) | WP-20 | Boxed `if`, `for`/`every`/`some` als Boxed, `filter`. Spec-Beispiele grün. |
| WP-27 | Hit Policies vollständig | WP-09 | Ergänzt **P, O, C+, C<, C>, C#** inkl. Output Order & Priorität via Output-Wertelisten. |
| WP-28 | DRG-Verkettung & Eval-Plan | WP-10 | Multi-Decision-DAG, Zyklus-Erkennung, topologische Auswertung, Zwischenvariablen. |
| WP-29 | Decision Services | WP-28 | Teilauswertung mit definierten In-/Output-Decisions; encapsulated vs. output decisions korrekt. |
| WP-30 | Typecheck-Phase | WP-20, WP-28 | Statische Typprüfung wo möglich (Item Definitions), Diagnostics mit Position. |
| WP-31 | Item Definitions / Typsystem-Bindung | WP-30 | Benutzerdefinierte Typen, Listen-/Struct-Typen, Constraints (allowed values). |
| WP-32 ✅ | HTTP-Service-Wrapper | WP-10 | **done** — `service.Server` über `*dmn.Engine` (nur öffentliche `dmn/`-API, kein `internal/`). Endpunkte: `POST /v1/models` (XML → kompilieren+cachen, content-addressed `sha256:`-ID, idempotent), `GET /v1/models/{id}` (Index), `POST /v1/models/{id}/evaluate` (`{decision,input}` → Outputs/Decisions/Diagnostics), `POST /v1/evaluate` (stateless: XML+Input in einem Request), `GET /healthz`/`/readyz`. Go-1.22-Mux (kein externer Router), Request-Body-Limit, RFC-7807 `application/problem+json` mit stabilen Codes (`MALFORMED_XML`/`MODEL_NOT_FOUND`/`DECISION_NOT_FOUND`/`INVALID_REQUEST`/`EVALUATION_FAILED`). `cmd/temisd` startet den Server (`-addr`). `service/openapi.yaml`. httptest-Suite über alle Endpunkte; manuell mit `dish_15.dmn` per `curl` verifiziert. (In-Memory-Cache = WP-35-Vorstufe; Hot-Reload/Eviction → WP-35. Limits-Konfiguration → WP-34.) |
| WP-33 | gRPC-Service-Wrapper | WP-10 | `dmn.proto`, Evaluate/Compile-RPCs, Streaming für Batch. |
| WP-34 | Sicherheits-/Ressourcenlimits | WP-32 | Limits (Rekursion, Iteration, Listengröße, Compile-Timeout) konfigurierbar & erzwungen. Fuzz-getestet. |
| WP-35 | Model-Cache im Service | WP-32 | Kompilierte Modelle werden gecacht (ID/Hash); Hot-Reload bei Änderung. |

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

dmn-js erzeugt und liest **Standard-DMN-XML**. Es gibt nichts „proprietär" zu adaptieren —
die Engine muss exakt dieses XML lesen/schreiben können (das ist WP-02). Für ein optionales
Test-Frontend:

- **F-01** (Beta, optional): Minimal-Webseite, die dmn-js einbettet, Modell an den
  HTTP-Service schickt und Ergebnisse anzeigt. Rein zu Demo-Zwecken, **kein** Produktziel.
- Round-trip-Pflicht: Eine in dmn-js gespeicherte Datei muss von der Engine ladbar sein
  **und** eine von der Engine (un)veränderte Datei muss in dmn-js wieder öffnen.
