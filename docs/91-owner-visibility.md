# Eigentümer-Sichtbarkeit (WP-106)

> **Status:** umgesetzt in `service/` (Auth-Adapter, ADR-0011/ADR-0028). Der
> Engine-Kern (`package dmn`) ist unberührt. Aufsetzend auf die **scoped API-Keys**
> („wie bei clio mittels Token", ADR-0028): Modelle/Flows gehören dem Key, der sie
> erstellt hat, und ein Nutzer sieht **nur seine eigenen** (plus geteilte).
>
> **Ausdrücklich offen:** eine **Team-/Gruppen-Sichtbarkeit** („die meines Teams")
> ist hier **nicht** modelliert — sie wird separat gelöst. Dieses Dokument
> beschreibt nur die Eigentümer-Ebene.

## 1. Warum überhaupt

Der Token-Schutz (Bearer `<kid>.<secret>`, nur `sha256(secret)` gespeichert,
Scopes pro Route) existierte bereits (ADR-0028). Was fehlte, war die
**Sichtbarkeits-Grenze**: bislang sah jeder Key mit `models:read` **alle** Modelle.
WP-106 ergänzt eine **Eigentümer-Isolation**, ohne den offenen Default zu brechen.

## 2. Das Modell

- **Eigentümer = `kid`.** Wer ein Modell/Flow anlegt oder speichert, wird als
  Eigentümer eingetragen. Weil Modelle **content-addressed** sind (die ID ist der
  SHA-256 des Inhalts), kann die Zugehörigkeit nicht *im* Artefakt liegen — sie
  lebt in einem **Seitenindex** `id → { kid }` (`service/ownership.go`).
- **Geteilt (unowned).** Ein Artefakt ohne Eigentümer ist für **alle** sichtbar.
  Das betrifft die gebündelten Beispiele, git-deklarierte Flows (ADR-0032) und
  Modelle, die vor Aktivierung der Auth entstanden sind.
- **`admin` sieht alles.** Ein Admin-Key (und damit das Legacy-`-token`) umgeht die
  Isolation — byte-identisch zum bisherigen Verhalten.

### Sichtbarkeitsregel (`ownership.visible`)

Ein Aufrufer sieht Ressource `id`, wenn **eine** davon gilt:
1. der Key ist `admin` (`seeAll`), **oder**
2. `id` hat keinen Eigentümer (geteilt), **oder**
3. der eigene `kid` ist Eigentümer.

Sonst: **404** — dieselbe Antwort wie bei einer wirklich fehlenden Ressource, damit
die Isolation nicht verrät, dass die ID existiert.

## 3. Was durchgesetzt wird (HTTP)

| Ort | Verhalten |
|---|---|
| `GET/POST /v1/models/{id}/…`, `…/flows/{id}/…` | Nicht sichtbar ⇒ **404** (harte Isolation, zentral in `requireScope`). |
| `GET /v1/models`, `GET /v1/flows` | Katalog **gefiltert**: nur Sichtbares wird gelistet. |
| `GET /v1/models?owner=me` / `…/flows?owner=me` | Verengt auf **nur meine** (per `kid` erstellte) — schließt Geteiltes aus. |
| `DELETE /v1/models/{id}` | `admin`-Scope ⇒ sieht/löscht alles. |

**Explizite Prefix-Scopes gewinnen (WP-105).** Ein Key, der ausdrücklich auf eine
Ressource gepinnt ist (`models:read:sha256:…`, `evaluate:/orders/*`), erreicht genau
diese Ressource — die Eigentümer-Isolation setzt für diesen bewussten
Betreiber-Grant aus.

## 4. Persistenz

Ist ein Modell-Store konfiguriert (`-models-dir`), wird der Eigentümer-Index atomar
als `owners.json` **daneben** persistiert — sonst würden persistierte Modelle nach
einem Neustart eigentümerlos neu laden und damit für alle sichtbar (eine stille
Isolations-Regression). Ohne Store lebt der Index nur im Speicher, genau wie die
Modelle selbst.

## 5. Abwärtskompatibilität

- **Kein Key konfiguriert ⇒ API offen wie bisher.** Ohne Auth wird nie ein
  Eigentümer eingetragen; jedes Artefakt bleibt geteilt. Verhalten byte-identisch.
- **Legacy-`-token`** ist ein synthetischer Admin-Key ⇒ sieht weiterhin alles.

## 6. Grenzen (bewusst, ehrlich benannt)

- **Team-/Gruppen-Sichtbarkeit** ist **nicht** Teil von WP-106 (bewusst zurückgestellt,
  wird anders gelöst). Heute ist jede Sicht an genau **einen** Key gebunden: teilen
  mehrere Personen Artefakte, brauchen sie denselben Key — oder das Artefakt bleibt
  ungeteilt (jeder Eigentümer sieht nur seins). Sobald das Team-Modell steht, wird es
  hier ergänzt.
- **MCP / gRPC.** Die co-lokierte MCP-Oberfläche (`list_models`, `load_model`, …)
  und der gRPC-Dienst authentifizieren und prüfen Scopes, **filtern aber noch nicht**
  nach Eigentümer. Wer harte Isolation auch dort braucht, gibt MCP-Keys eng gescopte
  Prefix-Grants oder betreibt `-mcp=false`. Das Durchreichen der Identität in das
  `mcp`-Paket ist ein eigenständiger Folge-Schritt.

## Verwandte Dokumente

- **ADR-0028** — scoped API-Keys (`kid.secret`, Scopes, Lifecycle).
- **`docs/10-architecture.md` §6** — Sicherheit/Adapter-Grenze (ADR-0011).
- **`docs/80-clio-decision-log.md`** — Authorship (`clioauthkid`) im Audit-Log.
