# Changelog

Alle nennenswerten Änderungen an Temis werden hier dokumentiert.

Das Format orientiert sich an [Keep a Changelog](https://keepachangelog.com/de/1.1.0/),
die Versionierung an [Semantic Versioning](https://semver.org/lang/de/). Der SemVer-Vertrag
gilt für die öffentliche Go-API (`package dmn`) und die HTTP-API (ADR-0019,
`docs/40-api-contract.md §4`); `internal/` ist ausgenommen.

> **Pflege:** Neue Einträge unter `[Unreleased]` sammeln. Beim Release den Abschnitt in
> eine Versionsüberschrift `[x.y.z] - JJJJ-MM-TT` umbenennen, einen neuen leeren
> `[Unreleased]` anlegen und den Tag `vx.y.z` setzen — die Release-Pipeline
> (`.github/workflows/release.yml`) zieht die Notizen dieses Abschnitts in den
> GitHub-Release.

## [Unreleased]

Vor-1.0-Entwicklung. Bis zum ersten getaggten Release tragen die Binaries die Version
`0.0.0-dev`. Bisher umgesetzt (Auszug, voller Stand in `docs/20-roadmap.md`):

### Added

- **Engine-Kern (WP-01–11):** DMN-1.5-XML-Decoding (tolerant 1.3/1.4) mit `DMNDI`-Round-Trip;
  vollständige FEEL-Pipeline (Lexer → Parser → Compile-to-Closures); Decimal-Numbers (`apd`);
  Decision Tables mit Hit Policies; öffentliche Library-API `package dmn` (`Compile`/`Evaluate`).
- **FEEL vollständig (WP-20–22):** Comprehensions/Filter/Pfad-Projektion, alle nicht-temporalen
  Built-ins, Date/Time/Duration samt temporaler Built-ins und `@`-Literalen.
- **Boxed Expressions & DRG (WP-23–26):** Boxed Context/Invocation/Function, BKM, DRG-Verkettung,
  Decision Services.
- **Hit Policies & Typsystem (WP-27, WP-30–31):** alle Hit Policies (inkl. PRIORITY/OUTPUT ORDER);
  Typsystem, `instance of`, advisory statische Typprüfung (ADR-0017), Item-Definition-Constraints.
- **Robustheit & Betrieb (WP-34–35, WP-42, WP-44):** Ressourcenlimits/Sandboxing (ADR-0008),
  LRU-Modell-Cache, Performance-Budget-CI-Gate, Fuzzing über jede untrusted-Input-Schicht.
- **Service & Agenten (WP-32, WP-50–53):** HTTP-Service `temisd` (REST, OpenAPI, `/ui`-Playground);
  MCP-Server `temis-mcp` (stdio + HTTP), Entscheidungsspur, striktes Eingabe-Schema.
- **Modellierungs-Assistent (WP-80, ADR-0024):** eingebauter LLM-Chat im Modeler, der beim Bauen
  von Decisions hilft und seine Vorschläge mit `evaluate` gegen die echte Engine verifiziert.
  Anbieter-agnostisch (Anthropic/OpenAI) über das neue Paket `assist` — reine Standardbibliothek,
  kein SDK, keine neue Dependency (konsistent mit ADR-0014). Endpunkt `POST /v1/chat` (opt-in,
  Default aus), aktiviert über `temisd -llm-provider/-llm-token/-llm-model/-llm-base-url`; Token
  server-seitig **plus** optionaler Browser-Key (`X-LLM-Token`, `-llm-allow-byok`, nie persistiert),
  vom selben `-token` bewacht. Der Agent-Loop läuft server-seitig auf dem geteilten Modell-Cache mit
  sieben Werkzeugen (inspizieren/auswerten/bauen); Frontend: angedocktes Chat-Panel mit
  Tool-Schritt-Anzeige und automatischem Reload bei Modelländerungen.
- **Ko-lokalisierter MCP-Endpoint (ADR-0021):** `temisd` bedient optional `POST /mcp` (Flag `-mcp`,
  Default an) auf **demselben Modell-Cache** wie Modeler und `/v1`-API — vorgeladene Beispiele und
  Modeler-Modelle sind über MCP sichtbar und umgekehrt, eine `modelId` über alle Oberflächen; das
  eigenständige `temis-mcp` (stdio/HTTP) bleibt unverändert.
- **clio-Entscheidungs-Logbuch (WP-54, ADR-0023):** `temisd` protokolliert optional jede
  Einzel-Decision-Auswertung als manipulationssicheres `com.temis.decision.evaluated.v1`-CloudEvent
  in einer [clio](https://github.com/pblumer/clio)-Instanz — Flags `-clio-url`/`-clio-token`/
  `-clio-source`/`-clio-subject-prefix`/`-clio-subject-key`/`-clio-strict` (`$TEMIS_CLIO_*`), Default
  **aus** (byte-identisch). Idempotent per clio-Precondition (`inputHash`); `-clio-strict` macht den
  Sink fail-closed (`502 AUDIT_WRITE_FAILED`), sonst best-effort. Reine stdlib, kein Go-Import von
  clio (Kopplung nur über dessen HTTP-API, ADR-0011/0014).
- **Re-Audit-/Replay-Tool `temis-reaudit` (WP-55, ADR-0023):** `package audit` + Binary
  `cmd/temis-reaudit` lesen die Decision-Events aus clio (`run-query`), rechnen jede Entscheidung
  `input`@`modelId` über die `dmn`-API erneut nach und vergleichen kanonisch mit den protokollierten
  `outputs` — Verdikt je Event (reproduced/discrepancy/model_unavailable/eval_error), Exit-Code
  (0/1) wie clios `verify`. Modelle werden über ein DMN-Verzeichnis (`-models`) per `sha256:`-`modelId`
  aufgelöst. Read-only; ergänzt clios *Unverändert*-Beweis um den *Regelkonformitäts*-Beweis.
- **Nullkonfiguration & Env-Opt-out (`temisd`):** Ein nackter Start (`temisd`, keine Flags,
  keine Env-Variablen) bringt sofort einen voll ausgestatteten Server — Modeler, Swagger-UI,
  Beispiele, Modell-Listing, MCP-Endpunkt und der **Modellier-Assistent** sind ab Start aktiv.
  Der Assistent ist damit **standardmäßig an** (zuvor opt-in): ohne serverseitigen Schlüssel
  läuft er im **BYOK-Modus** (Endpunkt live, antwortet sobald ein Aufrufer `X-LLM-Token`
  mitschickt), mit `TEMIS_LLM_TOKEN` nutzt der Server den eigenen Key; Abschalten via
  `-assist=false`/`TEMIS_ASSIST=false`. Für den Profi lässt sich **jedes** Feature allein über
  Umgebungsvariablen ab-/umschalten (`TEMIS_ADDR`, `TEMIS_EXAMPLES`, `TEMIS_MCP`,
  `TEMIS_LIST_MODELS`, `TEMIS_ASSIST`, `TEMIS_LLM_ALLOW_BYOK`, `TEMIS_CACHE_SIZE`,
  `TEMIS_MAX_*`, `TEMIS_CLIO_*` u. a.) — kein Flag nötig (container-freundlich); ein explizites
  Flag hat weiterhin Vorrang vor der Env-Variable.
- **API-Stabilisierung (WP-43):** `package dmn` als v1 zugesagt; SemVer-/Deprecation-Policy;
  Golden-Surface-Test gegen unbeabsichtigte Brüche.
- **Doku & Release (WP-45–46):** godoc-Beispiele, Integrations-/Quickstart-Leitfaden; versionierte
  Release-Pipeline, Container-Image für `temisd`, dieses Changelog.

### Docs

- **OpenAPI & API-Vertrag mit dem Modeler synchronisiert:** Die 13 Modeler-Endpunkte
  (ADR-0016 — Graph, Item Definitions, Decision-Tables, Literal-Expressions, BKM, Save)
  sowie `GET /v1/models/{id}/xml` und `POST /v1/models/{id}/evaluate-graph` sind jetzt in
  `service/openapi.yaml` (Pfade + Schemas) und `docs/40-api-contract.md` §2.1 dokumentiert;
  README entsprechend ergänzt. Ein neuer Test (`TestOpenAPICoversDataRoutes`) gleicht die
  registrierten `/v1`-Routen gegen die OpenAPI-Pfade ab, sodass die Spec nicht mehr stillschweigend
  von der Implementierung abdriften kann.
- **Entscheidungs-Logbuch via clio (ADR-0023, WP-54–56):** ADR-0023 und
  `docs/80-clio-decision-log.md` spezifizieren ein revisionssicheres Entscheidungs-Logbuch über das
  Schwesterprojekt [clio](https://github.com/pblumer/clio) — ein versionierter
  `com.temis.decision.evaluated.v1`-CloudEvent-Vertrag (Eingabe/Ausgabe/Spur/content-addressed
  `modelId`), der opt-in-Sink in `temisd` (umgesetzt, siehe oben) und ein noch offenes
  Re-Audit-/Replay-Werkzeug (WP-55).

[Unreleased]: https://github.com/pblumer/temis/commits/main
