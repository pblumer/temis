# ADR-0014: MCP über Standardbibliothek implementieren (kein offizielles Go-MCP-SDK)

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-50 (löst die in ADR-0013 zugesagte Folge-Entscheidung zu SDK/Transport ein)

## Kontext

ADR-0013 hat temis als Agent-First-Verifikationswerkzeug positioniert und als erste
Säule den MCP-Server (WP-50) festgelegt — samt der ausdrücklichen Folgeaufgabe, die
Wahl von **MCP-SDK und Transport** in einem eigenen ADR mit Abhängigkeitsbegründung
festzuhalten (Goldene Regel 6: neue Dependency = ADR + Begründung). Dieses ADR ist
diese Entscheidung.

Die Frage ist **nicht**, ob temis den MCP-Standard spricht — das tut es in jedem Fall.
**MCP ist das Protokoll (JSON-RPC 2.0), nicht das SDK.** Ein korrekt implementierter
Server ist auf dem Draht für jeden MCP-Client ununterscheidbar von einem SDK-basierten;
Interkonnektivität und Standardkonformität hängen an der Protokolltreue, nicht an der
gewählten Bibliothek. Die eigentliche Frage ist, **womit** wir den Standard
implementieren: mit dem offiziellen Go-SDK oder mit der Standardbibliothek.

Auslöser für eine bewusste Entscheidung war der berechtigte Einwand, für
Interkonnektivität solle man „auf Standards setzen". Die empirische Prüfung des
offiziellen SDK (`github.com/modelcontextprotocol/go-sdk`, Stand 2026-06) ergab harte
Kosten, die gegen den unbestreitbaren Nutzen abzuwägen sind:

- **Go-Mindeststand.** Die aktuellen stabilen Versionen (ab v1.4.x, latest v1.6.x)
  verlangen **Go ≥ 1.25**. temis steht auf `go 1.23`. Nach ADR-0011 ist die Library das
  Primärartefakt mit SemVer-Disziplin — ein angehobener Go-Mindeststand trifft **jeden
  Einbetter** und ist selbst eine spürbare Kompatibilitätsänderung. Nur die ältere
  v1.3.x-Reihe bleibt auf Go 1.23.
- **Abhängigkeitsbaum.** Das SDK zieht ~9 transitive Module (u. a. `golang-jwt`,
  `golang.org/x/oauth2`, `golang.org/x/tools`, `google/jsonschema-go`,
  `segmentio/encoding`+`asm`, `uritemplate`). temis hat heute **genau eine** Dependency
  (`apd/v3`, ADR-0007) und pflegt diese Schmalheit bewusst (eigener FEEL-Lexer/Parser
  ADR-0004, eigener HTTP-Mux statt Router, eingebettete UI ohne CDN). Ein Großteil der
  SDK-Deps bedient Transporte/Features (OAuth, HTTP), die der stdio-Server nicht braucht.

Demgegenüber steht der reale Nutzen des SDK: automatisches Nachziehen der
Spec-Evolution, abgedeckte Konformität in Randfällen (Versionsverhandlung, Batching,
Cancellation/Progress), künftige Transporte (Streamable HTTP/SSE) ohne Eigenbau und der
Status als kanonische Referenz (Anthropic + Google).

## Optionen

1. **Offizielles SDK, latest (v1.6.x).** Beste Spec-/Feature-Abdeckung, zukunftssicher,
   kanonisch. — Hebt den Library-Mindeststand Go 1.23 → 1.25 (trifft alle Einbetter,
   ADR-0011) und bläht das 1-Dependency-Projekt auf ~10 Module auf, großteils für nicht
   benötigte Transporte/Features.
2. **Offizielles SDK, gepinnt v1.3.x.** Standard ohne Go-Bump (bleibt Go 1.23). — Weiter
   ~9 transitive Deps; ein bis zwei Releases hinter latest, neueste Spec-Zusätze erst mit
   späterem Go-Bump. Der Hauptnutzen „immer aktuelle Spec" wird damit teilweise
   aufgegeben, die Hauptkosten (Dep-Baum) bleiben.
3. **Eigenbau über die Standardbibliothek** (diese Entscheidung). JSON-RPC 2.0 über
   stdio mit `encoding/json` + `bufio`. — Null neue Abhängigkeiten, kein Go-Bump, volle
   Kontrolle und Konsistenz mit der Projektkultur. Kosten: Spec-Evolution und
   Randfall-Konformität müssen selbst gepflegt, weitere Transporte selbst gebaut werden.

## Entscheidung

Option 3. Der MCP-Server (`package mcp`, `cmd/temis-mcp`) wird mit der
Standardbibliothek implementiert; das offizielle Go-MCP-SDK wird **vorerst nicht**
aufgenommen.

Begründung: Für den heutigen Scope von WP-50 — **ein** Transport (stdio), **vier**
Tools, ein schlanker Initialize/`tools`-Handshake — ist die SDK-Oberfläche
überdimensioniert, und ihr konkreter Preis (Go-1.25-Zwang **oder** Versionsrückstand,
in beiden Fällen ~9 transitive Deps in einem 1-Dependency-Projekt) überwiegt den Nutzen.
Der Eigenbau gibt **keine** Standardkonformität auf: Das Draht-Protokoll ist identisch,
die Interkonnektivität mit MCP-Clients bleibt vollständig erhalten. Die Entscheidung
ist außerdem die einzige, die ohne neue externe Abhängigkeit auskommt und damit Goldene
Regel 6 nicht aktiviert — passend zur belegten Linie des Projekts.

**Konformität absichern.** Da die Spec-Treue nun in unserer Verantwortung liegt, wird
sie durch Tests gegen die MCP-Verhaltensregeln abgesichert (Handshake, Notifications
ohne Antwort, Fehlercodes, Tool-Result-Form mit `isError`) statt einem SDK anvertraut.

## Konsequenzen

**Positiv**
- Keine neue Abhängigkeit; das Projekt bleibt bei einer einzigen (`apd/v3`).
- Kein angehobener Go-Mindeststand — kein Kompatibilitätsbruch für Library-Einbetter
  (ADR-0011).
- Volle Kontrolle über die schmale Protokolloberfläche, konsistent mit ADR-0004
  (eigener FEEL-Stack) und dem eigenen HTTP-Mux.
- Standardkonformität und Interkonnektivität bleiben erhalten (identisches Draht-Protokoll).

**Negativ / Kosten**
- Spec-Evolution (neue Protokollversionen/Features) muss selbst nachgezogen werden;
  Gegenmaßnahme: Konformitätstests gegen die Spec-Beispiele.
- Weitere Transporte (Streamable HTTP/SSE) und fortgeschrittene MCP-Features
  (Resources, Prompts, Sampling, Elicitation, OAuth) wären Eigenbau.

**Revisit-Trigger (wann dieses ADR abgelöst wird)**
Ein neues ADR ersetzt dieses und nimmt das offizielle SDK auf, sobald **beides** gilt:
1. Ein Nicht-stdio-Transport (Streamable HTTP/SSE) **oder** fortgeschrittene
   MCP-Features werden gefordert, deren Eigenbau die SDK-Kosten überwiegt; **und**
2. ein Go-Mindeststand von 1.25 ist für die Library akzeptabel (bzw. das SDK ist dann
   wieder mit dem Projekt-Go-Stand kompatibel).

> **Update (ADR-0015):** Bedingung 1 ist eingetreten — ein remote/HTTP-routebarer
> Transport wurde gefordert. Sie wurde jedoch **nativ** erfüllt (stdlib
> Streamable HTTP, da unser Server reines Request/Response ist, ohne Streaming/
> Sampling), sodass Bedingung 2 nicht aktiviert wurde. Das SDK bleibt
> zurückgestellt; dieses ADR bleibt gültig. Details in **ADR-0015**.
