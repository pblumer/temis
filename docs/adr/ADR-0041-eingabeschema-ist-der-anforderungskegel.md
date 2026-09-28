# ADR-0041: Das Eingabeschema einer Decision ist ihr Anforderungskegel

- **Status:** accepted
- **Datum:** 2026-09-25
- **Kontext-WP:** WP-52 (Agent-Schema & strenge Eingabevalidierung); korrigiert
  ADR-0040 (deklarierter Eingabetyp), berührt ADR-0026 (erreichbares Schema für
  Flow-Schritte), ADR-0019 (SemVer-Kontrakt)

## Kontext

ADR-0040 hat festgelegt, dass eine Eingabe nach dem im Modell deklarierten Typ
umgewandelt wird, und die Umwandlung an `CompiledDecision.inputs` gehängt — die
Inputs, die **die Decision selbst deklariert**.

Das ist für ein Blatt richtig und für alles darüber wertlos. Wer eine Decision
auswertet, wertet alles aus, was sie anfordert, und liefert deshalb die
Blatt-Eingaben des ganzen Kegels. Eine Decision, die nur andere Decisions
anfordert, deklariert selbst nichts — ihr `InputSchema()` ist leer. Genau das ist
die Form, die ein sauber geschichtetes DRG oben hat, und genau dorthin zeigt ein
Aufrufer.

Gemessen an einem Modell, dessen Input `stichtag` eine Ebene tiefer als `date`
deklariert ist und dessen obere Decision nur `requiredDecision`-Kanten hat:

| Aufruf | vorher | erwartet |
|---|---|---|
| `Frist` (Blatt) mit `stichtag: "2026-03-01"` | `true` | `true` |
| `Urteil` (zusammengesetzt), gleiche Eingabe | **`"nachher"`** | `"vorher"` |
| `Urteil` mit `WithStrictInput()`, korrekte Eingabe | **`UNKNOWN_INPUT: … expected one of (none)`** | akzeptiert |
| `Urteil` mit `WithStrictInput()`, `stichtag: 42` | **akzeptiert** | `TYPE_MISMATCH` |
| Service über `Urteil`, gleiche Eingabe | **`"nachher"`** | `"vorher"` |

Dieselbe Decision, direkt ausgewertet, ist richtig. Das ist, was den Fehler so
schwer sichtbar macht: er entsteht nicht dort, wo er wirkt. `stichtag` erreicht
`Frist` als Zeichenkette, `stichtag < date(...)` ist null, `frist` ist null, und
eine Tabelle kann null nicht von `false` unterscheiden. Es gibt keine Diagnose,
keinen Trace-Eintrag und keinen Fehler — nur ein anderes Ergebnis.

Bemerkenswert ist, dass dieses Haus die Regel bereits kennt und zweimal
aufgeschrieben hat. `flow/evaluate.go` typisiert die Verdrahtung eines Schritts
ausdrücklich gegen `ReachableInputSchema` und begründet es im Kommentar
(ADR-0026); `mcp/server.go` veröffentlicht `inputs` und `reachableInputs`
nebeneinander. Nur der Kern, der auswertet, folgt ihr nicht.

## Optionen

**A — Jede Aufrufstelle einzeln auf `ReachableInputSchema` umstellen.**
Kleinster Eingriff. Verworfen: es sind drei Stellen im Kern (Umwandlung, strikte
Prüfung, Service-Arbeitsmenge) und zwei weitere ausserhalb, die es bereits
richtig machen. Genau diese Verteilung hat den Fehler erzeugt — jede Stelle
entscheidet für sich, welche Frage sie stellt, und drei von fünf haben sich
geirrt. Eine sechste Stelle würde sich wieder irren.

**B — Den Kegel bei jeder Auswertung laufen.** Korrekt, aber unnötig: die
Anforderungskanten stehen nach dem Kompilieren fest. Ein Graphlauf pro Auswertung
widerspricht ausserdem der Linie, dass beim Kompilieren gerechnet wird und beim
Auswerten nicht.

**C — Die Kegel-Union einmal beim Kompilieren auflösen und auf der
`CompiledDecision` führen.** Gewählt.

## Entscheidung

Option C. Nach dem Verdrahten der Anforderungskanten löst `resolveReachableInputs`
für jede Decision die Union ihres Kegels auf und legt sie als `reachable` samt
`reachableConstraints` auf die `CompiledDecision`. Alles, was eine Eingabe gegen
ein Schema liest, liest diese eine Menge:

- die Umwandlung nach deklariertem Typ (`inputToValuesTyped`),
- die strikte Validierung unter `WithStrictInput()`,
- `Definitions.ReachableInputSchema` und `ValidateReachableInput`, die bisher
  denselben Kegel bei jedem Aufruf neu gelaufen sind,
- `CompiledService.declaredInputs()`, die Arbeitsmenge der Service-Grenze.

`CompiledDecision.InputSchema()` bleibt unverändert und bleibt die *Deklaration*
der Decision. Es ist weiterhin die richtige Antwort auf „was erklärt diese
Decision über sich selbst" und weiterhin die falsche auf „was schickt ein
Aufrufer" — der Unterschied steht jetzt am Feld.

Die öffentliche Fläche ändert sich nicht: kein Symbol kommt hinzu, keines fällt
weg, `dmn.api` bleibt gleich. Das Verhalten ändert sich, und zwar in drei Punkten
(Umwandlung, strikte Prüfung, Service). Nach ADR-0019 ist das keine additive
Änderung, sondern eine Korrektur der Fehlergrenze — Minor, weil sie ausschliesslich
Fälle betrifft, in denen die bisherige Antwort nachweislich falsch war, und weil
ein Blatt sich nicht bewegt.

## Konsequenzen

- **Positiv.** Ein deklarierter temporaler Typ wirkt, wo er deklariert ist, und
  nicht nur, wenn der Aufrufer zufällig die Decision benennt, die ihn deklariert.
- **Positiv.** `WithStrictInput()` ist an einer zusammengesetzten Decision
  erstmals brauchbar. Vorher meldete es jede legitime transitive Eingabe als
  `UNKNOWN_INPUT` mit `expected one of (none)` und liess jeden Typfehler durch —
  es war nicht streng, sondern zufällig.
- **Positiv.** Ein Decision Service über einer zusammengesetzten Output-Decision
  wandelt seine Eingaben um. Das war die in ADR-0040 offen gebliebene Schwäche der
  Service-Grenze und ist damit zur Hälfte geschlossen; ein veröffentlichtes
  Eingabeschema für `CompiledService` bleibt offen.
- **Negativ.** Eine strikte Auswertung, die sich bisher auf die Ablehnung
  transitiver Eingaben verlassen hat, sieht sie nun akzeptiert. Das war kein
  Vertrag, sondern ein Defekt; eine Anwendung, die ihn ausgenutzt hat, hat etwas
  ausgenutzt, das nie zugesichert war.
- **Negativ.** Ein Modell mit temporalem Typ unter einer zusammengesetzten
  Decision liefert ein anderes Ergebnis als vorher — das richtige. Das ist
  dieselbe Art Änderung, die ADR-0040 bereits beschlossen hat, nur erreicht sie
  jetzt die Fälle, die ADR-0040 verfehlt hat.
- **Neutral.** Für ein Blatt ändert sich nichts: sein Kegel ist es selbst.
- **Aufwand beim Kompilieren.** Eine Kegel-Union pro Decision, einmalig. Die
  vorherige Fassung hat denselben Lauf bei *jedem* Aufruf von
  `ReachableInputSchema` gemacht; unter dem Strich wird weniger gerechnet.

## Was dieser Record nicht entscheidet

Ob `CompiledService` ein eigenes, veröffentlichtes Eingabeschema bekommt
(`InputSchema()` und `ValidateInput`), gespeist aus `<inputData>` und
`<inputDecision>` des Service-Elements. Solange es das nicht gibt, bleibt
`WithStrictInput()` an einem Service wirkungslos, und ein Aufrufer, der einen
Service benennt, bekommt keine Typprüfung. Das ist ein eigener Record.
