# ADR-0042: Ein Decision Service veröffentlicht, was sein Aufrufer liefert

- **Status:** accepted
- **Datum:** 2026-09-28
- **Kontext-WP:** WP-52 (Agent-Schema & strenge Eingabevalidierung); beantwortet die
  in ADR-0041 offen gelassene Frage, berührt ADR-0040 (deklarierter Eingabetyp),
  ADR-0026 (Flow-Schritte) und ADR-0019 (SemVer-Kontrakt)

## Kontext

ADR-0041 hat das Eingabeschema einer Decision an ihren Anforderungskegel gebunden
und für den Decision Service ausdrücklich offen gelassen, ob er ein eigenes,
veröffentlichtes Schema bekommt. Solange er keines hat, fehlen vier Dinge:

1. `WithStrictInput()` ist an einem Service wirkungslos — angenommen und ignoriert.
2. Ein Aufrufer, der einen Service benennt, bekommt keine Typprüfung. Ein
   Business Rule Task, der dieselbe falsch typisierte Eingabe an die Decision
   schickt, wird abgewiesen; schickt er sie an den Service darüber, antwortet
   dieser still.
3. Eine **Input-Decision** wird nicht umgewandelt. Die Arbeitsmenge der
   Service-Grenze bestand aus den Kegeln der Output-Decisions, und ein Kegel nennt
   Input Data, keine Decisions. Eine als `date` deklarierte Input-Decision, die ein
   JSON-Aufrufer als Text liefert, erreicht die gekapselten Decisions als
   Zeichenkette.
4. Ein Flow-Schritt auf einen Service wird weder verdrahtungs- noch typgeprüft
   (`flow.Validate` überspringt ihn, `flow.Evaluate` baut seine Eingabe ohne
   Schema).

Gemessen an einem Service `Freigabe` mit der Output-Decision `Urteil`, den
gekapselten Decisions `Frist` und `Tragbar`, der Input-Decision `Stichtag`
(`date`) und dem Input `betrag` (`number`):

| Aufruf | vorher | jetzt |
|---|---|---|
| `betrag: 30000`, `stichtag: "2026-03-01"` | **`"abgelehnt"`** | `"bewilligt"` |
| `WithStrictInput()`, `betrag: "30000"` | **ausgewertet** | `TYPE_MISMATCH` auf `betrag` |
| `InputSchema()` | nicht vorhanden | `betrag: number`, `stichtag: date` |
| Flow-Schritt auf einen Service, Eingabe `"abc"` für `number` | **ausgewertet** | `*InputError` |
| Flow-Schritt, Zahl als Dezimal-String `"30"` | **als Text übergeben** | als Zahl übergeben |

## Welche Menge ist das Schema?

Zwei Kandidaten, und ADR-0041 hat den ersten bereits skizziert.

**A — die Deklaration des `<decisionService>`-Elements**: seine `<inputData>`
und `<inputDecision>`. Für A spricht viel. Es ist die Signatur, die DMN dem
Service gibt; sie bleibt stabil, wenn das Innere umgebaut wird, was der Zweck eines
Service ist; und die FEEL-Funktion, unter der eine Decision einen Service aufruft
(`registerServiceInvocables`), hat genau diese Parameterliste.

**B — was die Auswertung liest**: die Kegel der Output-Decisions, abgeschnitten
an den Input-Decisions, plus die Input-Decisions selbst.

Gewählt ist **B**. Das Schema hat drei Konsumenten: die Umwandlung nach
deklariertem Typ, die strikte Prüfung und die Veröffentlichung. Die ersten beiden
existieren, um die Auswertung vorherzusagen. Auf einem wohlgeformten Modell sind A
und B gleich. Sie weichen genau dann ab, wenn die Deklaration unvollständig ist —
und dann ist die Deklaration das Falsche:

- Eine auf A gebaute Umwandlung wandelte weniger um als heute. Ein `date`, das eine
  gekapselte Decision liest, das der Service aber nicht auflistet, würde wieder als
  Text ankommen — der Fehler, den ADR-0040 und ADR-0041 beseitigt haben.
- Eine auf A gebaute Prüfung nennte eine tatsächlich gelesene Eingabe
  `UNKNOWN_INPUT` und prüfte ihren Typ nicht.

Eine unvollständige Deklaration ist kein theoretischer Fall. Wer eine Decision in
den Service verschiebt, muss deren Inputs in der Deklaration nachtragen; genau das
war in einem Atlas-Modell (`kreditfreigabe`) beim Verschieben von `Bonität` nötig
und musste von Hand geschehen.

Der Preis von B: Das Schema ändert sich, wenn ein Umbau des Inneren ändert, was
gelesen wird. Dann haben sich aber auch die Pflichten des Aufrufers geändert, und
ein Schema, das das verschweigt, beschreibt nicht den Service, sondern seine
Deklaration.

## Entscheidung

- `CompiledService` bekommt `InputSchema() []InputField` und
  `ValidateInput(in Input) []InputProblem`, mit denselben Codes und Regeln wie
  `CompiledDecision.ValidateInput`.
- Die Menge wird einmal beim Kompilieren aufgelöst (`resolveServiceInputs`), nachdem
  Output-Decisions und Grenze feststehen. `coneOf` bekommt dafür eine Grenze: eine
  Anforderung auf eine Input-Decision wird nicht betreten, genau wie der Evaluator
  sie nicht berechnet (`eval.go`).
- **Input Data** kommen aus dem abgeschnittenen Kegel, mit den Feldern, die die
  lesenden Decisions deklarieren — dieselbe Antwort, die `ReachableInputSchema`
  gibt, einschliesslich des Typs aus der Tabellenspalte.
- **Input-Decisions** werden unter ihrem FEEL-Bezeichner geführt, typisiert durch
  den `typeRef` ihrer Variable, als erforderlich, mit den erlaubten Werten ihres
  Typs. Ein Kegel kann sie nicht nennen, weil sie Decisions sind.
- Ein Wert, den nur die Logik einer Input-Decision liest, gehört **nicht** dazu:
  Der Service berechnet diese Decision nie und liest deshalb auch nichts unter ihr.
- `CompiledService.Evaluate` beachtet `WithStrictInput()` und wandelt nach derselben
  Menge um.
- `flow`: Ein Service-Schritt wird gegen `InputSchema()` verdrahtungsgeprüft
  (`checkWiring`), seine Eingabe wird dagegen umgewandelt (`buildInput`) und
  geprüft (`ValidateInput`) — wie ein Decision-Schritt.

## Konsequenzen

- **Positiv.** Ein Aufrufer bekommt für einen Service dieselbe Typprüfung wie für
  eine Decision. Atlas kann damit einen falsch typisierten Aufruf eines Service so
  abweisen, wie es das für eine Decision tut.
- **Positiv.** Eine als temporaler Typ deklarierte Input-Decision wird umgewandelt.
- **Positiv.** `WithStrictInput()` hat an einem Service eine Bedeutung.
- **Positiv.** Ein Flow behandelt einen Service-Schritt wie einen Decision-Schritt.
- **Negativ (Verhalten).** Wer `WithStrictInput()` an einen Service übergeben hat,
  bekommt bei unpassender Eingabe jetzt einen `*InputError` statt eines `Result`.
  Ein Flow mit falsch verdrahtetem Service-Schritt wird jetzt von `Validate`
  abgewiesen, statt mit null ausgewertet zu werden.
- **Neutral.** Die Umwandlung erfasst Werte unter einer Input-Decision nicht mehr,
  die bisher über den vollen Kegel mitgenommen wurden. Sie werden über keine
  deklarierte Anforderung gelesen; kein Ergebnis ändert sich dadurch.
- **Oberfläche.** Zwei Methoden kommen hinzu (`testdata/api/dmn.api`). Nach
  ADR-0019 additiv; das geänderte Verhalten ist dieselbe Art Korrektur wie in
  ADR-0040 und ADR-0041 und betrifft nur Fälle, deren bisherige Antwort falsch war.

## Was dieser Record nicht entscheidet

- **Eine Diagnose für eine unvollständige Deklaration.** Weichen Deklaration und
  gelesene Menge ab, wäre eine Warnung beim Kompilieren der richtige Ort, das dem
  Autor zu sagen. Sie ist nicht Teil dieses Records.
- **Die Signatur der FEEL-Funktion eines Service** folgt weiterhin der Deklaration.
  Auf einem Modell mit unvollständiger Deklaration geben Schema und Signatur
  deshalb zwei Antworten.
- **Eine fehlende Eingabe an einen Service.** `CompiledDecision.Evaluate` bricht bei
  fehlendem erforderlichem Input mit `CodeMissingInput` ab; `CompiledService.Evaluate`
  prüft das ohne `WithStrictInput()` nicht. Diese Asymmetrie bleibt bestehen.
