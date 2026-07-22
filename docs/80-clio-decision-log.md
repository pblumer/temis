# Entscheidungs-Logbuch mit clio (revisionssicheres Audit)

> **Status:** Der **`temisd`-Sink (Abschnitt 2, WP-54)** und das
> **Re-Audit-Tool (Abschnitt 4, WP-55)** sind umgesetzt. Das **Agent-Muster**
> (Abschnitt 5) funktioniert ganz ohne neuen temis-Code. Die **Gegenrichtung**
> (Abschnitt 6, `temis-clio-worker`) löst Entscheidungen per Command-Event aus.
> Vertrag & Begründung: ADR-0023, ADR-0033.

temis trifft **deterministische, begründete** Entscheidungen (ADR-0013); das
Schwesterprojekt **[clio](https://github.com/pblumer/clio)** (`cliostore`) ist ein
abhängigkeitsfreier, **append-only Event Store** mit **SHA-256-Hash-Kette**, optionalen
**Ed25519-Signaturen**, Preconditions, CEL-Queries und Live-`observe`. Zusammen ergeben
sie ein **manipulationssicheres, reproduzierbares Entscheidungs-Logbuch**:

- clios Hash-Kette beweist: *der Eintrag wurde nicht nachträglich verändert.*
- temis' Determinismus beweist: *die Entscheidung war zur damaligen Modellversion
  regelkonform* (per Re-Audit nachrechenbar, Abschnitt 4).

> **Themis** trifft das Urteil, **Clio** schreibt die Geschichte auf.

Architekturprinzip (ADR-0023): Die Kopplung läuft **ausschließlich über clios
HTTP-API** — kein Go-Import, kein gemeinsamer Prozess, keine neue Pflichtabhängigkeit.
Der Engine-Kern (`package dmn`) bleibt unberührt (ADR-0011); die Emission lebt opt-in in
der Adapter-Schicht (`temisd`). Beide bleiben unabhängige, dependency-freie Single-
Binaries.

---

## 1. Der Decision-Event-Vertrag

Jede Auswertung wird auf ein **CloudEvent** abgebildet, das clio über `write-events`
unverändert annimmt. Der `type` ist **versioniert** (`.v1`) und unterliegt SemVer
(ADR-0019, `docs/40-api-contract.md`).

```json
{
  "source":      "temisd",
  "subject":     "/orders/42",
  "type":        "com.temis.decision.evaluated.v1",
  "clioauthkid": "k_ci01",
  "data": {
    "modelId":  "sha256-1f3a…",
    "decision": "Dish",
    "input":    { "Season": "Winter", "Guest Count": 8 },
    "outputs":  { "Dish": "Roastbeef" },
    "trace":    { "...": "opt-in: welche Regeln warum gefeuert haben" },
    "engine":   "temisd v1.2.3",
    "strict":   true
  }
}
```

`id`, `time`, `specversion` und die Hash-Kette ergänzt **clio** beim Schreiben — temis
liefert nur `source`/`subject`/`type`/`data`.

### Feld-Referenz

| Feld | Quelle in temis | Hinweis |
|---|---|---|
| `source` | Konfiguration (`-clio-source`, Default `temisd`) | identifiziert die schreibende Instanz |
| `subject` | Mapping aus der Eingabe/Decision (Abschnitt 3) | die clio-Geschäftsentität, hierarchischer Pfad |
| `type` | konstant `com.temis.decision.evaluated.v1` | versioniert; Schema-Änderung ⇒ `.v2` |
| `clioauthkid` | `kid` des authentifizierenden API-Keys (WP-105, ADR-0028) | CloudEvents-**Extension** für Authorship; **ausgelassen** bei offener API/Legacy-Token; clio bindet sie in die Hash-Kette. Kennt eine clio die Extension nicht (alter Stand oder strenges Event-Schema) und lehnt den Write mit `400 unknown field "clioauthkid"` ab, schreibt der Sink das Event **einmal** ohne die Extension nach und stempelt Authorship danach nicht mehr — der Audit-Trail geht nie verloren, nur der Autorenstempel. Mit `-clio-authorship=false` / `TEMIS_CLIO_AUTHORSHIP=false` lässt sich das Stempeln vorab abschalten. |
| `data.modelId` | content-addressed `modelId` (`load_model` / Modell-Cache) | **exakte** Modellversion ⇒ reproduzierbar |
| `data.decision` | `decision`-Argument der Auswertung | Name oder ID |
| `data.input` | die übergebene `Input` | FEEL-Numbers als exakter **Dezimal-String** (ADR-0007) |
| `data.outputs` | `Result.Outputs` | dito für numerische Outputs |
| `data.trace` | `Result.Trace` (WP-51) | **nur** wenn `explain`/Spur-Logging aktiv |
| `data.engine` | `internal/version` | Nachvollziehbarkeit der Engine-Version |
| `data.strict` | ob `WithStrictInput()` aktiv war (WP-52) | dokumentiert die Validierungsstrenge |

> **Optionales clio-Schema.** Der `data`-Vertrag lässt sich in clio als JSON Schema
> hinterlegen (`register-event-schema` für `com.temis.decision.evaluated.v1`), sodass
> clio fehlerhafte Events schon beim Schreiben ablehnt.

---

## 2. Opt-in-Sink in `temisd` (WP-54, umgesetzt)

`temisd` emittiert das Event **nach** jeder Einzel-Decision-Auswertung
(`POST /v1/evaluate`, `POST /v1/models/{id}/evaluate`) sowie nach jeder
**Whole-Graph-Auswertung** (`POST /v1/models/{id}/evaluate-graph` — der „Auswerten"-Pfad
des Modelers; **ein Decision-Event je ausgewerteter Decision**) — nur wenn konfiguriert.
Default aus ⇒ Verhalten **byte-identisch** zu heute.

```sh
temisd -addr :8080 \
  -clio-url            http://127.0.0.1:3000 \
  -clio-token          kid_ci01.dein-geheimnis \
  -clio-source         temisd-prod \
  -clio-subject-prefix /decisions \
  -clio-subject-key    "Order ID" \
  -clio-strict=false
# entsprechend per Env: TEMIS_CLIO_URL / TEMIS_CLIO_TOKEN / TEMIS_CLIO_SOURCE
```

| Flag / Env | Default | Bedeutung |
|---|---|---|
| `-clio-url` / `$TEMIS_CLIO_URL` | — (aus) | Basis-URL der clio-Instanz. Leer ⇒ Sink **aus**. |
| `-clio-token` / `$TEMIS_CLIO_TOKEN` | — | clio-API-Key (`kid.secret`); eng gescopt, siehe Abschnitt 3. |
| `-clio-source` / `$TEMIS_CLIO_SOURCE` | `temisd` | CloudEvents-`source`. |
| `-clio-subject-prefix` | `/decisions` | Subject-Präfix (Abschnitt 3). |
| `-clio-subject-key` | — | Eingabefeld als Entitäts-Segment; leer ⇒ Decision-Name. |
| `-clio-strict` | `false` | `true` = fail-closed (siehe unten). |

Eigenschaften (verbindlich laut ADR-0023):

- **Auswertung führt, Audit folgt.** Ein clio-Fehler verfälscht das Ergebnis nie. Die
  Politik ist konfigurierbar:
  - *best-effort* (Default): Auswertung antwortet normal (`200`), Sink-Fehler wird
    geloggt.
  - *strikt / fail-closed* (`-clio-strict`): Schlägt das Schreiben fehl, schlägt der
    Request mit `502 AUDIT_WRITE_FAILED` fehl (für Domänen, in denen „nicht
    protokolliert = nicht entschieden" gilt). Ein `409` der Precondition (bereits
    protokolliert) gilt **immer** als Erfolg.
- **Reine stdlib** (`net/http`) — keine neue Go-Abhängigkeit, kein Go-Bump (ADR-0014).
  Die Kopplung lebt im Adapter (`service`), nicht im Engine-Kern (`package dmn`).
- **Spur opt-in:** `data.trace` wird nur gefüllt, wenn die Spur ohnehin angefordert ist
  (`explain`) — der allokationsarme Default-Pfad (WP-42) bleibt unangetastet.

### Idempotenz über Preconditions

Damit Retries (Netz-Timeout, At-least-once) **keine** Doppel-Einträge erzeugen, schreibt
der Sink mit einer clio-**Precondition**: nur schreiben, wenn für dieselbe Entität und
denselben Eingabe-Hash noch kein Event existiert.

```json
{
  "events": [ { "source":"temisd", "subject":"/orders/42",
                "type":"com.temis.decision.evaluated.v1", "data": { "...": "..." } } ],
  "preconditions": [
    { "type":"isQueryResultEmpty",
      "payload": { "subject":"/orders/42",
                   "where":"event.type == 'com.temis.decision.evaluated.v1' && event.data.inputHash == '…'" } }
  ]
}
```

Schlägt die Precondition fehl, antwortet clio mit **409** und schreibt nichts — der Sink
wertet das als „bereits protokolliert" (Erfolg).

---

## 3. Subject-Mapping & Sicherheit

Das **`subject`** ist der Dreh- und Angelpunkt: Es bestimmt, unter welcher
Geschäftsentität die Entscheidung in clio einsortiert wird, und damit auch, was clios
`GET /state/<subject>` und Reduce-Specs später falten können.

- **Empfohlen:** Mapping aus einem Eingabefeld, z. B. `/orders/{Order ID}` oder
  `/customers/{Customer ID}/decisions`. So liegen alle Entscheidungen einer Entität auf
  einem Stream und sind per `read-events`/`state` direkt abrufbar.
- **Alternativ:** nach Modell/Decision gruppieren, z. B.
  `/decisions/{decision}/{entityId}`.

**clio-Scopes (ADR-025 in clio).** Der `-clio-token` sollte ein **eng gescopter
Write-Key** sein, idealerweise auf den Subject-Teilbaum beschränkt
(`write:/decisions/*` bzw. `write:/orders/*`) — nicht der Admin-Key.

**Datensensibilität.** `data.input`/`data.trace` können fachlich sensibel sein. Vor dem
Aktivieren klären: Welche Felder dürfen ins Logbuch? Bei Bedarf Subject/Felder
einschränken. clios **Signaturen** (`CLIO_SIGNING_KEY`) und **Authorship**
(`CLIO_EVENT_AUTHORSHIP`) erhöhen die Beweiskraft zusätzlich.

### Quality-Events (Import-Cockpit, ADR-0031)

Neben dem Decision- und dem Flow-Event schreibt ein **Produktivlauf** des Import-Cockpits
ein **Quality-Event** `com.temis.quality.evaluated.v1` — **auf der Entität** (Subject
`/quality/<entity>`), nicht pro Decision. Es hält Modell, Entität, Fall, Eingabe,
Ergebnisse und die erwarteten Werte fest sowie ein **`violation`-Flag** (`true`, wenn ein
erwarteter Wert verletzt wurde; `false` bei Deckung; weggelassen ohne Erwartung). So lassen
sich **Verletzungen je Entität** reporten, z. B. per clio-Query
`event.type == 'com.temis.quality.evaluated.v1' && event.data.violation == true`.
Idempotenz wie sonst über die Precondition (Subject + `data.inputHash`). Die Zustellung
läuft **entkoppelt über eine garantierte Queue mit Backpressure** (`QualityQueue`): der
schnelle Batch-Response wartet nicht auf clio; Hintergrund-Worker liefern mit Retry, und
`temisd` drainiert die Queue beim Graceful-Shutdown. Ein **Testlauf** schreibt **nichts**.

---

## 4. Re-Audit / Replay-Verifikation (WP-55, umgesetzt)

Der eigentliche Mehrwert gegenüber einem reinen Log: weil temis **deterministisch** ist,
lässt sich jede historische Entscheidung **nachrechnen**. Das Binary **`temis-reaudit`**
(`cmd/temis-reaudit`, Kern in `package audit`) tut genau das — **read-only**, es schreibt
nie zurück:

1. `run-query` auf clio mit `where: event.type == 'com.temis.decision.evaluated.v1'`.
2. Für jedes Event: `data.input` + `data.modelId` erneut über die `dmn`-API auswerten.
3. Ist-Ausgabe **kanonisch (JSON)** mit `data.outputs` vergleichen.

```sh
temis-reaudit \
  -clio-url   http://127.0.0.1:3000 \
  -clio-token kid_ro.secret \
  -models     ./models            # Verzeichnis der DMN-Dateien (löst modelId auf)
# → re-audited 127 decision event(s) against 9 model(s): 127 reproduced — OK ✓
# Exit 0 = alles reproduziert, 1 = mind. eine Abweichung/Fehler (skriptbar wie clios verify).
```

Je Event eine Verdikt-Klasse: **reproduced** ✓, **discrepancy** ✗ (Ausgabe weicht ab),
**model_unavailable** (modelId nicht im `-models`-Verzeichnis — inkonklusiv) oder
**eval_error** (Compile/Decision/Evaluate schlägt fehl). Die Modell-Auflösung läuft über
content-addressed `modelId`: `temis-reaudit` hasht die DMN-Dateien mit demselben
`sha256:`-Schema wie der Server-Cache, sodass eine zur Aufzeichnung passende Modellversion
exakt gefunden wird. **Abgrenzung:** clios `verify` prüft Hash-Kette und Signaturen
(*unverändert?*); `temis-reaudit` ergänzt den orthogonalen *Regelkonformitäts*-Beweis
(*korrekt gerechnet?*).

```text
clio  ──(run-query: decision events)──▶  temis-reaudit  ──(re-evaluate input@modelId)──▶  dmn
                                              │
                                              ▼
                        Report: reproduced ✓ / discrepancy ✗ / unavailable / eval_error
```

---

## 5. Agent-Muster: delegieren → protokollieren (heute nutzbar)

Für KI-Agenten (ADR-0013) braucht es **keinen** neuen temis-Code. Ein Agent, der eine
regelbasierte Entscheidung nicht „raten" will, **delegiert** sie an temis und
**protokolliert** das Ergebnis anschließend selbst in clio — beides über bestehende
Werkzeuge:

```text
Agent
  │  1) temis.evaluate(modelId, input, explain=true)   ── deterministische, begründete Antwort
  │  2) clio.write-events(subject, type, data=…)        ── revisionssicher protokolliert
  ▼
```

So entsteht dasselbe Logbuch wie über den `temisd`-Sink (Abschnitt 2) — nur agenten-
statt servergetrieben. Der Sink ist der serverseitige Weg für nicht-Agent-Integrationen
(HTTP-Clients, Batch); dieses Muster ist der Weg für einen Agenten, der die Entscheidung
ohnehin schon in der Hand hält.

### Schritt 1 — an temis delegieren

Über das temis-MCP-Tool `evaluate` (oder `POST /v1/evaluate`). `explain: true` liefert die
Spur mit, `strict: true` prüft die Eingabe vorab:

```jsonc
// MCP-Tool-Aufruf: evaluate
{ "modelId": "sha256:1f3a…", "decision": "Dish",
  "input": { "Season": "Winter", "Guest Count": 8 },
  "explain": true }
// → { "outputs": { "Dish": "Roastbeef" }, "trace": { … } }
```

### Schritt 2 — in clio protokollieren

Das Ergebnis aus Schritt 1 wird 1:1 in den Decision-Event-Vertrag (Abschnitt 1) gegossen
und an clios `write-events` geschickt (MCP-Tool bzw. `POST /api/v1/write-events`):

```bash
curl -X POST http://127.0.0.1:3000/api/v1/write-events \
  -H "Authorization: Bearer kid_agent.secret" -H "Content-Type: application/json" \
  -d '{"events":[{
        "source":"claude-agent","subject":"/orders/42",
        "type":"com.temis.decision.evaluated.v1",
        "data":{"modelId":"sha256:1f3a…","decision":"Dish",
                "input":{"Season":"Winter","Guest Count":8},
                "outputs":{"Dish":"Roastbeef"}}}]}'
```

Für **Idempotenz** kann der Agent — genau wie der Sink — eine Precondition mitschicken
(Abschnitt 2), sodass ein wiederholter Aufruf denselben Eintrag nicht dupliziert. Später
verifiziert `temis-reaudit` (Abschnitt 4) auch diese agenten-geschriebenen Events, weil
sie denselben Vertrag erfüllen.

> **Themis entscheidet, Clio merkt es sich** — ohne dass der Agent selbst zur
> Entscheidungslogik oder zum Speicher wird.

---

## 6. Gegenrichtung: Entscheidungen per Command-Event auslösen (ADR-0033)

Die Abschnitte 1–5 beschreiben **eine** Richtung: temis wertet aus, das Ergebnis fliesst
als Event nach clio. ADR-0033 ergänzt die **Gegenrichtung**: ein in clio geschriebenes
**Command-Event** löst eine Auswertung aus, und deren Ergebnis landet — korreliert — wieder
im Logbuch. In CQRS-Sprache: **Commands** lösen einen DRG aus, **Events** protokollieren die
Entscheidung. So wird clio zur **entkoppelnden Naht**: ein Umsystem (App, Agent, Dienst)
schreibt nur „entscheide das für Entität X" und muss temis nicht kennen.

> **Abgrenzung (ADR-0025).** Der Consumer ist **zustandslos** — ein reiner
> `event → evaluate → event`-Transform: kein Warten, keine Timer, kein Fall-Zustand, keine
> Kompensation. clio hält den gesamten Zustand (Command-Log, Result-Log, „schon
> beantwortet?"-Query). Damit bleibt er **Decisioning** (deterministisch, re-auditierbar) und
> wird **nicht** zur Prozess-Engine (das ist chrampfer/L2b).

### Der Command-Event-Vertrag

```json
{
  "source":  "orders-app",
  "subject": "/orders/42",
  "type":    "com.temis.decision.requested.v1",
  "data": {
    "modelId":  "sha256:1f3a…",
    "decision": "Dish",
    "input":    { "Season": "Winter", "Guest Count": 8 },
    "explain":  true
  }
}
```

| `data`-Feld | Bedeutung |
|---|---|
| `modelId` | content-addressed Modell — für eine **Einzel-Decision** oder den **ganzen Graph** |
| `flowId` | content-addressed Flow (**DRG**) — schliesst `modelId` aus |
| `decision` | mit `modelId`: Name der Einzel-Decision; **weggelassen** ⇒ ganzer Modell-Graph (ein Ergebnis-Event je Decision) |
| `input` | die FEEL-Eingabe |
| `explain` | opt-in: Entscheidungsspur ins Ergebnis übernehmen |

**Diskriminator:** `flowId` → Flow (`com.temis.flow.evaluated.v1`); `modelId`+`decision` →
Einzel-Decision (`com.temis.decision.evaluated.v1`); `modelId` allein → ganzer Graph (je
Decision ein `evaluated.v1`).

Die **Ergebnis-Events** sind exakt der Vertrag aus Abschnitt 1 / WP-93, ergänzt um ein
additives Korrelationsfeld **`data.requestId`** (= die clio-Event-ID des Commands) und
**unter demselben `subject`** wie das Command abgelegt. Dadurch sind command-getriebene
Ergebnisse byte-gleich zu den vom Sink geschriebenen und werden von `temis-reaudit`
**identisch** nachgerechnet. Eine nicht auswertbare Anfrage wird mit
**`com.temis.decision.failed.v1`** beantwortet — so bekommt **jedes** Command eine Antwort.

### Der Consumer `temis-clio-worker`

Ein eigenständiges Binary (`cmd/temis-clio-worker`, Kern im `package consume` — nur `dmn`/
`flow`/`audit`, **kein** `internal/`, **kein** `service`), symmetrisch zu `temis-reaudit`:

```sh
temis-clio-worker \
  -clio-url    http://127.0.0.1:3000 \
  -clio-token  kid_worker.secret \
  -models      ./models            # *.dmn/*.xml + *.flow.json, content-addressed
# → beobachtet Command-Events (observe), wertet aus, schreibt evaluated.v1 zurück
```

| Flag / Env | Default | Bedeutung |
|---|---|---|
| `-clio-url` / `$TEMIS_CLIO_URL` | — | Basis-URL der clio-Instanz. |
| `-clio-token` / `$TEMIS_CLIO_TOKEN` | — | clio-Key mit **read** (Command-Subtree) **+ write** (Result-Subtree). |
| `-clio-source` / `$TEMIS_CLIO_SOURCE` | `temis-clio-worker` | CloudEvents-`source` der Ergebnis-Events. |
| `-models` / `$TEMIS_MODELS_DIR` | — | Verzeichnis der Modelle/Flows, die `modelId`/`flowId` auflösen. |
| `-subject` | `/` | clio-Subject-Scope, der beobachtet wird. |
| `-observe-path` | `/api/v1/observe` | clio-Live-`observe`-Route. |
| `-poll` / `-poll-interval` | aus / `2s` | statt `observe` per `run-query` pollen (reconnect-frei). |
| `-once` | aus | nur den Rückstand verarbeiten und beenden (Cron/Test). |

Verbindliche Eigenschaften (ADR-0033):

- **Zustandslos.** Einziger Speicher ist ein In-Process-Dedupe-Set (Beschleuniger, geht bei
  Neustart verloren); die Wahrheit über „schon beantwortet?" steht in clio.
- **Live + robust.** Neue Commands kommen latenzarm über `observe`; ein `run-query`-Backfill
  beim Start und bei jeder Reconnect fängt auf, was während einer Trennung geschrieben wurde.
- **Idempotenz über Preconditions.** Jeder Result-Write trägt eine Precondition auf
  `data.requestId` (bei Graph-Commands zusätzlich `data.decision`). Re-Delivery,
  Backfill-Überlappung oder Neustart beantworten **nie** doppelt; `409` = erfolgreicher
  No-op — dasselbe Muster wie der Sink.
- **Kein Loop.** `observe`/`run-query` filtern hart auf `type == 'com.temis.decision.requested.v1'`;
  die eigenen Ergebnis-Events (anderer Typ) triggern den Worker nicht erneut.
- **Kein durable Retry.** Schlägt ein clio-Write fehl, bleibt das Command unbeantwortet (nicht
  als verarbeitet markiert); der nächste Backfill holt es nach — die Wiederaufnahme ist die
  read-Seite, kein Timer im Consumer.

**Sicherheit/Scopes.** Der Worker-Key braucht **read** auf den Command-Subtree und **write**
auf den Result-Subtree (clio-Scopes, ADR-025 in clio). Wie beim Sink können `input`/`trace`
fachlich sensibel sein — Subject-Mapping und Feldumfang gehören in den Betriebsleitfaden.

### Schema-Validierung an der Grenze (optional, empfohlen)

Die `data`-Verträge liegen maschinenlesbar als **JSON Schema** in
[`docs/schemas/`](schemas/) (`com.temis.decision.requested.v1`, `…evaluated.v1`,
`…flow.evaluated.v1`, `…failed.v1`). Registriert man das **Command**-Schema in clio
(`register-event-schema`), lehnt clio ein fehlerhaftes Command **schon beim Schreiben** ab —
statt es erst nachgelagert im Worker als `failed.v1` zu quittieren:

```jsonc
// clio-MCP-Tool: register_event_schema
{ "type": "com.temis.decision.requested.v1",
  "schema": { /* Inhalt von docs/schemas/com.temis.decision.requested.v1.schema.json */ } }
```

Das Command-Schema erzwingt den Diskriminator (genau eines von `modelId`/`flowId`,
`decision` nur mit `modelId`) per `oneOf` und ist streng (`additionalProperties: false`),
sodass Tippfehler wie `modelID` an der Quelle auffallen. Die **Ergebnis**-Schemas sind offen
(`additionalProperties: true`) und tragen `requestId` optional, damit **ein** Schema
sowohl sink- als auch worker-geschriebene Events validiert. Details & Registrierung:
[`docs/schemas/README.md`](schemas/README.md).

---

## 7. Im Modeler: clio-Events einlesen & nachspielen (Operate, Read-Side)

Die **Operate**-Ansicht des Modelers hat neben „Auswerten" (Eingaben von Hand) ein Panel
**„Aus clio nachspielen"**. Es schließt die Lücke zwischen „temis hat protokolliert" und
„ich will das im Modeler nachvollziehen": Der Modeler liest die in clio abgelegten
Entscheidungen zurück und **spielt jede aufgezeichnete Eingabe erneut durch das offene
Modell** — der Replay erscheint als normaler Lauf in der History und mit Ergebnis-Pills auf
dem Diagramm, identisch zu einem Live-„Auswerten".

**Das Mapping** definiert man direkt im Panel — es ist die Antwort auf „wo in clio liegen
die Events":

| Feld | Bedeutung |
|---|---|
| **Subject** | Der clio-Pfad-Teilbaum, der gelesen wird (rekursiv). Leer ⇒ der serverseitig konfigurierte `-clio-subject-prefix`. |
| **Event-Typ** | Filter auf **einen** CloudEvents-`type` (`com.temis.decision.evaluated.v1`, `…flow.evaluated.v1`, `…decision.requested.v1`) oder „alle Typen". |
| **Limit** | Höchstzahl eingelesener Events (Default 200, max. 1000). |

Das Mapping wird **pro Modell** (nach Modellname, überlebt content-adressierte Re-Saves) im
`localStorage` des Browsers gemerkt und beim ersten Laden aus `subjectPrefix`/`subjectKey`
des Sinks vorbefüllt (siehe `GET /v1/status`).

**Serverseitig** liest der Modeler über **`GET /v1/clio/events?subject=…&type=…&limit=…`**
(Audit-Scope, wie `/v1/status`). Der Endpunkt ist **secret-frei**: `temisd` fragt clio über
die **bestehende Sink-Verbindung** (`ClioSink.Query` → clio-`run-query`) ab, der **Browser
sieht den clio-Token nie**. Ohne konfigurierten Sink antwortet er `enabled:false` (kein
Fehler), ein clio-Lesefehler ist ein `502`. Es werden nur die replay-relevanten Felder
zurückgegeben (`subject`, `type`, `time`, `modelId`/`flowId`, `decision`, `input`,
`outputs`).

Das ist **reines Lesen + lokal neu Auswerten** — kein Schreiben zurück nach clio, keine
Prozess-Reaktivität (ADR-0025-Grenze bleibt gewahrt). Wer Entscheidungen **automatisch**
per Event auslösen und das Ergebnis zurückschreiben will, nutzt den Command-Consumer aus
§6 (`temis-clio-worker`); das Operate-Panel ist die **interaktive** Read-/Nachspiel-Seite.

---

## Verwandte Dokumente

- **ADR-0033** — Command-Consumer (diese Gegenrichtung): Vertrag, Zustandslosigkeit, ADR-0025-Grenze.

- **ADR-0023** — Entscheidung & Begründung dieses Integrationsmusters.
- **ADR-0013** — temis als Verifikationsorakel; Entscheidungsspur (`explain`).
- **ADR-0011 / ADR-0005** — Library-first, Service als dünner Adapter (warum der Sink
  *nicht* in den Kern gehört).
- **`docs/40-api-contract.md`** — stabile Oberfläche inkl. Decision-Event-Vertrag.
- **clio:** `README.md` / `ARCHITECTURE.md` (Event-Modell, Hash-Kette, Preconditions,
  Scopes, `register-event-schema`).
