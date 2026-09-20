# Testarchitektur & Teststrategie — Handbuch

> **Korrektheit schlägt Geschwindigkeit.** Dieses Dokument ist die *eine*
> verbindliche Quelle dafür, wie Temis mit Tests sicherstellt, dass nichts
> schiefläuft — von der einzelnen FEEL-Funktion bis zum Modeler im echten
> Browser. „Tests zuerst" ist Pflicht (siehe `60-ai-agent-guide.md`,
> `CONTRIBUTING.md`). Kein Arbeitspaket (WP) gilt als `done`, solange das
> Gate rot ist.

Das Handbuch hat zwei Teile:

- **§1–§9 — die Test­schichten:** was jede Schicht prüft und wie sie erzwungen
  wird. Diese Nummern sind stabil; Code und Makefile zitieren sie (z. B. `§3`
  Fuzzing, `§6` Budget, `§8` Coverage) — beim Umbau bitte erhalten.
- **§10–§14 — die Architektur drumherum:** CI-Landschaft, Datei-Layout,
  Fixtures/Test-Doubles, ein Rezept zum Schreiben neuer Tests und die
  Fehlerklassen→Gate-Matrix, die zeigt, *welche* Schicht *welchen* Fehler fängt.

---

## Schnelleinstieg

```bash
make verify          # das autoritative Gate: fmt + vet + lint + race-Tests + bench-smoke + budget + tck-runner
make cover           # Coverage-Floor der Kernpakete (≥ 90 %)
make web-e2e         # Frontend bauen + Playwright im echten Browser
make tck-conformance # offizielle DMN-TCK-Konformität gegen das gepinnte Korpus
make fuzz            # alle Fuzz-Ziele je FUZZTIME (Default 10 s), crash-frei
make help            # alle Targets auflisten
```

Größenordnung (Stand dieses Dokuments): **158** Go-`*_test.go`-Dateien mit
~**1030** Test-/Fuzz-/Benchmark-Funktionen, **24** Playwright-E2E-Specs, dazu
das komplette offizielle DMN-TCK-Korpus als Konformitäts-Gate.

## Prinzipien

1. **Am Verhalten testen, nicht an Interna.** Unit-Tests gegen die öffentliche
   Paket-API, E2E gegen sichtbaren Zustand (Text, ARIA, Sichtbarkeit).
2. **Ein Fehler, ein Gate.** Jede Fehlerklasse hat eine Schicht, die sie
   deterministisch fängt (§14). Deckt keine, ist das eine Lücke, kein Zufall.
3. **Ratchets statt Wunschzahlen.** TCK-Quote (§5) und Coverage (§8) haben
   Böden, die nur nach oben wandern — Regressionen brechen CI.
4. **Determinismus vor Timing.** Wo Zeit rauscht (Benchmarks), wacht die
   maschinenunabhängige Metrik (`allocs/op`), nicht die Uhr (§6).
5. **Ehrlichkeit über Abdeckung.** Bewusste Auslassungen werden benannt
   (`docs/tck-exceptions.md`, die Lücken-Hinweise in §7) — nicht kaschiert.

---

## 1. Test-Pyramide

```
        ┌─────────────────────┐
        │  TCK-Konformität     │  offizielle DMN-Testsuite (oberstes Korrektheitsmaß)  §5
        ├─────────────────────┤
        │  E2E (Playwright)    │  echter Browser → temisd → WASM-FEEL (voller Stack)   §4
        ├─────────────────────┤
        │  Integration         │  XML→Compile→Evaluate je Feature, Service-HTTP/gRPC   §2
        ├─────────────────────┤
        │  Unit (breit)        │  Lexer, Parser, Built-ins, Hit Policies, Values …     §2
        └─────────────────────┘
   quer: Property-Tests + Fuzzing (§3) · Benchmarks/Budget (§6) · Race (§7) · API-Surface-Golden (§2)
```

Die Basis ist breit und schnell (Millisekunden, pro PR), die Spitze schmal und
teuer (Browser, Korpus-Download). Je weiter oben ein Test fehlschlägt, desto
näher am realen Nutzerpfad ist der Fehler.

## 2. Unit- & Integrationstests

- **Tabellengetrieben.** Besonders FEEL: Beispiele aus der Spec als
  `{expr, input, want}`-Tabellen. Pro Built-in mind. Normalfall + `null`-Fall.
- Jede Hit Policy bekommt eigene Tabellentests inkl. Aggregation & leerem Treffer.
- Decimal-Arithmetik: Genauigkeits-Fälle (`0.1+0.2`, Rundung, Division).
- **Integration** heißt hier: über die *öffentliche* API einer Schicht — z. B.
  `XML → Compile → Evaluate` je Feature (`dmn`), oder ein echter HTTP-Handler
  gegen `httptest` (`service/*_test.go`) statt gemockter Interna.
- **API-Surface-Golden (`dmn/apisurface_test.go`).** `TestPublicAPISurface`
  parst die exportierte Oberfläche von `package dmn`, trimmt alles Unexportierte
  und vergleicht sie gegen `dmn/testdata/api/dmn.api`. Jede Änderung am
  SemVer-stabilen v1-Vertrag (ADR-0011, ADR-0019) bricht den Test und erzwingt
  eine bewusste Entscheidung: additive Änderung ⇒ Golden aktualisieren
  (`go test ./dmn -run TestPublicAPISurface -update-api`); Bruch ⇒ zusätzlich
  Major-Bump.

Verteilung der ~158 Go-Testdateien (grob): `internal/*` 59 · `dmn` 42 ·
`service` 31 · `vcs` 6 · `mcp` 6 · `assist` 4 · `flow` 3 · `audit`/`consume`/`cmd` je 2 ·
`quality` 1.

## 3. Property-/Fuzz-Tests

- `go test -fuzz` (WP-44). **Invariante:** kein Panic, kein Hang (Timeout), kein OOM
  bei beliebigem Input. Die Fuzz-Ziele decken jede Schicht ab, die untrusted Input
  sieht:
  - **FEEL-Front-end & Wertemodell:** die Fuzz-Ziele des Lexers/Parsers
    (`FuzzLexer`/`FuzzParser`), der beschränkten Auswertung (`FuzzBoundedEvaluation`) und
    der Decimal-/Dauer-Parser (`FuzzParseNumber`/`FuzzParseDuration`) leben seit ADR-0039
    im externen Modul `github.com/pblumer/feel` und laufen dort in dessen CI. temis' eigener
    `make fuzz`-Lane deckt nur noch die temis-eigenen Grenzschichten ab:
  - `internal/xml.FuzzDecode` — DMN-XML-Decoder (+ anschließendes Encode).
  - `dmn.FuzzCompile` — End-to-End über die **öffentliche** API: `Compile` und dann
    `Decision`/`Evaluate` jeder Decision unter engen `Limits`. Malformed Input ergibt
    Fehler/Diagnostics, nie einen Panic.
- `make fuzz` läuft alle Ziele je `FUZZTIME` (Default 10s) crash-frei; Failing-Inputs
  würden als Seed-Corpus unter `testdata/fuzz/<FuzzName>/` persistiert und so zum
  Regressionstest. **Nicht** Teil von `make verify` (zeitgebunden) — stattdessen als
  nächtliche Sweep-Lane (`FUZZTIME=60s`, siehe §10).
- Property: `parse(print(ast)) ≡ ast` für FEEL-Ausdrücke (Round-trip), wo ein Printer
  existiert.

## 4. E2E — Playwright gegen den echten Modeler

Die E2E-Schicht fährt den **vollen Stack wie ein Nutzer**: ein echtes Chromium
gegen den von `temisd` ausgelieferten Modeler, inklusive der **WASM-FEEL-Engine**
und der gebauten `web/dist`. Kein Mock — was grün ist, funktioniert im Browser.

- **Ort:** `web/e2e/*.spec.ts` (Playwright). Konfiguration: `web/playwright.config.ts`.
- **Lauf:** `make web-e2e` (baut Frontend via `make web`, dann `playwright test`).
  Die `webServer`-Direktive startet `go run ./cmd/temisd -examples=true` und fährt
  ihn danach herunter; die Specs werten gegen die gebündelten Beispielmodelle aus.
- **Browser:** von Playwright verwaltet, oder ein vorinstalliertes über
  `PLAYWRIGHT_CHROMIUM_PATH`; Port via `TEMIS_E2E_PORT` (Default 8099).
- **CI:** eigene Lane `web-e2e` in `.github/workflows/ci.yml` (`retries: 1`,
  `trace: on-first-retry`, `forbidOnly` gegen vergessenes `test.only`). **Nicht**
  Teil von `make verify` (braucht Browser + Node-Build), sondern separat —
  analog zur `tck`-Lane.
- **Abdeckung & „was testet welche Spec":** der gepflegte Katalog liegt bei den
  Tests: [`web/e2e/README.md`](../web/e2e/README.md). Grob: Auswertung/
  Operate-Cockpit/Live-Graph, Modellierung & Editor (Palette, Tabellen, BKM,
  Completion, Highlighting), Verwaltung/Import/Clio-Status sowie gezielte
  Sicherheits- und Boot-Regressionen (XSS-Escaping, leerer Server).
- **Konvention:** Jede Spec trägt oben einen Intent-Kommentar (*was* + *warum*,
  bei Regressionen mit Audit-/WP-/ADR-Bezug). Neue Spec ⇒ Katalog mitpflegen.

> **DMN-Round-trip** (Datei laden → serialisieren → gültig & `DMNDI` erhalten) ist
> auf Go-Ebene abgedeckt (`internal/xml`, `internal/model`, `dmn`), nicht hier.

## 5. TCK (Technology Compatibility Kit) — das zentrale Korrektheitsmaß

- Quelle: offizielles DMN-TCK-Repository (github.com/dmn-tck/tck). Es wird **an einem
  gepinnten Commit bezogen, nicht vendored** (18 MB XML): `make tck-corpus` holt es nach
  `.tck-corpus/` (gitignored); die CI-Lane `tck` tut dasselbe. So bleibt das Repo schlank.
- `internal/tck`-Runner: liest `*.dmn` + zugehörige Testcase-XML, ruft die Engine, vergleicht
  **pro Case** die Ziel-Decision (ein Compile-Fehler in einer Decision schlägt nur deren Cases
  fehl, nicht die ganze Suite). Der Runner selbst hat einen Mini-Fixture-Selbsttest
  (`internal/tck/testdata/tckdemo.dmn` + `tckdemo-test.xml`), damit er auch offline grün prüft.
- **Zahl-Vergleich mit Oracle-Präzision:** Die Engine rechnet spec-konform in decimal128 (34
  Stellen, ADR-0007); transzendente/irrationale Ergebnisse tragen mehr Stellen als die gerundeten
  TCK-Erwartungswerte. Der Runner rundet daher das Ist-Ergebnis auf die Dezimalstellen-Zahl des
  Erwartungswerts, bevor er vergleicht (`numClose`) — **additiv**: ganzzahlige/exakte Erwartungen
  bleiben streng, echte Abweichungen scheitern weiter. Details + bewusst offene float64-Referenz-
  Fälle in `docs/tck-exceptions.md`.
- Gate: `internal/tck.TestOfficialTCKConformance` erzwingt einen **Ratchet-Floor**
  (`conformanceFloor`), der nur nach oben wandert; ohne `TCK_CORPUS` skippt der Test
  (offline grün). Lokal: `make tck-conformance`.
- **Aktueller Stand:** 96,5 % (Level 2 + 3). **1.0-Ziel (WP-41):** ≥ 95 % der *anwendbaren*
  Cases — **erreicht**; weitere Fixes heben den Floor. Stand, Kategorien und bewusste, dokumentierte
  Auslassungen (externe Java-Funktionen ohne JVM; float64-präzise TCK-Referenzwerte) stehen in
  `docs/tck-exceptions.md`.
- CI bricht, wenn die TCK-Quote unter den Floor fällt (Regressionsschutz).

## 6. Benchmarks & Performance-Budget (CI-Gate, WP-42)

Benchmarks in `dmn/bench_test.go` (`go test -bench -benchmem`). Das CI-Gate ist
`TestPerformanceBudget` (`dmn/budget_test.go`), ausgeführt von `make budget`
**ohne** Race-Detektor (der Zeiten & Allokationen verfälscht) und Teil von
`make verify`. Der Test skippt unter `-race` (Build-Tag-Schalter `raceEnabled`
in `dmn/race_on_test.go` / `race_off_test.go`) und läuft race-frei über `make budget`.
Die Budgets sind in WP-42 fixiert (Richtwert ≈ gemessen × Headroom):

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

- **Die gesamte Suite läuft unter `-race`** (`make test` / `make verify`) — der
  Race-Detektor deckt Datenrennen in jedem Testpfad auf, nicht nur in dediziertem
  Nebenläufigkeits-Code.
- **Design-Garantie:** eine `CompiledDecision` ist nach `Compile` unveränderlich und
  damit nebenläufig sicher wiederverwendbar (dokumentiert an
  `dmn.ExampleCompiledDecision_Evaluate`); der Wiederverwendungs-Pfad wird unter
  `-race` mitgeprüft.
- **Bekannte Lücke:** ein *dedizierter* Stresstest (dieselbe `CompiledDecision` aus
  N Goroutinen gleichzeitig, Ergebnisgleichheit) existiert noch nicht. Er ist als
  gezielte Ergänzung vorgemerkt — bis dahin ruht die Nebenläufigkeits-Zusicherung
  auf `-race` über den bestehenden Reuse-Tests, nicht auf einem Lastszenario.

## 8. Coverage

- Zielmarke: ≥ 90 % für `dmn`, `internal/feel`, `internal/boxed`, `internal/value`,
  `internal/model` — **durchgesetzt** über `make cover` (eigenes CI-Lane, WP-137). `internal/drg`
  ist ein leeres Scaffold (Graph-Logik liegt in `dmn`), daher nicht in der Liste.
- Der Floor (`COVER_MIN`, Default 90) sitzt bewusst *unter* der real gemessenen
  Abdeckung: er soll eine echte Regression auslösen, nicht bei jedem Rauschen zicken.
- Coverage ist Indikator, nicht Selbstzweck — TCK + E2E zählen mehr.

## 9. `make verify` — das autoritative Gate

```
make verify  ==  fmt-check  (gofmt-clean)
               + vet        (go vet)
               + lint       (golangci-lint, no-op-friendly wenn nicht installiert)
               + test       (go test ./... -race)
               + bench      (go test -run=^$ -bench=. -benchmem — Smoke)
               + budget     (TestPerformanceBudget, race-frei — §6)
               + tck        (internal/tck-Runner; Konformitätstest skippt ohne Korpus — §5)
```

Daneben laufen in CI (`.github/workflows/ci.yml`) eigene, von `verify` getrennte
Lanes (§10), weil sie zusätzliche Toolchains/Zeit brauchen. Kein WP gilt als
`done`, solange `make verify` **oder** eine dieser Lanes rot ist.

---

## 10. CI-Landschaft

Zwei Workflows halten das Netz gespannt:

**`ci.yml` — pro Push auf `main` und pro Pull Request:**

| Lane | Kommando | Absichert |
|---|---|---|
| `verify` | `make verify` + `make cover` | das Go-Gate (§9) + Coverage-Floor (§8) |
| `web-e2e` | `make web` → `playwright test` | Modeler im echten Browser (§4) |
| `tck` | `make tck-conformance` | offizielle DMN-Konformität (§5) |
| `web` | `npm run typecheck` + build + `git diff --exit-code dist` | Frontend-Typen & dass das committete `dist` aktuell ist |
| `security` | `govulncheck ./...` (Go `stable`) | bekannte CVEs in Deps/Stdlib |
| `proto` | `make proto-check` | committeter gRPC-Code nicht stale (ADR-0020) |
| `docker` | `docker build` | Release-Image baut (Dockerfile-Bruch fällt hier, nicht erst beim Tag) |

**`nightly.yml` — täglich 03:37 UTC (+ `workflow_dispatch`):**

| Lane | Kommando | Warum nächtlich |
|---|---|---|
| `fuzz` | `make fuzz` (`FUZZTIME=60s`) | zu langsam pro PR, wertvoll als Sweep (§3) |
| `vuln` | `govulncheck ./...` | frisch veröffentlichte Advisories tauchen auch ohne PR auf |

Der `security`-Scan nutzt bewusst Go `stable` (nicht die `go.mod`-Mindestversion):
Stdlib-CVEs werden per Toolchain-Update behoben, also soll der Scan der neuesten
gepatchten Stdlib folgen.

## 11. Datei- & Verzeichnis-Layout

- **Go-Tests liegen neben dem Code** (`foo.go` → `foo_test.go`), im selben Paket
  für White-Box- oder in `paket_test` für Black-Box-/API-Tests (z. B. `dmn_test`).
- **Fixtures** stehen in `testdata/`-Ordnern (vom Go-Toolchain ignoriert):
  - `dmn/testdata/models` — DMN-XML-Modelle für Compile/Evaluate.
  - `dmn/testdata/api/dmn.api` — Golden der öffentlichen API-Oberfläche (§2).
  - `internal/xml/testdata/models` — XML-Round-trip-Fixtures.
  - `internal/model/testdata/golden` — Golden-Modelldateien.
  - `internal/tck/testdata` — Mini-DMN + Testcase für den Runner-Selbsttest (§5).
  - `flow/testdata` — `loan/premium/risk/service.dmn` für Flow-Verkettung.
- **E2E:** `web/e2e/*.spec.ts` + `web/e2e/README.md` (Katalog) + `web/playwright.config.ts`.
  Fixtures sind hier die gebündelten Beispielmodelle (`-examples=true`), keine
  separaten Dateien.
- **Build-Tag-Zwillinge:** `dmn/race_on_test.go` (`//go:build race`) und
  `race_off_test.go` (`//go:build !race`) liefern die Konstante `raceEnabled`,
  damit das Budget-Gate sich unter `-race` selbst abschaltet (§6).

## 12. Fixtures, Testdata & Test-Doubles

- **Echte Artefakte statt Mocks, wo möglich.** DMN-Verhalten wird gegen echte
  `.dmn`-Dateien geprüft, nicht gegen handgebaute ASTs; der Modeler gegen den
  echten `temisd`.
- **Golden-Files** für stabile Verträge: API-Oberfläche (`dmn.api`),
  Modell-Serialisierung (`internal/model/testdata/golden`). Bewusste Änderungen
  werden über ein `-update…`-Flag neu aufgezeichnet, nie von Hand editiert.
- **Test-Doubles nur an der Systemgrenze** — dort, wo ein externer Dienst nicht
  im Test leben soll:
  - Ausgehendes HTTP gegen `net/http/httptest`-Server statt echter Endpunkte:
    `assist/anthropic`, `assist/openai` (LLM-Clients inkl. Timeout-Verhalten),
    `vcs/github` (Git-Backend), `service/ratelimit` u. a.
  - VCS-/Model-Store: In-Memory-Fakes für die Persistenzschicht in `vcs/*_test.go`.
- **Kein Netz, keine Uhr, kein Zufall** in Unit-Tests: Zeit/IDs werden injiziert
  oder gegen Muster geprüft, damit Tests deterministisch bleiben.

## 13. Einen Test schreiben — Rezept nach Schicht

1. **Neue FEEL-Funktion / Hit Policy / Value-Regel?** → Tabellentest im jeweiligen
   `internal/*`-Paket: Normalfall, `null`-Fall, Grenzfälle. Sieht die Schicht
   untrusted Input, zusätzlich ein Fuzz-Ziel oder Seed erweitern (§3).
2. **Neues Engine-Verhalten (Compile/Evaluate/DRG)?** → Integrationstest in `dmn`
   über die öffentliche API mit einer `.dmn`-Fixture unter `dmn/testdata`. Ändert
   sich die exportierte Oberfläche, `TestPublicAPISurface` bewusst via `-update-api`
   nachziehen (§2). Prüfe, ob ein passender TCK-Case existiert (§5).
3. **Neuer HTTP-/gRPC-Endpunkt oder Service-Verhalten?** → `service/*_test.go`
   gegen `httptest`; externe Aufrufe als Double (§12).
4. **Neue Modeler-Funktion (UI)?** → Playwright-Spec in `web/e2e` mit
   Intent-Kommentar, gegen sichtbaren Zustand assertieren, Katalog in
   `web/e2e/README.md` mitpflegen (§4).
5. **Heißer Pfad berührt?** → Benchmark in `dmn/bench_test.go`; überschreitet er
   ein Budget, ist das der erwartete Alarm (§6).
6. **Zum Schluss:** `make verify` (+ ggf. `make web-e2e`, `make tck-conformance`)
   grün. Erst dann ist das WP `done`.

## 14. Fehlerklassen → Gate-Matrix

Welche Schicht fängt welchen Fehler — so ist „nichts läuft schief" nachprüfbar
statt Hoffnung:

| Fehlerklasse | primär gefangen von | Gate |
|---|---|---|
| Falsche FEEL-Semantik / Hit Policy | Unit-Tabellentests (§2) + TCK (§5) | `verify`, `tck` |
| Abweichung vom DMN-Standard | TCK-Ratchet (§5) | `tck` |
| Panic/Hang/OOM bei bösartigem Input | Fuzzing (§3) | `nightly` |
| Kaputter Compile→Evaluate-Pfad | Integrationstests `dmn` (§2) | `verify` |
| Regression im UI / kaputter Nutzerfluss | Playwright-E2E (§4) | `web-e2e` |
| XSS / Escaping / Boot-Fehler im Frontend | E2E-Sicherheits-Specs (§4) | `web-e2e` |
| Bruch des öffentlichen v1-Vertrags | API-Surface-Golden (§2) | `verify` |
| Performance-/Allokations-Regression | Budget-Gate (§6) | `verify` |
| Datenrennen | `-race` über die ganze Suite (§7) | `verify` |
| Ungetesteter Code schleicht sich ein | Coverage-Floor (§8) | `cover` |
| Verwundbare Abhängigkeit / Stdlib-CVE | `govulncheck` (§10) | `security`, `nightly` |
| Stale generierter gRPC-Code | `proto-check` (§10) | `proto` |
| Frontend-`dist` nicht neu gebaut | `git diff --exit-code dist` (§10) | `web` |
| Kaputtes Release-Image | `docker build` (§10) | `docker` |

Findet sich eine reale Fehlerklasse hier **nicht** wieder, ist das eine
Test-Lücke und gehört adressiert — nicht ignoriert.

## 15. Referenzen

- Prozess/Pflicht: `CONTRIBUTING.md`, `docs/60-ai-agent-guide.md`
- Architektur der Engine: `docs/10-architecture.md`
- API-Vertrag & SemVer: `docs/40-api-contract.md`, ADR-0011, ADR-0019
- Ressourcen-Schranken (Fuzz-Terminierung): ADR-0008
- E2E-Katalog: `web/e2e/README.md`
- TCK-Auslassungen: `docs/tck-exceptions.md`
- Gates in Code/CI: `Makefile`, `.github/workflows/ci.yml`, `.github/workflows/nightly.yml`
