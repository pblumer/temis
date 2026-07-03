# Repository- und Code-Qualitäts-Audit — Temis

**Stand:** 2026-07-02 · Branch `main` @ `d3b91e7` (Merge PR #136)
**Methode:** Vollständige Repo-Analyse (vier unabhängige Review-Stränge: Engine-Kern, Service-Layer/Security, Web-Frontend, Doku/CI/Prozess), objektive Metriken (Build, Tests, Coverage, `go vet`, `tsc`, `npm audit`) sowie empirische Verifikation der kritischen Findings.

---

## 1. Management Summary

Temis ist für ein vier Tage altes Repository (203 Commits, 2026-06-29 bis 2026-07-02) in einem **aussergewöhnlich reifen Zustand**: Der Engine-Kern ist architektonisch sauber, decimal-korrekt, limitiert gegen feindliche Eingaben und mit 97–100 % Testabdeckung versehen; die Doku-Suite (10 nummerierte Dokumente + 33 ADRs) und die CI-Gates (Format, Lint, Race-Tests, Performance-Budget, Drift-Checks, Playwright-E2E) sind auf einem Niveau, das viele Produktionsprojekte nicht erreichen.

**Gesamtbewertung: gut bis sehr gut (5,0 von 6), mit drei klaren Handlungsfeldern:**

1. **Ein kritischer Robustheits-Fehler:** Der FEEL-Parser hat kein Rekursionstiefen-Limit. Eine 8-MiB-Eingabe (innerhalb des HTTP-Body-Limits) bringt den gesamten `temisd`-Prozess per Stack-Overflow zum Absturz — empirisch verifiziert. Dazu kommen zwei Frontend-Injection-Lücken (unvollständiges HTML-Escaping) und riskante Betriebs-Defaults (keine Outbound-/Server-Timeouts, offener LLM-Kostenendpunkt bei Fehlkonfiguration).
2. **Das Produkt existiert nach aussen nicht:** Kein einziges Release (keine Git-Tags), die Release-Pipeline (WP-46) ist ungetestete Theorie, das CHANGELOG kennt nur `[Unreleased]`, und der zentrale Konformitätsnachweis (WP-41, offizielles DMN-TCK ≥ 95 %) ist offen.
3. **Drift an manuell gepflegten Stellen:** README-Status, API-Contract (§1.1 dokumentiert nicht existente Optionen), Architektur-Doku vs. Code (`internal/drg` ist leer), WP-100 doppelt vergeben.

Die Massnahmen sind in **17 Arbeitspaketen (WP-130 bis WP-146)** organisiert, gegliedert in drei Roadmap-Horizonte: **Härten (sofort) → Release & Nachweis (kurzfristig) → 1.0-Reife (mittelfristig)**. Siehe Abschnitt 7 und 8.

---

## 2. Repository-Überblick & Metriken

| Metrik | Wert |
|---|---|
| Produktivcode Go (ohne Tests, ohne `internal/gen`) | ~26 900 Zeilen, 303 Dateien |
| Testcode Go | ~24 900 Zeilen (Verhältnis ~0,93:1) |
| Frontend TypeScript (`web/src`) | 10 595 Zeilen, 41 Module |
| E2E-Tests (Playwright) | 17 Specs, 1 368 Zeilen |
| Direkte Go-Dependencies | **4** (connect, apd, x/net, protobuf) |
| Frontend-Runtime-Dependencies | **3** (diagram-js, direct-editing, dagre) · 0 npm-Vulnerabilities |
| `TODO`/`FIXME`/`HACK` im gesamten Code | **0** |
| ADRs | 33, mit Index und Template |
| Git-Tags / Releases | **0** |
| Commits / aktive Tage | 203 / 4 (fast vollständig über PRs, #23–#136) |

**Quality-Gates (dieses Audit, lokal ausgeführt):**

| Gate | Ergebnis |
|---|---|
| `go build ./...` | ✅ grün |
| `go vet ./...` | ✅ grün |
| `go test ./...` | ⚠️ grün bis auf `TestHTTPScopeAuthorization/git_ok` — der Test ruft die **echte GitHub-API** auf und ist damit nicht hermetisch (schlägt in Umgebungen ohne freien GitHub-Zugang fehl) |
| `tsc --noEmit` (web) | ✅ grün |
| `npm audit` | ✅ 0 Vulnerabilities |
| Coverage unter `-covermode=atomic` | ⚠️ `internal/feel` flaky (2 von 6 Läufen `fatal error: stack overflow`) — die Rekursionstests liegen nahe an der Stack-Grenze; Coverage-Instrumentierung vergrössert die Frames (gleiche Wurzel wie Finding K1) |

**Testabdeckung je Package (Statement-Coverage):**

| Package | Coverage | Package | Coverage |
|---|---|---|---|
| `internal/tck` | 100,0 % | `dmn` | 97,0 % |
| `internal/feel/builtins` | 99,9 % | `mcp` | 94,0 % |
| `internal/model` | 99,5 % | `audit` | 93,3 % |
| `internal/boxed` | 98,9 % | `flow` | 91,9 % |
| `vcs/github` | 98,4 % | `assist` | 86,4 % |
| `internal/feel` | 98,4 % | `service` | 78,9 % |
| `internal/value` | 97,3 % | `internal/xml` | 79,1 % |

---

## 3. Zustand der Lösung (Lieferstand)

Referenz: `docs/20-roadmap.md` (Live-Status). Höchste vergebene WP-Nummer: **WP-121**. ⚠️ **WP-100 ist doppelt vergeben** (Flow-Registry *und* Keystore-Kern).

| Etappe | WPs | Stand |
|---|---|---|
| MVP („lädt dmn-js-Dateien und entscheidet") | WP-01–11 | ✅ komplett |
| Beta („vollständiges DMN 1.5", HTTP/gRPC, Limits, Cache, Persistenz) | WP-20–36 | ✅ komplett |
| Agent-First (MCP, Trace, Schema, Remote-MCP) | WP-50–53 | ✅ komplett |
| Entscheidungs-Logbuch (clio-Sink, Re-Audit) | WP-54–56 | ✅ komplett |
| Git-gestützte Modelle | WP-70–74 | WP-70–73 ✅ · WP-74 offen (optional) |
| Modellierungs-Assistent (LLM) | WP-80 | ✅ |
| Decision-Flow (L2a) inkl. Flow-Designer | WP-90–100, 115, 116 | ✅ komplett |
| Command-Consumer (clio) | WP-120–121 | ✅ komplett |
| Zugriffskontrolle (scoped API-Keys) | WP-100–105 | ✅ komplett |
| Betriebs-Observability | WP-110–114 | WP-110–112 ✅ · **WP-113 (Metriken), WP-114 (slog) offen** |
| **1.0** („TCK-konform, schnell, stabil") | WP-40–46 | WP-40, 42–46 ✅ · **WP-41 (TCK ≥ 95 %) offen — das zentrale unerfüllte 1.0-Kriterium** |
| Eigener Modeler (ADR-0016) | WP-60–67 | WP-60/61/67 ✅ · WP-62 offen, WP-63/65/66 teilweise, WP-64 formal offen |

**Fazit Lieferstand:** Funktional ist das Produkt weit jenseits „Beta" — Engine, FEEL, Boxed Expressions, DRG, Decision Services, HTTP+gRPC, MCP, Git-Workflows, Audit, Flows, Auth und der Grossteil der Observability sind fertig und testgedeckt. Was fehlt, ist der **Nachweis nach aussen**: kein Release, kein TCK-Ergebnis, keine versionierte Go-Modul-Oberfläche. Inkonsistenzen: WP-67 ist „done" markiert, obwohl seine Abhängigkeiten WP-64/65 nicht done sind; der README-Status hinkt der Roadmap hinterher.

---

## 4. Code-Qualität je Bereich

### 4.1 Engine-Kern (`dmn`, `internal/*`, `flow`, `consume`) — Note 5,0

| Package | Note | Kern |
|---|---|---|
| `dmn/` | 5,0 | Klare Public-API, typisierte Fehlercodes, API-Golden-Test; Abzug für coverage-getriebene Testdateien und Graph-Logik am „falschen" Ort |
| `internal/feel/` | 4,0 | Exzellente Struktur (Slot-Scope, reine Closures), aber **fehlender Parser-Tiefenschutz (K1)** |
| `internal/value/` | 5,5 | Korrekte Dezimal-Semantik (apd, 34 Stellen, half-even), robuste Null-Propagation |
| `internal/xml/` | 4,5 | Tolerant, DMNDI-Round-Trip; niedrigste Coverage, kein eigener Tiefenschutz |
| `internal/model/` | 5,5 | Versionstoleranter Mapper, forward-kompatibel |
| `internal/drg/` | 2,0 | **Leeres Scaffold** — Architektur-Doku weist ihm Kernverantwortung zu, die real in `dmn/` liegt |
| `internal/boxed/` | 5,0 | Alle Hit-Policies sauber, Trace-Chokepoint |
| `flow/` | 4,5 | Korrekt, aber `Evaluate` (~90 Zeilen) zu komplex |
| `consume/` | 4,5 | Klar; Descriptor-Doppelparse mit ignoriertem Fehler |

**Stärken:** Compile-to-Closures konsequent (kein AST-Walk zur Laufzeit); Ressourcenlimits (ADR-0008) kooperativ im gesamten Eval-Pfad, Ranges lazy gestreamt; Division/Overflow → `null` statt Panik; dreiwertige Logik korrekt; Fuzz-Targets über jede Untrusted-Input-Schicht.

### 4.2 Service-Layer (`service`, `cmd`, `mcp`, `vcs`, `assist`, `audit`) — Note 4,5

| Aspekt | Bewertung |
|---|---|
| Security-Architektur | Scoped Keys (ADR-0028) sauber: Konstantzeit-Vergleich, nur Hashes persistiert, 0600-Keystore, `DisallowUnknownFields`, Body-Limits überall |
| Betriebs-Defaults | ⚠️ Kein TLS im Binary, keine Server-/Outbound-Timeouts, kein Rate-Limiting, offener `/v1/chat`-Kostenendpunkt bei bestimmter Fehlkonfiguration |
| Code-Qualität | `service/http.go` mit 1 571 Zeilen; sechs Handler duplizieren wortgleich das Ende, obwohl `respondSaved` existiert |
| Betrieb | Vorbildlich: ehrliche Liveness/Readiness-Trennung, `/v1/status`, Graceful Shutdown in korrekter Reihenfolge, distroless-nonroot-Dockerfile |

### 4.3 Web-Frontend (`web/`) — Note 4,5

| Aspekt | Bewertung |
|---|---|
| Architektur | Diszipliniertes Vanilla-TS, `api.ts` als einziger typisierter HTTP-Layer (43 Funktionen, RFC-7807-bewusst); Schwachpunkt `main.ts` als 1 280-Zeilen-Gott-Modul |
| Typdisziplin | Referenzniveau: `strict`, **0× `any`** in 10,6 kZeilen, 3 chirurgische Casts |
| Sicherheit | ⚠️ Drei verschiedene HTML-Escaper, einer unvollständig → Injection-Lücke (H1/H2) |
| Tests | Starke E2E-Suite gegen den echten `temisd` inkl. WASM; **null Unit-Tests** trotz viel purer Logik |
| Dependencies | 3 Runtime-Deps, 368 KB Bundle (109 KB gzip), ADR-0016 real umgesetzt (kein CDN, kein dmn-js) |

### 4.4 Doku, CI/CD, Prozess — Note 5,0

- **Doku:** Umfang und Systematik überdurchschnittlich (Zielgruppen-Trennung Mensch/Agent/Integrator); Drift genau dort, wo kein automatischer Check existiert (README-Status, `40-api-contract.md` §1.1 mit nicht existenten `WithClock`/`WithLocale`, `00-overview.md` D6, ADR-Status-Hygiene, dupliziertes Auth-Kapitel und private Ops-Interna in `SKILL.md`).
- **CI:** 4 Jobs (verify inkl. Performance-Budget, web + dist-Drift, Playwright-E2E, proto-Drift) — stark. Fehlend: Coverage-Gate (obwohl docs/50 §8 ≥ 90 % *fordert*), `govulncheck`/Security-Scanning, Dependabot, Nightly-Fuzz, Docker-Build-Smoke, `go-version-file` statt hart codiertem "1.23".
- **Release:** `release.yml` vollständig gebaut (Cross-Builds, GHCR, CHANGELOG-Extraktion) — **nie ausgeführt**.
- **Hygiene:** Exzellente PR-Disziplin; aber `feel.wasm` (4,5 MB) **doppelt** committet (`web/public/` + `web/dist/`), `web/dist` von 66 Commits angefasst → irreversibles Repo-Wachstum. Keine Governance-Dateien (SECURITY.md, CONTRIBUTING.md, CODEOWNERS).

---

## 5. Konsolidiertes Findings-Register

Schweregrade: **K** = Kritisch, **H** = Hoch, **M** = Mittel, **N** = Niedrig. ✔ = im Audit empirisch verifiziert.

### Kritisch

| ID | Finding | Ort | WP |
|---|---|---|---|
| K1 ✔ | FEEL-Parser ohne Rekursionstiefen-Limit → `fatal error: stack overflow` crasht den ganzen Prozess; Eingabe (8 MiB `-…-1`) liegt innerhalb `maxBodyBytes`. Verstoss gegen ADR-0008. Gleiche Wurzel macht `internal/feel`-Tests unter `-covermode=atomic` flaky | `internal/feel/parser.go` (parseUnary/parsePostfix/parseExpr) | WP-130 |

### Hoch

| ID | Finding | Ort | WP |
|---|---|---|---|
| H1 | `esc()` escapt keine Anführungszeichen, wird aber in `value="…"`-Attributen genutzt → funktionaler Bug bei FEEL-String-Literalen **und** Stored-XSS-Vektor über server-persistierte Flow-Descriptoren | `web/src/flows.ts:44`, Verwendungen in `flow-editor.ts` | WP-131 |
| H2 | Ungeescaptes `innerHTML` im Typ-Dropdown; Typnamen kommen vom Server (Item-Definitions) | `web/src/main.ts:173` | WP-131 |
| H3 ✔ | First-Run gebrochen: `boot()` returnt auf leerem Server vor der Verdrahtung von `showModel`/Suche/Flows → „Neues Modell" endet im `ReferenceError` | `web/src/main.ts:642` | WP-134 |
| H4 | LLM- und GitHub-Outbound-Calls ohne Timeout (`http.DefaultClient` bzw. `nil`); hängender Provider blockiert Handler-Goroutine unbegrenzt (Assist bis 8 Turns) | `assist/anthropic/anthropic.go:77`, `assist/openai/openai.go:75`, `vcs/github/github.go:72` | WP-132 |
| H5 | Kein TLS im Binary, h2c-Klartext; alle Credentials (API-Keys, Git-/LLM-Token) reisen unverschlüsselt, Betriebsannahme „externer TLS-Proxy" nirgends erzwungen oder sichtbar | `service/http.go:500`, `cmd/temisd/main.go:239` | WP-132 |
| H6 | `/v1/chat` als offener Kosten-Proxy: server-seitiger `TEMIS_LLM_TOKEN` gesetzt + keine Keys → anonyme Aufrufer verbrennen LLM-Guthaben; Warnung existiert für clio, nicht hierfür | `cmd/temisd/main.go:138`, `service/http.go:429` | WP-133 |
| H7 | Architektur-Drift: `internal/drg` ist leere Hülle; die dokumentierte Verantwortung (DAG, Zyklen, Eval-Plan) liegt real in `dmn/graph.go`/`eval.go` | `internal/drg/doc.go`, `docs/10-architecture.md` §2/§3.2 | WP-144 |
| H8 | Kein einziges Release: keine Tags, `release.yml` nie gelaufen, CHANGELOG nur `[Unreleased]`, README verspricht Artefakte, die es nie gab | Repo-weit | WP-136 |
| H9 | WP-41 (TCK-Konformität ≥ 95 %) offen — die Kernbehauptung „DMN 1.5 / volles FEEL" ist extern nicht belegt; `docs/tck-exceptions.md` (referenziert in docs/50 §5) existiert nicht | `internal/tck/` (Runner fertig, Korpus fehlt) | WP-41 |

### Mittel

| ID | Finding | Ort | WP |
|---|---|---|---|
| M1 | Keine HTTP-Server-Timeouts (`ReadHeader`/`Read`/`Write`/`Idle`) → Slowloris-anfällig | `cmd/temisd/main.go:239` | WP-132 |
| M2 | Kein Rate-Limiting auf HTTP/gRPC/MCP; `evaluate`, `/v1/chat`, rekompilierende Modeler-Edits ungedrosselt | `service/http.go` | WP-133 |
| M3 ✔ | `DELETE /v1/models/{id}` löscht nur aus dem Cache; mit `-models-dir` kehrt das Modell beim nächsten `GET` zurück (kein `diskStore.delete`) | `service/http.go:732`, `service/persist.go` | WP-135 |
| M4 | Unbegrenzt wachsende Dedupe-Map im clio-Worker (nie beschnitten, zudem redundant zur clio-Precondition) | `cmd/temis-clio-worker/main.go:119` | WP-135 |
| M5 ✔ | `TestHTTPScopeAuthorization/git_ok` ruft die echte GitHub-API → Testsuite nicht hermetisch | `service/authz_http_test.go:147` | WP-135 |
| M6 | Race beim Modellwechsel: `showModel` ohne Generation-Guard; veralteter Graph kann Canvas gewinnen, Save schriebe Zustand von Modell A in Modell B | `web/src/main.ts:1174` | WP-142 |
| M7 | WASM-Ladefehler wird für immer gecached; FEEL-Validierung „fails open" ohne Nutzerhinweis, kein Retry | `web/src/feel.ts:36` | WP-142 |
| M8 | LLM-BYOK-Key im Klartext in `localStorage` (mit H1/H2 exfiltrierbar) | `web/src/assist.ts:12` | WP-131 |
| M9 | Kein Coverage-Gate im CI, obwohl docs/50 §8 „≥ 90 %" fordert — Behauptung nicht durchgesetzt | `.github/workflows/ci.yml`, `Makefile` | WP-137 |
| M10 | Kein `govulncheck`/gosec/CodeQL/Container-Scan, kein Dependabot, kein Nightly-Fuzz | `.github/` | WP-137 |
| M11 | Doku-Drift: README (WP-33 „offen", Go-Badge 1.24 vs. go.mod 1.23), `40-api-contract.md` §1.1 (`WithClock`/`WithLocale` existieren nicht), `00-overview.md` D6 (dmn-js trotz ADR-0016), SKILL.md (Duplikat + private Ops-Interna), ADR-Status „proposed" trotz Umsetzung | diverse | WP-138 |
| M12 | WP-100 doppelt vergeben; WP-67 „done" trotz offener Abhängigkeiten WP-64/65 | `docs/20-roadmap.md` | WP-138 |
| M13 | `service/http.go` 1 571 Zeilen; sechs Handler kopieren wortgleich den `compileAndStore`-Endblock statt `respondSaved` | `service/http.go:767–1122` | WP-141 |
| M14 | Frontend-Duplikation: `el()` 5×, `fmt` 5×, `coerce` 2×, HTML-Escaper 3×; `main.ts` mit 9 fast identischen `open*/create*`-Paaren, `ModelerHandle` mit 24 Callback-Settern | `web/src/*` | WP-142 |
| M15 | Keine Frontend-Unit-Tests (pure Logik in `layout.ts`, `flows.ts`, `evaluate.ts` nur transitiv per E2E gedeckt); kein ESLint | `web/` | WP-143 |
| M16 | `flow.Evaluate` zu lang/komplex; `consume` parst Descriptor doppelt mit verworfenem Fehler | `flow/evaluate.go:108`, `consume/consume.go:295` | WP-144 |
| M17 | Architektur-Doku verspricht vorab berechneten topologischen Eval-Plan; real: memoisierte DFS pro Call (funktional korrekt, Doku falsch) | `docs/10-architecture.md` §1/§3.2 | WP-138 |

### Niedrig (Auswahl)

| ID | Finding | Ort | WP |
|---|---|---|---|
| N1 | `feel.wasm` doppelt committet (2× 4,5 MB), `web/dist` 66× angefasst → irreversibles Repo-Wachstum | `web/public/`, `web/dist/` | WP-145 |
| N2 | Release ohne SBOM/Signatur/Provenance | `.github/workflows/release.yml` | WP-146 |
| N3 | Keine Governance-Dateien (SECURITY.md, CONTRIBUTING.md, CODEOWNERS) | Repo-Wurzel | WP-139 |
| N4 | `/docs` lädt Swagger-UI vom CDN (bricht air-gapped; widerspricht Offline-Linie des Modelers) | `service/openapi.go:28` | WP-141 |
| N5 | `/v1/status` bei offener API öffentlich (Infra-Disclosure); Default-clio-URL zeigt auf Dritt-Host | `service/status.go:92`, `cmd/temisd/main.go:29` | WP-133 |
| N6 | GitHub-`ref`/`path` nicht gegen `..`-Segmente validiert; Modell-ID nicht gegen `^sha256:[0-9a-f]{64}$` geprüft (Defense-in-Depth) | `vcs/github/github.go:357`, `service/persist.go:34` | WP-135 |
| N7 | Graph-Eval stempelt kein `AuthKid` in den DecisionRecord (WP-105-Lücke bei Whole-Graph-Evals) | `service/http.go:1206` | WP-135 |
| N8 | 19 `*coverage*_test.go`-Dateien: inhaltlich überwiegend legitime Branch-Tests, aber Organisation nach Coverage-Ziel statt Feature | `dmn/`, `service/`, u. a. | WP-144 |
| N9 | Toter Code: `saveModel` (`api.ts:252`) ohne Aufrufer; `web/feel-spike/` obsolet; `boolBinop`/`NullExpr`-Indirektionen | diverse | WP-142/144 |
| N10 | Overlays ohne `role="dialog"`/Fokus-Trap (Vorlage existiert in `operate.ts`); 2 fixe `waitForTimeout(200)` in E2E | `web/src/dialog.ts`, `web/e2e/orientation.spec.ts` | WP-143 |
| N11 | Clio-Status-Polling alle 20 s auch bei verstecktem Tab; Vite 5/TS 5.6 altern | `web/src/main.ts:548`, `web/package.json` | WP-142 |

---

## 6. Stärken (bewusst festgehalten)

1. **Architektur-Governance:** 33 begründete ADRs, nummerierte Doku mit Zielgruppen-Trennung, Roadmap mit testbaren Akzeptanzkriterien pro WP — und der Code hält sich fast überall daran.
2. **Drift-Schutz, wo automatisierbar:** API-Golden-Test (`dmn/testdata/api/dmn.api`), OpenAPI↔Routen-Test, dist-/proto-Drift-Checks im CI.
3. **Dependency-Disziplin:** 4 direkte Go-Deps, 3 Frontend-Runtime-Deps — die „keine neuen Deps"-Regel wird sichtbar gelebt.
4. **Limit-Disziplin im Eval-Pfad** (ADR-0008): kooperative Schritte-/Tiefen-/Listen-Limits, lazy Ranges, typisierte `LimitError` → `CodeLimitExceeded`.
5. **Dezimal-Korrektheit** (ADR-0007): apd-Kontext, half-even, `0.1+0.2=0.3`, Grenzfälle → `null` statt Panik.
6. **Testkultur:** ~25 kZeilen Tests, Tabellentests, Fuzz über jede Untrusted-Schicht, Performance-Budget als CI-Gate, E2E gegen den echten Auslieferungspfad (embedded Frontend + WASM).
7. **Security-Grundlagen:** Konstantzeit-Key-Vergleich, nur Hashes persistiert, Body-Limits an allen Boundaries, Bootstrap-Secret nur per Env.
8. **Betriebsreife-Ansätze:** ehrliche Liveness/Readiness-Trennung, `/v1/status`, Graceful Shutdown, distroless-nonroot-Container.
9. **Frontend-Typdisziplin:** `strict` ohne ein einziges `any` in 10,6 kZeilen; `api.ts` als einziger, vollständig typisierter HTTP-Layer.

---

## 7. Massnahmenplan — Arbeitspakete

Nummerierung schliesst an die bestehende Systematik an (höchste vergebene Nummer: WP-121; WP-122–129 bleiben als Puffer für laufende Produktarbeit frei). Format wie `docs/20-roadmap.md`: ID, Abhängigkeiten, testbares Akzeptanzkriterium (AK). Aufwand: S ≤ 0,5 Tag · M ≤ 2 Tage · L ≤ 5 Tage.

### Etappe H1 — „Härten" (Sicherheit & Stabilität; vor jedem Release)

| WP | Titel | Abhängt von | Aufwand | Akzeptanzkriterium |
|---|---|---|---|---|
| WP-130 | **Parser-/Decoder-Tiefenschutz** (K1) | – | M | Tiefenzähler im FEEL-Parser (Limit via `Limits`, Default z. B. 10 000) → `*ParseError` statt Stack-Overflow; Fuzz-Seeds mit tiefer Schachtelung (`----…1`, `((((…))))`, `[[[[…]]]]`); analoger Schutz für `internal/xml`-Decoder und `detectCycles`. AK: die heute crashende 8-MiB-Eingabe liefert eine Diagnostic; `internal/feel` unter `-covermode=atomic` 10× stabil; `make verify` grün. |
| WP-131 | **Frontend-Escaping konsolidieren** (H1, H2, M8) | – | M | Ein zentraler `escapeHtml()` inkl. `"`/`'` in `dom.ts`; `flows.ts:44`, `highlight.ts:26`, `api.ts:214` konsolidiert; `main.ts:173` auf `createElement`; innerHTML-Templates in `flow-editor.ts` auf `el()`-Builder migriert. LLM-Key-Persistenz optional (Session-only als Default). AK: Flow mit FEEL-String-Literal (`"Winter"`) editierbar ohne Attribut-Riss; E2E-Spec mit feindlichem Descriptor-/Typnamen zeigt kein Skript-/Attribut-Injection. |
| WP-132 | **Timeouts & TLS-Transparenz** (H4, H5, M1) | – | M | `http.Server` mit `ReadHeaderTimeout`/`ReadTimeout`/`WriteTimeout`/`IdleTimeout`; Default-`http.Client` mit Timeout in `assist/*` und `vcs/github` (nil-Client → Default **mit** Timeout); optional `-tls-cert`/`-tls-key`; Startup-Log macht Klartext-Betrieb explizit. AK: hängender Fake-LLM-Server bricht nach Timeout ab (Test); `temisd`-Start loggt Transportmodus. |
| WP-133 | **Missbrauchs-Schutz** (H6, M2, N5) | – | M | Startup-Warnung (oder Verweigerung per Flag) bei `TEMIS_LLM_TOKEN` ohne Key-/Token-Schutz; einfacher Token-Bucket vor `requireScope` (mind. `/v1/chat` + rekompilierende Modeler-Routen); `/v1/status` bei offener API überdenken (dokumentieren oder gaten). AK: Fehlkonfiguration erzeugt Warnlog; Flut auf `/v1/chat` wird 429-gedrosselt (Test). |
| WP-134 | **First-Run reparieren** (H3) | – | S | `boot()` returnt nicht mehr früh; leere Liste wird gerendert, alle Aktionen verdrahtet. AK: E2E-Spec mit `-examples=false`: „Neues Modell", „Neuer Flow", „Neuer Ordner", Suche funktionieren auf leerem Server. |
| WP-135 | **Service-Korrektheit & hermetische Tests** (M3, M4, M5, N6, N7) | – | M | `diskStore.delete(id)` implementiert und von `handleDeleteModel` aufgerufen; Worker-Dedupe-Map entfernt oder LRU-begrenzt; `git_ok`-Test gegen `httptest`-Fake statt echter GitHub-API; `ref`/`path`-Segmente und `modelId` (`^sha256:[0-9a-f]{64}$`) validiert; `AuthKid` im Graph-Eval-Record. AK: „Delete überlebt Restart-Fallback"-Test grün; `go test ./...` offline vollständig grün. |

### Etappe H2 — „Release & Nachweis" (Sichtbarkeit, Vertrauen, CI-Härtung)

| WP | Titel | Abhängt von | Aufwand | Akzeptanzkriterium |
|---|---|---|---|---|
| WP-136 | **Erstes Release `v0.1.0`** (H8) | WP-130–135 | S | CHANGELOG-Abschnitt geschnitten, Tag gepusht, `release.yml` läuft erstmals real durch: Binaries (6 Plattformen), `checksums.txt`, GHCR-Image, Release-Notes aus CHANGELOG. AK: `go get github.com/pblumer/temis@v0.1.0` und `docker pull ghcr.io/…` funktionieren. |
| WP-137 | **CI-Härtung** (M9, M10) | – | M | (a) `govulncheck ./...`-Step; (b) `dependabot.yml` für gomod/npm/actions; (c) Coverage-Messung mit Schwellwert für `dmn`, `internal/feel`, `internal/boxed` (oder 90-%-Ziel aus docs/50 streichen); (d) `go-version-file: go.mod`; (e) Docker-Build-Smoke im PR-CI; (f) Nightly-Fuzz-Lane (`make fuzz`). AK: alle Lanes grün auf main; absichtliche Coverage-Regression bricht CI. |
| WP-138 | **Doku-Drift & Roadmap-Hygiene** (M11, M12, M17) | – | S | README-Status (WP-33, Go-Badge), `40-api-contract.md` §1.1 bereinigt (`WithClock`/`WithLocale` raus oder implementiert), `00-overview.md` D6 auf ADR-0016, SKILL.md dedupliziert + Ops-Interna in privates Repo, ADR-Status proposed→accepted für Umgesetztes, WP-100-Kollision aufgelöst (Flow-Registry → neue ID), WP-67/64/65-Status konsistent, `10-architecture.md` beschreibt memoisierte DFS statt Eval-Plan. AK: Stichproben-Greps finden keine der gelisteten Drifts mehr. |
| WP-139 | **Governance-Dateien** (N3) | – | S | `SECURITY.md` (Disclosure, unterstützte Versionen, Default-Posture „offen ohne Keys" dokumentiert), `CONTRIBUTING.md` (verweist auf docs/60), `CODEOWNERS`. AK: Dateien vorhanden, von README verlinkt. |
| WP-113 | Metriken (`expvar` + `/metrics`) — *bestehendes WP* | WP-111 | M | AK unverändert aus `docs/20-roadmap.md`. |
| WP-114 | Strukturierte Logs (`log/slog`) — *bestehendes WP* | WP-32 | M | AK unverändert aus `docs/20-roadmap.md`. |
| WP-41 | **TCK-Konformität** — *bestehendes WP, höchste Priorität dieser Etappe* (H9) | WP-40 | L | Offizielles DMN-TCK-Korpus eingebunden (Runner existiert); Quote im CI eingefroren; `docs/tck-exceptions.md` angelegt; Ziel ≥ 95 % der anwendbaren Cases. AK: CI-Lane zeigt Quote; README nennt die Zahl mit Badge. |

### Etappe H3 — „1.0-Reife" (Wartbarkeit, Skalierung, Restarbeiten)

| WP | Titel | Abhängt von | Aufwand | Akzeptanzkriterium |
|---|---|---|---|---|
| WP-141 | **`service/http.go` entflechten** (M13, N4) | – | M | Aufteilung entlang Modeler/Eval/Flow/Keys; sechs duplizierte Handler-Enden auf `respondSaved`; Swagger-UI-Assets embedded statt CDN. AK: keine Datei > 800 Zeilen in `service/`; `/docs` offline funktionsfähig; Verhalten per bestehender Testsuite unverändert. |
| WP-142 | **Frontend-Konsolidierung** (M6, M7, M14, N9, N11) | WP-131 | L | `dom.ts`/`format.ts` (el/fmt/coerce dedupliziert); Generation-Guard in `showModel`/`showFlow`; `feel.ts`-Ladefehler nicht cachen + Status-Hinweis „ohne Validierung"; Boxed-Typ-Registry statt 9 `open*/create*`-Paaren und 24 Callbacks; toten Code (`saveModel`, `feel-spike/`) entfernen; Polling bei verstecktem Tab pausieren. AK: neuer Boxed-Typ = 1 Registry-Zeile; Doppelklick-Wechseltest (A→B schnell) zeigt nie veralteten Canvas. |
| WP-143 | **Frontend-Test-Lane** (M15, N10) | WP-142 | M | Vitest für pure Logik (`layout.ts`, `flows.ts` buildGraph/depthMap, `evaluate.ts` leafInputs/coerce); ESLint (`no-unsanitized`, `@typescript-eslint`) im `web-check`; `waitForTimeout` durch `expect.poll` ersetzt; Dialoge mit `role="dialog"`/Fokus-Trap. AK: `make web-check` umfasst Lint+Unit; CI grün. |
| WP-144 | **Engine-Hygiene** (H7, M16, N8, N9) | – | M | `internal/drg` mit Graph-Logik gefüllt **oder** gelöscht + Doku angepasst; `flow.Evaluate` in Decision-/Service-Zweig extrahiert; `consume`-Doppelparse beseitigt; `coverage_*_test.go` in Feature-Testdateien aufgelöst; `boolBinop`/`NullExpr` entfernt. AK: `go test ./...` grün, Coverage nicht gesunken, Paketstruktur = Doku. |
| WP-145 | **Binärartefakte eindämmen** (N1) | WP-136 | M | `feel.wasm` nur noch einfach im Baum (dist beim Build gefüllt) oder Git-LFS; Optionen dokumentiert als ADR (Trade-off „go build ohne Node" bleibt gewahrt, z. B. via Release-Artefakt oder `go generate`). AK: ein Commit, der die WASM erneuert, vergrössert das Repo nur noch 1×. |
| WP-146 | **Supply-Chain-Härtung** (N2) | WP-136 | M | SBOM (syft) für Archive + Image; cosign-Signaturen; optional SLSA-Provenance (`actions/attest-build-provenance`). AK: Release-Assets enthalten SBOM + Signaturen; Verifikations-Anleitung im README. |
| WP-62–66 | Modeler-Etappe abschliessen — *bestehende WPs* | — | L | AKs unverändert aus `docs/20-roadmap.md` (1.5-Round-Trip Client, Command-Stack, Table-/DRD-/Boxed-Editoren vollenden). |

---

## 8. Roadmap

```
        Horizont 1 (Woche 1–2)      Horizont 2 (Woche 3–8)          Horizont 3 (Quartal)
        „Härten"                    „Release & Nachweis"            „1.0-Reife"
        ──────────────────────      ──────────────────────────      ─────────────────────────
Sicherheit   WP-130 Parser-Limit    WP-137 CI-Härtung               WP-146 Supply-Chain
             WP-131 XSS/Escaping    WP-139 SECURITY.md              
             WP-132 Timeouts/TLS                                    
             WP-133 Rate-Limit                                      
Korrektheit  WP-134 First-Run       WP-41  TCK-Konformität ◀━ Kern  WP-144 Engine-Hygiene
             WP-135 Delete/Tests                                    
Sichtbarkeit                        WP-136 Release v0.1.0           v1.0.0 (nach WP-41)
                                    WP-138 Doku-Drift               
Betrieb                             WP-113 /metrics                 WP-145 Artefakte/LFS
                                    WP-114 slog                     
Frontend                                                            WP-142 Konsolidierung
                                                                    WP-143 Test-Lane
                                                                    WP-62–66 Modeler-Rest
```

**Meilensteine:**

| Meilenstein | Inhalt | Kriterium |
|---|---|---|
| **M1 „Gehärtet"** (Ende Horizont 1) | WP-130–135 | Kein bekannter Prozess-Crash durch Eingaben; keine bekannte Injection; Testsuite offline grün |
| **M2 „v0.1.0"** (Horizont 2) | WP-136–139, 113, 114 | Erstes öffentliches Release mit Binaries + Image; CI mit Vuln-/Coverage-Gates; Doku drift-frei |
| **M3 „v1.0.0"** (Horizont 3) | WP-41 + WP-141/144 + Modeler-Rest | TCK ≥ 95 % nachgewiesen und im CI eingefroren; `dmn`-API final; Modeler-Etappe DoD erfüllt |

**Priorisierungslogik:** Horizont 1 behebt alles, was einem öffentlichen Release entgegensteht (Crash, XSS, Kosten-DoS) — deshalb strikt vor WP-136. WP-41 ist der grösste Glaubwürdigkeits-Hebel des Projekts und dominiert Horizont 2/3; alle Refactorings (WP-141–144) sind bewusst nachgelagert, weil sie Verhalten nicht ändern und die Testbasis stark genug ist, sie jederzeit sicher durchzuführen.

---

## 9. Anhang: Bewertungsskala

Schweizer Schulnoten: 6 = hervorragend · 5 = gut · 4 = genügend · 3 = ungenügend · 2 = schwach · 1 = unbrauchbar. Aufwandsklassen: S ≤ 0,5 Personentag · M ≤ 2 PT · L ≤ 5 PT.

*Erstellt durch automatisiertes Audit (vier unabhängige Review-Stränge + empirische Verifikation), 2026-07-02.*
