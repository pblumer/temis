# ADR-0036: OIDC/Keycloak-Authenticator (Zielbild)

- **Status:** proposed
- **Datum:** 2026-07-21
- **Kontext-WP:** (künftig) Etappe „Zugriffskontrolle", Folge zu ADR-0028/0035

> **Siehe auch ADR-0038:** die „temis ist selbst der Issuer"-Variante — ein
> ko-lokalisierter, self-contained OAuth-2.1-Server, der Remote-MCP-Clients ohne
> externen IdP anbindet. Dieses ADR (0036) bleibt das Zielbild für die Validierung
> **fremder** JWTs (Keycloak); beide Authenticator-Varianten sind chainbar.

## Kontext

ADR-0028 gab temis scoped `kid.secret`-API-Keys hinter einem schmalen
`Authenticator`-Interface (`authenticate(bearer) → (*Key, bool)`); ADR-0035 ergänzte
public decisions. Für **menschliche** Nutzer (Modeler, Betrieb) ist ein zentraler
Identity-Provider oft Pflicht — typischerweise **Keycloak** (OIDC/OAuth2, SSO,
Gruppen/Rollen, MFA). Die Frage ist, **ob und wie** temis das später unterstützt,
und **was heute** zu beachten ist, um sich nicht zu verbauen.

## Optionen

1. **Keycloak als Token-Issuer, temis validiert JWTs.** Clients holen ein
   Access-Token bei Keycloak; temis prüft die Bearer-JWT gegen Keycloaks JWKS
   (`iss`/`aud`/`exp`, Signatur) und mappt Realm-/Client-Rollen → temis-Scopes.
   Kein Proxy nötig, temis bleibt self-contained.
2. **Vorgelagerter OIDC-Proxy/Gateway** (oauth2-proxy, Keycloak-Gatekeeper): der
   Proxy terminiert OIDC und reicht Identität per Header weiter; temis vertraut dem
   Proxy. Null Code in temis, aber eine Betriebs-Komponente mehr und eine
   Header-Vertrauensgrenze.
3. **Beides zulassen** — gewählt als Zielbild: eine JWT-`Authenticator`-Variante
   (Option 1) für „temis direkt am IdP", **und** Proxy-Betrieb (Option 2) bleibt
   ohnehin immer möglich, weil temis nur einen Bearer sieht.

## Entscheidung (Zielbild)

Ein **zweiter `Authenticator`** (`oidcKeystore` o. ä.) neben dem heutigen
`keystore`, gestaffelt als eigene Etappe umsetzbar:

- **JWT-Verifikation** gegen Keycloaks JWKS (Discovery `/.well-known/openid-configuration`,
  Key-Rotation via `kid`-Header), `iss`/`aud`/`exp`/`nbf`-Prüfung.
- **Rollen→Scope-Mapping** (konfigurierbar): Realm-/Client-Rolle bzw. Group-Claim
  → temis-Scope (`evaluate`, `models:read`, …). Das SemVer-stabile Scope-Vokabular
  (ADR-0028) ist genau die Brücke.
- **Composite/Chain:** temis kann `kid.secret`-Keys (Maschinen/Agenten/CI) **und**
  Keycloak-JWTs (Menschen) **gleichzeitig** akzeptieren — erst kid.secret, sonst JWT.
- **Ein gemeinsamer Realm mit clio** als naheliegende Erweiterung (beide Binaries
  ein IdP), analog zum heutigen gemeinsamen `kid.secret`-Modell.

## Was heute schon berücksichtigt ist

Damit später **nichts umgebaut** werden muss, ist das aktuelle Design bereits
issuer-agnostisch gehalten:

- **Naht vorhanden:** Auth lebt komplett hinter `Authenticator`; Transportschicht
  (`requireScope`, gRPC-Interceptor, MCP-Gate) reicht nur den rohen Bearer-String
  durch und autorisiert danach ausschließlich über temis-**Scopes**. Ein JWT ist
  auch nur ein Bearer.
- **Kern unberührt:** kein Auth-Import in `package dmn` (ADR-0011).
- **`whoami` issuer-neutral** (ADR-0035-Umfeld, WP-107): `{authEnabled, subject,
  scopes[], isAdmin}` — `subject` ist heute der `kid`, später der OIDC-`sub`.
- **Frontend-Credential opak** (`web/src/session.ts`): der Bearer wird nur
  gespeichert und mitgeschickt, nie als `kid.secret` geparst — ein OIDC-Redirect-Flow
  ersetzt später nur die „Key eingeben"-Eingabe, ohne aufrufende Logik.

## Konsequenzen

**Positiv:** SSO/MFA/zentrale Nutzerverwaltung für Menschen, ohne die
Maschinen-Keys aufzugeben; ein Auth-Modell (Scopes) über Issuer hinweg.

**Negativ / offen:** OIDC bringt realistisch eine **Dependency** (z. B.
`github.com/coreos/go-oidc` + `golang.org/x/oauth2`) — Abweichung von „reine
stdlib" (Golden Rule 6), aber in der **Adapter-Schicht** und **opt-in**, daher
vertretbar (analog zu connectrpc bei gRPC, ADR-0020). Der konkrete
Rollen→Scope-Mapping-Vertrag und die Discovery-/JWKS-Cache-Details sind bei
Umsetzung zu spezifizieren. **Kein Code heute** — dieses ADR hält nur die Naht und
das Zielbild fest.
