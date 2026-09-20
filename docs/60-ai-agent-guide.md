# Leitfaden für KI-Coding-Agenten

> **Zwei Rollen von Agenten — nicht verwechseln (ADR-0013):**
> - **Agent als Contributor** *(dieses Dokument)*: ein Coding-Agent, der temis **baut**.
>   Für ihn gelten die Arbeitsregeln unten.
> - **Agent als Konsument**: ein Agent, der temis zur **Laufzeit nutzt**, um eine
>   regelbasierte Entscheidung zu delegieren und sich gegen ein deterministisches,
>   nachvollziehbares Ergebnis abzusichern. Sein Einstieg ist `temis-mcp` (MCP-Server,
>   WP-50) bzw. der HTTP-Service — siehe README und ADR-0013, nicht dieses Dokument.
>   Baut ein solcher Agent **gemeinsam mit einem Menschen** Decisions (Agent via MCP,
>   Mensch via Modeler), gilt zusätzlich der Skill
>   `.claude/skills/temis-decision-modeling/` — der Zusammenarbeits-Vertrag (Modell per
>   `modelId`/Chip finden, `get_model_xml` lesen, `evaluate`/`explain` diagnostizieren,
>   als neue Version zurückgeben) wird beim MCP-`initialize` auch als `instructions`
>   ausgeliefert.
>
> Dieses Dokument richtet sich an Claude Code (o. ä.) in der **Contributor**-Rolle. Es ist
> bewusst direktiv und deterministisch formuliert. Wenn etwas hier mit deinem „Bauchgefühl"
> kollidiert, gewinnt dieses Dokument — außer die DMN-Spec/das TCK widerspricht, dann
> gewinnt die Spec.

## 1. Arbeitsschleife (für jedes Arbeitspaket)

1. **Kontext laden:** `00-overview.md`, `10-architecture.md`, dieses Dokument, alle ADRs.
   Für FEEL-WPs zusätzlich `30-feel-spec.md`; für API-WPs `40-api-contract.md`.
2. **WP wählen:** oberstes WP in `20-roadmap.md` mit Status `todo` und allen Abhängigkeiten
   `done`. Status auf `in-progress` setzen.
3. **AK in Tests übersetzen:** Schreibe zuerst die Tests, die das Akzeptanzkriterium prüfen.
   Sie müssen rot sein.
4. **Implementieren** bis Tests grün.
5. **`make verify`** muss grün sein (fmt, vet, lint, race, bench-smoke, tck-smoke).
6. **Doku:** GoDoc-Kommentare an allen exportierten Symbolen. Bei
   architekturrelevanten Entscheidungen ein ADR ergänzen.
7. **Status auf `done`** setzen, kurze Notiz, was gemacht wurde. Ein Commit pro WP
   (oder logischer Teilschritt), aussagekräftige Message.

## 2. Goldene Regeln

1. **Korrektheit vor Geschwindigkeit.** Optimiere nie auf Kosten der TCK-Konformität.
   Erst korrekt + getestet, dann mit Benchmark belegt optimieren.
2. **Spec ist die Wahrheit.** Bei FEEL/DMN-Fragen: DMN-1.5-Spec + TCK. Nicht raten,
   nicht „plausibel" implementieren. Unsicherheit → im Code als `// SPEC?:`-Kommentar
   markieren und einen failing/skip-Test mit Spec-Referenz hinterlassen.
3. **Keine Schein-Implementierung.** Kein `panic("TODO")` in gemergtem Code, keine
   stillschweigend falschen Defaults. Lieber ein klar dokumentierter, getesteter
   Teil-Scope als ein unvollständiges Ganzes, das so tut als ob.
4. **`internal/` ist privat.** `service/`, `cmd/` greifen nur über `package dmn` zu.
5. **Number ist Decimal, niemals float64** (außer explizit am Rand mit dokumentierter
   Konvertierung).
6. **Keine ungeprüften externen Abhängigkeiten.** Erlaubt: Standardbibliothek,
   Decimal-Lib (ADR-0007), Test-/Lint-Tools. Neue Dependency = ADR + Begründung.
7. **Eingaben sind feindlich.** Jeder Pfad, der fremdes XML/FEEL verarbeitet, muss
   Limits respektieren (ADR-0008). Kein unbeschränkter Rekursion/Allokation.

## 3. Code-Konventionen

- Go-Idiomatik: `gofmt`, `go vet`, `golangci-lint` (oder `staticcheck`) sauber.
- Fehler mit Kontext: `fmt.Errorf("compile decision %q: %w", id, err)`.
- Keine globalen veränderlichen Zustände. Statische Built-in-Tabellen sind ok.
- Exportierte Symbole: vollständige GoDoc-Kommentare beginnend mit dem Symbolnamen.
- Tests: tabellengetrieben, `t.Run(name, ...)` mit sprechenden Namen.
- Dateigröße moderat halten; ein Konzept pro Datei (erleichtert spätere KI-Edits).

## 4. Definition of Done (pro WP, Checkliste)

- [ ] Tests existieren, decken das Akzeptanzkriterium ab, waren zuerst rot.
- [ ] Implementierung vollständig im WP-Scope (keine versteckten TODOs).
- [ ] `make verify` grün (inkl. `-race`).
- [ ] Coverage der berührten Pakete ≥ Zielmarke (50-testing-strategy §8).
- [ ] Benchmarks vorhanden, wo das WP Hot-Path-Code berührt; keine Regression.
- [ ] GoDoc vollständig; ggf. ADR ergänzt.
- [ ] Roadmap-Status aktualisiert.

## 5. Umgang mit Mehrdeutigkeit

- **Spec-mehrdeutig?** Wähle die TCK-konforme Auslegung. Gibt es keinen TCK-Case,
  dokumentiere die Annahme in `docs/tck-exceptions.md` (oder einem ADR) und teste sie
  explizit, damit sie sichtbar ist.
- **Architektur-Frage offen?** Lege ein ADR mit Status `proposed` an, beschreibe Optionen
  und eine Empfehlung — nicht still entscheiden.
- **WP zu groß?** Splitte in Teil-WPs (z. B. `WP-21a/21b`), behalte AK bei, dokumentiere
  den Split in der Roadmap.

## 6. Was ein Agent NICHT tun darf

- Den FEEL-Number-Typ auf float64 „vereinfachen".
- Built-ins „nach Gefühl" implementieren ohne Spec-Beispiel-Test.
- `internal/`-Strukturen aus `service/`/`cmd/` direkt verdrahten.
- Die öffentliche API (`40-api-contract.md`) ohne ADR brechen.
- Tests löschen/„grün machen", statt den Code zu fixen.
- TCK-Cases als „not applicable" markieren, nur um die Quote zu schönen.

## 7. Reihenfolge-Empfehlung beim Kaltstart

WP-01 → WP-02 → (WP-03 → WP-04 → WP-05 → WP-06 → WP-07) → WP-08 → WP-09 → WP-10 → WP-11.
Damit ist der MVP erreicht und liefert sofort sichtbaren Wert (lädt echte dmn-js-Dateien).

## 8. Agent als Konsument: delegieren → protokollieren

> Dieser Abschnitt betrifft die **zweite** Rolle aus dem Callout oben — den Agenten, der
> temis zur **Laufzeit nutzt**, nicht baut. Er gehört hierher, weil viele Coding-Agenten
> beide Hüte tragen und das Muster kennen sollten.

Ein Agent, der vor einer **regelbasierten** Entscheidung steht (Förderfähigkeit, Tarif,
Bonität, Routing, Berechtigung), soll sie **nicht raten**. Das verlässliche Muster ist
zweistufig und braucht **keinen** neuen temis-Code (ADR-0013/0023):

1. **Delegieren:** die Entscheidung an temis geben — MCP-Tool `evaluate` (oder
   `POST /v1/evaluate`), gern mit `explain: true` (Begründung) und `strict: true`
   (Eingabe-Vorabprüfung). Ergebnis ist eine **deterministische, reproduzierbare**
   Grundwahrheit, gegen die der Agent sein Handeln absichert.
2. **Protokollieren:** das Ergebnis als `com.temis.decision.evaluated.v1`-CloudEvent in
   clio schreiben (`write-events`) — revisionssicher, hash-verkettet, später per
   `temis-reaudit` **nachrechenbar**.

> *Themis entscheidet, Clio merkt es sich.* Alternativ übernimmt der `temisd`-Sink
> (`-clio-url`) Schritt 2 serverseitig, ohne dass der Agent selbst schreibt.

**Lauffähiges Beispiel, Vertrag, Idempotenz und Betriebshinweise:**
`docs/80-clio-decision-log.md` (Abschnitt 5). Grundsatzentscheidung: **ADR-0023**;
temis als Verifikationsorakel: **ADR-0013**.
