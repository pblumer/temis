# Umsetzungsplan — Audit-Massnahmen Temis

**Stand:** 2026-07-02 · Begleitdokument zu `2026-07-02-code-quality-audit.md`
**Zweck:** Ausführungsreife Detailplanung der Arbeitspakete WP-130–146 (plus Einordnung der bestehenden offenen WPs). Jedes WP ist so geschnitten, dass ein einzelner Contributor/Agent es eigenständig, testgetrieben und ohne Rückfrage umsetzen kann.

> **Arbeitsregel (wie `docs/60-ai-agent-guide.md`):** Tests zuerst. Bearbeite das oberste offene WP, dessen Abhängigkeiten `done` sind. `make verify` (bzw. `make web-check`) muss nach jedem WP grün sein. Kein WP verändert die öffentliche `dmn`-API ohne bewusstes `-update-api`.

---

## 0. Sequenzierung & Abhängigkeiten

```
Horizont 1 „Härten"  (side-effect-frei, in einer Session machbar)
  WP-130 ─┐
  WP-131 ─┤ (unabhängig, parallelisierbar)
  WP-132 ─┤
  WP-133 ─┼─► WP-135 (nutzt Timeout-/Validierungs-Bausteine mit)
  WP-134 ─┘
                    │
Horizont 2 „Release & Nachweis"
  WP-137 (CI)  WP-138 (Doku)  WP-139 (Governance)   ← unabhängig, sofort
  WP-113 WP-114 (Observability)                     ← unabhängig
  WP-136 (Release v0.1.0) ◀── braucht Horizont 1 done + Tag-Push (manuell/freigabepflichtig)
  WP-41  (TCK) ◀── externes Korpus, grösster Brocken
                    │
Horizont 3 „1.0-Reife"  (Refactorings, verhaltensneutral)
  WP-141 WP-142 WP-143 WP-144 WP-145 WP-146  + Modeler-Rest WP-62–66
```

**Aufwandsklassen:** S ≤ 0,5 PT · M ≤ 2 PT · L ≤ 5 PT.
**Ein-Session-Eignung:** ✅ = vollständig automatisiert in einem Rutsch umsetzbar · ⚠️ = teilweise (externe Abhängigkeit/Entscheidung) · ⛔ = nicht one-shot (externe Aktion, Freigabe oder Mehrtages-Umfang).

---

## Horizont 1 — Härten

### WP-130 — Parser-/Decoder-Tiefenschutz · M · ✅

**Ziel:** K1 schliessen — kein `fatal error: stack overflow` mehr auf tiefer Eingabe.

**Betroffene Dateien:**
- `internal/feel/parser.go` (Descent-Funktionen: `parseExpr` ~114, `parseUnary` ~238, `parsePostfix` ~256, `parseListOrInterval` ~472)
- `internal/feel/limits.go` bzw. `dmn/limits.go` (neues Feld `MaxParseDepth`, Default z. B. 10 000)
- `internal/xml/decode.go` (Element-Tiefenzähler im Decoder)
- `dmn/graph.go` `detectCycles` (Tiefen-/Besuchsschranke absichern)
- Fuzz-Seeds: `internal/feel/testdata/fuzz/FuzzParser/` (+ neue tiefe Seeds)

**Schritte:**
1. **Test zuerst:** In `internal/feel/parser_test.go` (oder neuem `depth_test.go`) Fälle `strings.Repeat("-",N)+"1"`, `"("*N+"1"+")"*N`, `"["*N+"]"*N` mit N über Limit → erwartet `*ParseError` (kein Crash). Ausserdem ein `-covermode=atomic`-Stabilitätslauf dokumentieren.
2. `depth int` + `maxDepth` in den Parser-Zustand; in jeder Descent-Funktion `p.enter()`/`defer p.leave()`; bei Überschreitung `panic(&ParseError{…})`, das der bestehende `recover` in `ParseWithNames` (Z. 39–47) bereits fängt.
3. Limit über `Limits.MaxParseDepth` konfigurierbar, ungesetzt → `DefaultLimits`.
4. Analoge Tiefenschranke im XML-Decoder (Wrapper um `xml.Decoder`-Token-Stream statt `xml.Unmarshal`, oder Tiefenzähler in einem `UnmarshalXML`).
5. Fuzz-Seed mit tiefer Schachtelung ergänzen.

**AK:** die heute crashende 8-MiB-Eingabe liefert eine Diagnostic statt Absturz; `go test ./internal/feel -covermode=atomic -count=10` stabil; `make verify` grün; `make fuzz` crash-frei.

**Risiko:** niedrig — additive Schranke, keine Semantikänderung für gültige Ausdrücke. Grenze so wählen, dass reale DMN-Modelle (Schachtelung < 100) nie betroffen sind.

---

### WP-131 — Frontend-Escaping konsolidieren · M · ✅

**Ziel:** H1, H2, M8 schliessen.

**Betroffene Dateien:**
- neu `web/src/dom.ts` (zentrale `escapeHtml()` inkl. `"`/`'`, plus `el()`-Builder-Reexport)
- `web/src/flows.ts:44` (`esc` → entfernen/umleiten), `web/src/highlight.ts:26`, `web/src/api.ts:214` (drei Escaper konsolidieren)
- `web/src/main.ts:173` (Dropdown auf `createElement('option')`)
- `web/src/flow-editor.ts` (innerHTML-`value="…"`-Templates → `el()`-Builder oder escaped)
- `web/src/assist.ts:12` (Key-Persistenz)

**Schritte:**
1. **Test zuerst:** neuer E2E-Spec `web/e2e/escaping.spec.ts` — Flow mit FEEL-String-Literal `"Winter"` in der Verdrahtung editieren (Attribut darf nicht reissen); Item-Definition-Typname bzw. Descriptor mit `"><img onerror=…>` → kein injiziertes Element/Attribut im DOM.
2. `escapeHtml()` mit vollständigem Zeichensatz (`& < > " '`) in `dom.ts`.
3. Alle drei Alt-Escaper darauf umbiegen; `main.ts:173` auf DOM-Builder (Muster aus `table.ts:330`).
4. LLM-Key: Default `sessionStorage` (nicht persistent); Opt-in-Checkbox für `localStorage`; UI-Hinweis (`assist.ts:59`) präzisieren.

**AK:** neuer Spec grün; kein `esc()`-Aufruf ohne Attribut-Sicherheit mehr; `make web-check` grün.

**Risiko:** niedrig.

---

### WP-132 — Timeouts & TLS-Transparenz · M · ✅

**Ziel:** H4, H5, M1 schliessen.

**Betroffene Dateien:**
- `cmd/temisd/main.go:239` (Server-Timeouts, optional TLS, Transport-Log)
- `assist/anthropic/anthropic.go:77`, `assist/openai/openai.go:75` (Default-Client mit Timeout bei `nil`)
- `vcs/github/github.go:72` (dito)
- `service/assist.go:139` (Client-Weitergabe)

**Schritte:**
1. **Test zuerst:** `httptest`-Server, der `time.Sleep` > Timeout macht; Assist-/Git-Client bricht mit Timeout-Fehler ab (nicht Client-Disconnect). Kein Hänger.
2. `http.Server{ReadHeaderTimeout, ReadTimeout, WriteTimeout, IdleTimeout}` (Werte kommentiert; Streaming-gRPC beachten → `WriteTimeout` ggf. 0 für die h2c-Streaming-Route, sonst pauschal setzen und Batch-Stream testen).
3. `anthropic.New`/`openai.New`/`github.New`: bei `nil`-Client `&http.Client{Timeout: 60s}` statt `http.DefaultClient`.
4. `cmd/temisd`: expliziter `*http.Client` mit Timeout an `AssistConfig.HTTPClient` und Git.
5. Optional `-tls-cert`/`-tls-key` (`$TEMIS_TLS_*`) → `ListenAndServeTLS`; sonst Startup-Log „HTTP im Klartext (h2c) — TLS extern terminieren".

**AK:** Timeout-Test grün; `temisd`-Start loggt Transportmodus; bestehende gRPC-Batch-Stream-Tests weiter grün.

**Risiko:** mittel — `WriteTimeout` kann Streaming beeinträchtigen. Deshalb Batch-Stream-Test als Gegenprobe im selben WP.

---

### WP-133 — Missbrauchs-Schutz · M · ✅

**Ziel:** H6, M2, N5 schliessen.

**Betroffene Dateien:**
- `cmd/temisd/main.go` (Startup-Warnung LLM-Token ohne Auth; Analogie zu clio-Warnung `service/http.go:301`)
- `service/http.go` (Token-Bucket-Middleware vor `requireScope`; `dataRoutes`)
- `service/status.go:92` (`/v1/status`-Gating dokumentieren/prüfen)

**Schritte:**
1. **Test zuerst:** (a) Konfig `LLM-Token gesetzt + keine Keys` → erwartete Warnung/Verweigerung; (b) N+1 schnelle Requests auf `/v1/chat` → 429.
2. Einfacher In-Memory-Token-Bucket pro Key/IP (stdlib, `golang.org/x/time/rate` nur falls schon vorhanden — sonst eigener kleiner Bucket, keine neue Dep gemäss Regel 6), konfigurierbar `-rate-limit`/`$TEMIS_RATE_LIMIT`, Default aus.
3. Startup-Check in `main.go`.

**AK:** Warn-/429-Tests grün; Default-Verhalten unverändert (Rate-Limit opt-in), `make verify` grün.

**Risiko:** niedrig; Rate-Limit default-aus hält Rückwärtskompatibilität.

---

### WP-134 — First-Run reparieren · S · ✅

**Ziel:** H3 schliessen.

**Betroffene Dateien:** `web/src/main.ts:642` (früher `return` entfernen; Init fortsetzen).

**Schritte:**
1. **Test zuerst:** E2E-Spec `web/e2e/empty-server.spec.ts` mit `-examples=false` (playwright webServer-Args anpassen oder eigener Projekt-Eintrag) — „Neues Modell", „Neuer Flow", „Neuer Ordner", Suche funktionieren.
2. `boot()` so umbauen, dass die leere Liste gerendert wird, aber alle `mount*`/Verdrahtungen (Z. 726/735/940/1081/1174/1276) trotzdem laufen.

**AK:** Spec grün auf leerem Server; bestehende Specs unberührt.

**Risiko:** niedrig; deckt einen bisher untesteten Pfad ab.

---

### WP-135 — Service-Korrektheit & hermetische Tests · M · ✅

**Ziel:** M3, M4, M5, N6, N7 schliessen.

**Betroffene Dateien:**
- `service/persist.go` (neu `delete(id)` → `os.Remove`), `service/http.go:732` (`handleDeleteModel` ruft Store-Delete)
- `cmd/temis-clio-worker/main.go:119` (Dedupe-Map entfernen/LRU)
- `service/authz_http_test.go:147` (`git_ok` gegen `httptest`-Fake statt echter GitHub-API)
- `vcs/github/github.go:357` (`ref`/`path`-Segmentvalidierung), `service/persist.go:34` (`modelId` gegen `^sha256:[0-9a-f]{64}$`)
- `service/http.go:1206` (`AuthKid` in Graph-Eval-Record)

**Schritte:**
1. **Test zuerst:** „Delete überlebt Restart-Fallback" (mit `-models-dir`: DELETE → neuer Server auf gleichem Dir → GET = 404); `git_ok` mit Fake-Backend; Traversal-Reject-Test.
2. `diskStore.delete` implementieren + verdrahten.
3. `git_ok` auf einen konfigurierbaren GitHub-Basis-URL/Fake umstellen (bestehende `vcs/github`-Tests zeigen das Muster).
4. Worker-Dedupe entfernen (clio-`requestId`-Precondition genügt) oder TTL-begrenzen.
5. Segment-/ID-Validierung; `AuthKid` ergänzen.

**AK:** `go test ./...` **offline** vollständig grün (heute schlägt `git_ok` fehl); Restart-Delete-Test grün.

**Risiko:** niedrig; behebt zugleich das nicht-hermetische CI-Verhalten.

---

## Horizont 2 — Release & Nachweis

### WP-137 — CI-Härtung · M · ✅

**Betroffene Dateien:** `.github/workflows/ci.yml`, neu `.github/dependabot.yml`, neu `.github/workflows/nightly.yml`, `Makefile` (Coverage-Target), `docs/50-testing-strategy.md` (§8 mit Realität abgleichen).

**Schritte:** `govulncheck ./...`-Step; `dependabot.yml` (gomod/npm/actions, wöchentlich); Coverage-Messung mit Schwelle für `dmn`/`internal/feel`/`internal/boxed` (`go test -coverprofile` + `go tool cover` + Schwellwert-Skript) **oder** die 90-%-Zusage aus docs/50 §8 streichen; `go-version-file: go.mod` statt `"1.23"`; Docker-Build-Smoke im PR; Nightly-`make fuzz`-Lane (cron).

**AK:** alle Lanes grün auf main; absichtliche Coverage-Regression bricht CI; `govulncheck` als Pflicht-Step.

**Risiko:** niedrig; rein additive CI. ⚠️ `govulncheck` kann bestehende Deps als verwundbar melden → dann Folge-Update nötig.

---

### WP-138 — Doku-Drift & Roadmap-Hygiene · S · ✅

**Betroffene Dateien:** `README.md`, `docs/40-api-contract.md` §1.1, `docs/00-overview.md` D6, `SKILL.md`, `docs/adr/*` (Status), `docs/20-roadmap.md` (WP-100-Kollision, WP-67/64/65), `docs/10-architecture.md` §1/§3.2.

**Schritte:** README-Status (WP-33 done, Go-Badge 1.23) korrigieren; `WithClock`/`WithLocale` aus §1.1 entfernen (oder implementieren — Entscheidung dokumentieren); `00-overview.md` D6 auf ADR-0016; SKILL.md Auth-Duplikat entfernen + private Ops-Interna auslagern; ADR-Status `proposed→accepted` für umgesetzte ADRs; WP-100 (Flow-Registry) auf freie ID umnummerieren; `10-architecture.md` auf „memoisierte DFS + Compile-Zeit-Zyklencheck".

**AK:** Stichproben-Greps finden keine der gelisteten Drifts; ADR-Index konsistent.

**Risiko:** keiner (reine Doku). ⚠️ `WithClock`/`WithLocale`: falls implementieren gewünscht → separates WP, sonst nur streichen.

---

### WP-139 — Governance-Dateien · S · ✅

**Betroffene Dateien:** neu `SECURITY.md`, `CONTRIBUTING.md`, `.github/CODEOWNERS`.

**Schritte:** `SECURITY.md` (Disclosure-Weg, unterstützte Versionen, explizite Doku der Default-Posture „API ohne Keys offen"); `CONTRIBUTING.md` (verweist auf `docs/60`); `CODEOWNERS`.

**AK:** Dateien vorhanden, von README verlinkt.

**Risiko:** keiner.

---

### WP-113 / WP-114 — Observability (bestehende WPs) · je M · ✅

Metriken (`expvar` + optionaler `/metrics`) bzw. strukturierte Logs (`log/slog`). AK unverändert aus `docs/20-roadmap.md`. Beide stdlib-only, klar geschnitten, gute Kandidaten für einen Rutsch.

---

### WP-136 — Erstes Release `v0.1.0` · S · ⛔ (Tag-Push freigabepflichtig)

**Betroffene Dateien:** `CHANGELOG.md` (Abschnitt `[0.1.0]` schneiden), Git-Tag.

**Schritte:** CHANGELOG-Abschnitt aus `[Unreleased]` herauslösen; Tag `v0.1.0` pushen → `release.yml` läuft erstmals real (Binaries, GHCR, Notes). **Erfordert bewusste Freigabe** (Tag ist outward-facing, GHCR-Push, öffentliches Release) und einen grünen main-Stand nach Horizont 1.

**AK:** `go get …@v0.1.0` und `docker pull ghcr.io/…` funktionieren; Release-Assets vollständig.

---

### WP-41 — TCK-Konformität (bestehendes WP) · L · ⚠️

**Betroffene Dateien:** `internal/tck/` (Runner existiert), neu `internal/tck/testdata/corpus/` (offizielles DMN-TCK), `.github/workflows/ci.yml` (TCK-Quote-Lane), neu `docs/tck-exceptions.md`.

**Schritte:** Offizielles DMN-TCK-Korpus **vendoren/beziehen** (externer Download — Lizenz prüfen, ggf. als Git-Submodule oder committet), Runner darüber laufen lassen, Quote einfrieren, Ausnahmen dokumentieren, README-Badge mit Zahl.

**AK:** CI zeigt Quote; ≥ 95 % der anwendbaren Cases grün. ⚠️ **Grösster Brocken, externe Datenquelle** — nicht in einem Rutsch mit dem Rest; eigenes Fokus-WP.

---

## Horizont 3 — 1.0-Reife (verhaltensneutrale Refactorings)

### WP-141 — `service/http.go` entflechten · M · ✅
Aufteilung entlang Modeler/Eval/Flow/Keys (`http_models.go`, `http_eval.go`, `http_flow.go`, `http_keys.go`); sechs duplizierte Handler-Enden → `respondSaved`; Swagger-UI-Assets per `go:embed` statt CDN (`service/openapi.go:28`). **AK:** keine `service/`-Datei > 800 Zeilen; `/docs` offline; bestehende Testsuite unverändert grün.

### WP-142 — Frontend-Konsolidierung · L · ✅
`dom.ts`/`format.ts` (el/fmt/coerce dedupliziert, baut auf WP-131); Generation-Guard in `showModel`/`showFlow` (M6); `feel.ts`-Ladefehler nicht cachen + Status-Hinweis (M7); Boxed-Typ-Registry statt 9 `open*/create*`-Paaren + 24 Callbacks (M14); toter Code raus (`saveModel`, `feel-spike/`); Polling bei verstecktem Tab pausieren. **AK:** neuer Boxed-Typ = 1 Registry-Zeile; schneller A→B-Wechsel zeigt nie veralteten Canvas.

### WP-143 — Frontend-Test-Lane · M · ✅
Vitest für pure Logik (`layout.ts`, `flows.ts`, `evaluate.ts`); ESLint (`no-unsanitized`, `@typescript-eslint`) in `web-check`; `waitForTimeout` → `expect.poll`; Dialoge mit `role="dialog"`/Fokus-Trap (Vorlage `operate.ts`). **AK:** `make web-check` umfasst Lint+Unit; CI grün.

### WP-144 — Engine-Hygiene · M · ✅
`internal/drg` füllen **oder** löschen + Doku (H7); `flow.Evaluate` extrahieren (M16); `consume`-Doppelparse beseitigen; `coverage_*_test.go` in Feature-Dateien auflösen (N8); `boolBinop`/`NullExpr` entfernen (N9). **AK:** `go test ./...` grün, Coverage nicht gesunken, Paketstruktur = Doku.

### WP-145 — Binärartefakte eindämmen · M · ⚠️
`feel.wasm` nur einfach im Baum (dist beim Build) oder Git-LFS; Trade-off als ADR dokumentieren. ⚠️ Berührt `go build`-ohne-Node-Zusage und ggf. CI-Drift-Check → Entscheidung (LFS vs. Build-Artefakt) vorab treffen.

### WP-146 — Supply-Chain-Härtung · M · ⚠️
SBOM (syft) + cosign-Signaturen + optional SLSA-Provenance in `release.yml`. ⚠️ Sinnvoll erst mit/nach WP-136; braucht Signatur-Schlüssel-Setup (Keyless-cosign via OIDC bevorzugt).

### WP-62–66 — Modeler-Etappe abschliessen · L · ⛔
1.5-Round-Trip-Client, Command-Stack, Table-/DRD-/Boxed-Editoren vollenden. Mehrtägig, eigene Etappe — nicht Teil dieses Audit-Massnahmenpakets im engeren Sinn.

---

## 1. „In einem Rutsch?" — realistische Einschätzung

| Bündel | WPs | One-Shot? | Begründung |
|---|---|---|---|
| **Hardening-Pack** | WP-130–135 | ✅ ja | Side-effect-frei, klar umrissen, hohe Testbasis, alle kritischen/hohen Findings. **Empfohlener erster Rutsch.** |
| **CI/Doku/Governance** | WP-137, 138, 139 | ✅ ja | Rein additiv/redaktionell, kein Produktcode-Risiko. |
| **Observability** | WP-113, 114 | ✅ ja | stdlib, spezifiziert. |
| **Refactorings** | WP-141–144 | ✅ ja (eigener Rutsch) | Verhaltensneutral, aber Umfang → besser getrennt vom Hardening, damit Reviews klein bleiben. |
| **Release v0.1.0** | WP-136 | ⛔ | Tag-Push/GHCR/öffentliches Release = outward-facing, freigabepflichtig. |
| **TCK** | WP-41 | ⚠️ | Externes Korpus, Lizenz, L-Aufwand — eigenes Fokus-WP. |
| **Artefakte/Supply-Chain/Modeler** | WP-145, 146, 62–66 | ⚠️/⛔ | Entscheidungen (LFS), Schlüssel-Setup bzw. Mehrtages-Umfang. |

**Fazit:** *Alle 17 WPs gleichzeitig in einem einzigen Rutsch* — nein, das wäre weder review- noch qualitätssicher (Release braucht Freigabe, TCK/Modeler sind mehrtägig/extern). **Sinnvoll und machbar in einem Rutsch ist das Hardening-Pack (WP-130–135)** — es beseitigt jeden verifizierten Crash/XSS/Kosten-DoS und macht die Testsuite offline grün. Danach in getrennten, überschaubaren Rutschen: CI/Doku/Governance, dann Observability, dann Refactorings; Release und TCK als bewusst freigegebene bzw. fokussierte Einzelschritte.
