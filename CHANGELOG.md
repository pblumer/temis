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
- **Ko-lokalisierter MCP-Endpoint (ADR-0021):** `temisd` bedient optional `POST /mcp` (Flag `-mcp`,
  Default an) auf **demselben Modell-Cache** wie Modeler und `/v1`-API — vorgeladene Beispiele und
  Modeler-Modelle sind über MCP sichtbar und umgekehrt, eine `modelId` über alle Oberflächen; das
  eigenständige `temis-mcp` (stdio/HTTP) bleibt unverändert.
- **API-Stabilisierung (WP-43):** `package dmn` als v1 zugesagt; SemVer-/Deprecation-Policy;
  Golden-Surface-Test gegen unbeabsichtigte Brüche.
- **Doku & Release (WP-45–46):** godoc-Beispiele, Integrations-/Quickstart-Leitfaden; versionierte
  Release-Pipeline, Container-Image für `temisd`, dieses Changelog.

### Docs

- **Entscheidungs-Logbuch via clio (ADR-0023, geplant WP-54–56):** ADR-0023 und
  `docs/80-clio-decision-log.md` spezifizieren ein revisionssicheres Entscheidungs-Logbuch über das
  Schwesterprojekt [clio](https://github.com/pblumer/clio) — ein versionierter
  `com.temis.decision.evaluated.v1`-CloudEvent-Vertrag (Eingabe/Ausgabe/Spur/content-addressed
  `modelId`), ein opt-in-Sink in `temisd` und ein Re-Audit-/Replay-Werkzeug. Kopplung nur über
  clios HTTP-Vertrag, kein Go-Import (ADR-0011); noch kein Code.

[Unreleased]: https://github.com/pblumer/temis/commits/main
