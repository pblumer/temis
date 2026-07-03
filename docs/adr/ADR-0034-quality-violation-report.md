# ADR-0034: Quality-Report — welcher Datensatz welche Regel verletzt (Lese-Seite der Produktivläufe)

- **Status:** accepted
- **Datum:** 2026-07-02
- **Kontext-WP:** Import-Cockpit / Quality-Events (ADR-0031), clio-Kopplung (ADR-0023)

## Kontext

ADR-0031 schreibt für einen **Produktivlauf** pro ausgewertetem Fall ein **Quality-Event**
`com.temis.quality.evaluated.v1` auf die **Entität** (`/quality/<entity>`) mit einem
`violation`-Flag — und nennt „Reporting-Views/Queries über `violation`-Events" ausdrücklich als
**offene Folgeaufgabe**. Damit war die Schreib-Seite fertig, die **Auswertung** aber nicht: Der
typische Wunsch ist „ich lasse ein ganzes Regelset über 70 000 Server laufen und will am Schluss
wissen, **welcher Server welche Regel nicht bestanden hat**". Bis hierhin lagen die Events
sauber pro Entität im Logbuch, aber niemand fasste sie zu dieser Tabelle zusammen.

Zwei Fragen waren zu klären: **(a)** Wie fällt aus einem Regelset überhaupt „welche Regel"
heraus? und **(b)** Wo lebt die Aggregation, damit CLI, HTTP und Web sie **teilen**?

## Optionen

1. **Pro Verstoß eine eigene Decision** (`pass`/`fail` je Regel), zu einem Graphen verkettet —
   ausdrucksstark, aber viele Decisions und der Batch verzichtet auf Traces, „welche Regel"
   müsste also mühsam aus mehreren Outputs rekonstruiert werden.
2. **Eine Decision-Table mit Hit Policy `COLLECT`**, deren Output je Regel die **Regel-ID** ist,
   sodass der Wert die **Liste der verletzten Regeln** ist. Jede Regelzeile prüft genau eine
   Spalte (`-` = beliebig), die Regeln sind also unabhängige Einzel-Checks. Gewählt — die
   verletzten Regeln fallen als **Output-Wert** heraus (nicht aus dem Trace, den der Batch
   bewusst weglässt), also sind sie auch im schnellen Batch-Pfad vorhanden.
3. **Aggregation clientseitig im Browser** — bräuchte den clio-Token im Browser (unerwünscht,
   ADR-0031) und skaliert nicht auf Flottengröße.
4. **Aggregation in einem geteilten Go-Paket, das aus dem clio-NDJSON-Stream liest** — read-only,
   ohne `service`-Import, symmetrisch zu `package audit` (ADR-0023). Gewählt.

## Entscheidung

- **Geteilter Kern `package quality`:** liest die Quality-Events als NDJSON (genau die Form, die
  clios `run-query` liefert) und aggregiert sie zu einem **Report** — distinct Entitäten, wie
  viele bestanden, und je **verletzender** Entität die de-duplizierte, sortierte Liste der Regel-IDs
  plus eine **Rangliste je Regel** (betroffene Entitäten). Bestehende Entitäten werden **gezählt,
  aber nicht behalten**, damit ein sauberer Report über eine große Flotte klein bleibt (wie der
  Re-Audit-Report, ADR-0023). Verletzte Regeln kommen aus den **Listen-Outputs** der Decisions
  (auto-detektiert; ein `ruleField` kann genau einen Output benennen); der `violation`-Flag ohne
  benannte Regel lässt eine Entität dennoch „durchfallen" (reiner Erwartungsabgleich).
- **Drei Kanäle, ein Kern:**
  - **CLI `temis-quality-report`** (read-only, wie `temis-reaudit`): liest clio direkt, druckt
    Text oder JSON, `-fail-on-violation` macht es CI-gattbar.
  - **HTTP `GET /v1/quality/report`** (Scope `audit`): der Server **fragt clio selbst ab** (er
    hält den Token), sodass ein Browser ihn nie sieht. `409 CLIO_NOT_CONFIGURED`, wenn kein Sink
    konfiguriert ist.
  - **Web:** ein Report-Panel im Import-Cockpit konsumiert den Endpoint und zeigt die Tabelle
    „Entität × verletzte Regeln" plus die Regel-Rangliste.
- **Beispiel-Regelset `server_compliance`** (gebündelt): eine COLLECT-Tabelle mit fünf
  unabhängigen Server-Checks (Patch-Alter, TLS-Version, freier Speicher, Firewall, Root-SSH), die
  die verletzten Regel-IDs als Liste ausgibt — der greifbare Einstieg für den ganzen Ablauf.

## Konsequenzen

- **Positiv:** Der Ablauf ist **durchgängig** — Regelset über einen Datensatz laufen lassen
  (Batch, ADR-0031), pro Entität ein revisionssicheres Quality-Event, und am Schluss über **CLI,
  HTTP und Web** die Auswertung „welcher Server welche Regel". Ein Kern, drei Kanäle, kein
  Token im Browser.
- **Negativ / Grenzen:** Ein einzelner Batch-Request bleibt an das **8-MiB-Body-Limit** gebunden
  (~50 000 reiche Zeilen für dieses Schema); größere Flotten streamt ein Client in **Blöcken**.
  Der Report liest so viele Events, wie `limit` erlaubt (Default = Batch-Cap); sehr große
  Historien filtert man über `subject`/`limit`. Die verletzte Regel muss als **Output-Wert**
  modelliert sein (Muster 2) — reine Trace-Information erscheint im Batch nicht.
- **Folgeaufgaben:** eine clio-**Reduce-Spec**, die den Report direkt im Logbuch materialisiert
  (vierter Kanal), sowie Zeitfenster-/Diff-Sichten („neu verletzt seit gestern").
