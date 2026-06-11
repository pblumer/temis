# FEEL-Implementierungsplan

> FEEL (Friendly Enough Expression Language) ist der aufwändigste Teil. Dieses Dokument
> ist der Bauplan für `internal/feel`. Quelle der Wahrheit ist die DMN-1.5-Spezifikation,
> Kapitel zu FEEL (Grammatik in EBNF, Semantik, Built-in-Funktionstabellen). Bei jedem
> Zweifel: **Spec + TCK schlagen Intuition.**

## 1. Pipeline innerhalb FEEL

```
source → [lexer] → tokens → [parser] → AST → [typecheck] → typed AST → [compile] → CompiledExpr
```

Jede Stufe ist ein eigenes, isoliert testbares Paket-Stück.

## 2. Lexer (`token.go`, `lexer.go`)

Besonderheiten, die ein Agent leicht übersieht:
- **Namen mit Leerzeichen**: FEEL erlaubt mehrteilige Bezeichner wie `Applicant Age`.
  Der Lexer kann Namen **nicht** allein entscheiden — Namenserkennung ist kontextsensitiv
  und gehört teilweise in den Parser (longest-match gegen bekannte Namen im Scope).
  Strategie: Lexer liefert Name-Fragmente + Schlüsselwörter; Parser fügt Namen zusammen.
- **Datums-/Zeitliterale**: `date("2024-01-01")`, `@"2024-01-01"`, `time(...)`,
  `duration("P1D")`, `date and time(...)`.
- **Zahlen**: Dezimal, optional Exponent; **kein** Hex/Oktal. Punkt als Dezimaltrenner.
- **Strings**: doppelte Anführungszeichen, Escapes inkl. Unicode `\uXXXX`.

## 3. Parser (`ast.go`, `parser.go`)

Grammatik = EBNF aus der Spec. Wichtige Konstrukte:
- Arithmetik mit korrekter Präzedenz: `**` (Potenz) > unär `-` > `* /` > `+ -`.
- Vergleiche, `between x and y`, `in`-Tests.
- `if c then a else b` (else ist Pflicht in FEEL).
- Iteration: `for i in L1, j in L2 return expr` (kartesisch, geordnet).
- Quantoren: `some i in L satisfies cond`, `every i in L satisfies cond`.
- Pfadnavigation: `a.b.c`; Filter: `L[cond]`, `L[index]`.
- Funktionsaufruf: positional `f(1,2)` und named `f(a:1, b:2)`.
- Literale: Liste `[...]`, Kontext `{ k: v, ... }`, Range `[1..10]`, `(1..10]`, `]1..10[`.
- Funktionsdefinition: `function(a, b) external? body`.

**Output:** AST-Knoten als Go-Structs, jeweils mit Quellposition (Zeile/Spalte) für
Diagnostics.

## 4. Typsystem (`types.go`, `typecheck.go`)

FEEL-Typen (Built-in):
`Null, Boolean, Number, String, Date, Time, Date-Time, Days-and-Time-Duration,
Years-and-Months-Duration, List<T>, Context, Range<T>, Function, Any`.

- FEEL ist **teilweise** statisch typisierbar. Wo Item Definitions vorhanden sind
  (typisierte Inputs), statisch prüfen; sonst `Any` und Laufzeitprüfung.
- **`null`-Semantik**: Fast alle Built-ins und Operatoren propagieren `null` bei
  ungültiger Eingabe statt zu fehlern. Das exakt nachzubilden ist TCK-kritisch.
- Typ-Coercion-Regeln (z. B. Singleton-Liste ↔ Element) gemäß Spec implementieren.

## 5. Werte (`value.go`)

| FEEL-Typ | Go-Repräsentation | Hinweis |
|---|---|---|
| Number | Decimal (`apd.Decimal`) | **nie** float64; 34-stellige Präzision laut Spec |
| String | `string` | UTF-8 |
| Boolean | `bool` |  |
| Date / Time / Date-Time | `time.Time`-basiert + Flags | Zeitzonen & „local vs offset" beachten |
| Durations | zwei Typen! | days-time (`time.Duration`-ähnlich) vs years-months (Monatszähler) |
| List | `[]Value` | geordnet |
| Context | geordnete Key→Value-Map | Reihenfolge kann relevant sein |
| Range | Struct {lo, hi, loClosed, hiClosed} |  |
| Function | Closure + Signatur | Built-in oder user-defined |
| Null | `nil` |  |

## 6. Compiler (`compile.go`)

- Wandelt typisierten AST in `CompiledExpr = func(Scope) (Value, error)`.
- **Variablen** werden zu **Slot-Indizes** aufgelöst (Scope kennt zur Compile-Zeit das
  Layout). Kein Map-Lookup im Hot Path.
- Built-in-Aufrufe werden **direkt** an die Go-Funktion gebunden (kein Namens-Dispatch
  zur Laufzeit).
- Konstante Teilausdrücke werden zur Compile-Zeit gefaltet, wo gefahrlos möglich.

## 7. Built-in-Funktionen (`builtins/`)

Organisation: eine Datei pro Kategorie, je Funktion ein Eintrag in der Registry plus
Tests. **Vollständige** Liste gemäß DMN 1.5 — Kategorien:

- **Conversion**: `date, time, date and time, duration, number, string, ...`
- **Boolean**: `not`
- **String**: `substring, string length, upper case, lower case, contains,
  starts with, ends with, matches (regex), replace, split, string join, ...`
- **List**: `list contains, count, min, max, sum, mean, all, any, sublist, append,
  concatenate, insert before, remove, reverse, index of, union, distinct values,
  flatten, product, median, stddev, mode, sort, ...`
- **Numeric**: `decimal, floor, ceiling, round*, abs, modulo, sqrt, log, exp, even, odd, ...`
- **Date/Time/Range**: `now, today, day of week, day of year, week of year, month of year,
  ...` und Range-Built-ins `before, after, meets, overlaps, during, ...` (1.5)
- **Context**: `get value, get entries, context put, context merge, ...`
- **Sort**: `sort(list, precedes-function)`

> **Agent-Regel:** Implementiere Built-ins **datengetrieben**. Lege eine Tabelle
> `{name, arity, paramTypes, fn}` an. Jede Funktion bekommt mindestens einen Test für
> den Normalfall **und** einen für `null`/Fehler-Propagation.

## 8. Teststrategie speziell für FEEL

- Die DMN-Spec enthält im FEEL-Kapitel zahlreiche Beispieltabellen („expression →
  result"). Diese als parametrisierte Testtabellen abtippen → sofort hoher
  Konformitätsgrad.
- TCK enthält FEEL-lastige Modelle → über den TCK-Runner (WP-40) abdecken.
- Fuzzing auf Lexer + Parser (dürfen nie paniccen).

## 9. Reihenfolge der Umsetzung (mappt auf WPs)

1. Lexer (WP-03) → 2. Parser (WP-04) → 3. Werte/Number (WP-05) →
4. Compiler-Kern (WP-06) → 5. Built-ins Kern (WP-07) → … → vollständig (WP-20/21/22).

## 10. Stolperfallen (explizit für KI-Agenten dokumentiert)

- **Number ist Decimal, nicht float.** `0.1 + 0.2` muss exakt `0.3` ergeben.
- **`else` ist in FEEL-`if` Pflicht** — anders als in vielen Sprachen.
- **Zwei Dauer-Typen** sind nicht ineinander umrechenbar (Monate haben variable Tage).
- **`null`-Propagation** ist der Normalfall, nicht die Ausnahme.
- **Namen mit Leerzeichen** brechen naive Tokenizer.
- **Listen sind 1-indiziert** in FEEL-Filtern (`L[1]` ist das erste Element).
- **Range-Endpunkte** können offen/geschlossen sein und Strings/Dates umfassen, nicht nur Zahlen.
