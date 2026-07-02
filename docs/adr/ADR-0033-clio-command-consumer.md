# ADR-0033: Clio-Command-Consumer — Entscheidungen per Event auslösen, Ergebnis zurück ins Logbuch (opt-in, adaptergetrieben)

- **Status:** proposed
- **Datum:** 2026-07-02
- **Kontext-WP:** neue Etappe „Command-Consumer" (Ausbau von ADR-0023, im Rahmen von ADR-0025)

## Kontext

ADR-0023 verdrahtet **eine Richtung**: temis wertet aus, der `ClioSink` schreibt das
Ergebnis als revisionssicheres CloudEvent nach clio (`com.temis.decision.evaluated.v1` /
`com.temis.flow.evaluated.v1`). `temis-reaudit` liest die Events **read-only** zurück und
rechnet sie nach. Was fehlt, ist die **Gegenrichtung**: ein in clio geschriebenes Event
soll eine Auswertung **auslösen**, und deren Ergebnis soll — korreliert — wieder in clio
landen. In CQRS-Sprache: **Commands** lösen einen DRG aus, **Events** protokollieren die
Entscheidung.

Das ist ein natürliches Muster über einem Event Store: ein Umsystem (eine App, ein Agent,
ein anderer Dienst) schreibt ein Command-Event „entscheide das für Entität X" auf einen
clio-Subject, ohne temis direkt zu kennen; ein Consumer beantwortet es. So wird clio zur
**entkoppelnden Naht** zwischen Auslöser und Entscheider — dieselbe Naht, die schon das
Logbuch trägt.

**Spannungsfeld zu ADR-0025.** ADR-0025 zieht eine harte Grenze: auf **Events reagieren**,
**warten**, **wiederholen**, **Zustand halten** ist **Prozess-Orchestrierung** (chrampfer/
L2b), ausdrücklich **nicht** temis. Ein „Event triggert eine Decision" klingt zunächst
genau danach. Der Litmus-Test entscheidet: Muss sich ein Schritt zwischen Aufrufen etwas
**merken**, **warten** oder auf Folge-Events reagieren → Prozess. Ist es „gegeben diese
Inputs, berechne jetzt die Antwort" → Decisioning.

Ein Command-Consumer, der **ein** Command liest, es **zustandslos** auswertet und **ein**
Ergebnis zurückschreibt, fällt auf die **Decisioning-Seite**: keine Timer, kein Warten auf
Folge-Events, keine Kompensation, kein durable Fall-Zustand. **clio** hält den gesamten
Zustand — das Command-Log, das Result-Log und die „schon beantwortet?"-Query. Der Consumer
ist ein **reiner `event → evaluate → event`-Transform**. Die Kunst dieser ADR ist, diese
Grenze so zu ziehen, dass der Consumer nicht schleichend zur Mini-Prozess-Engine wird.

**Spannungsfeld zu ADR-0011 / ADR-0005.** Wie beim Sink gilt: kein clio-/HTTP-Import im
Engine-Kern (`package dmn`), keine neue Pflichtabhängigkeit, kein verändertes
Default-Verhalten. Die Reaktivität lebt **außerhalb** des Kerns.

## Optionen

1. **Reaktivität in `temisd` / den Kern bauen** (ein „Watch clio"-Modus mit Fall-Zustand,
   Retry-Policy, evtl. Timern). — Bricht ADR-0025 (temisd würde reaktiv, der Grat zur
   Prozess-Engine ist schmal) und ADR-0011 (Zustand/Zeit im bzw. nahe am Kern). Verworfen.
2. **Reines Agent-/Umsystem-Muster (nur Doku).** Ein Agent liest selbst aus clio, ruft
   `temis.evaluate`, schreibt selbst zurück — kein temis-Code. — Stark und **bleibt gültig**
   als Muster (wie ADR-0023 §5), aber ohne serverseitige Option bleibt jede nicht-Agent-
   Integration ohne fertigen Consumer. Als **ergänzend** dokumentiert, nicht als Ersatz.
3. **Eigenständiger, zustandsloser Consumer als Adapter/Tool** (diese Entscheidung). Ein
   **stabiler, versionierter Command-Event-Vertrag** plus ein **eigenes Binary**
   (`temis-clio-worker`), das über die **öffentliche `dmn`/`flow`-API** auswertet — kein
   `internal/`-Import, kein Zustand, symmetrisch zu `temis-reaudit`. Kopplung nur über den
   **HTTP-Vertrag** von clio (`observe`/`run-query` lesen, `write-events` schreiben).
4. **Opt-in-Hintergrund-Worker in `temisd`.** Bequem (ein Prozess, geteilter Cache), aber
   macht `temisd` reaktiv und rückt an die ADR-0025-Grenze. Als spätere additive Option
   möglich, wenn der Consumer sich bewährt; **nicht** der erste Schritt.

## Entscheidung

**Option 3, ergänzt um Option 2 als dokumentiertes Muster.** Herzstück ist ein
**versionierter Command-Event-Vertrag**; der Consumer ist ein dünnes, opt-in, zustandsloses
Adapter-Binary — kein Kern-Feature.

### 1. Der Command-Event-Vertrag (das eigentliche Artefakt)

Ein Umsystem schreibt ein CloudEvent, das clio unverändert annimmt:

| CloudEvents-Feld | Inhalt |
|---|---|
| `source` | der Auslöser (frei, z. B. eine App/ein Agent) |
| `subject` | die **Geschäftsentität** (clio-Pfad), z. B. `/orders/42` — die Ergebnis-Events werden **unter demselben Subject** abgelegt (Korrelation) |
| `type` | `com.temis.decision.requested.v1` (versioniert) |
| `data.modelId` | content-addressed Modell-ID (für Einzel-Decision **oder** ganzen Graph) |
| `data.flowId` | content-addressed Flow-ID (für einen Decision-Flow/DRG) — schließt `modelId` aus |
| `data.decision` | mit `modelId`: Name der Einzel-Decision; **weggelassen** ⇒ ganzer Modell-Graph |
| `data.input` | die Eingabe (FEEL-Werte) |
| `data.explain` | opt-in: Entscheidungsspur ins Ergebnis übernehmen |

**Diskriminator:** `flowId` gesetzt → Flow → ein `com.temis.flow.evaluated.v1`.
`modelId` + `decision` → Einzel-Decision → ein `com.temis.decision.evaluated.v1`.
`modelId` allein → ganzer Graph → **ein `evaluated.v1` je Decision** (wie der
Evaluate-Graph-Pfad des Sinks, ADR-0023 §2).

**Ergebnis-Events** sind exakt der bestehende Vertrag aus ADR-0023/WP-93, ergänzt um ein
additives Korrelationsfeld **`data.requestId`** (= die clio-Event-ID des Commands). Damit
sind command-getriebene Ergebnisse **byte-gleich** zu den vom Sink geschriebenen und werden
von `temis-reaudit` **identisch** nachgerechnet. Eine nicht auswertbare Anfrage (Modell/
Flow nicht auflösbar, Compile-/Laufzeitfehler) wird mit **`com.temis.decision.failed.v1`**
beantwortet — so bekommt **jedes** Command eine Antwort (Ergebnis **oder** Fehler).

### 2. Der Consumer `temis-clio-worker` (opt-in, zustandslos)

Ein eigenes Binary (`cmd/temis-clio-worker`, Kern im neuen `package consume`):

- **Zustandslos.** Kein Fall-Zustand, keine Timer, kein Warten auf Folge-Events, keine
  Kompensation. Der einzige Speicher ist ein **In-Process-Dedupe-Set** verarbeiteter
  Command-IDs (reiner Beschleuniger, geht bei Neustart verloren) — die **Wahrheit** über
  „schon beantwortet?" steht in **clio**.
- **Öffentliche API, kein Kern-Import.** `package consume` importiert nur `dmn`, `flow` und
  `audit` (für die content-addressierte Modell-Auflösung) — **kein** `internal/`, **kein**
  `service`. Symmetrisch zu `package audit`/`temis-reaudit`.
- **Live über `observe`, robust über `run-query`.** Neue Commands kommen latenzarm über
  clios **`observe`**-Stream; ein **`run-query`-Backfill** beim Start und bei jeder
  Reconnect fängt alles auf, was während einer Trennung geschrieben wurde. `-poll` bietet
  einen reconnect-freien Polling-Modus, `-once` verarbeitet nur den Rückstand und endet
  (für Cron/Tests).
- **Idempotenz über clio-Preconditions.** Jeder Result-Write trägt eine Precondition auf
  `data.requestId` (und, bei Graph-Commands, `data.decision`), sodass Re-Delivery,
  Backfill-Überlappung oder ein Neustart **nie** doppelt beantworten. Ein `409` ist ein
  erfolgreicher No-op — dasselbe Muster wie der Sink (ADR-0023).
- **Kein Loop.** Der Consumer filtert `observe`/`run-query` hart auf
  `type == 'com.temis.decision.requested.v1'`; seine eigenen Ergebnis-Events (anderer Typ)
  triggern ihn nicht erneut.
- **Kein durable Retry.** Schlägt ein clio-Write fehl, bleibt das Command **unbeantwortet**
  (nicht als verarbeitet markiert); der nächste Backfill holt es nach. Kein eigener
  Retry-Zustand — die Wiederaufnahme ist die read-Seite, nicht ein Timer im Consumer.

### 3. Modell-/Flow-Auflösung

Der Worker löst `modelId`/`flowId` aus einem **lokalen Verzeichnis** auf (`-models`),
content-addressiert mit demselben `sha256:`-Schema wie Server-Cache und `temis-reaudit`
(`audit.ModelID`). `*.dmn`/`*.xml` sind Modelle, `*.flow.json` sind Flow-Deskriptoren.
Das hält den Consumer **read-only bzgl. der Modelle** und unabhängig von einem laufenden
`temisd`.

### 4. Agent-/Umsystem-Muster (nur Doku)

Wie ADR-0023 §5 bleibt das Muster gültig, das **keinen** temis-Code braucht: ein Agent liest
selbst, ruft `evaluate`, schreibt selbst zurück. `temis-clio-worker` ist der serverseitige
Weg für Umsysteme, die nur ein Command-Event schreiben wollen und nichts weiter.

## Konsequenzen

**Positiv**
- Die **Gegenrichtung** zum Logbuch, ohne die ADR-0025-Grenze zu brechen: der Consumer ist
  Decisioning (zustandslos, deterministisch, re-auditierbar), **kein** Prozess.
- clio wird zur **entkoppelnden Naht** zwischen Auslöser und Entscheider; das Umsystem muss
  temis nicht kennen — es schreibt nur ein Event.
- **Keine Architektur-Schuld:** Kopplung nur über clios HTTP-Vertrag; Kern unberührt
  (ADR-0011), reine stdlib, keine neue Go-Abhängigkeit (ADR-0014). Symmetrie zu
  `temis-reaudit` hält die clio-Werkzeuge konsistent (eines liest & prüft, eines liest,
  entscheidet & schreibt).
- Ergebnis-Events erfüllen denselben Vertrag ⇒ `temis-reaudit` prüft auch command-getriebene
  Entscheidungen unverändert.

**Negativ / Kosten**
- Zwei neue Teile der **öffentlichen Oberfläche** unter SemVer: der Command-Event-`type`/
  `data`-Vertrag und das additive `requestId`-Feld der Ergebnis-Events (Änderungen nur
  versioniert, `.v2`). In `docs/40-api-contract.md`/`docs/80` festzuhalten.
- Eine **optionale** Betriebskomponente mehr (ein laufender Worker) inkl. clio-Scopes
  (**read** auf den Command-Subtree **+ write** auf den Result-Subtree) und der
  Auflösbarkeit der referenzierten Modelle/Flows im `-models`-Verzeichnis.
- Die Zustandslosigkeit ist eine **Disziplin-Grenze**: „mal eben" Timer/Fall-Zustand in den
  Worker zu bauen, verschöbe ihn nach L2b und bräche ADR-0025. Bewusst ausgelassen.
- **`observe`-Vertrag:** der genaue clio-`observe`-Endpunkt ist außerhalb dieses Repos
  spezifiziert; der Worker macht den Pfad konfigurierbar (`-observe-path`) und degradiert
  sauber auf `run-query` (`-poll`), der verifiziert funktioniert.

**Folgeaufgaben**
- `docs/80-clio-decision-log.md`: die Gegenrichtung (Command-Vertrag, Consumer, Idempotenz,
  Sicherheit/Scopes) dokumentieren.
- `docs/40-api-contract.md`: Command-Event + `requestId` als Teil der stabilen Oberfläche.
- Optional später: opt-in-Modus in `temisd` (Option 4), falls sich der Consumer bewährt.
- `docs/60-ai-agent-guide.md`: das Umsystem-Muster „Command schreiben → Antwort beobachten".
