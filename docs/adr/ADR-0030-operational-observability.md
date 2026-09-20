# ADR-0030: Betriebs-Observability (Status-Endpoint, `expvar`-Metriken, `slog`) — und die Grenze zur externen Control Plane

- **Status:** accepted
- **Datum:** 2026-07-01
- **Kontext-WP:** WP-110 – WP-114 (neue Etappe „Betriebs-Observability")

## Kontext

`temisd` spricht inzwischen mehrere **Umsysteme** an — das Entscheidungs-Logbuch
**[clio](https://github.com/pblumer/clio)** (ADR-0023), einen **LLM**-Anbieter für den
Modellier-Assistenten (ADR-0024) und **Git/GitHub** für versionierte Modelle (ADR-0022) —
und bedient eine breite Oberfläche (REST, gRPC, MCP, Modeler). Von außen ist über den
**Betriebszustand** dieser Kopplungen und die **Auslastung** des Prozesses heute jedoch
praktisch nichts sichtbar:

- **`/readyz` ist eine Unwahrheit.** `handleHealth` (`service/http.go`) beantwortet
  **sowohl `/healthz` als auch `/readyz`** statisch mit `{"status":"ok"}` — `/readyz`
  meldet „bereit", selbst wenn ein konfiguriertes Umsystem down ist. Liveness und
  Readiness sind nicht getrennt.
- **Der clio-Sink hat keinen nach außen sichtbaren Zustand.** `ClioSink.Record`
  (`service/cliosink.go`) ist best-effort (loggt den Fehler) oder fail-closed
  (`-clio-strict`), führt aber **keinen Zähler, kein `lastError`, kein „zuletzt
  erreichbar", keine Erfolgs-/Fehlerquote**. Ob Audits tatsächlich durchgehen, ist von
  außen unsichtbar — gerade im (Default-)best-effort-Modus ein blinder Fleck.
- **Keine Metriken.** Kein `expvar`, kein `/metrics`, keine Throughput-, Fehler- oder
  Latenzzahlen. Auslastung (Evaluations/s, Fehlerrate, Cache-Hits/Evictions, LLM-Calls)
  ist nirgends abrufbar.

Zugleich fragt der Betrieb legitim: *Ist clio erreichbar und funktioniert es? Wie hoch ist
die Last? Was ist der Zustand der Umsysteme?* Diese Frage klingt nach einer **Control
Plane**, kollidiert aber frontal mit den Rahmenentscheidungen: library-first / Service als
dünner Wrapper (D5, ADR-0005), **reine stdlib / keine neue Dependency** (ADR-0014),
**stateless per Default** (ADR-0027), Nicht-Ziel „kein Decision-Management-UI, keine
Deployment-Konsole" und „kein verteilter Cluster-Betrieb" (`00-overview.md` §3). Ein
zustandsbehaftetes, multi-instanz Monitoring-Produkt ins Engine-Binary zu ziehen wäre das
Gegenteil von „ein Binary, Nullkonfiguration".

Die Auth-/Token-Seite derselben Betriebsfrage ist bereits in **ADR-0028** (scoped
API-Keys, Keystore, `/v1/keys*`) entschieden — dieses ADR ergänzt die **Observability**-Seite
und zieht die Grenze zur externen Control Plane.

## Optionen

1. **Status quo behalten.** Kein Aufwand, aber `/readyz` bleibt irreführend, clio-Ausfälle
   bleiben unsichtbar, keine Auslastungszahlen. **Verworfen.**

2. **Volle Control Plane im Binary** — ein zustandsbehaftetes Ops-Subsystem mit
   Dashboards, Zeitreihen, Alerting, Multi-Instanz-Sicht. Deckt den Wunsch maximal ab,
   verletzt aber D5/ADR-0014/ADR-0027 und die Nicht-Ziele, verlangt neue Dependencies
   (Metrik-/TSDB-/UI-Stack) und Zustand. **Verworfen.**

3. **Prometheus-Client-Bibliothek einziehen** (`client_golang`) für Metriken. Ergonomisch,
   aber eine **neue, schwergewichtige Dependency** — direkter Verstoß gegen Golden Rule 6
   (ADR-0014). **Verworfen.**

4. **In-Binary-Observability rein aus stdlib, opt-in; Aggregation bleibt extern** —
   **gewählt.** temis wird *observierbar* (ehrliche Health/Readiness, ein
   Umsystem-Status-Endpoint, `expvar`-Zähler, strukturierte `slog`-Logs), **überwacht sich
   aber nicht selbst**. Dashboards/Alerting/Multi-Instanz-Auslastung scrapt die externe
   Ops-Schicht (Prometheus/Grafana, Loki) — oder irgendwann ein eigenes Single-Binary
   (à la clio/chrampfer). Kein neuer Dependency, kein Kern-Import, Persistenz nicht nötig.

## Entscheidung

**Option 4.** Leitsatz: **temis ist *observierbar*, nicht *selbst-überwachend*.** Das
Binary **erzeugt** wahrheitsgetreue Signale; das **Aggregieren, Dashboarden und Alerten**
ist die externe Ops-Schicht. Alles lebt in der **Adapter-Schicht** (`service`/`cmd/temisd`);
der Engine-Kern (`package dmn`) bleibt unberührt (ADR-0011). Vier Bausteine, alle **stdlib**,
alle **opt-in bzw. hinter einem ADR-0028-Scope**:

### 1. Liveness/Readiness ehrlich trennen

- **`/healthz` = Liveness** (Prozess lebt): bleibt statisch `ok`.
- **`/readyz` = Readiness** (kann Verkehr bedienen): spiegelt **nur harte** Startbedingungen
  (Engine initialisiert, `-models-dir` lesbar falls gesetzt). **best-effort-Umsysteme
  dürfen `/readyz` nicht rot färben** — ein clio-Schluckauf im best-effort-Modus darf die
  Instanz nicht aus dem Load-Balancer nehmen. Nur `-clio-strict` (fail-closed) macht clio
  zu einer readiness-relevanten Dep. Die feinkörnige Umsystem-Sicht kommt aus `/v1/status`.

### 2. `GET /v1/status` — die Umsystem-Sicht („erreichbar & funktioniert")

Ein read-only JSON-Endpoint, gespeist **primär aus echtem Verkehr** (passiv, kostenlos,
wahrheitsgetreu) plus **optionalem aktivem Ping**. Pro Umsystem ein kleiner Record:

- **engine:** Version, Uptime, gecachte Modelle, Cache-Größe/Evictions.
- **clio:** `enabled`, `mode` (`best-effort`/`strict`), `url`, Zähler
  `writesOk`/`writesFailed`/`idempotentSkips`, `lastOk`/`lastError` (Zeit + gekürzte
  Meldung), `reachable` (aus den letzten echten Writes; **optionaler** aktiver
  `GET /healthz` gegen clio als Bonus, per Flag).
- **llm:** `enabled`, `provider`, `byok`, Zähler `callsOk`/`callsFailed`, `lastOk`.
  **Kein aktiver Ping** — ein LLM-Call kostet Geld; Health nur passiv aus echten Calls.
- **git:** `available` (per-Request-Token, keine stehende Verbindung — nichts zu pingen).

Der Endpoint liegt hinter einem ADR-0028-Scope (`admin`/`audit`) und leakt **nie** Secrets
(kein Token, kein Key — nur „konfiguriert ja/nein"). Ohne Auth-Konfiguration folgt er dem
„offen"-Default wie die übrigen `/v1`-Routen.

### 3. Auslastung via `expvar` (stdlib) — optional `/metrics` als Bonus

`expvar` (Standardbibliothek) veröffentlicht Counter/Gauges als JSON unter `/debug/vars` —
**ohne** Prometheus-Client. Damit werden Auslastung und Fehlerraten sichtbar:
Evaluations gesamt/Fehler, clio-Writes ok/fail/idempotent, LLM-Calls ok/fail,
Cache-Hits/Misses/Evictions, Modellanzahl, Uptime. Die Zähler hooken an bestehenden
Stellen ein (`ClioSink.Record` success/fail, der Evaluate-Pfad, der LRU-Cache) und teilen
sich die Quelle mit `/v1/status`. Wer Prometheus will, bekommt einen **kleinen
Text-Format-Exporter** (`GET /metrics`) über **dieselben** Zähler — weiterhin reine stdlib,
keine Dependency. `/debug/vars` und `/metrics` sind **opt-in** (Flag/Env) und hinter dem
`admin`/`audit`-Scope.

### 4. Strukturierte Logs via `log/slog` (stdlib)

Der bestehende `logf`-String-Log wird auf `log/slog` (stdlib seit Go 1.21) gehoben:
maschinenlesbare, key/value-strukturierte Ops-Logs (JSON-Handler optional), die eine
externe Log-Pipeline (Loki o. Ä.) auswerten kann. Level und Format über Flag/Env.

### Grenze zur Control Plane (explizit)

**In-Binary:** ehrliche Health/Readiness, `/v1/status`, `expvar`/`/metrics`, `slog`.
**Extern (nicht temis):** Dashboards, Zeitreihen, Alerting, Trends, Multi-Instanz-Auslastung
— gescrapt von `/debug/vars`/`/metrics`/`/v1/status`, geloggt über `slog`. Entsteht Bedarf
an einer echten Control Plane, ist sie ein **eigenes Single-Binary** neben temis (Muster
clio/chrampfer), das mehrere Instanzen aggregiert — **kein** Feature von `temisd`.

## Konsequenzen

**Positiv**
- **`/readyz` wird ehrlich** — Load-Balancer-Probes stimmen; best-effort-clio nimmt die
  Instanz nicht mehr fälschlich aus dem Verkehr.
- **clio-Ausfälle werden sichtbar** — Zähler + `lastError` + `reachable` machen den bisher
  blinden best-effort-Modus beobachtbar; Betrieb sieht, ob Audits durchgehen.
- **Auslastung messbar** — Throughput/Fehler/Latenz-nahe Signale ohne neue Dependency.
- **Ein konsistentes Betriebsbild** neben ADR-0028: Auth/Token dort, Observability hier;
  beide stdlib, opt-in, Adapter-Schicht, Kern unberührt.
- **Rückwärtskompatibel & ethos-treu** — alles opt-in bzw. scoped; ohne Konfiguration
  verhält sich `temisd` byte-identisch (bis auf die korrigierte `/readyz`-Semantik).

**Negativ / Kosten**
- Neue (kleine) **Zähl-Zustände** in der Adapter-Schicht (atomare Counter, `lastOk/Error`)
  inkl. Nebenläufigkeits-Disziplin (`sync/atomic`); muss thread-safe und allokationsarm im
  Hot Path bleiben (Qualitätsziel 2).
- **`/v1/status`-Schema** und das **Metrik-/Scope-Mapping** werden Teil der öffentlichen
  Oberfläche (SemVer, `docs/40-api-contract.md`); neue Umsysteme/Routen brauchen eine
  bewusste Zuordnung.
- Achtung **kein Informationsleck**: Status/Metriken nie mit Secrets; Scope-geschützt;
  keine Routen-Existenz preisgeben, wenn deaktiviert (analog Listing-`404`).
- **`/readyz`-Verhaltensänderung** (statisch-`ok` → echte Readiness) ist eine bewusste,
  dokumentierte Korrektur — im `-clio-strict`-Betrieb kann `/readyz` nun `503` liefern.

**Folgeaufgaben**
- Neue Roadmap-Etappe „Betriebs-Observability" mit **WP-110 – WP-114**
  (`docs/20-roadmap.md`).
- `docs/40-api-contract.md`: `/v1/status`-Schema, Health/Readiness-Semantik und das
  Metrik-Inventar als stabile Oberfläche; OpenAPI-Sync (Routen + Schemas).
- Betriebsleitfaden (erweitert `docs/70` bzw. neu `docs/85`): Health-Probes, Status lesen,
  `expvar`/`/metrics` scrapen, `slog`-Pipeline, Abgrenzung zur externen Ops-Schicht.
- README-Env-Tabelle um die neuen Flags/Env ergänzen (`-status`, `-metrics`, `-log-format`,
  optionaler aktiver clio-Ping).
