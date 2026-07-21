# DMN/FEEL-`null`-Fallen — ausgearbeitete Beispiele

Reale Fälle, jeder mit der Spur, die ihn entlarvt, und dem Fix. Alle Ausgaben stammen aus
`evaluate` mit `explain: true`. Testdaten durchgehend: `Testliste = ["test1", "test2", "test3"]`.

---

## Falle 1 — Voller Ausdruck in einer Tabellen-Eingabezelle (Unary Test)

**Symptom:** `Entscheidung_1` gibt `null` zurück, obwohl die Formel „richtig aussieht".

Die Decision ist eine **Entscheidungstabelle** (Hit Policy U) mit einer Eingabespalte
`Testliste` und dieser Zelle:

```feel
if count(Testliste) > 0 then Testliste[1] else null
```

**Trace:**

```json
"rules": [{ "matched": false,
  "conditions": [{ "input": "Testliste",
    "entry": "if count(Testliste) > 0 then Testliste[1] else null",
    "matched": false }]}],
"matched": null
```

**Warum:** Eine **Eingabezelle** ist ein *Unary Test* — sie wird mit dem Spalten-Eingabeausdruck
(`Testliste`) verglichen. Die Engine prüft also „`Testliste = (if count(Testliste) > 0 then
Testliste[1] else null)`" → `["test1","test2","test3"] = "test1"` → **false**. Keine Regel
trifft → Hit Policy U → `null`.

**Fix A (Literal-Ausdruck):** die Decision von Tabelle auf Literal umstellen, Ausdruck als Rumpf:

```feel
if count(Testliste) > 0 then Testliste[1] else null    // → "test1"
```

**Fix B (Tabelle behalten):** den Ausdruck in die **Ausgabespalte** legen, Eingabebedingung `-`.

---

## Falle 2 — Aufruf eines leeren BKM

**Symptom:** `Entscheidung_1 = for eintrag in Testliste return Bewertung(eintrag)` gibt
`[null, null, null]` zurück (im Modeler ggf. verkürzt als „null" angezeigt).

**Warum:** `Bewertung` ist ein BKM ohne hinterlegte Logik. Ein Aufruf eines BKM **ohne Rumpf**
liefert `null` — pro Eintrag einmal.

| `Bewertung` | Ergebnis |
|---|---|
| ohne Logik | `[null, null, null]` |
| mit `if count(liste) > 0 then "gut" else "nicht gut"` (Param `liste`) | `"gut"` bei Aufruf `Bewertung(Testliste)` |
| mit `"Bewertung von " + eintrag` (Param `eintrag`) | `["Bewertung von test1", …]` |

**Fix:** dem BKM einen **formalen Parameter** und einen **FEEL-Rumpf** (Literal oder Tabelle) geben.

---

## Falle 3 — Boolescher Guard in einer Tabellen-Eingabezelle

**Symptom:** `Entscheidung_1` gibt **immer** `"nicht gut"` zurück, nie `"gut"`.

Tabelle mit Eingabespalte `Testliste`:

| # | Eingabe (`Testliste`) | Ausgabe |
|---|---|---|
| 1 | `count(Testliste) > 0` | `"gut"` |
| 2 | `-` | `"nicht gut"` |

**Trace:** Regel 1 `matched:false`, Regel 2 (`-`) `matched:true` → `"nicht gut"`.

**Warum:** dieselbe Wurzel wie Falle 1. `count(Testliste) > 0` in der Eingabezelle heißt
„`Testliste = (count(Testliste) > 0)`" → `Liste = true` → false. Regel 1 feuert nie, die
Auffangregel gewinnt immer.

**Fix (Literal):**

```feel
if count(Testliste) > 0 then "gut" else "nicht gut"    // → "gut"
```

**Fix (Tabelle):** Spalten-Eingabeausdruck von `Testliste` auf **`count(Testliste)`** ändern,
Regel-1-Zelle auf nur **`> 0`**. Dann ist die Zelle ein echter Unary Test gegen die Zahl.

---

## Logik in ein BKM auslagern (Wiederverwendung)

Die Guard-Logik gehört einmalig in ein **BKM**, die Decision ruft es nur auf:

```xml
<businessKnowledgeModel id="Bewertung" name="Bewertung">
  <variable name="Bewertung" typeRef="string"/>
  <encapsulatedLogic kind="FEEL">
    <formalParameter name="liste" typeRef="Any"/>
    <literalExpression>
      <text>if count(liste) &gt; 0 then "gut" else "nicht gut"</text>
    </literalExpression>
  </encapsulatedLogic>
</businessKnowledgeModel>
```

```feel
// Entscheidung_1 (mit knowledgeRequirement auf #Bewertung):
Bewertung(Testliste)        // → "gut"
```

Der Parametername (`liste`) ist frei wählbar; der Aufruf `Bewertung(Testliste)` übergibt die
konkrete Liste positional. So ist `Bewertung(<beliebige Liste>)` wiederverwendbar.

---

## Merksätze

- **Eingabezelle = Vergleich, nicht Ausdruck.** In die Zelle gehört nur `> 0`, `"test1"`,
  `not("x")`, `[1..10]`. Der zu prüfende *Wert* gehört in den **Spalten-Eingabeausdruck**.
- **Im Zweifel Literal-Ausdruck** statt Tabelle, wenn du ein Ergebnis *berechnen* (nicht
  *nachschlagen*) willst.
- **Immer `explain: true`** — die Spur (`matched`, `entry`) zeigt die Falle sofort.
- **`strict: true`** macht aus stillem `null` einen präzisen Eingabefehler.
