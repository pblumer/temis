# Entscheidungs-Logbuch mit clio (revisionssicheres Audit)

> **Status:** Der **`temisd`-Sink (Abschnitt 2, WP-54)** und das
> **Re-Audit-Tool (Abschnitt 4, WP-55)** sind umgesetzt. Das **Agent-Muster**
> (Abschnitt 5) funktioniert ganz ohne neuen temis-Code. Vertrag & Begründung:
> ADR-0023.

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
  "source":  "temisd",
  "subject": "/orders/42",
  "type":    "com.temis.decision.evaluated.v1",
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
(`POST /v1/evaluate`, `POST /v1/models/{id}/evaluate`) — nur wenn konfiguriert. Default
aus ⇒ Verhalten **byte-identisch** zu heute.

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

## Verwandte Dokumente

- **ADR-0023** — Entscheidung & Begründung dieses Integrationsmusters.
- **ADR-0013** — temis als Verifikationsorakel; Entscheidungsspur (`explain`).
- **ADR-0011 / ADR-0005** — Library-first, Service als dünner Adapter (warum der Sink
  *nicht* in den Kern gehört).
- **`docs/40-api-contract.md`** — stabile Oberfläche inkl. Decision-Event-Vertrag.
- **clio:** `README.md` / `ARCHITECTURE.md` (Event-Modell, Hash-Kette, Preconditions,
  Scopes, `register-event-schema`).
