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

- **Quality-Report – welcher Datensatz welche Regel verletzt (ADR-0034):** Die **Lese-Seite** der
  Produktivläufe (ADR-0031) und die Antwort auf „ich lasse ein ganzes Regelset über 70 000 Server
  laufen und will am Schluss die Auswertung". Neues, read-only **`package quality`** aggregiert die
  `com.temis.quality.evaluated.v1`-Events (aus clios `run-query`-NDJSON) zu einem Report: distinct
  Entitäten, wie viele bestanden, je **verletzender** Entität die sortierte Liste der Regel-IDs und
  eine **Rangliste je Regel**. Drei Kanäle über denselben Kern: **CLI `temis-quality-report`**
  (Text/JSON, `-fail-on-violation` als CI-Gate), **`GET /v1/quality/report`** (Scope `audit`; der
  Server fragt clio selbst ab, kein Token im Browser; `409 CLIO_NOT_CONFIGURED` ohne Sink) und ein
  **Report-Panel im Import-Cockpit** (Tabelle „Entität × verletzte Regeln"). Dazu das gebündelte
  Beispiel-Regelset **`server_compliance`** (COLLECT-Tabelle mit unabhängigen Server-Checks, die die
  verletzten Regel-IDs als Liste ausgibt). Ein End-to-End-Test streamt 70 000 synthetische Server
  durch den Batch und prüft die aggregierten Tallies. Reine Standardbibliothek, kein neuer Dependency.

- **clio-Command-Consumer – Entscheidungen per Event auslösen (WP-120/121, ADR-0033):** Die
  **Gegenrichtung** zum Logbuch. Ein in clio geschriebenes **Command-Event**
  `com.temis.decision.requested.v1` löst eine Auswertung aus — Einzel-Decision (`modelId`+
  `decision`), ganzer Modell-Graph (`modelId`) oder Decision-Flow/DRG (`flowId`) —, und das
  Ergebnis fliesst **korreliert** (`data.requestId`, gleicher `subject`) als bestehendes
  `com.temis.decision.evaluated.v1`/`com.temis.flow.evaluated.v1` zurück; nicht auswertbar →
  `com.temis.decision.failed.v1`, sodass **jedes** Command eine Antwort bekommt. Neues
  `package consume` (über `dmn`/`flow`/`audit`, **kein** `internal/`-/`service`-Import,
  symmetrisch zu `package audit`) + Binary **`temis-clio-worker`**: beobachtet Commands über
  clios **`observe`**-Stream (mit `run-query`-Backfill je Reconnect; `-poll`/`-once`-Modi),
  wertet über die öffentliche Engine-API aus und schreibt idempotent zurück (Precondition auf
  `requestId`, `409` = No-op). **Zustandslos** — clio hält den gesamten Zustand; damit bleibt
  der Consumer Decisioning und wird **nicht** zur Prozess-Engine (Grenze aus ADR-0025 gewahrt).
  Kopplung nur über clios HTTP-Vertrag, Kern unberührt, reine stdlib (ADR-0011/0014). Die
  `data`-Verträge liegen maschinenlesbar als **JSON Schema** in `docs/schemas/` (Command +
  Ergebnis-Events); das Command-Schema lässt sich in clio (`register-event-schema`)
  hinterlegen, sodass fehlerhafte Commands **beim Schreiben** abgewiesen werden. Ein
  `consume/schema_test.go` bindet die Schemas dependency-frei an die erzeugten Events.
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
  Default an) auf **demselben Modell-Cache und Flow-Katalog** wie Modeler und `/v1`-API — vorgeladene
  Beispiele und Modeler-Modelle sind über MCP sichtbar und umgekehrt, eine `modelId` über alle
  Oberflächen; ebenso erscheint ein über MCP `load_flow`/`git_load_flow` registrierter Flow im
  Flow-Katalog des Modelers (`GET /v1/flows`) und umgekehrt (`mcp.WithFlowStore` /
  `Server.FlowStore()`); das eigenständige `temis-mcp` (stdio/HTTP) bleibt unverändert.
- **Decision-Flow – transitive Step-Inputs (ADR-0026, L2a):** Ein Flow-Step auf eine
  **zusammengesetzte** Decision darf jetzt deren **transitiv benötigte** Blatt-Inputs
  verdrahten — Inputs, die die Ziel-Decision nur über eine Sub-Decision desselben Modells
  bezieht (z. B. `FinalPremium`, das `VehicleValue` allein über `BasePremium` braucht). Zuvor
  waren solche Decisions in Flows faktisch unbenutzbar: der transitive Input wurde als
  `FLOW_UNKNOWN_INPUT` abgelehnt bzw. lief bei Weglassen still auf `null`. Wiring-Validierung
  und Auswertung eines Decision-Steps arbeiten nun gegen die **Requirements-Cone** der
  Ziel-Decision statt nur gegen ihre direkt deklarierten Inputs; die transitiven Werte werden
  bis in die Sub-Decisions durchgereicht (inkl. Typ-Koerzierung numerischer Inputs). Echte
  unbekannte Inputs (`FLOW_UNKNOWN_INPUT`) und fehlende required-Inputs (`FLOW_INPUT_UNWIRED`)
  werden weiterhin präzise gemeldet. Neue additive `dmn`-API `ReachableInputSchema` /
  `ValidateReachableInput` (cone-gescopt, analog zu `ModelInputSchema`/`ValidateModelInput`);
  MCP `describe_decision` weist die Menge additiv als `reachableInputs` neben `inputs` aus.
- **Modeler – Deluxe-JSON-Editor an jedem JSON-Eingabefeld (ADR-0016):** Überall, wo ein Feld
  seinen Wert als FEEL/JSON entgegennimmt — die **Auswerten**-Eingaben (Operate), das
  **Flow-auswerten**-Panel und das **Testen**-Formular des Flow-Designers — steht jetzt neben dem
  einzeiligen Feld ein **`{ }`-Icon**, das einen **großzügigen JSON-Editor** als Modal öffnet.
  Der Editor gibt eine Monospace-Textfläche mit viel Platz, **Live-Validierung** (gültiges JSON ✓
  bzw. die Parser-Meldung), **Formatieren**/**Kompakt**/**Kopieren**-Werkzeuge, Tab-Einrückung und
  Tastatur-Shortcuts (Strg/Cmd+Enter = Übernehmen, Esc = Abbrechen). Beim Öffnen wird vorhandenes
  JSON eingerückt dargestellt, beim „Übernehmen" kompakt ins Feld zurückgeschrieben. Geschlossene
  Aufzählungsfelder (`<select>`) bekommen kein Icon. Rein additiv, keine Backend-Änderung.
- **Flow-Designer – Flows via UI erstellen & designen (WP-116, ADR-0026):** Decision-Flows lassen
  sich jetzt **im Modeler visuell erstellen, designen und testen**, nicht nur ansehen & ausführen.
  Ein neuer **Flow-Designer** (betretbar über das **+** in der FLOWS-Sidebar oder **„✎ Bearbeiten"**
  im Studio) bietet einen **strukturierten Inspector** — Flow-Name/Version, deklarierte Inputs
  (Name + FEEL-Typ), Steps mit **Modell- + Decision-Picker** und **FEEL-Input-Verdrahtung**
  (Vorschläge aus Flow-Inputs + Step-IDs; **Auto-Wiring** übernimmt die Inputs der gewählten
  Decision und referenziert gleichnamige Flow-Inputs) sowie Output-Mapping — neben einer
  **Live-Graph-Preview**, die den Cross-Model-DRG beim Tippen neu zeichnet. **„Testen"** wertet den
  Entwurf inline aus (`POST /v1/flow/evaluate`, ohne Registrierung) und *illuminiert* die Preview;
  **„Prüfen"** validiert gegen die geladenen Modelle; **„Registrieren & Öffnen"** legt den Flow im
  Katalog ab und öffnet ihn im Studio; **„Export"** lädt den `*.flow.json`-Deskriptor herunter.
  **Git bleibt die dauerhafte Quelle (ADR-0032):** die Registrierung ist der flüchtige Dev-Pfad,
  der Export der Weg in den Repo (`flows/` + `git_propose`) — kein neuer server-seitiger
  Schreibpfad. Rein additiv, keine Backend-Änderung.
- **Modeler – Auto-Layout mit orthogonalem Routing & Orientierungs-Umschalter (ADR-0016):** Modelle ohne
  authorede `DMNDI`-Bounds werden nicht länger als diagonaler „Spaghetti" gezeichnet. Das Auto-Layout
  richtet die Knoten spaltenweise aus (jeder Knoten wird über/unter seine Nachbarn gezogen) und führt
  jede Requirement-Kante als **rechtwinkligen Konnektor**: die Eingänge eines Hubs laufen als sauberer
  Kamm zusammen, und lange „Skip"-Kanten werden durch eine freie Bahn zwischen den Spalten gefädelt, statt
  durch Knoten hindurchzulaufen. Ein Toolbar-Knopf **Bottom-up / Top-down** schaltet um, ob die Eingabe-
  Pillen die Decisions von unten (Pfeile nach oben, Default) oder von oben (Pfeile nach unten) speisen, und
  ordnet das ganze Diagramm entsprechend neu an. Authorede `DMNDI`-Layouts bleiben unangetastet (bis der
  Umschalter ein Neu-Anordnen erzwingt); der Decision-Flow-Canvas ist unverändert.
- **Modeler – Modelle verwalten (ADR-0016):** Im Modeler lässt sich ein Modell jetzt komplett neu
  (leer) anlegen statt nur eine `.dmn`-Datei hochzuladen, sowie **umbenennen** und **löschen**
  (inkl. des gesamten Revisions-Verlaufs). Zwei neue HTTP-Endpunkte: `POST /v1/models/{id}/rename`
  (setzt den Definitions-Namen, legt eine neue Revision an) und `DELETE /v1/models/{id}` (entfernt
  eine Revision aus dem Cache); neue Library-Funktion `dmn.SetModelName`. Anlegen, Umbenennen und
  Löschen laufen über eigene In-App-Dialoge (kein `window.prompt`), mit Hinweis bei Namensdopplung.
- **Modeler – Operate-Cockpit (ADR-0016):** Die **Operate**-Sicht (Auswerten/Betreiben) ist jetzt klar
  von der **Design**-Sicht abgegrenzt — eigener, kühler „Cockpit"-Look (blaue Chrome-Farbwelt, getönter
  Canvas) und read-only Graph. Sie besteht aus drei Bausteinen: (1) eine **Läufe-Historie oben** über
  dem Diagramm, rein per Tastatur blätterbar (↑/↓/←/→/j/k, Pos1/Ende, Enter) als ARIA-`listbox` mit
  `aria-activedescendant`/`aria-selected`; der Wechsel des aktiven Laufs aktualisiert Diagramm und
  Overlays. (2) **Halbtransparente, schwebende Overlays** (frosted/Backdrop-Blur, ein-/ausblendbar)
  direkt über dem Diagramm fassen Eingangsdaten (links/oben) und Ergebnisse (rechts/unten) zusammen,
  während die grünen Ergebnis-Pills an den Knoten erhalten bleiben. (3) **Hover-Grafik**: über einer
  Ergebniszeile erscheint die Entscheidungstabelle als Matrix mit hervorgehobener getroffener Regel,
  numerische Werte als Mini-Bars. Reines Frontend, baut auf derselben Auswerte-Logik auf (kein neuer
  Endpunkt, keine neue Dependency).
- **Modeler – FEEL-Editor-Assistenz überall (ADR-0016):** Syntax-Highlighting (Funktionen, Variablen,
  Schlüsselwörter, Strings, Zahlen als farbige Token hinter dem transparenten Feld) und
  Code-Completion (In-Scope-Variablen + Engine-Built-ins, aufklappend unter dem Cursor beim Tippen
  oder per Ctrl/Cmd+Leertaste) stehen jetzt in **allen** FEEL-Eingabefeldern zur Verfügung — nicht
  mehr nur im Literal-, Decision-Table- und BKM-Editor, sondern auch in den Boxed-Editoren
  **Conditional** (Wenn/Dann/Sonst), **Filter**, **Iteration**, **Liste**, **Relation** und
  **Boxed Context**. Alle Felder laufen über eine gemeinsame Primitive (`attachFeelField`), sodass
  Highlighting und Completion nicht mehr auseinanderlaufen oder bei neuen Editoren vergessen werden
  können. Der Funktionskatalog kommt weiterhin direkt aus der echten Engine (WASM), reines Frontend.
- **Operate – Entscheidungs-Pfad in der Tabelle:** Ein Doppelklick auf eine Decision mit Tabelle zeigt
  im Operate-Modus jetzt den **genommenen Weg** grafisch: eine Chip-und-Pfeil-Leiste
  (Eingabewert → getroffene Regel → Ergebnis), der getestete Eingabewert je Spaltenkopf und eine
  **Pass/Fail-Heatmap** über alle Regeln mit leuchtend hervorgehobener getroffener Regel. Das
  Hover-Popover der Ergebnis-Overlays wurde korrekt im Viewport positioniert (lag zuvor außerhalb des
  Sichtbereichs) und hoverbare Zeilen sind mit einem ⊞-Marker gekennzeichnet.
- **Modeler – Import-Cockpit (ADR-0016):** Ein dritter Modus **Import** neben Design/Operate — ein
  Testfall-Stapellauf als **Fließband**. Man lädt eine **Vorlage** (CSV **oder** JSON) herunter, die
  exakt zu den Leaf-Inputs des Modells passt (dieselbe autoritative Eingabemenge wie das Auswerte-
  Formular, `leafInputs`), füllt sie mit Testdaten — von Hand, in der Tabellenkalkulation oder von
  einem **KI-Agenten** (dokumentiertes, agentenfreundliches Format) — und importiert sie (Datei-
  Auswahl oder Drag & Drop). Optionale `→Decision`-Spalten machen aus einer Zeile eine **Pass/Fail-
  Erwartung**. „Durchlaufen lassen" wertet den **ganzen Stapel in EINEM Batch-Request** aus und lässt
  die Datensätze von links (**Eingang**) durch die **Evaluation** nach rechts in den **clio Store**
  fliegen — samt berechneter Ergebnisse und Pass/Fail-Badges. Eigene kühle Cyan-Chrome-Farbwelt
  (`--imp`), respektiert `prefers-reduced-motion`.
  **Durchsatz (Folge-Fix):** Neuer Endpunkt **`POST /v1/models/{id}/evaluate-graph-batch`** wertet
  viele Eingabezeilen in einem Round-Trip aus (die Engine schleift in-memory, ohne Traces; jede Zeile
  unabhängig — eine abgelehnte Zeile bricht den Batch nicht ab). Damit laufen **5000 Testfälle in
  ~50 ms** statt tausender Einzel-Requests. Das Cockpit ruft den Batch statt einer Schleife auf,
  verzichtet auf künstliche Pro-Datensatz-Pausen und **begrenzt die gezeichneten Karten pro Lane**
  (Zähler + Overflow-Hinweis zeigen die echte Menge) — die Animation ist bewusst nur *angedeutet* als
  gestaffelte CSS-Kaskade, statt tausende DOM-Knoten einfrieren zu lassen.
  **Test- vs. Produktivlauf & clio-Quality-Events (ADR-0031):** Das Cockpit unterscheidet einen
  **Testlauf** (Default, schreibt **nichts**) von einem **Produktivlauf**, der pro ausgewertetem Fall
  ein **Quality-Event** `com.temis.quality.evaluated.v1` **auf der Entität** nach clio schreibt
  (Subject `/quality/<entity>`, mit `violation`-Flag aus den erwarteten Werten) — so werden Reports
  über Verletzungen je Entität möglich. Die Entität kommt aus einer **`entity`-Vorlagenspalte**, sonst
  einem wählbaren Eingabefeld, sonst dem Fall-Label. Die Zustellung läuft **entkoppelt über eine
  garantierte Queue mit Backpressure** (`QualityQueue`): der Batch-Response kehrt sofort zurück,
  Hintergrund-Worker liefern mit Retry & Idempotenz (clio-Precondition), `temisd` drainiert die Queue
  beim **Graceful-Shutdown**. Ohne konfiguriertes clio wird ein Produktivlauf klar mit
  **`409 CLIO_NOT_CONFIGURED`** abgelehnt (opt-in, Default aus — kein Datenabfluss).
  **Ergebnis-CSV:** Nach einem Lauf schreibt das Cockpit die **berechneten Decision-Ausgaben** je
  Fall in eine CSV (Fall/Entität/Eingaben + eine Spalte je Decision, plus `status`-Spalte
  OK/Abweichung/Fehler bei Erwartungen) und bietet sie als **„Ergebnisse · CSV ↓"** zum Download an —
  das ausgefüllte Testblatt mit den Outputs.
- **clio-Audit auch für Whole-Graph-Auswertung:** Der „Auswerten"-Pfad des Modelers
  (`POST /v1/models/{id}/evaluate-graph`) wird jetzt ebenfalls protokolliert — **ein
  `com.temis.decision.evaluated.v1`-Event je ausgewerteter Decision** (best-effort, bzw. `502` bei
  `-clio-strict`; idempotent per `(modelId, decision, input)`). Zuvor auditierte der Sink nur
  Einzel-Decision- und Flow-Auswertungen, sodass genau die interaktive Graph-Auswertung nicht im
  Logbuch landete.
- **clio-Entscheidungs-Logbuch (WP-54, ADR-0023):** `temisd` protokolliert optional jede
  Einzel-Decision-Auswertung als manipulationssicheres `com.temis.decision.evaluated.v1`-CloudEvent
  in einer [clio](https://github.com/pblumer/clio)-Instanz — Flags `-clio-url`/`-clio-token`/
  `-clio-source`/`-clio-subject-prefix`/`-clio-subject-key`/`-clio-strict` (`$TEMIS_CLIO_*`), Default
  **aus** (byte-identisch). Idempotent per clio-Precondition (`inputHash`); `-clio-strict` macht den
  Sink fail-closed (`502 AUDIT_WRITE_FAILED`), sonst best-effort. Reine stdlib, kein Go-Import von
  clio (Kopplung nur über dessen HTTP-API, ADR-0011/0014).
- **Dateisystem-Modell-Store (ADR-0027):** `temisd` persistiert seinen Modell-Cache optional
  auf das Dateisystem — Flag `-models-dir` (`$TEMIS_MODELS_DIR`), Default **aus** (byte-identisch
  rein in-memory). Hochgeladene und im Modeler editierte Modelle werden content-adressiert als rohes
  DMN-XML (`<sha256>.dmn`, atomarer Write) abgelegt und beim Start wieder in den Cache geladen, sodass
  sie einen Neustart überleben. Nur das rohe XML liegt auf der Platte (Kompilat/Index/Diagnostik werden
  deterministisch neu erzeugt); ein aus dem LRU-Cache verdrängtes, aber persistiertes Modell wird
  on-demand von der Platte rekompiliert. Die gebündelten Beispiele werden nie persistiert (re-embed per
  `go:embed`). Reine stdlib, kein neuer Dependency; Persistenz hängt am einzigen Choke-Point
  `compileAndStore`/`lookup`, greift also auch für Modeler-Saves, MCP, gRPC und Git-Load.
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
  Flag hat weiterhin Vorrang vor der Env-Variable. Das clio-Audit-Logbuch zeigt jetzt
  standardmäßig auf die gehostete clio (`https://clio.blumer.cloud`), bleibt aber **aus, bis
  ein `TEMIS_CLIO_TOKEN` gesetzt ist** — kein Datenabfluss im Default, Anschalten ist ein
  einziger Schritt (Token setzen oder `-clio-url` auf die eigene clio zeigen); der Start-Banner
  weist auf die Verfügbarkeit hin.
- **Betriebs-Observability (WP-110–112, ADR-0030):** `temisd` ist jetzt *observierbar*.
  `/healthz` (Liveness) und `/readyz` (echte Readiness) sind **ehrlich getrennt** — `/readyz`
  liefert `503`, wenn eine harte Startbedingung fehlt (z. B. ein fail-closed `-clio-strict`
  clio unerreichbar ist); ein best-effort-clio-Ausfall lässt es bewusst bei `200`. Neu:
  **`GET /v1/status`** zeigt den Zustand der Umsysteme (clio/LLM/Git) und die Last der Engine
  — clio `writesOk`/`writesFailed`/`idempotentSkips`, `lastOk`/`lastError`, `reachable`, dazu
  Version/Uptime/Cache-Zähler; **secret-frei** (kein Token/Key im Body) und hinter dem
  `audit`-Scope (ADR-0028; `admin`-Keys lesen ebenfalls, offen ohne Auth-Konfig).
  clio-Erreichbarkeit standardmäßig **passiv** aus echten Writes;
  `-clio-active-probe`/`TEMIS_CLIO_ACTIVE_PROBE` schaltet einen aktiven Health-Ping zu. Reine
  stdlib (`sync/atomic`), Zähler allokationsfrei im Hot Path, Engine-Kern unberührt (ADR-0011).
- **API-Stabilisierung (WP-43):** `package dmn` als v1 zugesagt; SemVer-/Deprecation-Policy;
  Golden-Surface-Test gegen unbeabsichtigte Brüche.
- **Doku & Release (WP-45–46):** godoc-Beispiele, Integrations-/Quickstart-Leitfaden; versionierte
  Release-Pipeline, Container-Image für `temisd`, dieses Changelog.

### Fixed

- **Modeler – Palette „klebendes" Element (ADR-0016):** Ein aus der Design-Palette gezogenes
  Element blieb gelegentlich am Cursor „kleben" und ließ sich nur per Esc/Neuladen lösen.
  Ursache war der Geister-Klick, den der Browser nach einem abgebrochenen nativen Drag noch
  auf den Palette-Eintrag feuert — er startete eine zweite, verwaiste Erstell-Sitzung. Die
  Klick-Aktion ignoriert diesen Nachzügler jetzt (und einen Klick, während schon eine Sitzung
  läuft). Zusätzlich bekommen neu erstellte Elemente eindeutige Vorgabenamen
  („Neue Decision", „Neue Decision 2", …), damit zwei gleichnamige Knoten nicht stumm
  kollidieren.

### Docs

- **OpenAPI & API-Vertrag mit dem Modeler synchronisiert:** Die 13 Modeler-Endpunkte
  (ADR-0016 — Graph, Item Definitions, Decision-Tables, Literal-Expressions, BKM, Save)
  sowie `GET /v1/models/{id}/xml` und `POST /v1/models/{id}/evaluate-graph` sind jetzt in
  `service/openapi.yaml` (Pfade + Schemas) und `docs/40-api-contract.md` §2.1 dokumentiert;
  README entsprechend ergänzt. Ein neuer Test (`TestOpenAPICoversDataRoutes`) gleicht die
  registrierten `/v1`-Routen gegen die OpenAPI-Pfade ab, sodass die Spec nicht mehr stillschweigend
  von der Implementierung abdriften kann.
- **Entscheidungs-Logbuch via clio (ADR-0023, WP-54–56 komplett):** ADR-0023 und
  `docs/80-clio-decision-log.md` beschreiben ein revisionssicheres Entscheidungs-Logbuch über das
  Schwesterprojekt [clio](https://github.com/pblumer/clio) — versionierter
  `com.temis.decision.evaluated.v1`-CloudEvent-Vertrag, opt-in-Sink in `temisd` (WP-54, siehe oben)
  und Re-Audit-Tool `temis-reaudit` (WP-55, siehe oben). WP-56 ergänzt das **Agent-Muster
  „delegieren → protokollieren"** (`docs/80` §5 mit lauffähigem Beispiel, `docs/60-ai-agent-guide.md`
  §8) — ein Agent gibt die Entscheidung an temis (`evaluate`) und schreibt sie selbst nach clio
  (`write-events`), ganz ohne neuen temis-Code.

[Unreleased]: https://github.com/pblumer/temis/commits/main
