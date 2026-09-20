# ADR-0038: Self-contained OAuth-2.1-Authorization-Server für Remote-MCP

- **Status:** accepted
- **Datum:** 2026-07-21
- **Kontext-WP:** Etappe „Zugriffskontrolle" (Folge zu ADR-0028/0035/0036)

## Kontext

Remote-MCP-Clients wie der **claude.ai-Web-Connector** verbinden sich mit einem
MCP-Server ausschließlich über einen **OAuth-Flow**: der Client navigiert den
Nutzer auf `<server>/authorize` (Authorization Code + PKCE/S256) und tauscht den
Code an `<server>/token` gegen ein Bearer-Token, das er anschließend am
`/mcp`-Endpoint präsentiert.

temis hatte bisher **keinen** OAuth-Endpunkt — die Auth ist ein scoped
`kid.secret`-Bearer (ADR-0028) hinter der einen `Authenticator`-Naht. Der
Web-Connector lief damit auf `/authorize` → **404**, die Verbindung war ohne
CLI-Header (statisches Bearer) unmöglich. Ziel: der Web-Connector soll ohne
externe Komponente funktionieren.

## Optionen

1. **Nur CLI-Header** (`claude mcp add --header "Authorization: Bearer kid.secret"`).
   Null Code, aber der **Web**-Connector bleibt außen vor.
2. **Externen IdP verlangen** (Keycloak, ADR-0036 Option 1): temis validiert
   fremde JWTs, der IdP stellt `/authorize`/`/token`. Sauber, aber eine
   Betriebs-Komponente mehr — für einen self-contained Betrieb zu schwer.
3. **temis wird selbst OAuth-Server** (gewählt): Authorization- **und**
   Resource-Server ko-lokalisiert. Kein externer IdP, der Web-Connector läuft direkt.

## Entscheidung

temis ist ein **self-contained OAuth-2.1-Server** für den `/mcp`-Endpoint:

- **Endpunkte** (nur gemountet, wenn `-external-url` **und** ein managed Keystore
  `-keys-dir` gesetzt sind): `/.well-known/oauth-authorization-server` (RFC 8414),
  `/.well-known/oauth-protected-resource` (RFC 9728), `POST /register`
  (RFC 7591, public clients), `GET/POST /authorize`, `POST /token`,
  `POST /oauth/login|logout`.
- **Access-Token = opaker Managed-Key.** `/token` prägt über den bestehenden
  Keystore (`createKey`, ADR-0028) einen **kurzlebigen** scoped Key (`kid.secret`,
  Default-TTL 1 h) und gibt ihn als `access_token` zurück. Dadurch akzeptieren
  `/mcp`, `/v1` und gRPC ihn **unverändert** über dieselbe `Authenticator`-Naht —
  kein zweiter Validierungspfad, native Widerrufbarkeit/Ablauf. Ein rotierender
  `refresh_token` erneuert stillschweigend.
- **Delegierte Scopes** least privilege: Default `evaluate, models:read,
  models:write, flow, git` (kein `admin`/`assist`/`audit`), via `-oauth-scopes`
  einstellbar; die Menge wird zusätzlich auf das begrenzt, was der anmeldende
  Schlüssel selbst grant­en kann.
- **Menschliche Identität am `/authorize`** über eine **echte Server-Cookie-Session**
  (`temis_session`, HttpOnly, Secure bei TLS, SameSite=Lax): erstmalige Anmeldung
  mit einem vorhandenen `kid.secret`, danach wiederverwendet. In-memory, TTL 12 h.
- **Sicherheit:** PKCE **S256 Pflicht** (`plain` abgelehnt); Auth-Codes einmalig,
  60 s TTL, gebunden an `client_id`/`redirect_uri`/`code_challenge`/`resource`;
  **`redirect_uri`-Allowlist** (Default `claude.ai` + Loopback; `-oauth-redirect-allow`);
  Audience/`resource`-Prüfung (RFC 8707); CSRF-Token + Origin-Check am Consent;
  konstantzeitige Vergleiche; `WWW-Authenticate` mit `resource_metadata` auf 401.

## Abgrenzung zu ADR-0036

ADR-0036 (OIDC/Keycloak) beschreibt temis als **Resource-Server, der fremde JWTs
eines externen Issuers validiert**. ADR-0038 ist die **„temis ist selbst der
Issuer"-Variante** — beide können später koexistieren (Chain aus `kid.secret`,
OAuth-Managed-Keys und optional Keycloak-JWTs). Die opaken Managed-Keys hier sind
bewusst **kein** JWT: keine neue Signier-/JWKS-Infrastruktur, Widerruf gratis.

## Konsequenzen

- **+** Der claude.ai-Web-Connector verbindet ohne externe Infrastruktur; der
  Mensch⇄Agent-Roundtrip läuft direkt gegen das Deployment.
- **+** Tokens sind normale scoped Keys — ein Auth-Modell, ein Audit-Pfad
  (`clioauthkid`), Widerruf/Ablauf inklusive.
- **−** Neuer, sicherheitskritischer HTTP-Oberflächenanteil (Consent/Login,
  Grant-Stores). Durch Table-/E2E-Tests (`service/oauth_test.go`) und einen
  Security-Review abgesichert.
- **−** OAuth setzt einen persistenten Keystore (`-keys-dir`) voraus; Sessions
  und Grant-Stores sind in-memory (ein Neustart meldet Menschen ab, die bereits
  ausgestellten Tokens überleben als Keys).
- **Anhäufung begrenzt:** Jeder `/token`-Aufruf prägt einen persistierten Key und
  legt einen Refresh-Grant an. Ein Hintergrund-**Reaper** (`RunOAuthReaper`,
  stündlich, an den Shutdown gebunden) entfernt abgelaufene Auth-Codes, abgelaufene
  Refresh-Grants (TTL 30 d) und abgelaufene Access-Token-Keys aus Keystore/`keys.json`,
  sodass ein Dauerbetrieb keine toten Credentials anhäuft.
