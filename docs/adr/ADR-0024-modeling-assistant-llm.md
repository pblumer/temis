# ADR-0024: Eingebauter Modellierungs-Assistent über einen LLM-Provider (Standardbibliothek, kein SDK)

- **Status:** accepted
- **Datum:** 2026-06-30
- **Kontext-WP:** WP-80 (Chat-Assistent / LLM-Modellierungshilfe im Modeler)

## Kontext

Bisher war temis konsequent die **gerufene** Seite einer KI-Interaktion: ADR-0013
positioniert die Engine als **Verifikationswerkzeug**, das ein externer Agent über
MCP/HTTP delegiert, um eine regelbasierte Entscheidung deterministisch zu prüfen. Der
neue Wunsch dreht die Richtung um: ein **eingebauter Chat-Assistent im Modeler** soll
Anwender beim **Bauen** von Decisions unterstützen — FEEL erklären, Decision-Tables
vorschlagen und auf Wunsch direkt anlegen/ändern. Dafür muss temis selbst einen
**LLM aufrufen**, angebunden an ein Konto bei **Anthropic oder OpenAI** (API-Token).

Das berührt drei harte Projektlinien, die eine bewusste Entscheidung verlangen:

1. **Goldene Regel 6 (keine ungeprüften externen Abhängigkeiten).** Beide Anbieter
   haben Go-SDKs; das Projekt pflegt jedoch bewusst Schmalheit (eine Laufzeit-Dep,
   `apd/v3`) und hat dieselbe Frage für MCP schon einmal zugunsten der
   Standardbibliothek entschieden (ADR-0014). Ein LLM-Aufruf ist HTTP+JSON — genau die
   Klasse, die temis schon zweimal stdlib-nativ gelöst hat (MCP, GitHub-Provider in
   `vcs/github`).
2. **Anbieter-Bindung.** „Anthropic **oder** OpenAI" heißt: nicht an einen Anbieter
   koppeln. Das spiegelt das bewährte Provider-Interface-Muster aus ADR-0022 (`vcs`:
   ein Interface, GitHub als erster Provider).
3. **Datenschutz / Vertraulichkeit.** Anders als die übrige Engine, die rein lokal und
   deterministisch arbeitet, **verlässt** beim Assistenten Modellkontext (Decisions,
   FEEL, Beispiel-Eingaben) den Prozess Richtung Dritter. Das muss **opt-in**,
   per Default **aus** und transparent sein — keine stille Exfiltration.

## Optionen

**Provider-Anbindung**

1. **Offizielles Anthropic-/OpenAI-SDK je Anbieter.** Beste Feature-Abdeckung,
   Streaming/Retry „kostenlos". — Zwei fette Dep-Bäume in einem 1-Dep-Projekt,
   Versions-/Go-Stand-Kopplung, Anbieter-Lock pro SDK. Widerspricht ADR-0014-Linie.
2. **Ein Provider-Interface + stdlib-Clients** (diese Entscheidung). `assist.Provider`
   als schmale Schnittstelle (ein nicht-streamender `Complete`-Aufruf mit Tools), je
   ein `net/http`+`encoding/json`-Client für Anthropic (Messages API) und OpenAI (Chat
   Completions). — Null neue Deps, kein Go-Bump, voller Anbieter-Wechsel per Konfig.
   Kosten: Spec-/Feature-Evolution (Streaming, neue Tool-Formate) selbst pflegen.

**Token-/Betriebsmodell**

1. **Nur server-seitig** (`-llm-token`/Env in `temisd`): Token nie im Browser; der
   Server proxyt. Konsistent mit dem bestehenden `-token`-Muster, sicher für geteilte
   Deployments. — Erzwingt aber Server-Config für jeden Nutzer.
2. **Nur Browser (BYOK)**: Nutzer trägt eigenen Key ein, Browser ruft den Anbieter
   direkt. — Kein Server-Secret, aber Key im Browser, CORS/Anbieter-Limits, und der
   Agent-Loop (Tool-Calling gegen temis) müsste im Browser laufen.
3. **Beides** (diese Entscheidung): Server-Token als Default; optional darf der Nutzer
   im UI einen eigenen Key hinterlegen, der pro Anfrage vorrangig genutzt wird. Der
   Agent-Loop läuft **immer server-seitig** (ein Adressraum mit dem Modell-Cache,
   wie ADR-0021), der BYOK-Key wird nur durchgereicht, nie persistiert.

## Entscheidung

- Ein neues, anbieter-agnostisches Paket **`assist/`** (reine Standardbibliothek):
  - `assist.Provider` — schmale Schnittstelle: `Complete(ctx, Request) (Response, error)`
    mit System-Prompt, Nachrichten, Tool-Katalog. **Nicht streamend** (ein
    Request/Response je Modell-Zug), wie schon bei ADR-0015 die ausreichende Form.
  - `assist.Agent` — der **Tool-Calling-Loop**: ruft den Provider, führt verlangte
    Tools über einen `assist.Executor` aus, speist die Ergebnisse zurück, bis das
    Modell eine finale Textantwort liefert (durch `maxSteps` beschränkt — Goldene
    Regel 7).
  - `assist.Executor` — die **Werkzeug-Oberfläche** (wie `vcs.Reader` ein Interface,
    das die Aufrufseite implementiert), damit `assist` frei von `service`/`internal`
    bleibt.
  - `assist/anthropic` und `assist/openai` — je ein stdlib-Client, der `Provider`
    erfüllt.
- Der **Service-Adapter** (`service/assist.go`) implementiert `assist.Executor` über
  den **geteilten Modell-Cache** (Tools spiegeln die vorhandenen MCP-/`/v1`-
  Operationen: `list_models`, `describe_decision`, `get_decision_table`, `evaluate`,
  `load_model`, `save_decision_table`) und bietet **`POST /v1/chat`** an. Der
  Endpunkt ist **per Default aus** und nur aktiv, wenn `temisd` mit
  `WithAssist(...)` (Flags/Env) gestartet wird. Server-Token als Default,
  optionaler BYOK-Key per `X-LLM-Token`-Header.
- Wie die übrigen `/v1`-Endpunkte wird `/v1/chat` vom optionalen `-token` bewacht.

Begründung: Der Eigenbau gibt **keine** Interoperabilität auf — auf dem Draht ist der
Aufruf identisch mit einem SDK-basierten —, hält das Projekt bei **einer** Laufzeit-Dep,
vermeidet einen Go-Bump und ist konsistent mit ADR-0014 (MCP-stdlib) und ADR-0022
(Provider-Interface). Der server-seitige Agent-Loop teilt den Adressraum mit dem
Modell-Cache (ADR-0021), sodass der Assistent dieselben Modelle sieht, die im Modeler,
über `/v1` und über MCP leben — und über `load_model`/`save_decision_table` Erstelltes
sofort dort erscheint.

## Konsequenzen

**Positiv**
- Keine neue Abhängigkeit; das Projekt bleibt bei `apd/v3`. Kein Go-Bump.
- Anbieter-agnostisch: Anthropic **oder** OpenAI per Konfiguration, weitere Anbieter
  später als zusätzlicher `Provider` (ein Interface, eine Datei).
- Der Assistent ist ein **Agent-First-Bürger**: er nutzt dieselben deterministischen
  Werkzeuge (`evaluate`, `describe_decision`), die ADR-0013 externen Agenten gibt —
  er kann seine eigenen Decision-Table-Vorschläge gegen die echte Engine **prüfen**,
  statt zu raten.
- Default aus, opt-in, Token server-seitig: keine Verhaltensänderung für bestehende
  Deployments, keine stille Datenweitergabe.

**Negativ / Kosten**
- Modellkontext verlässt beim aktiven Assistenten den Prozess Richtung Dritter
  (Anthropic/OpenAI). Gegenmaßnahme: opt-in, dokumentiert; nur der für die Anfrage
  nötige Kontext wird gesendet; BYOK-Key wird nie persistiert.
- Spec-/Feature-Evolution der Anbieter-APIs (Streaming, neue Tool-/Content-Block-
  Formate) muss selbst nachgezogen werden. Gegenmaßnahme: Konformitätstests gegen die
  dokumentierten Request/Response-Formen je Anbieter (httptest-Fakes).
- Nicht-streamende Antworten: lange Generierungen erscheinen erst am Stück. Akzeptiert
  für den heutigen Scope (kurze Modellierungs-Dialoge); Streaming bleibt nachrüstbar.

**Revisit-Trigger**
Ein neues ADR ersetzt dieses, sobald **Streaming**, **mehr als drei Anbieter** oder
fortgeschrittene Provider-Features (Vision, Prompt-Caching-Steuerung, Batch) gefordert
sind, deren Eigenbau die SDK-Kosten überwiegt — analog zum Revisit-Mechanismus von
ADR-0014.
