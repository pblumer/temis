---
name: temis-decision-modeling
description: >
  Anleitung für einen KI-Agenten, der GEMEINSAM MIT EINEM MENSCHEN DMN-Decisions in temis
  modelliert und debuggt — der Agent über die temis-MCP-Tools, der Mensch über den Modeler,
  auf demselben geteilten Modell-Cache. Nutze diesen Skill, wenn du in temis eine Decision
  bauen/ändern sollst, ein unerwartetes Ergebnis (besonders `null`) erklären sollst, ein
  Modell des Nutzers per Name oder modelId finden, sein FEEL zurücklesen (get_model_xml),
  mit Trace auswerten (evaluate/explain) und eine korrigierte Version zurückgeben musst.
  Auslöser u. a.: „temis Decision modellieren", „DMN mit Agent bauen", „warum null",
  „FEEL debuggen", „Entscheidungstabelle", „BKM", „evaluate explain",
  „list_models/get_model_xml", „gemeinsam mit dir modellieren".
version: 1.0.0
---

# temis — Decisions gemeinsam mit einem Menschen modellieren

Dieser Skill beschreibt, wie du als Agent **Hand in Hand mit einem Menschen** DMN-Decisions
in [temis](https://github.com/pblumer/temis) baust und debuggst: **du arbeitest über die
temis-MCP-Tools, der Mensch über den Modeler** (das Web-GUI). Beide teilen sich **einen**
Modell-Cache (co-located `temisd`, ADR-0021) — was der eine speichert/lädt, sieht der andere.

> **Rolle abgrenzen (ADR-0013):** Dieser Skill ist für den Agenten als **Modellier-Partner
> zur Laufzeit**. Für den Agenten, der temis *baut* (Contributor), gilt `docs/60-ai-agent-guide.md`.

## Das mentale Modell

- **temis ist ein Verifikationswerkzeug**, kein Ratespiel. Es ist **deterministisch**: gleiche
  Eingaben → gleiches Ergebnis, mit nachvollziehbarer Spur. Rate nie ein Decision-Ergebnis —
  werte es aus und lies die Spur.
- **Ein geteilter Cache, zwei Oberflächen.** Ein im Modeler gespeichertes Modell erscheint dir
  über `list_models`; ein von dir per `load_model` geladenes Modell erscheint im Modeler
  (Nutzer klickt **⟳ Aktualisieren**). Die `modelId` (`sha256:…`) ist über beide Seiten identisch.
- **Modelle sind content-adressiert.** Jeder Speicherstand ist eine **eigene** `modelId`
  (Revision). Derselbe Name kann viele Revisionen haben.

## Arbeitsschleife (bei jeder Modellier-/Debug-Aufgabe)

1. **Das richtige Modell finden.**
   - `list_models` liefert je Modell `name` (wie im Modeler), `decisions`, `inputs`.
   - **Der Name ist NICHT eindeutig.** Um den *exakten aktuellen* Stand zu treffen, bitte den
     Menschen, die **modelId aus dem Chip in der Modeler-Toolbar** zu kopieren und dir zu geben.
   - Nur **gespeicherte** Modelle sind sichtbar. Wirkt ein Ergebnis veraltet: bitte den Menschen,
     im Modeler zu **speichern**, dann neu prüfen.
2. **Das echte FEEL lesen, nicht raten.** `get_model_xml` gibt das rohe DMN/FEEL eines
   gecachten Modells zurück. Lies es, bevor du etwas änderst.
3. **Diagnostizieren mit Beweis.** `evaluate` mit `explain: true` — die Spur zeigt, welche
   Regeln (nicht) gefeuert haben und warum. Mit `strict: true` werden stille `null`/Nicht-Treffer
   zu präzisen Eingabefehlern (`TYPE_MISMATCH`, `UNKNOWN_INPUT`, `MISSING_INPUT`).
4. **Korrigieren, ohne die Arbeit des Menschen zu zerstören.** Bevorzugt das **zurückgelesene
   XML** bearbeiten (erhält Diagramm-Layout und andere Knoten) statt neu aufzubauen. Nutze die
   **DMN-2023-Namespace** `https://www.omg.org/spec/DMN/20230324/MODEL/`, sonst gibt es eine
   `UNKNOWN_NAMESPACE`-Warnung.
5. **Als neue Version zurückgeben.** `load_model` mit **demselben `name`** → erscheint im Modeler
   als neuester Stand. Das ist **additiv** (neue `modelId`), überschreibt nichts.
6. **Verifizieren und übergeben.** Vor der Übergabe mit `evaluate` bestätigen. Sag dem Menschen
   **welche `modelId`** die korrigierte Version ist und dass er **⟳** drücken soll.

## Die häufigsten `null`-Fallen (immer zuerst prüfen)

Ausführliche Vorher/Nachher-Beispiele: [`references/dmn-feel-traps.md`](references/dmn-feel-traps.md).

1. **Eingabezelle einer Entscheidungstabelle = Unary Test.** Der Inhalt einer Eingabespalten-Zelle
   wird **mit dem Spalten-Eingabeausdruck verglichen**, nicht als eigenständiger Boolescher
   Ausdruck ausgewertet. Ein berechneter/Boolescher Ausdruck dort (`count(x) > 0`,
   `if … then … else …`) wird zu „`Eingabewert = (dieser Ausdruck)`" — trifft fast nie → Regel
   feuert nicht → Hit Policy U ergibt `null` (oder die `-`-Auffangregel gewinnt).
   **Fix:** den *Wert* in den **Spalten-Eingabeausdruck** (z. B. `count(liste)`), nur den
   *Vergleich* in die Zelle (`> 0`); oder die Decision als **Literal-Ausdruck** modellieren.
2. **Aufruf eines leeren BKM ergibt `null`.** `for x in liste return Bewertung(x)` über ein BKM
   ohne Rumpf ergibt `[null, null, …]`. **Fix:** dem BKM einen **formalen Parameter + FEEL-Rumpf**
   (Literal oder Tabelle) geben.
3. **`typeRef` ist der FEEL-Ergebnistyp** (`string`/`number`/`boolean`/`list`/`date`/…);
   **leer = `Any`**, nicht „kein Wert". `list` (bzw. `list<string>`) für Collections. Das ist kein
   JSON-/Format-Feld — Werte tippt man JSON-artig ins Ausdrucksfeld, der Typ ist davon getrennt.

## Übergabe-Etikette (Mensch ⇄ Agent)

- Der Mensch **speichert** im Modeler; du arbeitest auf **gespeicherten** Schnappschüssen.
- Kommuniziert die `modelId` **in beide Richtungen** (Toolbar-Chip → an dich; deine neue
  Revision → an ihn).
- **Überschreibe nichts still.** Eine neue Version ist additiv; benenne klar, welche `modelId`
  die korrigierte ist.
- **Wiederverwenden statt duplizieren:** gemeinsame Logik in ein **BKM** auslagern
  (`Bewertung(liste)`), die Decision ruft nur noch `BKM(args)` auf.
- **Dauerhaft/reviewbar:** In-Memory-Modelle überleben Neustarts nur mit `TEMIS_MODELS_DIR`.
  Für versionierte, geprüfte Modelle **git** nutzen (`git_load_model` / `git_propose`).

## Tool-Spickzettel

| Tool | Wofür |
|---|---|
| `list_models` | Modelle finden — je mit `name`, `decisions`, `inputs` (Name nicht eindeutig) |
| `get_model_xml` | rohes DMN/FEEL eines gecachten Modells zurücklesen |
| `describe_decision` | typisierte Inputs, die eine Decision erwartet |
| `evaluate` (`explain`, `strict`) | auswerten + Spur + strikte Eingabevalidierung |
| `load_model` | DMN-XML kompilieren+cachen → neue Revision (erscheint im Modeler) |
| `git_load_model` / `git_propose` | versionierte Modelle aus/als git (Branch/PR) |
| `load_flow` / `evaluate_flow` / `describe_flow` | Decision-Flows (mehrere Modelle verketten) |

## Minimalbeispiel (Ende-zu-Ende)

```text
1. Mensch: „Warum gibt Entscheidung_1 in test23 null zurück?" + modelId aus dem Chip
2. get_model_xml(modelId)                → FEEL lesen
3. evaluate(decision, explain:true)      → Spur: Regel 1 (count(x)>0) matched:false → Unary-Test-Falle
4. XML anpassen: Entscheidung_1 als Literal `if count(Testliste) > 0 then "gut" else "nicht gut"`
   (Namespace 2023, restliche Knoten/Layout unverändert lassen)
5. load_model(xml, name="test23")        → neue Revision
6. evaluate(...)                         → "gut" ✓
7. Mensch: neue modelId nennen, „bitte ⟳ drücken"
```
