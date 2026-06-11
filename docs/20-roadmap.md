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
| WP-03 | FEEL Lexer | WP-01 | Tokenisiert alle FEEL-Lexeme (Zahlen, Strings mit Escapes, Namen mit Leerzeichen!, Datums-/Zeitliterale, Operatoren). Property-Test: kein Input paniced. |
| WP-04 | FEEL Parser → AST | WP-03 | Vollständige Ausdrucks-Grammatik (Operatorpräzedenz, `if/then/else`, `for/in/return`, `some/every`, Pfadzugriff, Funktionsaufruf, Listen/Contexts). Fehler liefern Position. |
| WP-05 | FEEL-Werte & Number-Decimal | WP-01 | `Value`-Typen inkl. Decimal-Number (kein float), date/time/duration. Arithmetik spec-konform (Rundung, `null`-Propagation). |
| WP-06 | FEEL-Compiler (AST→Closure) Kern | WP-04, WP-05 | Arithmetik, Vergleiche, Boolesche Logik, `if`, Variablen-Lookup über Slot-Index. `Compile` einmal, `Eval` allokationsarm. Benchmark vorhanden. |
| WP-07 | Built-ins Kern | WP-06 | Mindestset: `not, count, sum, min, max, mean, contains, starts with, substring, string length, number, date, ...`. Tabellen-gebunden zur Compile-Zeit. |
| WP-08 | Unary Tests | WP-06 | Kompiliert Decision-Table-Eingabezellen: Werte, Intervalle, Aufzählungen, `-`, Negation, Ausdrücke. Match-Funktion getestet. |
| WP-09 | Decision Table Compiler + Hit Policies U/A/F/R/C | WP-07, WP-08 | Korrekte Treffer-Auswahl & Aggregation. Tabellengetriebene Tests gegen erwartete Outputs. |
| WP-10 | Public Library-API (Compile/Evaluate) | WP-09 | `dmn.Engine`, `CompiledDecision`, `Context`, `Result` gemäß `40-api-contract.md`. End-to-End-Test: dmn-js-Datei → Ergebnis. |
| WP-11 | MVP-Beispiele & Golden-Tests | WP-10 | ≥ 5 reale dmn-js-Dateien (Loan, Discount, Routing…) als E2E-Golden-Tests im Repo. |

---

## Etappe Beta — „Vollständiges DMN 1.5"

**Ziel:** Voller FEEL, **alle** Boxed Expressions, vollständige DRD-Verkettung,
Decision Services, plus Service-Wrapper (HTTP + gRPC).

| WP | Titel | Abhängt von | Akzeptanzkriterium |
|---|---|---|---|
| WP-20 | FEEL vollständig | WP-06 | `for`-Comprehensions (multi-`in`), `some/every`, Range-Funktionen, String-Slicing, Pfad-/Filter-Ausdrücke `list[cond]`, Kontext-Filter, vollständige Operatorensemantik. |
| WP-21 | FEEL Built-ins vollständig | WP-07, WP-20 | **Alle** Built-ins der DMN-1.5-Tabelle (conversion, boolean, string, list, numeric, date/time, range, temporal, sort, context functions). Je Built-in ≥ 1 Test inkl. `null`-Fälle. |
| WP-22 | Date/Time/Duration vollständig | WP-05 | Zeitzonen, `@`-Literale, Dauer-Arithmetik, Vergleiche, `is`-Gleichheit. Gegen FEEL-Spezifikationsbeispiele getestet. |
| WP-23 | Boxed Context & Invocation | WP-20 | Context mit Result-Cell; Invocation ruft BKM mit benannten Parametern. |
| WP-24 | Boxed Function & BKM | WP-23 | Definition + Aufruf, Closures über Kontext, Rekursion mit Tiefenlimit. |
| WP-25 | Boxed List & Relation | WP-20 | List-/Relation-Auswertung zu FEEL-Listen/Kontextlisten. |
| WP-26 | Conditional / Iterator / Filter (1.4/1.5) | WP-20 | Boxed `if`, `for`/`every`/`some` als Boxed, `filter`. Spec-Beispiele grün. |
| WP-27 | Hit Policies vollständig | WP-09 | Ergänzt **P, O, C+, C<, C>, C#** inkl. Output Order & Priorität via Output-Wertelisten. |
| WP-28 | DRG-Verkettung & Eval-Plan | WP-10 | Multi-Decision-DAG, Zyklus-Erkennung, topologische Auswertung, Zwischenvariablen. |
| WP-29 | Decision Services | WP-28 | Teilauswertung mit definierten In-/Output-Decisions; encapsulated vs. output decisions korrekt. |
| WP-30 | Typecheck-Phase | WP-20, WP-28 | Statische Typprüfung wo möglich (Item Definitions), Diagnostics mit Position. |
| WP-31 | Item Definitions / Typsystem-Bindung | WP-30 | Benutzerdefinierte Typen, Listen-/Struct-Typen, Constraints (allowed values). |
| WP-32 | HTTP-Service-Wrapper | WP-10 | `POST /evaluate` (Modell-ID + Inputs → Outputs), `POST /compile`, OpenAPI. Nur über `dmn/`. |
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
