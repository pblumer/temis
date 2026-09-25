# ADR-0040: Deklarierter Eingabetyp — Schema, Validierung und Auswertung stimmen überein

- **Status:** accepted
- **Datum:** 2026-09-25
- **Kontext-WP:** WP-52 (Agent-Schema & strenge Eingabevalidierung); berührt ADR-0013
  (Agent-First), ADR-0017 (Typprüfung ist advisory), ADR-0039 (`feel/value` ist
  öffentlich)

## Kontext

WP-52 hat die Selbstbeschreibung gebaut und dabei ein Versprechen gegeben, das in
`docs/40-api-contract.md` §1.3 wörtlich steht:

> Selbstbeschreibung der erwarteten Inputs samt Typen, plus präzise, maschinenlesbare
> Validierungsfehler **statt stillschweigend falscher Defaults**.

Für `date`, `time` und `date and time` wird dieses Versprechen heute nicht gehalten, und
zwar in beide Richtungen zugleich. Gemessen auf diesem Stand, mit einem Modell, dessen
Input `typeRef="date"` deklariert und dessen Entscheidungstabelle eine `date`-typisierte
Spalte mit der Zelle `< date("2026-01-01")` führt:

| Gesendeter Wert | `ValidateInput` | Auswertung |
|---|---|---|
| `"2025-06-01"` (String) | **keine Beanstandung** | `"neu"` — **falsch**, die Catch-all-Zeile gewinnt |
| `"nonsense"` | **keine Beanstandung** | — |
| `""` | **keine Beanstandung** | — |
| `42` | `TYPE_MISMATCH: expects date, got number` | — |
| `true` | `TYPE_MISMATCH: expects date, got boolean` | — |
| `time.Time` | `TYPE_MISMATCH: expects date, got date and time` | `"neu"` — falsch |
| `value.NewDate(2025, June, 1)` | `TYPE_MISMATCH: expects date, **got value.Date**` | `"alt"` — **richtig** |

Drei Aussagen, die nicht alle zugleich stimmen können:

1. `ReachableInputSchema` sagt, die Eingabe sei vom Typ `date`.
2. `ValidateInput` sagt, **jeder** String sei dafür konform — auch `"nonsense"` und `""`.
3. `toValue` macht aus diesem String eine FEEL-**Zeichenkette**, weshalb jeder
   Datumsvergleich in der Tabelle scheitert und die Catch-all-Zeile gewinnt.

Und die letzte Zeile der Tabelle dreht es vollends um: Der **einzige** Wert, der korrekt
auswertet — eine echte `value.Date`, seit ADR-0039 aus `github.com/pblumer/feel/value`
öffentlich konstruierbar — wird von der strengen Validierung als Typkonflikt abgelehnt,
mit `got value.Date`. Das ist kein FEEL-Typname; der Pfad, der den Ist-Typ benennt,
kennt genau denjenigen Wertetyp nicht, mit dem der Evaluator selbst rechnet.

Damit gilt heute: **Was `WithStrictInput` durchlässt, wertet falsch aus. Was korrekt
auswertet, lehnt `WithStrictInput` ab.**

Der Schaden ist die Klasse von Fehler, gegen die WP-52 angetreten ist. Es gibt keine
Diagnose, keinen Trace-Eintrag und keinen Fehler — die Regel trifft einfach nicht, die
Catch-all-Zeile antwortet, und der Aufrufer bekommt eine plausible falsche Antwort.
Nachgelagert ist sie von einer richtigen nicht zu unterscheiden.

Kein Aufrufer kann das von aussen beheben. Ein String wird nicht konvertiert; ein
`time.Time` wird eine `date and time` und ist damit der falsche Typ; eine `value.Date`
ist der richtige Wert, fällt aber durch die Validierung.

### Was hier *nicht* das Problem ist

FEEL kennt keine implizite Umwandlung von String zu `date` — das ist richtig so und
steht nicht zur Debatte. Die Abbildung **Go-Wert → FEEL-Wert** liegt jedoch ausserhalb
von FEEL: DMN schreibt sie nicht vor, sie ist die Grenze, die die Engine definiert.
temis definiert sie in `toValue`, veröffentlicht mit `InputField.Type` den erwarteten
FEEL-Typ und prüft mit `ValidateInput` dagegen. Diese Grenze gehört temis bereits; hier
geht es nur darum, dass ihre drei Teile dasselbe sagen.

Ebenso wenig geht es um ADR-0017 (statische Typprüfung bleibt advisory). Das betrifft
Ausdrücke im Modell. Hier geht es um Werte, die von aussen hereingereicht werden, und
für die WP-52 eine strenge Prüfung ausdrücklich vorgesehen hat.

## Optionen

1. **Konvertieren nach deklariertem Typ, und den Ist-Typ korrekt benennen.**
   `inputToValues` erhält den Kontext des Schemas: Wo ein Input `date`, `time` oder
   `date and time` deklariert und der Wert ein String im zugehörigen ISO-8601-Format
   ist, entsteht der passende FEEL-Wert. Ein String, der das Format nicht erfüllt, wird
   `TYPE_MISMATCH` statt stillschweigend eine Zeichenkette. Zusätzlich lernt die
   Typbenennung `value.Date`, `value.Time` und `value.DateTime`, damit ein bereits
   korrekt konstruierter Wert nicht mehr abgelehnt wird.

2. **Nur die Validierung schärfen, nicht konvertieren.** `ValidateInput` lehnt einen
   String für `date` ab. Der stille Fehler wird sichtbar, aber jeder JSON-sprechende
   Aufrufer — und das sind alle über HTTP und MCP — kann danach überhaupt kein Datum
   mehr übergeben, solange er nicht `value.Date` baut. Für Nicht-Go-Aufrufer ist das
   eine Sackgasse.

3. **Nur die Typbenennung reparieren** (`value.Date` wird als `date` erkannt), sonst
   nichts. Behebt die umgekehrte Hälfte: Ein Go-Aufrufer kann dann korrekt übergeben und
   streng validieren. Der String bleibt still falsch, und HTTP/MCP bleiben aussen vor.

4. **Nichts ändern, dokumentieren.** Jedes Modell trägt künftig `date(Stichtag)` um
   seine Datumsspalten. Konformes FEEL, aber Kompensation für eine Engine-Lücke in
   Dokumenten, die die Geschäftsregel sein sollen — und der Grund dafür ist in einem
   Jahr nicht mehr lesbar.

## Entscheidung

**Option 1**, mit Option 3 als deren zweiter Hälfte.

Die Konvertierung ist ausdrücklich **keine** FEEL-Koerzierung (DMN §10.3.2.9.4, in
`coerceToType`), die konform hält oder `null` erzeugt. Sie ist die Abbildung an der
Aussengrenze, in der Zeile darüber:

- `date` ← String im Format `YYYY-MM-DD`
- `time` ← String im Format `HH:MM:SS` (mit optionalem Offset)
- `date and time` ← String im Format `YYYY-MM-DDTHH:MM:SS` (mit optionalem Offset)
- `duration` ← String im ISO-8601-Dauerformat (`P…`)

Genau diese Schreibweisen und keine weiteren. `dd.MM.yyyy` und Verwandte bleiben
aussen vor: Sie sind gebietsabhängig, und eine Engine, die rät, ist die Fehlerklasse,
die dieser Record schliesst.

Drei Regeln halten es beherrschbar:

- **Ein Wert wird nie stillschweigend verworfen.** Ein String, den der deklarierte Typ
  nicht hergibt, wird `TYPE_MISMATCH` mit `expected`/`got` — nicht `null` und nicht
  weiterhin eine Zeichenkette.
- **Die Konvertierung ist total und rein.** Gleicher Wert, gleicher deklarierter Typ,
  gleiches Ergebnis — unabhängig von Zeitzone, Uhrzeit und Reihenfolge.
- **Ohne deklarierten Typ ändert sich nichts.** `InputField.Type == ""` (Custom Item
  Definition, WP-31) lässt den Wert unverändert.

## Konsequenzen

**Positiv**

- Schema, Validierung und Auswertung sagen wieder dasselbe. Heute tun sie es nicht, und
  die Abweichung ist von keiner der drei Seiten allein zu sehen.
- Eine Entscheidungstabelle, die Datumsangaben vergleicht, funktioniert über HTTP, MCP
  und die Library — ohne dass das Modell `date(…)` um einen Wert legen muss, den das
  Schema bereits `date` nennt.
- `WithStrictInput` hält, was §1.3 verspricht, statt genau den Fall durchzulassen, der
  still falsch auswertet.

**Negativ**

- Ein Aufrufer, der sich darauf verlässt, dass ein `date`-deklarierter Input als String
  ankommt (Teilstring-Vergleiche, `string length`), ändert sein Verhalten.

### Einstufung nach ADR-0019

Die **exportierte Oberfläche ist unverändert**: `testdata/api/dmn.api` bleibt gleich,
der Golden-Test bestätigt das. Geändert hat sich Verhalten an zwei Stellen:

1. **Lenient (ohne `WithStrictInput`).** Ein `date`-deklarierter Input, der als ISO-Text
   ankommt, wird jetzt als Datum ausgewertet statt als Zeichenkette. Ein Aufrufer, der
   den String als String gelesen hat, bekommt ein anderes Ergebnis.
2. **Strict.** `Evaluate(…, WithStrictInput())` lehnt jetzt einen String ab, den der
   deklarierte Typ nicht hergibt. Ein Aufruf, der bisher ein `Result` lieferte, liefert
   jetzt `*InputError`.

Punkt 2 ist nach dem Wortlaut von ADR-0019 ein **Breaking Change** („Verschieben der
Fehlergrenze") und damit **Major**. Dagegen lässt sich halten, dass `WithStrictInput`
laut §1.3 genau dies immer tun sollte und die Grenze hier nicht verschoben, sondern
erstmals dort hingelegt wird, wo sie dokumentiert ist. Dieser Record trifft die
technische Entscheidung; **welche Version daraus folgt — Minor mit CHANGELOG-Hinweis
oder Major mit `/vN`-Modulpfad — ist eine Release-Entscheidung und wird hier
ausdrücklich nicht getroffen.**
- Die Engine hält damit eine Meinung darüber, welche String-Schreibweisen ein Datum
  sind. Diese Meinung ist klein und aufgeschrieben, muss aber getestet bleiben.

**Folgeaufgaben**

Erledigt mit diesem Record:

- Tabellengetriebene Tests über die Library (`dmn/inputtype_test.go`): je Eingabeform
  — gültiger ISO-Text, Text ohne Datum, leerer Text, Datum mit Uhrzeit, gebietsabhängige
  Schreibweise, Zahl, Wahrheitswert, `time.Time`, echter FEEL-Wert — gegen Auswertung
  *und* Validierung, plus der Fall ohne deklarierten Typ.
- `docs/40-api-contract.md` §1.3 trägt die Abbildungstabelle.
- **TCK-Lage geprüft:** `make verify` inklusive `internal/tck` ist grün. Keine
  TCK-Testcase bestand deshalb, weil ein String als Datum durchgereicht wurde.
- `CompiledService.Evaluate` konvertiert nach denselben Deklarationen wie die Decisions,
  die der Service veröffentlicht, damit ein Service ein `date` nicht anders behandelt
  als die Decision dahinter.

Offen:

- Ein Service **veröffentlicht** weiterhin kein Schema (§1.3), `WithStrictInput` bleibt
  dort wirkungslos. Die Konvertierung greift, die strenge Prüfung nicht. Das
  Service-Schema bleibt die Folgearbeit, die §1.3 schon nennt.
- HTTP und MCP reichen `Input` unverändert durch und erben die Konvertierung damit;
  eigene Tests dafür stehen aus.
- Die Release-Einstufung oben.
