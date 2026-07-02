# clio Event-Schemas (JSON Schema)

Die versionierten CloudEvent-`data`-Verträge, die temis mit clio austauscht — als
**JSON Schema** (Draft 2020-12), maschinenlesbar. Sie sind das kanonische Artefakt zu
`docs/40-api-contract.md` und `docs/80-clio-decision-log.md`; jede `.v1`-Datei unterliegt
derselben SemVer-Disziplin wie die übrige öffentliche Oberfläche (ADR-0019).

| Schema | Richtung | Erzeuger |
|---|---|---|
| [`com.temis.decision.requested.v1`](./com.temis.decision.requested.v1.schema.json) | **Command** (löst aus) | Umsystem/Agent → clio |
| [`com.temis.decision.evaluated.v1`](./com.temis.decision.evaluated.v1.schema.json) | **Ergebnis** (Decision) | `temisd`-Sink · `temis-clio-worker` |
| [`com.temis.flow.evaluated.v1`](./com.temis.flow.evaluated.v1.schema.json) | **Ergebnis** (Flow/DRG) | `temisd`-Sink · `temis-clio-worker` |
| [`com.temis.decision.failed.v1`](./com.temis.decision.failed.v1.schema.json) | **Ergebnis** (Fehler) | `temis-clio-worker` |

Ein Schema beschreibt den **`data`**-Teil eines CloudEvents; `id`/`time`/`specversion`
und die Hash-Kette ergänzt clio beim Schreiben.

## In clio registrieren (optional, empfohlen für Commands)

clio kann pro Event-`type` ein JSON Schema hinterlegen (`register-event-schema`) und
**lehnt beim Schreiben** jedes nicht-konforme Event ab. Für das **Command**-Schema ist das
besonders nützlich: die Producer sind extern/untrusted, und so wird Unsinn (fehlender
`input`, `modelId` **und** `flowId` zugleich, `decision` bei einem Flow, kaputte
`sha256:`-ID) **an der Quelle** abgewiesen, statt erst nachgelagert im Worker ein
`failed.v1` zu erzeugen.

```jsonc
// clio-MCP-Tool: register_event_schema
{
  "type":   "com.temis.decision.requested.v1",
  "schema": { /* Inhalt von com.temis.decision.requested.v1.schema.json */ }
}
```

Der Diskriminator aus ADR-0033 (genau eines von `modelId`/`flowId`, `decision` nur mit
`modelId`) wird im Command-Schema per `oneOf` **maschinell erzwungen**.

## Hinweise

- **Ergebnis-Schemas** sind bewusst offen (`additionalProperties: true`): dasselbe Schema
  validiert sink- **und** worker-geschriebene Events. Das additive `requestId` (nur bei
  command-getriebenen Events) ist daher optional; `strict` setzt nur der Sink.
- Das **Command-Schema** ist streng (`additionalProperties: false`), um Tippfehler wie
  `modelID` an der Grenze zu fangen. Bei Bedarf lässt sich das lockern.
- Ein Drift zwischen diesen Dateien und den von `package consume` erzeugten Events wird
  durch `consume/schema_test.go` in CI abgesichert.
