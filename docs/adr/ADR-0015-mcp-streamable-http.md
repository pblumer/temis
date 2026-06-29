# ADR-0015: Remote-MCP über nativen Streamable-HTTP-Transport (`temis-mcp -http`)

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-53 (verfeinert ADR-0013/ADR-0014)

## Kontext

`temis-mcp` (ADR-0013, WP-50) sprach MCP bisher ausschließlich über **stdio**: Der Agent
startet das Binary als lokalen Subprozess. Das deckt die lokale Nutzung ab, ist aber
**nicht netzwerk-erreichbar** — kein Port, nicht hinter einem Reverse Proxy (Traefik o. ä.)
routebar. Sobald ein Agent-Runtime temis *remote* als geteilten Dienst ansprechen soll,
fehlt ein Netzwerk-Transport.

Das ist exakt der in ADR-0014 dokumentierte **Revisit-Trigger** („Nicht-stdio-Transport
gefordert"). ADR-0014 verknüpfte ihn mit der Frage, ob dann das offizielle Go-MCP-SDK
aufzunehmen sei (das Streamable HTTP mitbrächte), zum Preis von Go ≥ 1.25 und ~9
transitiven Modulen.

Entscheidend ist die Natur unseres Servers: Er ist **reines Request/Response** — vier
Tools, ein `initialize`/`tools`-Handshake, **keine** server-initiierten Nachrichten
(kein Sampling, keine Resources, keine Subscriptions, kein Progress-Streaming). Für so
einen Server ist der **MCP-Streamable-HTTP**-Transport minimal: ein einzelner Endpoint,
bei dem jeder `POST` genau eine JSON-RPC-Nachricht trägt und die Antwort als
`application/json` zurückkommt; ein `GET` (server→client SSE-Stream) entfällt und darf
laut Spec mit `405` beantwortet werden. Damit ist der Transport eine dünne Schicht über
der **bereits vorhandenen** Dispatch-Logik (`handleMessage`), nicht der Grund, ein SDK
einzuziehen.

## Optionen

1. **Streamable HTTP nativ, im bestehenden `temisd` mounten.** Ein routebarer Dienst für
   REST **und** MCP. — Vermischt zwei Protokolle in einem Binary; MCP-Konsumenten und
   REST-Konsumenten teilen sich Port/Auth/Lifecycle, obwohl sie unabhängig sind.
2. **Streamable HTTP nativ, als `-http`-Modus von `temis-mcp`** (diese Entscheidung).
   Das MCP-Binary erhält neben stdio einen eigenen HTTP-Listener. — Hält MCP sauber von
   `temisd` getrennt; ein klar abgegrenzter Dienst, separat routebar/absicherbar. Kosten:
   ein zweiter Dienst im Deployment.
3. **Offizielles Go-MCP-SDK für dessen HTTP-Transport aufnehmen.** Zukunftssicher für
   Streaming/Sampling/Resources. — Hebt den Go-Mindeststand auf 1.25 und bläht den
   Dependency-Baum, ohne dass wir die zusätzlichen Features brauchen; für reines
   Request/Response überdimensioniert.

## Entscheidung

Option 2. `temis-mcp` bekommt einen **`-http host:port`**-Modus, der MCP über **Streamable
HTTP** mit reiner Standardbibliothek anbietet; ohne das Flag bleibt stdio der Default.

- **Endpoint:** `POST /mcp` (eine JSON-RPC-Nachricht je Request → JSON-RPC-Antwort als
  `application/json`; Notifications → `202 Accepted`), `GET /mcp` → `405` (kein
  SSE-Stream), `GET /healthz` für Load-Balancer-Probes.
- **Wiederverwendung:** Der HTTP-Handler ruft dieselbe `handleMessage`-Dispatch wie der
  stdio-Transport; die Tool-Logik ist transport-agnostisch.
- **Auth:** optionaler Bearer-Token (`-token` / `TEMIS_API_TOKEN`, `WithHTTPToken`),
  konstantzeit-verglichen; gilt nur für HTTP (stdio ist ein vertrauenswürdiger lokaler
  Subprozess).

Das offizielle SDK wird weiterhin **nicht** aufgenommen (ADR-0014 bleibt gültig): Da der
Eigenbau hier eine dünne, transport-konforme Schicht ist und kein Streaming/Sampling
ansteht, ist der SDK-Preis (Go-1.25-Floor, ~9 Deps) nicht gerechtfertigt. Standardtreue
bleibt gewahrt — am Draht ist es MCP-Streamable-HTTP.

## Konsequenzen

**Positiv**
- temis ist nun **remote als MCP-Dienst** nutzbar und hinter einem Reverse Proxy
  routebar, ohne dass `temisd` MCP mit-tragen muss.
- Kein neuer Dependency, kein angehobener Go-Mindeststand (ADR-0014-Linie gewahrt).
- HTTP- und stdio-Transport teilen sich die gesamte Dispatch-/Tool-Logik.

**Negativ / Kosten**
- Ein zweiter netzwerk-erreichbarer Dienst (neben `temisd`), der separat geroutet und
  abgesichert werden muss.
- Der Eigenbau deckt bewusst nur das Request/Response-Profil ab; ein künftiger Bedarf an
  **server-initiiertem Streaming** (SSE), Sampling oder Resources würde den
  ADR-0014-Revisit erneut auslösen — dann mit höherem Eigenbau-Aufwand und damit
  stärkerem Argument für das SDK.

**Folgeaufgaben**
- README/`docs/40-api-contract.md`: `-http`-Modus, Endpoint und Token dokumentieren.
- Optional später: Origin-/Session-Handling (`Mcp-Session-Id`) ergänzen, falls ein
  Client es verlangt — derzeit zustandslos je Request.
