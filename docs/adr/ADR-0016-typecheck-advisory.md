# ADR-0016: Statische Typprüfung ist advisory (Warnung), FEEL bleibt dynamisch

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-30 (Typecheck-Phase)

## Kontext

WP-30 fügt eine statische Typprüfung hinzu: Auf Basis der deklarierten Typen
(Input-Data-`typeRef`, Decision-Variable-`typeRef`, Item Definitions) prüft die
`Compile`-Phase die FEEL-Ausdrücke einer Entscheidung und meldet beweisbare
Typfehler mit Position. Offen war, **mit welcher Schwere** ein gefundener
Typkonflikt in den öffentlichen Diagnostik-Vertrag eingeht — als Fehler (wie ein
Compile-Fehler) oder als Warnung.

Zwei Eigenschaften des Systems prägen die Antwort:

- **FEEL ist dynamisch typisiert.** Die Engine wertet einen Typ-Mismatch zur
  Laufzeit spec-konform zu `null` aus (z. B. `number + string → null`), nicht zu
  einem Fehler — das ist bewusst Teil des Auswertungsvertrags (siehe
  `40-api-contract.md`: ein spec-konformes `null` ist **kein** Fehler). Ein
  ill-typisierter Ausdruck ist also lauffähig und liefert ein definiertes
  Ergebnis.
- **Statische Prüfung ist notwendig partiell** („wo möglich"). Wo ein Typ
  unbekannt ist (kein `typeRef`, Built-in-Aufrufe, geschachtelte Boxed-Formen),
  inferiert der Checker `Any` und schweigt. Er meldet nur, wenn **beide** Seiten
  konkret und nachweislich unvereinbar sind.

Würde ein statischer Typfund als **Fehler** gemeldet, bräche das zwei Dinge: Ein
weiterhin lauffähiges Modell würde an der `Compile`-Grenze als „fehlerhaft"
markiert (`diags.HasErrors()` → true), und die Konservativität des Checkers würde
zur Korrektheitsbedingung — ein einziger False Positive verwandelt ein gültiges
Modell in ein abgelehntes.

## Optionen

1. **Typfund als Fehler** (`Severity: error`). Stärkstes Signal, erzwingt
   Korrektur. — Widerspricht der dynamischen Null-Semantik (das Modell läuft
   trotzdem), macht `HasErrors()` von der Checker-Konservativität abhängig, und
   jeder False Positive blockiert ein gültiges Modell.
2. **Typfund als Warnung** (`Severity: warning`, Code `TYPE_ERROR`) — diese
   Entscheidung. Surface mit Position, ohne den Compile-Status zu kippen; die
   Auswertung bleibt unverändert (Null-Semantik). — Schwächeres Signal; Aufrufer
   müssen Warnungen aktiv auswerten.
3. **Konfigurierbar (strict-Modus hebt Warnungen zu Fehlern).** Flexibel. —
   Mehr API-Oberfläche, ohne dass heute ein Bedarf belegt ist; vertagbar, ohne
   Option 2 zu brechen.

## Entscheidung

Option 2. Statische Typfunde sind **Warnungen** mit stabilem Code `TYPE_ERROR`
(der Code benennt die Klasse, nicht die Schwere — vgl. `dmn/codes.go`) und tragen
die Quellposition. Sie kippen `diags.HasErrors()` nicht; die Auswertung folgt
unverändert FEEL-`null`-Semantik. Der Checker bleibt bewusst konservativ:
unbekannte Typen sind `Any` und werden nie gemeldet, sodass ein wohltypisiertes
Modell **keine** Funde erzeugt (durch einen Test über alle Beispielmodelle
abgesichert).

Der `instance of`-Operator ist davon unabhängig: Er ist ein reguläres
Laufzeit-Sprachkonstrukt (liefert boolean) und keine Diagnostik.

## Konsequenzen

**Positiv**
- Konsistent mit ADR-0004/Auswertungsvertrag: dynamisches FEEL bleibt dynamisch;
  ein lauffähiges Modell wird nicht an der Compile-Grenze abgelehnt.
- `HasErrors()` bleibt robust gegenüber der (notwendig unvollständigen)
  Checker-Heuristik; False Positives sind ärgerlich, aber nicht blockierend.
- Liefert dennoch echten Wert: beweisbare Fehler werden mit Position früh sichtbar.

**Negativ / Kosten**
- Warnungen sind leichter zu übersehen als Fehler; ein Aufrufer, der strikte
  Ablehnung will, muss `TYPE_ERROR`-Diagnosen selbst auswerten.

**Revisit-Trigger**
Sobald ein striktes Verifikationsszenario (z. B. agentengestützte Modellprüfung)
„ablehnen bei Typfehler" verlangt, wird Option 3 (ein `WithStrictTypes`-artiger
Schalter, der `TYPE_ERROR`-Warnungen zu Fehlern hebt) nachgezogen — additiv,
ohne diese Entscheidung zu brechen.
