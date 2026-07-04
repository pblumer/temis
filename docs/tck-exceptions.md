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
| Compliance Level 2 + 3 | **3272 / 3495 Cases grün (93,6 %)** |
| Suites | 146 (0 laden fehlerhaft) |
| Ratchet-Floor im CI | 93,6 % |

Das WP-41-Ziel ist **≥ 95 % der anwendbaren Cases**. Der Weg dahin ist als
Kategorien unten dokumentiert; der Floor wird mit jedem Fix angehoben, sodass
Regressionen den Gate brechen.

> **Wichtiger Runner-Fix (frühere Etappe):** Der TCK-Runner bewertet **pro
> Case** die Ziel-Decision, statt eine ganze Suite abzubrechen, sobald **irgendeine**
> Decision im Modell einen Compile-Fehler hat. Das ist die korrekte TCK-Semantik und
> hat die real messbare Case-Zahl von 480 auf 3495 gehoben.

## In dieser Etappe behoben — `in`/Range mit null-Endpunkten (WP-41.16, +9)

- **`in` als 3-wertige Disjunktion**: Ein null-Testwert gegen eine Range oder ein
  **expliziter null-Endpunkt** macht den Membership-Test **null** (nicht `false`):
  `null in [1..10]`, `5 in [null..10]`, `5 in [1..null)` → `null`. Ein **weggelassener**
  (unbounded) Endpunkt bleibt davon unberührt (`5 in (< 10)` → `true`). 0072: 5→0.
- **Range-Gleichheit unterscheidet unbounded ↔ expliziten null-Endpunkt**: `(< 10)`
  (Go-nil-Grenze) ist **nicht** gleich `(null..10)` (explizite null-Grenze). 0068
  range_006–009: 4→0.

Die Unterscheidung nutzt, dass ein weggelassener Endpunkt als Go-`nil` gespeichert
wird, ein explizites `null` dagegen als `value.Null`.

Netto **+9 Cases** (93,4 % → 93,6 %). Rest in 0068: Sekunden-Auflösung der Temporal-
Gleichheit (`time_005`) und Operator-Punkt-Range (`(=10)`).

## Früher behoben — Bindestrich-Namen & fraktionale `time`-Sekunden (WP-41.15, +43)

- **FEEL-Namen mit Bindestrich** (`Date-Time`, `Pre-bureauRiskCategory`, …): Der
  Parser assembliert eine Name-Referenz jetzt über einen `-` hinweg zu **einem**
  Namen, sobald das Namens-Orakel diesen kennt — statt `a - b` (Subtraktion) zu
  lesen. Dazu fließen die **Umgebungs-Variablennamen** einer Decision ins Orakel
  ein (`nameOracleWithEnv`). Ein bloßes `a - b` ohne gleichnamige Variable bleibt
  eine Subtraktion (Disambiguierung über die bekannte Namensmenge, DMN §10.3.1.2).
  Das war die Ursache ganzer Kaskaden: 0007 (Modell nutzt `Date-Time`) 15→0,
  0004-lending (`Pre-/Post-bureau…`) 7→0, 0087 7→0 u. a.
- **`time(h, m, s, offset?)` mit fraktionaler Sekunde**: Die Sekunden-Komponente
  darf einen Bruchteil tragen (`time(12,59,1.3,-PT1H)` → `12:59:01.3-01:00`);
  `Number.SecondsNanos` teilt sie in ganze Sekunden + Nanosekunden. 0007: letzter
  Rest → 0.

Netto **+43 Cases** (92,1 % → 93,4 %).

## Früher behoben — Kontext-Eintrags-Referenzen & string join (WP-41.14, +4)

- **Kontext-Einträge referenzieren frühere Einträge** (FEEL-Kontext-Semantik):
  `{a: 1+2, b: a+3}` → `{a:3, b:6}`. `compileContext` baut die Umgebung inkrementell
  auf und bindet jeden ausgewerteten Wert in den Scope der nachfolgenden Einträge.
- **`string join(null)`** (null-Liste) ergibt `null` statt `""` (1140).

Netto **+4 Cases** (92,0 % → 92,1 %); 0057: 4→2.

## Früher behoben — FEEL-Kommentare (WP-41.13, +3)

Der Lexer überspringt jetzt FEEL-Kommentare: `// …` bis Zeilenende und `/* … */`
Block-Kommentare (`1 + /* 1 + */ 1` → 2). 0073: 3→0.

Netto **+3 Cases** (91,9 % → 92,0 %).

## Früher behoben — `for`/Quantifier über Ranges (WP-41.12, +10)

`for i in a..b` (und `some`/`every`) enumeriert jetzt neben **Zahlen-Ranges** auch
**Date-Ranges** tageweise (`for i in @"1980-01-01"..@"1980-01-03"` → die drei Tage,
auf- und absteigend). Ranges anderer Typen (String, date-and-time, time, Dauer,
unbounded) sind **nicht iterierbar** → das Comprehension-Ergebnis ist **null**
(zuvor eine leere Liste). 0084: 13→3, 0016: 5→2.

Netto **+10 Cases** (91,6 % → 91,9 %).

## Früher behoben — Unicode-String-Escapes (WP-41.11, +7)

Der String-Lexer dekodiert jetzt **`\U`** (6-Hex-Codepoint, `\U01F40E` → 🐎) und
kombiniert **UTF-16-Surrogatpaare** `\uD83D\uDCA9` zu einem Codepoint (💩). Damit
zählt `string length` Codepoints korrekt und `=` vergleicht Emoji-Strings. 0083: 9→2.

Netto **+7 Cases** (91,4 % → 91,6 %). Rest in 0083: Emoji in Kontext-Keys
(Namens-Lexer).

## Früher behoben — `is()` auf Temporalen (WP-41.10, +9)

`is(v1, v2)` prüft Wert- **und** Typgleichheit. Für `date`/`time`/`date and time`
vergleicht es jetzt die **Repräsentation** statt des Instants: `is(@"23:00:50",
@"23:00:50Z")` und `is(@"20:00:50+00:00", @"21:00:50+01:00")` → **false** (gleicher
Instant, andere Darstellung). Zahlen/Listen/Kontexte bleiben unverändert (Wert-
gleichheit). 0103: 11→2.

Netto **+9 Cases** (91,2 % → 91,4 %).

## Früher behoben — `range()`-Validierung (WP-41.9, +12)

Die `range(from)`-Builtin weist jetzt malformte Range-Strings korrekt als **null** ab
(`validRangeBounds`): ein **unbounded** Endpunkt mit **geschlossener** Klammer
(`[1..]`, `[..2]`), **Typ-Mismatch** der Endpunkte (`[1.."b"]`, date vs date-and-time)
und **umgekehrte** Grenzen (`[3..1]`, `["z".."a"]`, reversed Temporale). 1156: 16→4.

Netto **+12 Cases** (90,8 % → 91,2 %).

## Früher behoben — Range-Literale aus Vergleichen (WP-41.8, +7)

`(< v)`, `(<= v)`, `(> v)`, `(>= v)`, `(= v)` parsen jetzt als **halb-/geschlossene
Range-Literale**: `(<10)` → `(..10)` (unbounded low), `(>=10)` → `[10..)`, `(=10)` →
`[10..10]`. Umgesetzt in `parseParenOrInterval`; `compileInterval` erzeugt für einen
fehlenden Endpunkt eine unbounded Range-Grenze (nil). Cross-cutting über
`0074` (5→0, komplett grün), `0068` (10→8) und Range-Membership (`5 in (>3)`).

Netto **+7 Cases** (90,6 % → 90,8 %). `!=` hat keine Ein-Range-Bedeutung und bleibt
außen vor.

## Früher behoben — Cross-Typ-Gleichheit → null (WP-41.7, +12)

`=` und `!=` zwischen zwei **nicht-null**-Werten **unterschiedlichen Typs** ergeben
jetzt **`null`** statt `false` (DMN §10.3.2.7): `100 = "100"`, `[] = 0`, `{} = []`,
`duration("P1Y") = duration("P365D")` → null. Chirurgisch nur an den `=`/`!=`-
**Operatoren** (`feelEqualOp`/`notBool` in `internal/feel/compile.go`) — das interne
`value.Equal`-Prädikat behält seinen booleschen Rückgabewert für Decision-Table-
Matching, `in` und `list contains`.

Netto **+12 Cases** (90,3 % → 90,6 %); 0068 von 22 auf 10. Rest: Range-Literale aus
Vergleichen (`(<10) = …`) — eigener Parser-Mechanismus.

## Früher behoben — `instance of` Funktionstypen (WP-41.6, +10)

Der Parser akzeptiert jetzt **Funktionstyp-Ausdrücke** `function<P, …> -> ReturnType`
in `instance of` (`function` ist ein Keyword-Token, das `parseTypeName` bisher
ablehnte); Parameterliste und Rückgabetyp werden konsumiert, aber verworfen —
`instance of` matcht nur auf die Funktions-**Art**. `BuiltinType` kennt jetzt
`function` (Kind `function`).

Netto **+10 Cases** (90,0 % → 90,3 %); 0070 von 25 auf 15. Rest in 0070:
benutzerdefinierte Typen (`t255`, braucht Item-Definition-Auflösung, cross-layer)
und generische Feld-Diskriminierung (`context<{a:number}>`) — eigenes WP.

## Früher behoben — Collection-Funktionen (WP-41.5, +16 → **90,0 %** 🎉)

Drei Collection-Builtins vervollständigt:
- **`context put(ctx, path, value)`** mit **Pfad-Liste** — verschachteltes Update:
  `context put({x:1, y:{a:0}}, ["y","a"], 2)` → `{x:1, y:{a:2}}` (1146).
- **`context(entries)`** — akzeptiert einen **einzelnen** Entry unverpackt und liefert
  bei **Duplikat-Keys** `null` (1145).
- **`list replace`** — Singleton-Koerzierung des Listen-Arguments, nicht-ganzzahlige
  Position truncatet Richtung null, Match-Funktion mit Arity ≠ 2 oder Nicht-Boolean-
  Ergebnis → `null` (1155).

Netto **+16 Cases** (89,6 % → **90,0 %**) — die 90-%-Marke ist erreicht.

## Früher behoben — `in`-Operator & `abs` (WP-41.4, +20 Cases)

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
