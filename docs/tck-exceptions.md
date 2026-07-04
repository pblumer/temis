# DMN-TCK-Konformität & Ausnahmen

> Referenziert aus `docs/50-testing-strategy.md` §5 und dem Roadmap-WP-41.

Temis wird gegen das **offizielle DMN Technology Compatibility Kit**
(github.com/dmn-tck/tck) geprüft. Das Korpus wird **nicht vendored** (18 MB XML),
sondern an einem gepinnten Commit bezogen und im CI ausgeführt:

- Pinned Commit: `0dbcaf9b98bc3af4e36d44a7aed95e9e85703a13`
- Lokal: `make tck-conformance` (holt das Korpus nach `.tck-corpus/`, gitignored)
- CI: Lane **`tck`** in `.github/workflows/ci.yml`
- Gate: `internal/tck.TestOfficialTCKConformance` — erzwingt einen **Ratchet-Floor**
  (`conformanceFloor`), der nur nach oben wandert. Ohne `TCK_CORPUS` **skippt** der
  Test, damit `go test ./...` offline grün bleibt.

## Aktueller Stand

| Metrik | Wert |
|---|---|
| Compliance Level 2 + 3 | **2807 / 3495 Cases grün (80,3 %)** |
| Suites | 146 (0 laden fehlerhaft) |
| Ratchet-Floor im CI | 80,0 % |

Das WP-41-Ziel ist **≥ 95 % der anwendbaren Cases**. Der Weg dahin ist als
Kategorien unten dokumentiert; der Floor wird mit jedem Fix angehoben, sodass
Regressionen den Gate brechen.

> **Wichtiger Runner-Fix (frühere Etappe):** Der TCK-Runner bewertet **pro
> Case** die Ziel-Decision, statt eine ganze Suite abzubrechen, sobald **irgendeine**
> Decision im Modell einen Compile-Fehler hat. Das ist die korrekte TCK-Semantik und
> hat die real messbare Case-Zahl von 480 auf 3495 gehoben.

## In dieser Etappe behoben — Arithmetik & Temporal (0100: 96 → 5 Fails)

- **Negative (BCE-/astronomische) Jahre** in `date`/`date and time`-Literalen
  (`@"-2021-01-01T10:10:10+11:00"`, auch mit IANA-Zone `@Australia/Melbourne`).
  Go's Referenz-Layout parst kein führendes `-`; `parseSignedTime` streift das
  Vorzeichen, parst und negiert das Jahr (Round-Trip über `Format`).
- **`date ± duration` bleibt `date`** (DMN §10.3.2.3.5): der Zeit-Anteil wird
  abgeschnitten (`@"2021-01-01" + @"P1D"` → `xsd:date 2021-01-02`), zuvor kam ein
  `date and time` heraus.
- **Gemischte `date`/`date and time`-Subtraktion** → `days and time duration`
  über die Instant-Differenz; ein `date` gilt dabei als UTC-verankert (zoned).
  Stimmt die Zonen-Kennzeichnung zweier Operanden **nicht** überein (zoned vs.
  lokal), ist das Ergebnis korrekt **null**.
- **ISO-`24:00:00`** (Ende-des-Tages-Mitternacht) wird als `00:00:00` des Folgetags
  normalisiert (`@"2021-01-01T24:00:00" + @"PT1S"` → `2021-01-02T00:00:01`).
- **`string + string`** konkateniert (`"foo" + "bar"` → `"foobar"`).

Diese Fixes heben neben `0100` auch die reinen Datums-/Zeit-Suites (`0007`,
`1115`/`1116`) an — netto **+103 Cases** (77,4 % → 80,3 %).

## Früher behoben

- **`in`-Operator vollständig** (0072, 224→21 Fails): operator-präfixierte Tests
  (`x in <= 10`), explizite Gleichheit (`x in = 10`), Komma-Test-Listen
  (`x in (1, < 5, >= 10)`) und Listen-Mitgliedschaft inkl. Range-Elementen.
- **`is(v1, v2)`** (Builtin) — Wert- **und** Typgleichheit (0103, war 0/50).
- **`list replace(list, position|match, newItem)`** (Builtin) (1155).
- **`number(from, grouping, decimal)`** — mehrargumentige Form (0058).
- **`range(from)`-Builtin** (1156, 56→19 Fails) + `instance of range<T>`.

## Offene Kategorien (Priorität nach Case-Zahl)

| Suite / Feature | ~Fails | Klasse | Anmerkung |
|---|---|---|---|
| `1117-…-date-and-time` | 35 | Engine | `date and time`-Konstruktor-Kombinationen & Offsets. |
| `0082-feel-coercion` | 28 | Engine | Singleton-Listen↔Wert-Koerzierung an Ausdrucksgrenzen. |
| `0070-feel-instance-of` | 25 | Typsystem | `instance of` für `list<T>`, `function`, benutzerdef. Typen (`range<T>` erledigt). |
| `0068-feel-equality` | 23 | Engine | Cross-Typ-Gleichheit, null-Fälle, Kontext-/Listen-Tiefvergleich. |
| `1111-feel-matches` | 21 | Engine | `matches`/Flags-Semantik (XPath-Regex-Details). |
| `0072-feel-in` | 21 | Engine | Rest-Randfälle des `in`-Operators. |
| `0084-feel-for-loops` | 21 | Engine | `for … return`-Randfälle (partielle Ergebnisse, verschachtelte Domains). |
| `1156-range-function` | 19 | Engine | `instance of range<T>`-Feindiskriminierung + String-Endpunkt-Randfälle. |
| `0074-feel-properties` | 17 | Engine | Property-Zugriff auf Temporale/Ranges. |
| `1155-list-replace` | 16 | Engine | Rest-Randfälle von `list replace`. |
| `0007`/`1116` date/time | ~28 | Engine | Temporale Konstruktor- & Property-Details. |
| `0069-feel-list` | 15 | Engine | Listen-Randfälle. |
| `0085`/`0034` decision services / DRG scopes | ~23 | Engine | Decision-Service-Invocation als FEEL-Funktion. |
| `0100-arithmetic` (Rest) | 5 | Engine | Dauer-Rundung (Tie-Richtung) + `**`-Assoziativität/`-x**y`-Präzedenz + Exponent-Präzision. |

### Bewusst nicht anwendbar (dokumentierte Ausnahmen)

- **`0076-feel-external-java`** (~18 Cases) — ruft externe **Java**-Funktionen über
  die DMN-`java`-Extension auf. Temis ist eine reine Go-Engine ohne JVM; dieses
  Feature ist **kein Ziel** (Sandbox-/Sicherheitsgrenze, ADR-0008-Geist). Diese
  Cases zählen nicht zu den „anwendbaren" Cases der 95-%-Quote.

## Vorgehen zur 95-%-Quote

1. Temporale Detailfälle (`1117`/`0007`/`1116`, ~63) — Konstruktoren, Properties, Offsets.
2. Koerzierung/Gleichheit/`instance of` (~76) — FEEL-Typsemantik an den Grenzen (`list<T>`, `function`).
3. `matches`-Flags (21), `for`-Randfälle (21), Properties (17), Rest-`feel-in`/`range` (~40).
4. Arithmetik-Rest (5) — Dauer-Rundung, `**`-Assoziativität/`-x**y`-Präzedenz, Exponent-Präzision.

Jeder Schritt hebt `conformanceFloor` in `internal/tck/conformance_test.go` an.
