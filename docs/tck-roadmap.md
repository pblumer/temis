# DMN-TCK-Konformität — Umsetzungsplan (WP-41.x)

> Teilpakete unter dem Umbrella **WP-41** (`docs/20-roadmap.md`). Referenziert
> `docs/tck-exceptions.md` (aktueller Stand, offene Kategorien, Ausnahmen).

**Stand:** 3373 / 3495 Cases (**96,5 %**) nach WP-41.22, Ratchet-Floor 96,5 %.
**Endziel (WP-41): ≥ 95 % — ✅ erreicht.** Weitere Fixes heben den Ratchet-Floor.

Jedes Teilpaket ist ein eigener, getesteter PR mit Floor-Anhebung. Die Fehler
sind langschwänzig über ~100 Suiten verteilt; die Reihenfolge priorisiert
Ertrag/Risiko: risikoarme, ertragreiche Funktions-Familien zuerst.

## Weg zu > 90 %

| WP | Thema | Suiten (Auswahl) | ~Cases |
|---|---|---|---|
| **WP-41.1 ✅** | **FEEL-Invocation-Fehlersemantik → null** (falsche Arity / unbekannte·gemischte Named-Params ergeben `null` statt „nicht ausführbar"); quer über **alle** Funktions-Suiten | breit (1141–1144, 0056, 1101/1102, 0050, 1145/1146, …) | **+123** |
| **WP-41.2 ✅** | **TCK-Runner: item-verpackte Listen dekodieren** (`<list><item>…`); reiner Harness-Fix | quer (0008/0009/0012, alle Listen-Ergebnisse) | **+108** |
| **WP-41.2b** | FEEL-Zahl: kanonische Präzision der Ausgabe | quer (0012, 0100, Statistik) | ~20–40 |
| **WP-41.3 ✅** | **Property-Zugriff auf Temporale & Ranges** (Mehrwort-Member-Namen; Range-`start`/`end`/`…included`) | 0074 | **+9** |
| **WP-41.4 ✅** | **`in (=Y)`/`(!=Y)` parenthesiert + `abs(duration)`** | 0072, 0050 | **+20** |
| **WP-41.4b** | Math-Builtins (Überladungen + null/leer + Rundungs-Werte) | 0050 abs, 0052 exp, 0053 log, 0051 sqrt, 0062 mode, 0063 stddev, 0061 median, 0094 product, 0054/0055, 0058, 0075, 1141–1144/1100–1102 Rest | ~70 |
| **WP-41.5** | Listen-Funktionen | 0069, 0012, 0009 append/flatten-Rest, 0059/0060 all/any, 0011 insert/remove, 0010 concatenate, 0021 singleton | ~50 |
| **WP-41.5 ✅** | **Collection-Funktionen** (`context put`-Pfad, `context()`-Edges, `list replace`) | 1146, 1145, 1155 | **+16 → 90,0 %** |
| **WP-41.6** | Kontext-Rest + get value/entries, merge | 1147, 0080, 0081, 0057 | ~18 |
| **WP-41.7** | String- & Unicode-Funktionen | 0083 unicode, 1140 string join, 1109 replace, 1103 substring, 0067 split, 1105/1106 upper/lower | ~29 |
| **WP-41.8** | Koerzierung an BKM/Invocation/Decision-Service-Grenzen | 0082-Rest, 1131, 0005, 0009-invoc, 0030/0031 | ~30 |
| **WP-41.9** | Temporal-Rest | 0007, 1120/1121 duration, 0095–0098 date-parts, 0093 at-literals, 1116/1117-Rest | ~52 |
| **WP-41.15 ✅** | **Bindestrich-Namen** (`Date-Time`, `Pre-/Post-bureau…`; Orakel + Env-Namen) **+ fraktionale `time`-Sekunden** | 0007, 0004, 0087, quer | **+43 → 93,4 %** |
| **WP-41.16 ✅** | **`in`/Range mit null-Endpunkten** (3-wertiges `in`; Range-Gleichheit unbounded ↔ null) | 0072, 0068 | **+9 → 93,6 %** |
| **WP-41.17 ✅** | **Invocation-Null + Zahl-Wort-Namen + `defaultOutputEntry`** | 1131, 0020, 0034, quer | **+30 → 94,5 %** |
| **WP-41.18 ✅** | **`number()`-Validierung, `range()`-Konstruktoren, Regex `$N`/`x`-Flag, Escape-Durchreichung** | 0058, 1156, 1109, 1111 | **+21 → 95,1 % 🎯** |
| **WP-41.19 ✅** | **Rundungs-Skala-Bereich, `**`-Präzedenz (links-assoz. + unär), Time-Offset-Sekunden + `time(date)`** | 1141–1144, 0100, 1116 | **+19 → 95,6 %** |
| **WP-41.20 ✅** | **Decision Services als aufrufbare FEEL-Funktionen** (`feel.Func.Native`; Params = InputData ++ InputDecisions) | 0085 | **+5 → 95,8 %** |
| **WP-41.21 ✅** | **Typ-Koerzierung an Aufruf-Grenzen** (BKM-/Service-Argumente & -Rückgabe; geteilte `ConformsToType`/`CoerceToType`) | 0082, 0085 | **+10 → 96,1 %** |
| **WP-41.22 ✅** | **Zahl-Vergleich mit TCK-Präzision** (Runner rundet Ist-Ergebnis auf Erwartungswert-Stellen; decimal128 vs. gerundete Oracle-Werte) | 0052, 0009, 0008, 0063, 0041 | **+16 → 96,5 %** |

Bündel 41.4–41.9 adressieren ~330 Cases → **komfortabel über 90 %**, ohne die
schwierigen Brocken unten.

## Weg zu ≥ 95 % (danach)

| WP | Thema | Suiten | ~Cases |
|---|---|---|---|
| **WP-41.9** | Typsystem: `instance of` generics, Cross-Typ-Gleichheit, `is` | 0070, 0068, 0103 | ~59 |
| **WP-41.10** | `matches`/`replace` (XPath-Regex-Semantik) | 1111, 1109 | ~25 |
| **WP-41.11** | `in` + `range`-Rest | 0072, 1156 | ~40 |
| **WP-41.12** | Decision Services / DRG-Scopes | 0085, 0034, 0036, 0035, 0037 | ~36 |
| **WP-41.13** | Iteration/`for`, Boxed-Expr., Hit-Policies, `list replace`-Rest | 0084, 0016, 1150–1161, 0109–0119, 1155 | ~70 |

## Bewusst nicht anwendbar

- **`0076-feel-external-java`** (~18 Cases) — externe Java-Funktionen über die JVM.
  Reine Go-Engine ohne JVM (ADR-0008-Geist); zählt nicht zu den anwendbaren Cases.

## Arbeitsweise je WP

1. Ziel-Suite(n) diagnostizieren (Fail-Cluster + Ursache).
2. Fix in der Engine, mit Offline-Unit-Tests für jeden Pfad.
3. Voller Korpus-Lauf (Regressions-Check) + `go test ./...`.
4. `conformanceFloor` anheben, `docs/tck-exceptions.md` fortschreiben, CHANGELOG.
5. Commit, Push, PR, CI (`tck`-Lane) grün, Merge.
