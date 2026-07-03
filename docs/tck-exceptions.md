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
| Compliance Level 2 + 3 | **2704 / 3495 Cases grün (77,4 %)** |
| Suites | 146 (0 laden fehlerhaft) |
| Ratchet-Floor im CI | 77,0 % |

Das WP-41-Ziel ist **≥ 95 % der anwendbaren Cases**. Der Weg dahin ist als
Kategorien unten dokumentiert; der Floor wird mit jedem Fix angehoben, sodass
Regressionen den Gate brechen.

> **Wichtiger Runner-Fix (diese Etappe):** Der TCK-Runner bewertet jetzt **pro
> Case** die Ziel-Decision, statt eine ganze Suite abzubrechen, sobald **irgendeine**
> Decision im Modell einen Compile-Fehler hat. Das ist die korrekte TCK-Semantik und
> hat die real messbare Case-Zahl von 480 auf 3495 gehoben.

## In dieser Etappe behoben

- **`in`-Operator vollständig** (0072, 224→21 Fails): die RHS ist jetzt eine echte
  positive-Unary-Test-Liste — operator-präfixierte Tests (`x in <= 10`, `x in > 10`),
  explizite Gleichheit (`x in = 10`), Komma-Test-Listen (`x in (1, < 5, >= 10)`) und
  Listen-Mitgliedschaft inkl. Range-Elementen (`1 in [[2..4],[1..3]]`).
- **`is(v1, v2)`** (Builtin) — Wert- **und** Typgleichheit (0103, war 0/50).
- **`list replace(list, position|match, newItem)`** (Builtin, Positions- und
  Funktions-Match-Form) (1155).
- **`number(from, grouping, decimal)`** — mehrargumentige Form mit
  Trenner-Normalisierung (0058).
- **`range(from)`-Builtin** (1156, 56→19 Fails): Laufzeit-Parsing von Range-Strings
  (`[1..3]`, `(18..21]`, `]18..21[`, unbeschränkte Enden `[1..]`/`[..2]`, String- und
  `@`-Temporal-Endpunkte) + `instance of range<T>` (Range-Typ).

## Offene Kategorien (Priorität nach Case-Zahl)

| Suite / Feature | ~Fails | Klasse | Anmerkung |
|---|---|---|---|
| `0100-arithmetic` | 96 | Engine | 91 % grün; Rest sind numerische Rand-/Nullfälle & Exponenten-Grenzen. |
| `1117-…-date-and-time` | 35 | Engine | `date and time`-Konstruktor-Kombinationen & Offsets. |
| `0082-feel-coercion` | 28 | Engine | Singleton-Listen↔Wert-Koerzierung an Ausdrucksgrenzen. |
| `0070-feel-instance-of` | 25 | Typsystem | `instance of` für `list<T>`, `function`, benutzerdef. Typen (`range<T>` erledigt). |
| `1156-range-function` | 19 | Engine | Rest: `instance of range<T>`-Feindiskriminierung + String-Endpunkt-Randfälle. |
| `0068-feel-equality` | 23 | Engine | Cross-Typ-Gleichheit, null-Fälle, Kontext-/Listen-Tiefvergleich. |
| `0084-feel-for-loops` | 21 | Engine | `for … return`-Randfälle (partielle Ergebnisse, verschachtelte Domains). |
| `1111-feel-matches` | 21 | Engine | `matches`/Flags-Semantik (XPath-Regex-Details). |
| `0074-feel-properties` | 17 | Engine | Property-Zugriff auf Temporale/Ranges. |
| `0007`/`1115`/`1116` date/time | ~36 | Engine | Temporale Konstruktor- & Property-Details. |
| `0085`/`0034` decision services / DRG scopes | ~23 | Engine | Decision-Service-Invocation als FEEL-Funktion. |

### Bewusst nicht anwendbar (dokumentierte Ausnahmen)

- **`0076-feel-external-java`** (~18 Cases) — ruft externe **Java**-Funktionen über
  die DMN-`java`-Extension auf. Temis ist eine reine Go-Engine ohne JVM; dieses
  Feature ist **kein Ziel** (Sandbox-/Sicherheitsgrenze, ADR-0008-Geist). Diese
  Cases zählen nicht zu den „anwendbaren" Cases der 95-%-Quote.

## Vorgehen zur 95-%-Quote

1. Arithmetik-Randfälle (96) — überwiegend Grenz-/Nullverhalten & Exponenten.
2. Koerzierung/Gleichheit/`instance of` (~76) — FEEL-Typsemantik an den Grenzen (`list<T>`, `function`).
3. Temporale Detailfälle (~70) — Konstruktoren, Properties, Offsets.
4. `matches`-Flags (21), `for`-Randfälle (21), Properties (17), Rest-`feel-in`/`range` (~40).

Jeder Schritt hebt `conformanceFloor` in `internal/tck/conformance_test.go` an.
