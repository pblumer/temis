# Teststrategie

> Korrektheit schlägt Geschwindigkeit. Diese Strategie ist verbindlich für jedes WP.
> „Tests zuerst" ist Pflicht (siehe `60-ai-agent-guide.md`).

## 1. Test-Pyramide

```
        ┌─────────────────────┐
        │  TCK-Konformität     │  offizielle DMN-Testsuite (oberstes Korrektheitsmaß)
        ├─────────────────────┤
        │  E2E (dmn-js-Files)  │  reale Modelle → erwartete Outputs (Golden-Files)
        ├─────────────────────┤
        │  Integration         │  XML→Compile→Evaluate je Feature
        ├─────────────────────┤
        │  Unit (breit)        │  Lexer, Parser, Built-ins, Hit Policies, Values …
        └─────────────────────┘
   quer: Property-Tests + Fuzzing (Lexer/Parser/XML), Benchmarks (CI-Gate)
```

## 2. Unit-Tests

- **Tabellengetrieben.** Besonders FEEL: Beispiele aus der Spec als
  `{expr, input, want}`-Tabellen. Pro Built-in mind. Normalfall + `null`-Fall.
- Jede Hit Policy bekommt eigene Tabellentests inkl. Aggregation & leerem Treffer.
- Decimal-Arithmetik: Genauigkeits-Fälle (`0.1+0.2`, Rundung, Division).

## 3. Property-/Fuzz-Tests

- `go test -fuzz` (WP-44). **Invariante:** kein Panic, kein Hang (Timeout), kein OOM
  bei beliebigem Input. Die Fuzz-Ziele decken jede Schicht ab, die untrusted Input
  sieht:
  - `internal/feel.FuzzLexer`, `FuzzParser` — Lexer/Parser akzeptieren jeden String
    ohne Panic; Fehler kommen als `*ParseError`, erfolgreiche ASTs rendern panikfrei.
  - `internal/feel.FuzzBoundedEvaluation` — kompiliert **und** wertet FEEL unter engen
    `Limits` aus; dank ADR-0008-Schranken (Rekursion/Iteration/Listengröße) terminiert
    selbst feindlicher Input (z. B. `for i in 1..1000000000 …`) statt zu hängen.
  - `internal/value.FuzzParseNumber`, `FuzzParseDuration` — Decimal-/Dauer-Parser.
  - `internal/xml.FuzzDecode` — DMN-XML-Decoder (+ anschließendes Encode).
  - `dmn.FuzzCompile` — End-to-End über die **öffentliche** API: `Compile` und dann
    `Decision`/`Evaluate` jeder Decision unter engen `Limits`. Malformed Input ergibt
    Fehler/Diagnostics, nie einen Panic.
- `make fuzz` läuft alle Ziele je `FUZZTIME` (Default 10s) crash-frei; Failing-Inputs
  würden als Seed-Corpus unter `testdata/fuzz/<FuzzName>/` persistiert und so zum
  Regressionstest. Nicht Teil von `make verify` (zeitgebunden, separat ausgeführt).
- Property: `parse(print(ast)) ≡ ast` für FEEL-Ausdrücke (Round-trip), wo ein Printer
  existiert.

## 4. E2E mit echten dmn-js-Dateien

- `testdata/models/*.dmn` — in dmn-js erstellte/gespeicherte Dateien.
- Je Datei eine `*.cases.json`: Liste `{input, expectedOutputs}`.
- **Round-trip-Test:** Datei laden, unverändert serialisieren, erneut in (Headless-)dmn-js
  oder gegen das XSD validieren → muss gültig bleiben, `DMNDI` erhalten.

## 5. TCK (Technology Compatibility Kit) — das zentrale Korrektheitsmaß

- Quelle: offizielles DMN-TCK-Repository (github.com/dmn-tck/tck). Es wird **an einem
  gepinnten Commit bezogen, nicht vendored** (18 MB XML): `make tck-corpus` holt es nach
  `.tck-corpus/` (gitignored); die CI-Lane `tck` tut dasselbe. So bleibt das Repo schlank.
- `internal/tck`-Runner: liest `*.dmn` + zugehörige Testcase-XML, ruft die Engine, vergleicht
  **pro Case** die Ziel-Decision (ein Compile-Fehler in einer Decision schlägt nur deren Cases
  fehl, nicht die ganze Suite).
- Gate: `internal/tck.TestOfficialTCKConformance` erzwingt einen **Ratchet-Floor**
  (`conformanceFloor`), der nur nach oben wandert; ohne `TCK_CORPUS` skippt der Test
  (offline grün). Lokal: `make tck-conformance`.
- **Aktueller Stand:** 77,4 % (Level 2 + 3). **1.0-Ziel (WP-41):** ≥ 95 % der *anwendbaren*
  Cases. Stand, Kategorien und bewusste Auslassungen (z. B. externe Java-Funktionen) stehen in
  `docs/tck-exceptions.md`.
- CI bricht, wenn die TCK-Quote unter den Floor fällt (Regressionsschutz).

## 6. Benchmarks & Performance-Budget (CI-Gate, WP-42)

Benchmarks in `dmn/bench_test.go` (`go test -bench -benchmem`). Das CI-Gate ist
`TestPerformanceBudget` (`dmn/budget_test.go`), ausgeführt von `make budget`
**ohne** Race-Detektor (der Zeiten & Allokationen verfälscht) und Teil von
`make verify`. Die Budgets sind in WP-42 fixiert (Richtwert ≈ gemessen × Headroom):

| Szenario | Metrik | Budget (Gate) | gemessen (Referenz) |
|---|---|---|---|
| Compile Decision Table (10 Regeln, 4 Inputs) | ns/op · allocs | ≤ 5 ms · ≤ 5000 | ~0,27 ms · 2056 (einmalig, unkritisch) |
| Evaluate dieselbe Table (warm) | ns/op · allocs | ≤ 80 µs · ≤ 60 | ~4,3 µs · 41 |
| FEEL-Arithmetik-Ausdruck (über öffentl. API) | ns/op · allocs | ≤ 60 µs · ≤ 40 | ~4,5 µs · 26 |
| DRG mit 10 verketteten Decisions | ns/op · allocs | ≤ 150 µs · ≤ 130 | ~8,2 µs · 74 (≈ linear) |

Regeln:
- **`allocs/op`** ist der primäre, maschinenunabhängige Wächter (deterministisch);
  eine zusätzliche, bewusst großzügige **`ns/op`**-Decke fängt nur katastrophale
  oder Komplexitäts-Regressionen, nicht das Timing-Rauschen geteilter CI-Runner.
- Überschreitet ein Szenario sein Budget, **bricht CI** (`TestPerformanceBudget`).
- Bei bewusster Regression: Budget in `dmn/budget_test.go` anheben + Begründung im
  Commit/ADR.
- Der reine FEEL-Ausdruckskern bleibt sub-µs (`internal/feel` `BenchmarkEval`); die
  µs-Zahlen oben enthalten den öffentlichen Evaluate-Pfad (Input-Marshaling,
  Decimal-Arithmetik, Ergebnis-Konvertierung).

## 7. Race & Parallelität

- Gesamte Suite läuft auch mit `-race`.
- Dedizierter Test: dieselbe `CompiledDecision` aus N Goroutinen gleichzeitig evaluieren →
  deterministische, identische Ergebnisse, kein Datarace.

## 8. Coverage

- Zielmarke: ≥ 90 % für `dmn`, `internal/feel`, `internal/boxed`, `internal/value`,
  `internal/model` — **durchgesetzt** über `make cover` (eigenes CI-Lane, WP-137). `internal/drg`
  ist ein leeres Scaffold (Graph-Logik liegt in `dmn`), daher nicht in der Liste.
- Coverage ist Indikator, nicht Selbstzweck — TCK + E2E zählen mehr.

## 9. `make verify` (lokales & CI-Gate)

```
make verify  ==  gofmt-check + go vet + staticcheck/golangci-lint
                 + go test ./... -race
                 + go test -run=^$ -bench=. -benchmem (smoke)
                 + go test ./internal/tck (sofern Stand vorhanden)
```

Kein WP gilt als `done`, solange `make verify` rot ist.
