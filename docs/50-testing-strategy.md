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

- `go test -fuzz` für: Lexer, Parser, XML-Decoder. **Invariante:** kein Panic, kein
  Hang (Timeout), kein OOM bei beliebigem Input.
- Property: `parse(print(ast)) ≡ ast` für FEEL-Ausdrücke (Round-trip), wo ein Printer
  existiert.

## 4. E2E mit echten dmn-js-Dateien

- `testdata/models/*.dmn` — in dmn-js erstellte/gespeicherte Dateien.
- Je Datei eine `*.cases.json`: Liste `{input, expectedOutputs}`.
- **Round-trip-Test:** Datei laden, unverändert serialisieren, erneut in (Headless-)dmn-js
  oder gegen das XSD validieren → muss gültig bleiben, `DMNDI` erhalten.

## 5. TCK (Technology Compatibility Kit) — das zentrale Korrektheitsmaß

- Quelle: offizielles DMN-TCK-Repository (Modelle + Testdefinitionen im standardisierten
  Format). Wird als Submodule/Vendored-Copy eingebunden.
- `internal/tck`-Runner: liest `*.dmn` + zugehörige Testcase-XML, ruft die Engine, vergleicht.
- Report listet pro Case: pass/fail/not-applicable + Grund.
- **1.0-Ziel (WP-41):** ≥ 95 % der *anwendbaren* Cases grün. Jede bewusste Auslassung
  (z. B. PMML/ONNX, externe Funktionen) wird in `docs/tck-exceptions.md` mit Begründung
  geführt.
- CI bricht, wenn die TCK-Quote unter den eingefrorenen Stand fällt (Regressionsschutz).

## 6. Benchmarks & Performance-Budget (CI-Gate ab WP-42)

Benchmarks mit `go test -bench -benchmem`. Gemessen werden:

| Szenario | Metrik | Budget (Richtwert, in WP-42 final fixiert) |
|---|---|---|
| Compile mittlere Decision Table (10 Regeln, 4 Inputs) | ns/op | einmalig, unkritisch (Ziel < 1 ms) |
| Evaluate dieselbe Table (warm) | ns/op | **niedriger einstelliger µs-Bereich** |
| Evaluate, Allokationen | allocs/op | **niedrige zweistellige Zahl**, stabil |
| FEEL-Arithmetik-Ausdruck | ns/op | Sub-µs |
| DRG mit 10 verketteten Decisions | µs/op | linear skalierend, kein Map-Overhead im Hot Path |

Regeln:
- **`benchstat`** vergleicht gegen gespeicherte Baseline (`testdata/bench/baseline.txt`).
- Eine Regression über Schwelle (z. B. > 10 % ns/op oder mehr allocs/op) **bricht CI**.
- Bei bewusster Regression: Baseline-Update + Begründung im Commit/ADR.

## 7. Race & Parallelität

- Gesamte Suite läuft auch mit `-race`.
- Dedizierter Test: dieselbe `CompiledDecision` aus N Goroutinen gleichzeitig evaluieren →
  deterministische, identische Ergebnisse, kein Datarace.

## 8. Coverage

- Zielmarke: ≥ 90 % für `internal/feel`, `internal/boxed`, `internal/drg`, `dmn`.
- Coverage ist Indikator, nicht Selbstzweck — TCK + E2E zählen mehr.

## 9. `make verify` (lokales & CI-Gate)

```
make verify  ==  gofmt-check + go vet + staticcheck/golangci-lint
                 + go test ./... -race
                 + go test -run=^$ -bench=. -benchmem (smoke)
                 + go test ./internal/tck (sofern Stand vorhanden)
```

Kein WP gilt als `done`, solange `make verify` rot ist.
