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
| Compliance Level 2 + 3 | **3130 / 3495 Cases grün (89,6 %)** |
| Suites | 146 (0 laden fehlerhaft) |
| Ratchet-Floor im CI | 89,5 % |

Das WP-41-Ziel ist **≥ 95 % der anwendbaren Cases**. Der Weg dahin ist als
Kategorien unten dokumentiert; der Floor wird mit jedem Fix angehoben, sodass
Regressionen den Gate brechen.

> **Wichtiger Runner-Fix (frühere Etappe):** Der TCK-Runner bewertet **pro
> Case** die Ziel-Decision, statt eine ganze Suite abzubrechen, sobald **irgendeine**
> Decision im Modell einen Compile-Fehler hat. Das ist die korrekte TCK-Semantik und
> hat die real messbare Case-Zahl von 480 auf 3495 gehoben.

## In dieser Etappe behoben — `in`-Operator & `abs` (WP-41.4, +20 Cases)

- **`X in (= Y)` / `X in (!= Y)`** — ein **parenthesierter** Operator-Unary-Test
  (ohne Komma) parst jetzt (`isInTestList` erkennt einen führenden Vergleichs-
  operator nach `(`). Zuvor scheiterte `10 in (=10)` am Parsen (0072, 16 Fälle über
  alle Typen).
- **`abs(duration)`** — `abs` liefert jetzt auch für beide Dauer-Typen den Betrag
  (`abs(duration("-P1D"))` → `P1DT0H0M0S`), nicht nur für Zahlen (0050).

Netto **+20 Cases** (89,0 % → 89,6 %); 0072 21→5, 0050 7→0.

## Früher behoben — Property-Zugriff auf Temporale & Ranges (WP-41.3, 0074: 14 → 5)

FEEL-Member-Namen dürfen **Leerzeichen** enthalten (`time offset`, `start included`);
der Parser las nach `.` bisher nur ein Wort → `date and time(…).time offset` und
`[1..10].start included` scheiterten am Parsen. Der Parser assembliert jetzt den
Namens-Lauf (Keywords wie `and`/`in` sind eigene Token-Kinds und stoppen ihn
korrekt). Zusätzlich exponiert `value.Member` nun **Range**-Properties (`start`,
`end`, `start included`, `end included`) — die temporalen Accessoren (`time offset`,
`timezone`, Duration-Felder) existierten bereits.

Netto **+9 Cases** (88,7 % → 89,0 %). Rest in `0074` sind Range-**Literale** aus
Vergleichen (`(<10)`, `(>=10)`) — eigener Parser-Mechanismus, separates WP.

## Früher behoben — TCK-Runner: item-verpackte Listen (WP-41.2, +108 Cases)

Der Runner dekodierte erwartete Listen bisher nur in der Form
`<list><value>…</value></list>`. Das offizielle Korpus verwendet aber breit auch
`<list><item><value>…</value></item></list>` (inkl. verschachtelter Listen und
Kontext-Items). Diese item-verpackten Listen wurden als **leer** gelesen → viele
korrekte Engine-Ergebnisse zählten fälschlich als Fehlschlag.

`tckList` akzeptiert jetzt beide Kodierungen (`internal/tck/case.go`). **Reiner
Harness-Fix — keine Engine-Änderung:** +108 Cases (85,6 % → 88,7 %), quer über die
Listen-Suiten (0008/0009 je 10→0, 0012 12→2) und alle Suiten mit Listen-Ergebnissen.

## Früher behoben — FEEL-Invocation-Fehlersemantik (WP-41.1, +123 Cases)

Ein **syntaktisch gültiger** Funktionsaufruf mit **semantischem** Fehler — falsche
Argument-Anzahl, unbekannter oder mit Positional gemischter benannter Parameter —
ergibt jetzt zur Laufzeit **`null`** und lässt die Decision **ausführbar**, statt sie
als „nicht ausführbar" abzubrechen. Das ist FEEL's Total-Funktions-Semantik und die
korrekte TCK-Erwartung (`round up()`, `modulo(4)`, `floor(n:1.5, scal:1)` → null).

Umgesetzt an einer Naht im FEEL-Compiler (`bindArgs`/`bindNamedArgs` in
`internal/feel/compile.go`): diese Arity-/Named-Parameter-Fehler kompilieren zu
`null` (`c.nullCall`), ohne den fatalen Compile-Fehler zu setzen. **Echte** Fehler
(unbekannte Funktion, Nicht-Funktions-Callee, Syntaxfehler) bleiben unverändert
nicht ausführbar.

Netto **+123 Cases** (82,1 % → 85,6 %) — der größte Einzelhebel, quer über nahezu
alle Funktions-Suiten (allein die „error case"-Tests jeder Builtin-Suite).

## Früher behoben — Typ-Koerzierung am Decision-Output (0082: 28 → 13 Fails)

FEEL-Item-Definition-Koerzierung (DMN §10.3.2.9.4) an der Decision-Ausgabe: das
Ergebnis wird jetzt an den deklarierten `typeRef` der Decision-Variable angepasst,
bevor es zurückgegeben und nachgelagerten Decisions als Variablenwert zugewiesen wird
(`dmn/coerce.go`, angewandt in `eval.go`):

- **Singleton-Liste ↔ Skalar**: `["foo"]` bei Ziel `string` → `"foo"`; `[10]` bei
  Ziel `number` → `10`.
- **Typ-Konformität sonst → null**: ein Wert, der (nach etwaigem Entpacken) nicht
  zum deklarierten Typ passt, wird `null` (`2` bei Ziel `string`, `[1 2 foo]` bei
  Ziel `string`, Kontext bei Ziel Skalar). `null` ist Mitglied jedes Typs; `Any`
  (kein `typeRef`) erzwingt nichts.
- Listen/Kontexte werden element- bzw. feldweise gegen deklarierte Element-/Feld-
  typen geprüft.

Netto **+16 Cases** (81,7 % → 82,1 %). Die verbleibenden `0082`-Fälle liegen an
BKM-/Invocation-/Decision-Service-Grenzen (eigene Auswertungspfade, Follow-up).

## Früher behoben — Strikte Temporal-Lexik (1115/1116/1117: −15 Fails)

Die Konstruktoren (`date`/`time`/`date and time`) und `@"…"`-Literale weisen jetzt
lexikalisch **malformte** Strings korrekt als **null** ab, statt sie tolerant zu
akzeptieren (gated an den `Parse*`-Einstiegspunkten in `internal/value/temporal.go`):

- **Jahr**: nur führendes `-` (kein `+`); genau 4 Ziffern dürfen mit `0` beginnen,
  5+ Ziffern nicht; Betrag ≤ `999999999`. Verworfen: `998`, `01211`, `+2012`,
  `9999999999`, `+99999`; die numerische `date(y,m,d)`-Form prüft die Jahresgrenze
  ebenfalls (`date(-1000999999,…)` → null).
- **Zeit**: feste Feldbreiten — einstellige Stunde `T7:00:00` → null.
- **Offset**: über ±18:00 hinaus ungültig (`+19:00`/`-19:00` → null); reale Zonen
  (≤ ±14:00) bleiben gültig.

Netto **+15 Cases** (81,2 % → 81,7 %) über `1115` (−5), `1116` (−3), `1117` (−7).

## Früher behoben — `date and time`-Konstruktor & Rendering (1117: 35 → 10 Fails)

- **Zwei-Argument-Konstruktor `date and time(date, time)`** akzeptiert als erstes
  Argument nun auch ein `date and time` (dessen Datums-Teil genutzt wird), nicht nur
  ein `date` (1117, ~21 Fails).
- **`date and time("2012-12-24")`** — ein date-only-String promoviert zum Tagesbeginn
  (`2012-12-24T00:00:00`).
- **Sekundenbruchteile** überleben Parse **und** Rendering (`…:30.987@Europe/Paris`);
  ganze Sekunden lassen den Bruchteil weiterhin weg.
- **Jahre mit 1–9 Ziffern** (bis zur FEEL-Grenze `999999999`) parsen jetzt —
  `parseSignedTime` löst das Jahr über einen Platzhalter vom Referenz-Layout, das
  genau vier Ziffern konsumiert.

Netto **+32 Cases** (80,3 % → 81,2 %); hebt neben `1117` auch `1116` (13→9) an.

## Früher behoben — Arithmetik & Temporal (0100: 96 → 5 Fails)

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
| `0070-feel-instance-of` | 25 | Typsystem | `instance of` für `list<T>`, `function`, benutzerdef. Typen (`range<T>` erledigt). |
| `0068-feel-equality` | 23 | Engine | Cross-Typ-Gleichheit, null-Fälle, Kontext-/Listen-Tiefvergleich. |
| `1111-feel-matches` | 21 | Engine | `matches`/Flags-Semantik (XPath-Regex-Details). |
| `0072-feel-in` | 21 | Engine | Rest-Randfälle des `in`-Operators. |
| `0084-feel-for-loops` | 21 | Engine | `for … return`-Randfälle (partielle Ergebnisse, verschachtelte Domains). |
| `1156-range-function` | 19 | Engine | `instance of range<T>`-Feindiskriminierung + String-Endpunkt-Randfälle. |
| `0074-feel-properties` | 17 | Engine | Property-Zugriff auf Temporale/Ranges. |
| `1155-list-replace` | 16 | Engine | Rest-Randfälle von `list replace`. |
| `0007`-date-time | 15 | Engine | Temporale Konstruktor- & Property-Details. |
| `0069-feel-list` | 15 | Engine | Listen-Randfälle. |
| `0082-feel-coercion` (Rest) | 13 | Engine | Koerzierung an BKM-/Invocation-/Decision-Service-Grenzen (eigene Auswertungspfade). |
| `0085`/`0034` decision services / DRG scopes | ~23 | Engine | Decision-Service-Invocation als FEEL-Funktion. |
| `date and time`-Named-Params | 2 | Compiler | `date and time(date: …, time: …)` — 2-Arg-Signatur braucht Parameternamen (1117 087/088). |
| `0100-arithmetic` (Rest) | 5 | Engine | Dauer-Rundung (Tie-Richtung) + `**`-Assoziativität/`-x**y`-Präzedenz + Exponent-Präzision. |

### Bewusst nicht anwendbar (dokumentierte Ausnahmen)

- **`0076-feel-external-java`** (~18 Cases) — ruft externe **Java**-Funktionen über
  die DMN-`java`-Extension auf. Temis ist eine reine Go-Engine ohne JVM; dieses
  Feature ist **kein Ziel** (Sandbox-/Sicherheitsgrenze, ADR-0008-Geist). Diese
  Cases zählen nicht zu den „anwendbaren" Cases der 95-%-Quote.

## Vorgehen zur 95-%-Quote

1. Gleichheit/`instance of` (~48) — FEEL-Typsemantik an den Grenzen (`list<T>`, `function`).
2. Koerzierung an BKM-/Invocation-/Service-Grenzen (`0082`-Rest, 13) — dieselbe Regel wie am Decision-Output, an weiteren Auswertungspfaden.
3. `matches`-Flags (21), `for`-Randfälle (21), Properties (17), Rest-`feel-in`/`range` (~40).
4. `0007`-date-time (15) + `date and time`-Named-Params (2); Arithmetik-Rest (5).

Jeder Schritt hebt `conformanceFloor` in `internal/tck/conformance_test.go` an.
