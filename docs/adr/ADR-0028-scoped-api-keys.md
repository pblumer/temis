# ADR-0028: Scoped API-Key-Authentifizierung (`kid.secret`, Keystore) — angeglichen an clio

- **Status:** proposed
- **Datum:** 2026-07-01
- **Kontext-WP:** WP-100 – WP-105 (neue Etappe „Zugriffskontrolle / API-Keys")

## Kontext

`temisd` (und die ko-lokalisierten Endpunkte MCP, gRPC) schützt seine Datenrouten
heute mit **einem einzigen, optionalen Bearer-Token** (`-token` / `$TEMIS_API_TOKEN`,
`WithToken`). Der Vergleich ist konstantzeitig (`crypto/subtle`), der Token bewacht
**alle** `/v1`-Routen, `/mcp` und den gRPC-Interceptor **einheitlich** — alles-oder-nichts.
Das ist einfach, aber für die inzwischen breite Oberfläche zu grob:

- **Keine Least-Privilege-Keys.** Ein KI-Agent (ADR-0013), der nur *auswerten* soll,
  bekommt denselben Schlüssel wie ein Client, der Modelle **löscht**, den Modeler
  **editiert** oder den LLM-**Assistenten** (`/v1/chat`, kostenverursachend) fährt.
- **Keine Trennung read/write.** CI, das Modelle schreibt, kann nicht von einem
  read-only-Konsumenten unterschieden werden.
- **Keine Identität pro Aufrufer.** Das Entscheidungs-Logbuch (ADR-0023) kann nicht
  festhalten, *welcher* Schlüssel eine Entscheidung ausgelöst hat.
- **Kein Rotieren/Widerrufen ohne Redeploy.** Ein geleakter Token lässt sich nur durch
  Neustart mit neuem Wert entwerten; alle Clients müssen gleichzeitig umziehen.

Das Schwesterprojekt **[clio](https://github.com/pblumer/clio)** hat genau dieses
Problem bereits spec-sauber gelöst (clio ADR-025 „Key Lifecycle & Authentication",
ADR-032 „Audit Log", ADR-033 „Subject-Level Scopes"):

- **`kid.secret`-Bearer-Tokens** (Key-ID + Secret, z. B.
  `Authorization: Bearer kid_ci01.W8xq…`).
- Persistiert wird **nur der SHA-256-Hash** des Secrets (Klartext nie gespeichert),
  **konstantzeitiger** Vergleich.
- **Scopes** `read` / `write` / `admin` / `audit`, optional auf einen
  **Subject-Prefix** eingeschränkt (`read:/orders/*`). Fehlender/ungültiger Token → **401**,
  gültiger Token ohne passenden Scope → **403**.
- **Key-Lifecycle** über eine Admin-HTTP-API (`/api/v1/keys*`: create/list/rotate/revoke,
  Secret **einmalig** in der Antwort) **und** eine Offline-CLI (`cliostore keys …`) für
  den Lockout-Fall.
- **Bootstrap** über `CLIO_BOOTSTRAP_ADMIN_KEY`; der alte Einzeltoken `CLIO_API_TOKEN`
  ist **deprecated** (Legacy-Admin-Key) — der ohne `kid` gebildete Bearer wird nicht
  mehr akzeptiert.
- Optional: **Expiry** je Key, **Authorship** (`clioauthkid` als CloudEvents-Extension),
  **Auth-/Audit-Events**.

temis **konsumiert** clio-Keys bereits in genau diesem Format (`-clio-token kid.secret`,
`docs/80` §3, „eng gescopter Write-Key `write:/decisions/*`"). Es ist inkonsequent, dass
temis fremde Keys in diesem Modell versteht, sein **eigenes** Tor aber nur einen flachen
Shared Secret kennt. Ein gemeinsames Auth-Modell über beide Single-Binaries gibt
Betreibern **ein** mentales Modell.

**Spannungsfeld zu ADR-0011 / ADR-0005 / ADR-0014.** Der Engine-Kern (`package dmn`)
bleibt reine Library ohne Transport-/Auth-Importe — Authentifizierung lebt **im Adapter**
(`service`/`cmd/temisd`), wie schon der clio-Sink (ADR-0023). **Golden Rule 6 (keine neue
Dependency):** clio nutzt `bbolt` als Keystore; temis muss **reine stdlib** bleiben. Ein
persistenter Keystore darf daher nur an den **optionalen** Dateisystem-Store (ADR-0027)
andocken (JSON/atomic write), nicht eine neue DB einziehen. Und temis' „stateless per
Default"-Ethos (ADR-0027) heißt: der Persistenz-/Lifecycle-Teil bleibt **opt-in**.

## Optionen

1. **Status quo — ein Shared-Secret-Token behalten.** Minimal, aber löst keines der
   obigen Probleme (kein Least-Privilege, keine Identität, kein Rotieren). **Verworfen.**

2. **Volle clio-Parität in einem Schritt** — Keystore + Lifecycle-API + CLI +
   Subject-Scopes + Audit-Log sofort. Deckt sich exakt mit clio, ist aber ein großes,
   **zustandsbehaftetes** Subsystem, riskiert eine neue Dependency (bbolt) oder einen
   handgeschriebenen Store, und überschießt den Tag-1-Bedarf. **Verworfen als
   Einzelschritt** — aber als **End-Zustand** übernommen und in WPs gestaffelt.

3. **clios Token-*Vertrag* übernehmen (`kid.secret` + Scopes + SHA-256 + Konstantzeit),
   gestaffelt** — **gewählt.** temis spiegelt clios Schlüssel-**Format** und die
   401/403-Semantik, implementiert es rein in der Adapter-Schicht mit stdlib und
   **opt-in-Persistenz**, hält den Kern unberührt und lässt den heutigen Einzeltoken als
   **deprecated Legacy-Admin-Key** weiterlaufen (sanfte Migration, exakt wie clios
   ADR-025).

## Entscheidung

**Option 3.** temis bekommt ein **scoped API-Key-System im `kid.secret`-Modell von clio**,
gestaffelt in drei Phasen, jede für sich lauffähig und rückwärtskompatibel.

### 1. Auth-Vertrag (das stabile Artefakt)

- **Token-Format:** `Authorization: Bearer <kid>.<secret>`. Der `kid` ist öffentlich
  (loggbar, identifiziert den Schlüssel), das `secret` ist geheim.
- **Speicherung:** je Key `{kid, sha256(secret), scopes[], owner, expiresAt?, revoked}`.
  **Nur der SHA-256-Hash** des Secrets wird gehalten; Verifikation über
  `subtle.ConstantTimeCompare` (wie heute, nur pro Key nachgeschlagen).
- **Semantik:** kein Token / unbekannter `kid` / falsches Secret / abgelaufen / widerrufen
  → **401** (`WWW-Authenticate: Bearer`). Gültiger Token, aber Scope fehlt → **403**
  (`FORBIDDEN`, RFC-7807, wie die übrigen Fehler). Sind **keine** Keys **und** kein
  Legacy-Token konfiguriert, bleibt die API **offen** (Default heute).

### 2. Scope-Modell (clio-Scopes → temis-Oberfläche)

| Scope | Deckt ab |
|---|---|
| `evaluate` | `POST /v1/evaluate`, `/v1/models/{id}/evaluate`, `/evaluate-graph`; gRPC `Evaluate`/`EvaluateBatch`; MCP `evaluate`/`describe_decision` |
| `models:read` | `GET /v1/models`(Listing), `/{id}`, `/xml`, `/graph`, `/types`, alle `GET …/decisions/*`, MCP `list_models`/`load_model` (read) |
| `models:write` | `POST`/`DELETE /v1/models*`, `save`/`rename`/`create-*`, alle Modeler-Edits; gRPC `Compile` |
| `git` | `/v1/git/*` und MCP `git_*` (der Provider-Token bleibt per-Request `X-Git-Token`, WP-72) |
| `assist` | `POST /v1/chat` (LLM-Assistent, ADR-0024 — separat, weil kostenverursachend) |
| `flow` | künftige `/v1/flows*` (WP-91) und `evaluate_flow` (WP-92) |
| `admin` | Key-Management (`/v1/keys*`), Modell-`DELETE`, Betriebs-/Dev-Routen |
| `audit` | read-only Zugriff auf Auth-/Audit-Log (Phase 3), kombinierbar mit `admin` |

Analog zu clios Subject-Prefix-Scopes (ADR-033) ist eine **Ressourcen-Einschränkung**
vorgesehen (Phase 3): ein Key auf einen **Subject-Prefix** des clio-Sinks
(`evaluate:/orders/*`) bzw. auf gepinnte `modelId`s begrenzen. Der Kern-Scope-Satz oben
kommt zuerst.

### 3. Implementierungsschichten (im Adapter, stdlib, opt-in)

- **Phase 1 (WP-100/101/102): Scopes + statische Keys.** Ein `Authenticator`-Interface
  und ein In-Memory-**Keystore** in `service` (kein neues Paket-Dependency). Keys aus
  **Env/JSON-Datei** (`-keys-file` / `$TEMIS_KEYS_FILE`) sowie ein Bootstrap-Admin-Key
  (`$TEMIS_BOOTSTRAP_ADMIN_KEY`). Jede Route der `dataRoutes()`-Tabelle bekommt einen
  **required Scope**; `requireToken` wird zu `requireScope`. Der bestehende `-token` /
  `$TEMIS_API_TOKEN` läuft als **synthetischer Admin-Key** weiter (deprecated,
  dokumentiert wie clios `CLIO_API_TOKEN`). MCP- und gRPC-Gate werden scope-bewusst.
- **Phase 2 (WP-103/104): Persistenter Keystore + Lifecycle.** Store hängt am
  Dateisystem-Store (ADR-0027, atomarer JSON-Write, reine stdlib) und ist **opt-in**.
  Admin-HTTP-API `POST/GET /v1/keys`, `POST /v1/keys/{kid}/rotate|revoke` (Secret
  **einmalig** in der Antwort, danach nie wieder); Offline-CLI `temisd keys …` für den
  Lockout-Fall (Server gestoppt, DB direkt).
- **Phase 3 (WP-105): Parität & Ausbau.** Expiry-Durchsetzung, **Authorship** (`kid` als
  `clioauthkid`-Extension in Decision-Events, ADR-0023, in Hash/Signatur gebunden),
  Auth-/Audit-Events, Subject-/Modell-Prefix-Scopes.

Der **Engine-Kern bleibt unberührt** (kein Auth-Import in `package dmn`, ADR-0011);
alles lebt in `service`/`cmd/temisd`. **Keine neue Dependency** (ADR-0014): SHA-256 und
Konstantzeit sind stdlib, Persistenz reiner JSON über den vorhandenen Store.

## Konsequenzen

**Positiv**
- **Least-Privilege:** Agenten bekommen `evaluate`-only-Keys (passt zu ADR-0013), CI
  einen `models:write`-Key, Betrieb einen `admin`-Key — ohne den `/v1/chat`-Kostenhebel
  freizugeben.
- **Identität & Audit:** je Key ein `kid` → das Entscheidungs-Logbuch (ADR-0023) kann den
  Urheber stempeln (`clioauthkid`, Phase 3).
- **Rotation/Widerruf ohne Redeploy** (Phase 2); geleakte Keys sind sofort entwertbar.
- **Ein Auth-Modell über temis + clio** — Betreiber lernen `kid.secret`/Scopes **einmal**;
  temis versteht fremde clio-Keys ohnehin schon in diesem Format.
- **Rückwärtskompatibel:** ohne Konfiguration bleibt die API offen; der Einzeltoken
  funktioniert als Legacy-Admin-Key weiter (byte-identisches Verhalten für Bestandsläufe).

**Negativ / Kosten**
- Neues (opt-in) **zustandsbehaftetes** Subsystem ab Phase 2 (Keystore-Persistenz) inkl.
  Test-/Betriebsaufwand; muss diszipliniert **stdlib-only** bleiben (kein bbolt).
- Das **Scope→Route-Mapping** wird Teil der öffentlichen Oberfläche und unterliegt SemVer
  (`docs/40-api-contract.md`) — neue Routen brauchen eine bewusste Scope-Zuordnung.
- Mehr Fehlerpfade (401 vs 403 vs 404); Achtung, **kein Informationsleck** über
  Routen-Existenz (das Listing antwortet bereits `404`, wenn deaktiviert, WP-32).

**Folgeaufgaben**
- Neue Roadmap-Etappe „Zugriffskontrolle" mit **WP-100 – WP-105** (`docs/20-roadmap.md`).
- `docs/40-api-contract.md`: Scope-Vertrag + `kid.secret`-Format als stabile Oberfläche,
  OpenAPI `securitySchemes`.
- Betriebs-/Sicherheitsleitfaden (erweitert `docs/70` bzw. neu `docs/85`): Key-Erzeugung,
  Rotation, Bootstrap, Migration vom Einzeltoken, Lockout-Recovery.
- README-Env-Tabelle (`TEMIS_API_TOKEN` als *deprecated* markieren, neue Flags/Env
  ergänzen).
