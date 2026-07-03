# ADR-0023: Entscheidungs-Logbuch über clio (revisionssicherer Event-Sink, opt-in)

- **Status:** accepted
- **Datum:** 2026-06-30
- **Kontext-WP:** WP-54 / WP-55 / WP-56 (Etappe „Entscheidungs-Logbuch", Ausbau von ADR-0013)

## Kontext

ADR-0013 macht temis zum **deterministischen Verifikationsorakel**: gleiche Eingabe →
gleiche Ausgabe, plus eine aus der echten Auswertung abgeleitete **Entscheidungsspur**
(WP-51). Damit ist *eine einzelne* Entscheidung begründbar. Was bislang fehlt, ist das
**Gedächtnis**: Wer hat *wann*, mit *welcher Eingabe*, gegen *welche Modellversion*,
*welches* Ergebnis bekommen — und ist diese Aufzeichnung vor nachträglicher Veränderung
geschützt?

Genau das ist in regulierten Domänen der eigentliche Wert. Eine Engine, die
Förderfähigkeit, Bonitätsklassen, Tarife, Routing oder Berechtigungen entscheidet, ist
ohne **lückenloses, manipulationssicheres Protokoll** nur die halbe Miete. Ein
selbstgebautes Audit-Log (Datei, Tabelle, Log-Zeile) ist weder fälschungssicher noch
reproduzierbar und vermischt zudem Betriebs-Logging mit Geschäfts-Evidenz.

Das Schwesterprojekt **[clio](https://github.com/pblumer/clio)** (`cliostore`) ist genau
dafür gebaut: ein eigenständiger, abhängigkeitsfreier **Event Store** (CloudEvents,
append-only) mit **SHA-256-Hash-Kette** (jede nachträgliche Änderung ist kryptografisch
nachweisbar), optionalen **Ed25519-Signaturen** (Urheberschaft), **Preconditions** für
Optimistic Concurrency, **CEL-Queries**, Live-`observe` und einer gefalteten
Zustandssicht (`GET /state`). Thematisch ein Paar — **Themis** trifft das Urteil,
**Clio** schreibt die Geschichte.

Die Kombination kann etwas, das keines der beiden allein leistet:

> Jede Entscheidung wird als **manipulationssicheres, hash-verkettetes CloudEvent**
> gespeichert — inklusive Eingabe, Ausgabe, Begründung (Spur) und **content-addressed
> `modelId`**. Weil temis deterministisch ist, lässt sich jede historische Entscheidung
> später **nachrechnen** und damit beweisen, dass sie zur damaligen Modellversion
> korrekt war. clios Hash-Kette beweist *„der Eintrag wurde nicht verändert"*; temis'
> Determinismus beweist *„die Entscheidung war regelkonform"*. Zusammen = eine
> vollständige, prüfbare Beweiskette.

**Spannungsfeld zu ADR-0011 / ADR-0005.** Der Engine-Kern (`package dmn`) ist reine
Library ohne Transport-/Protokoll-Importe; Service und MCP sind dünne Adapter. Ein
Audit-Sink darf diese Schichtung **nicht** verletzen: keine HTTP-/clio-Kopplung im Kern,
keine neue Pflichtabhängigkeit, kein verändertes Default-Verhalten. Beide Projekte sind
zudem bewusst **dependency-frei, Single-Binary** — eine Integration darf das auf keiner
Seite aufweichen.

## Optionen

1. **clio als Go-Abhängigkeit in temis einbinden** (Library-Kopplung). — Verletzt
   ADR-0011 (Transport-/Storage-Import im bzw. unter dem Engine-Kern), zieht clios
   Storage (`bbolt` u. a.) in temis und koppelt zwei unabhängig versionierte Single-
   Binaries fest aneinander. Verworfen.
2. **Eigenes Audit-Log in temis bauen** (Datei/Tabelle, eigene Hash-Kette). — Dupliziert
   exakt das, was clio bereits spec-sauber löst (Hash-Kette, Signaturen, Verify, Backup,
   Query), und bürdet temis ein Storage-/Durability-Problem auf, das nicht sein Kern ist.
   Verworfen.
3. **Stabiler Decision-Event-Vertrag + opt-in-Sink über clios HTTP-API** (diese
   Entscheidung). temis definiert ein **versioniertes CloudEvent-Schema** für
   Entscheidungen und emittiert es — **opt-in** — aus der **Adapter-Schicht** (`service`/
   `temisd`) über clios bestehende `write-events`-Route. Kopplung ausschließlich über den
   **HTTP-Vertrag**, kein Go-Import, kein gemeinsamer Prozess. Der Kern bleibt unberührt.
4. **Reines Agent-/Orchestrierungsmuster (nur Doku).** Ein Agent ruft `temis.evaluate`,
   dann `clio.write-events` — kein temis-Code nötig. — Stark für die Agent-First-Story
   (ADR-0013), aber allein zu schwach: ohne serverseitige Option bleibt jede nicht-Agent-
   Integration (HTTP-Clients, Batch) ohne Protokoll. Wird als **ergänzendes** Muster
   dokumentiert, nicht als Ersatz.

## Entscheidung

**Option 3, ergänzt um Option 4 als dokumentiertes Muster.** Das Herzstück dieses ADR
ist ein **stabiler, versionierter Decision-Event-Vertrag**; die Emission ist ein dünner,
opt-in-Adapter.

### 1. Der Decision-Event-Vertrag (das eigentliche Artefakt)

Eine Auswertung wird als **CloudEvent** abgebildet, das clio unverändert akzeptiert:

| CloudEvents-Feld | Inhalt |
|---|---|
| `source` | Erzeuger, z. B. `temisd` (frei konfigurierbar je Instanz) |
| `subject` | die **Geschäftsentität** (clio-Pfad), z. B. `/orders/42` — konfigurierbar |
| `type` | `com.temis.decision.evaluated.v1` (versioniert) |
| `data.modelId` | content-addressed Modell-ID (exakte, reproduzierbare Modellversion) |
| `data.decision` | Name/ID der ausgewerteten Decision |
| `data.input` | die Eingabe (FEEL-Werte; Numbers als exakter Dezimal-String, vgl. ADR-0007) |
| `data.outputs` | das Ergebnis (`Result.Outputs`) |
| `data.trace` | **opt-in** die Entscheidungsspur (`Result.Trace`, WP-51), falls angefordert |
| `data.engine` | temis-Version (`internal/version`) |
| `data.strict` | ob strikte Eingabevalidierung aktiv war (WP-52) |

Der `type` trägt ein **`.v1`-Suffix**; das `data`-Schema unterliegt damit derselben
SemVer-Disziplin wie die übrige öffentliche Oberfläche (ADR-0019). Spezifiziert wird der
Vertrag in `docs/40-api-contract.md` und ausführlich in `docs/80-clio-decision-log.md`;
optional lässt er sich als clio-`register-event-schema` (JSON Schema) hinterlegen.

### 2. Opt-in-Sink in `temisd` (WP-54)

`temisd` bekommt optionale Flags (z. B. `-clio-url`, `-clio-token`, `-clio-source`) bzw.
die entsprechenden Env-Variablen. Sind sie gesetzt, POSTet der Server **nach** jeder
Auswertung das Decision-Event an clios `write-events`. Verbindliche Eigenschaften:

- **Default aus.** Ohne Konfiguration ist das Verhalten **byte-identisch** zu heute.
- **Im Adapter, nicht im Kern.** Die Emission lebt in `service`/`cmd/temisd`; `package
  dmn` erhält **keinen** clio-/HTTP-Import (ADR-0011). Reine stdlib (`net/http`),
  **keine neue Go-Abhängigkeit** und kein Go-Bump — konsistent mit ADR-0014.
- **Auswertung führt, Audit folgt.** Der Sink darf das Ergebnis nicht blockieren oder
  verfälschen: Ein clio-Fehler wird sichtbar gemeldet (Log/Metrik), entscheidet aber
  über eine konfigurierbare Politik (best-effort vs. strikt/fail-closed) — siehe
  `docs/80`. Performance-Budget des Hot Path (ADR-0011/WP-42) bleibt gewahrt; die Spur
  wird nur erzeugt, wenn sie geloggt werden soll.
- **Idempotenz** über clio-**Preconditions** (`isQueryResultEmpty` auf
  (Entität, Eingabe-Hash)): Retries erzeugen keine Doppel-Einträge.

### 3. Re-Audit / Replay-Verifikation (WP-55)

Ein eigenständiger Konsument (CLI/Tool) liest die Decision-Events aus clio
(`run-query`/`observe`, `type == 'com.temis.decision.evaluated.v1'`), schickt
`data.input` + `data.modelId` **erneut** durch temis und vergleicht mit `data.outputs`.
Ergebnis ist ein Compliance-Report („N von N historischen Entscheidungen reproduzieren
exakt"). Das ist der Mehrwert, den die Hash-Kette allein nicht liefert: nicht nur
*unverändert*, sondern *nachweislich regelkonform*. Liest nur — keine Sonderrechte, kein
`internal/`-Zugriff.

### 4. Agent-Muster (WP-56, nur Doku)

Für die Agent-First-Achse (ADR-0013) wird das Orchestrierungsmuster dokumentiert: Agent
ruft `temis.evaluate` (MCP), dann `clio.write-events` (MCP) — ganz ohne temis-Code.
Themis entscheidet, Clio merkt es sich.

## Konsequenzen

**Positiv**
- Revisionssicheres, reproduzierbares Entscheidungs-Logbuch für Compliance/Audit — der
  konsequente nächste Schritt nach ADR-0013, der die Spur (begründbar) um Persistenz
  (nachweisbar) erweitert.
- **Re-Audit** macht Determinismus operativ prüfbar: ein Beweis, den kaum ein anderes
  Stack-Setup so liefert.
- Keine Architektur-Schuld: Kopplung nur über einen **HTTP-Vertrag**, beide Single-
  Binaries bleiben unabhängig deploybar und je für sich dependency-frei (ADR-0005/0011).
- Wiederverwendung von clios geprüfter Tamper-Evidence/Signatur/Backup statt Eigenbau.

**Negativ / Kosten**
- Der Decision-Event-`type`/`data`-Vertrag wird Teil der öffentlichen Oberfläche und
  unterliegt SemVer (`docs/40-api-contract.md`) — Änderungen nur versioniert (`.v2`).
- Eine neue, **optionale** Betriebsabhängigkeit (eine erreichbare clio-Instanz) inkl.
  Fehler-/Retry-/Fail-Policy, die sauber dokumentiert sein muss.
- Inhaltliche Sorgfalt: `data.input`/`trace` können fachlich sensibel sein — Subject-
  Mapping, clio-Scopes (`write:/decisions/*`) und ggf. Feldfilter gehören in den
  Betriebsleitfaden (`docs/80`).

**Folgeaufgaben**
- Neue Roadmap-Etappe „Entscheidungs-Logbuch" mit **WP-54** (opt-in-Sink in `temisd`),
  **WP-55** (Re-Audit-Tool) und **WP-56** (Agent-Muster-Doku).
- `docs/80-clio-decision-log.md`: vollständiger Vertrag, Mapping, Betriebs-/Sicherheits-
  hinweise, Beispiele (Sink, Idempotenz, Re-Audit, Agent-Muster).
- `docs/40-api-contract.md`: Decision-Event als Teil der stabilen Oberfläche aufnehmen
  (Form, `.v1`-Stabilität, opt-in-Schalter).
- `docs/60-ai-agent-guide.md`: Agent-Muster „delegieren → protokollieren" ergänzen.
