# ADR-0035: Public Decisions — anonyme Auswertung trotz aktivierter Auth

- **Status:** accepted
- **Datum:** 2026-07-21
- **Kontext-WP:** WP-106, WP-107 (Etappe „Zugriffskontrolle", Folge zu ADR-0028)

## Kontext

ADR-0028 hat `temisd` scoped `kid.secret`-API-Keys gegeben (Scopes `evaluate`,
`models:read`, `models:write`, `git`, `assist`, `flow`, `admin`, `audit`). Das
Gate ist heute aber **binär pro Server**:

- **Keine Keys konfiguriert** → die *ganze* API ist offen — auch `models:write`,
  `DELETE`, der kostenverursachende `/v1/chat`. Zu offen.
- **Sobald ein Key existiert** → verlangt *jede* Route einen gültigen, passend
  gescopten Key, **auch `POST /v1/evaluate`**. Es gibt keinen Weg, nur die
  Auswertung (oder ausgewählte Decisions) öffentlich zu lassen und den Rest
  zuzusperren.

Ein sehr häufiger Betriebsfall fällt damit durch das Raster: **eine Entscheidung
soll von jedem auswertbar sein** (ein öffentlicher „berechne mir X"-Endpunkt,
eine Demo-Decision, ein Preis-/Tarif-Rechner), während Modelle-Schreiben,
Löschen und der LLM-Assistent hinter Token bleiben. Prefix-Scopes (ADR-0028
Phase 3, `evaluate:/orders/*`) schränken einen *vorhandenen* Key ein, lösen aber
nicht den **anonymen** Zugang: ein öffentlicher Aufrufer hat per Definition
keinen Key.

**Spannungsfeld.** Auth lebt in der Adapter-Schicht (`service`/`cmd/temisd`), der
Engine-Kern bleibt unberührt (ADR-0011). Eine Ausnahme vom Gate darf genau dort
sitzen und **nur** den `evaluate`-Scope betreffen — niemals `models:write`,
`admin`, `assist`, `git` oder `flow`, sonst wird aus „public decision" ein
offenes Scheunentor. Sie muss **opt-in** und beim Start **sichtbar** sein
(temis' „stateless/sicher per Default"-Ethos, ADR-0027).

## Optionen

1. **Nur ein globaler Schalter „evaluate offen".** Einfachste Variante, aber
   alles-oder-nichts für die Auswertung: man kann keine *einzelne* Decision
   freigeben, ohne alle freizugeben.
2. **Nur pro Modell/Decision als public markieren.** Feinste Kontrolle, deckt
   aber den Fall „diese Instanz ist generell ein öffentlicher Auswerte-Dienst"
   (z. B. hinter einem separaten, ohnehin öffentlichen Netz) nur umständlich ab.
3. **Beides, gestaffelt und unabhängig — gewählt.** Ein globaler Schalter für den
   Instanz-weiten Fall **und** eine Pro-Modell-Allowlist für den chirurgischen
   Fall. Beide öffnen ausschließlich `evaluate`, beide opt-in.
4. **Ein „anonymer Default-Scope-Satz"** (jeder Unauthentifizierte bekommt
   implizit z. B. `evaluate`). Flexibler, aber mehr Konzept-Oberfläche und
   verwischt die Grenze zwischen „authentifiziert" und „anonym". **Verworfen** —
   Option 3 deckt den realen Bedarf mit weniger Begriffen; ein anonymer
   Scope-Satz kann später additiv daraufsatteln.

## Entscheidung

**Option 3.** temis bekommt zwei unabhängige, opt-in Öffnungen des
`evaluate`-Scopes, beide rein in der Adapter-Schicht:

### A — Pro-Modell-Allowlist

`WithPublicModels(ids…)` / `-public-models` / `TEMIS_PUBLIC_MODELS` (Komma-Liste).
Ein Eintrag matcht ein Modell entweder per **content-adressierter `modelId`**
(`sha256:…`) **oder** per **Anzeigename**. Der Name-Match ist wichtig, weil jede
Speicherung eine neue `modelId` erzeugt (Content-Addressing): ein per Name
gelistetes Modell bleibt über Revisionen hinweg public. Der Name wird nur aus
dem **In-Memory-Cache** aufgelöst (nie ein Disk-Compile), damit ein anonymer
Aufrufer nicht allein durch Probing einer `modelId` Arbeit erzwingen kann.
Gilt für die id-adressierten HTTP-Routen (`/v1/models/{id}/evaluate`,
`…/evaluate-graph`, `…/evaluate-graph-batch`).

### B — Globaler Schalter

`WithPublicEvaluate(true)` / `-public-evaluate` / `TEMIS_PUBLIC_EVALUATE`. Öffnet
den ganzen `evaluate`-Scope anonym, inkl. dem stateless `POST /v1/evaluate` (kein
`{id}`, Modell im Body), über **HTTP, gRPC und MCP**.

### Semantik & Grenzen

- **Nur `evaluate`.** Das Gate konsultiert die Public-Regel ausschließlich für den
  `evaluate`-Scope. `models:write`/`admin`/`assist`/`git`/`flow` bleiben immer
  hinter einem Key.
- **Anonym, aber Authorship bleibt.** Ein öffentlicher Aufrufer wird ohne Token
  bedient; schickt er trotzdem einen gültigen Key mit, wird dessen `kid` weiter
  als `clioauthkid` ins Audit-Log gestempelt (ADR-0023). Ein *ungültiger* Token
  auf einer public Route wird nicht abgewiesen (die Route ist bewusst offen),
  sondern anonym bedient.
- **Transport-Grenze.** A (pro Modell) gilt für die id-adressierten HTTP-Routen;
  der stateless HTTP-Pfad sowie gRPC `Evaluate`/`EvaluateBatch` und das
  MCP-`evaluate`-Tool tragen kein stabiles Modell-Handle im Gate und werden
  daher nur von B (global) geöffnet. Das ist dokumentiert, nicht implizit.
- **Sichtbarkeit.** Beide Öffnungen werden beim Start laut geloggt (wie die
  Key-/Legacy-Token-Warnungen). Rate-Limiting (ADR-0028-Umfeld, WP-133) sitzt vor
  dem Gate und greift auch für anonyme Aufrufer.
- **Rückwärtskompatibel.** Default aus → byte-identisches Verhalten zu ADR-0028.

## Konsequenzen

**Positiv**
- Der reale „öffentliche Decision-Endpunkt" wird möglich, ohne die schreibende
  Oberfläche zu öffnen (löst genau die Lücke zwischen „alles offen" und „alles zu").
- Zwei orthogonale Granularitäten (Instanz-weit / pro Modell) mit einem Begriff
  (`evaluate` ist public) und ohne neuen Scope-Typ.
- Kern unberührt (ADR-0011), keine neue Dependency, reine Adapter-Schicht.

**Negativ / Kosten**
- Das Gate hat einen zusätzlichen Erlaubnispfad (public) — Testmatrix wächst um
  anonyme evaluate-Fälle über HTTP/gRPC/MCP.
- Die Transport-Asymmetrie (A nur HTTP-id-Routen) ist eine Feinheit, die man
  dokumentieren muss, damit „public" nicht als über alle Kanäle identisch
  missverstanden wird.
- Der Name-Match für A hängt am Cache-Zustand: ein aus dem Cache verdrängtes,
  nur per Name gelistetes Modell ist erst nach erneutem Laden wieder per Name
  public (per `modelId` immer). Für heiße öffentliche Decisions unkritisch.

**Folgeaufgaben**
- Roadmap-Zeilen WP-106 (Backend) / WP-107 (Admin-UI-Toggle) in
  `docs/20-roadmap.md`.
- Admin-UI: ein sichtbarer „Öffentlich"-Schalter pro Modell bzw. eine
  Server-Sicht der Public-Konfiguration (nur für `admin`-Keys). Siehe die offene
  Frage „Admin-Oberfläche für Zugriffskontrolle" — Key-Verwaltung (`/v1/keys*`)
  und Public-Toggles gehören in dieselbe, admin-gescopte Ecke.
- `docs/40-api-contract.md`: Public-Ausnahme als Teil der stabilen Oberfläche
  vermerken; OpenAPI-`security` der evaluate-Routen als „optional bei public"
  annotieren.
