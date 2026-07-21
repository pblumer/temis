# ADR-0037: Modell-Releases — benannte, unveränderliche Publikationen über content-adressierten Revisionen

- **Status:** proposed
- **Datum:** 2026-07-21
- **Kontext-WP:** Folge-Etappe „Modell-Releases" (Roadmap, unten); adressiert die in
  ADR-0027 offen gelassene Retention-Grenze und die in
  `docs/90-decision-organization.md §Versionierung` skizzierte Konsumenten-Pinning-Story.

## Kontext

In temis ist **jede Speicherung eine content-adressierte Revision**: der `modelId` ist der
`sha256:`-Hash des DMN-XML (ADR-0011), und der optionale Dateisystem-Store schreibt jede
Revision append-only als `<hex>.dmn` (ADR-0027, „**Keine Löschung/GC alter Revisionen**").
Der Modeler gruppiert Revisionen nach Anzeigename und legt die älteren als zusammenklappbare
„Historie" unter die neueste (`web/src/main.ts` `groupModels`, `modelSummary.Seq` in
`service/http.go`).

Das ist bewusst so — es macht das Zwischenspeichern beim Modellieren risikolos und die Engine
zustandslos/deterministisch. Die Kehrseite ist genau der Reibungspunkt, der dieses ADR
auslöst: **jeder Zwischenstand ist ein gleichwertiger, sichtbarer, dauerhaft persistierter
„Version"-Eintrag.** Beim aktiven Arbeiten entstehen schnell dutzende Revisionen — „das kann
schnell eine Menge an Versionen geben".

Dahinter liegen **zwei vermischte Bedürfnisse**:

1. **Sicheres Zwischenspeichern** beim Arbeiten — viele, flüchtige Stände. (Heute abgedeckt.)
2. **Stabile, benannte Stände**, auf die Konsumenten dauerhaft zeigen — Flow-Steps
   (ADR-0026 pinnen eine `modelId`), der clio-Command-Consumer (ADR-0033), MCP/HTTP-`evaluate`,
   ein nachgelagerter Prozess (ADR-0025). (Heute **nicht** abgedeckt: der Nutzer muss sich
   selbst eine opake `sha256:`-id merken und weiterreichen.)

Für **git-gestützte** Modelle (ADR-0022) existiert für Bedürfnis 2 bereits ein erstklassiger
Begriff: der **Git-Tag/Ref** (Branch/Tag/Commit als Herkunft). Für den Default-Pfad — den
in-memory/disk-Store, dessen Motivation ausdrücklich „jemand pflegt ein paar
Entscheidungstabellen über die Web-UI" ist (ADR-0027) — fehlt das Pendant. **Ein Release ist
das Store-Pendant zum Git-Tag.**

### Kräfte

- **Content-Adressierung nicht aufgeben.** Determinismus, Cache-Key (WP-35), Re-Audit
  (ADR-0023) und Flow-Pinning hängen daran. Ein Release darf den `modelId`-Begriff nicht
  ersetzen, nur *benennen*.
- **stdlib-only, opt-in, kein neuer Dep** (Goldene Regel 6, dieselbe Linie wie ADR-0027).
- **Deterministisch pinnbar.** Ein Release muss auf **genau eine** `modelId` auflösen, sonst
  bricht die Re-Auditierbarkeit der Konsumenten.
- **Engine-Reinheit wahren.** Release-Metadaten sind ein **Katalog-/Store-Konzept**, kein
  Engine-Zustand — die reine Berechnungsfunktion (ADR-0004/0011) bleibt unberührt.

## Optionen

1. **Status quo + reine UI-Kosmetik.** Historie im Modeler nur zusammenklappen. — Billig,
   aber Bedürfnis 2 (stabiles Konsumenten-Pinning) bleibt ungelöst, der Speicher wächst
   weiter append-only.
2. **Releases nur als Git-Tags (ADR-0022-Pfad).** Für Teams mit Repo ideal. — Zwingt aber
   Git auf **alle**; der Default-Pfad (Web-UI, disk-Store) hat kein Repo. Als *alleinige*
   Lösung verworfen — der Git-Tag bleibt aber der Release-Begriff für den Git-Pfad, was die
   Konzept-Symmetrie liefert.
3. **Neue Persistenz/DB mit Versionstabelle (SQLite/bbolt).** — Bricht die stdlib-only-Politik;
   bereits in ADR-0027 für reine XML-Blobs als überdimensioniert verworfen. Für ein schmales
   Metadaten-Manifest erst recht.
4. **Release = benannter, unveränderlicher Zeiger auf eine Revision, als schmales
   JSON-Manifest neben dem content-adressierten Store (gewählt).** Git-tag-artig, kein
   Inhaltsduplikat, reine stdlib, opt-in, spiegelt die Mechanik von ADR-0027.

## Entscheidung

**Option 4.** Ein Release **taggt** eine bereits existierende Revision — es erzeugt keinen
neuen Inhalt.

### Begriffe

- **Entwurf (Draft)** — das Ergebnis jedes Speicherns: eine content-adressierte Revision
  (`modelId`). Viele, flüchtig, das Arbeitsmaterial. **Unverändert zu heute.**
- **Release (Publikation)** — ein bewusst gesetzter, **unveränderlicher** Eintrag
  `(name, version) → modelId` mit `publishedAt` und optionalen `notes`. Dieselbe
  `(name, version)` zweimal zu setzen ist ein Konflikt (`409`); dieselbe `modelId` unter einer
  neuen Version zu publizieren ist erlaubt (z. B. Rollback-forward).
- **Kanal (Channel), optional** — ein **beweglicher** benannter Zeiger je Modell, z. B.
  `latest` / `stable` / `prod`, der auf eine Release-Version zeigt. `latest` wird beim
  Publizieren automatisch auf die höchste Version gesetzt; andere sind manuell.

### Versionsschema

**SemVer `major.minor.patch` empfohlen**, in Angleichung an ADR-0019 (SemVer-Disziplin für
Decision-Services): drückt breaking vs. additiv aus. Der Server erzwingt nur
**Wohlgeformtheit + Eindeutigkeit/Monotonie**, **nicht** die Semantik-Klassifizierung — die
bleibt menschliches Urteil, exakt wie in ADR-0019. Ein einfacher fortlaufender Modus
(`v1, v2, …`) ist ein Sonderfall davon (nur `major` hochzählen) und bleibt möglich: die Wahl
ist eine **Konvention**, kein harter Zwang. So bleibt der Server schema-agnostisch und
empfiehlt trotzdem eine Ordnung.

### Speicherung

Ein **Release-Manifest** als reines JSON **neben** dem `diskStore` (ADR-0027) — ein
`releases.json` je Store-Verzeichnis (oder `releases/<name>.json`), **atomar** geschrieben
(Temp + `os.Rename`, wie `diskStore.put`). Es enthält **nur Metadaten + `modelId`-Referenzen,
kein XML**:

```jsonc
{
  "Pricing": {
    "releases": [
      { "version": "2.1.0", "modelId": "sha256:7c3d…", "publishedAt": "2026-07-21T…", "notes": "Q3-Tarife" },
      { "version": "2.0.0", "modelId": "sha256:1a2b…", "publishedAt": "2026-06-…" }
    ],
    "channels": { "latest": "2.1.0", "stable": "2.0.0" }
  }
}
```

- **Beim Boot geladen** wie der Modell-Store; die referenzierte `modelId` muss im Store liegen
  (Boot rekompiliert das XML deterministisch, wie heute). Ein Release, dessen `modelId` fehlt,
  trägt eine **Diagnostik** statt stillschweigend zu verschwinden — dieselbe Linie wie die
  Flow-Registry (ADR-0032: „Flow mit nicht geladenen Modellen registriert trotzdem und trägt
  Diagnostics").
- **Opt-in**, an den bestehenden `-models-dir` gekoppelt (oder eigenes `-releases-dir`). Ohne
  Store bleibt alles rein flüchtig — Default-Verhalten byte-identisch zu heute.

### Auflösung (Resolver) — der eine Choke-Point

Eine Referenz `name@version`, `name@channel` oder eine nackte `sha256:…`-`modelId` löst auf
**genau eine** `modelId` auf. Diesen Resolver nutzen **alle** Konsum-Oberflächen:

- **Flow-Steps** (ADR-0026): `"model": "Pricing@2.1.0"` statt roher `sha256:` → Deskriptoren
  werden lesbar und bleiben re-auditierbar. **Empfehlung (Lockfile-Muster):** beim Registrieren
  eines Flows die Referenz auf die konkrete `modelId` auflösen **und einfrieren**, damit
  Re-Audit (ADR-0023) deterministisch bleibt — der lesbare Name ist Eingabe, die gepinnte
  `modelId` das Artefakt.
- **HTTP / MCP / gRPC `evaluate`**: akzeptiert `Pricing@stable`, wo heute eine `modelId` steht.
- **clio-Command-Consumer** (ADR-0033): Commands referenzieren Releases statt opaker ids.

### GC / Retention — löst die offene ADR-0027-Grenze

Ein **bewusster** Aufräum-Befehl (kein Auto-GC) darf Drafts löschen, die **alle drei**
Bedingungen erfüllen: (a) kein Release, (b) von keinem Kanal/Flow referenziert, (c) nicht die
aktuelle Bearbeitungs-Head-Revision. Releases und alles Referenzierte sind **unantastbar**.
Default bleibt append-only (unverändert sicher); GC ist opt-in — genau die „optionale
GC/Retention … spätere WP", die ADR-0027 offen gelassen hat.

### UI (Modeler) — der eigentliche Payoff für „Menge an Versionen"

- **Toolbar „Veröffentlichen"** auf dem geladenen Entwurf → fragt Version (+ optional Notes),
  setzt das Release und den `latest`-Kanal.
- **Release-zentrierte Sidebar:** je Modell primär die **Releases** (Version-Badge +
  Kanal-Chips); die rohen Zwischenentwürfe wandern hinter ein „Verlauf/Entwürfe (n)"-Disclosure
  oder werden ausgeblendet. Die bestehende `seq`/Namens-Gruppierung (`main.ts`) bleibt die
  Datenbasis — **nur die Präsentation ändert sich.**
- Ein seit dem letzten Release verändertes Modell zeigt **„Entwurf (unveröffentlicht)"**.

### Abgrenzung — was ein Release *nicht* ist

- **Kein Deployment-/Environment-Promotion-Workflow** (Approvals, Staging→Prod-Gates). Das ist
  die **externe Control Plane** — ADR-0030 zieht diese Grenze bewusst. Kanäle sind ein leichter
  Pointer, keine Freigabe-Pipeline.
- **Kein Ersatz für git-gestützte Modelle** (ADR-0022). Für review-getriebene, dauerhafte
  Governance bleibt Git die Quelle; dort **ist** ein Tag der Release. Der Store-Release ist das
  Pendant für den serverlokalen/Default-Pfad. Beide teilen den **einen Resolver-Begriff**
  `name@ref`.

## Konsequenzen

**Positiv**
- Klare Trennung **Arbeitsstand vs. veröffentlichter Stand**; die „Menge an Versionen"
  verschwindet aus der Standardsicht.
- Konsumenten pinnen **lesbare, stabile `name@version`** statt opaker `sha256:` — und bleiben
  deterministisch/re-auditierbar (Auflösung zu genau einer `modelId`).
- **Löst die in ADR-0027 offen gelassene Retention-Grenze** (GC nur für Nicht-Releases).
- Reine stdlib, kein neuer Dep, opt-in, **Engine-Kern unberührt** (Katalog-Konzept, kein
  Engine-Zustand). Konsistent mit ADR-0011/0027/0032.
- **Konzept-Symmetrie** mit ADR-0022 (Git-Tag = Release) über einen gemeinsamen Resolver.

**Negativ / Kosten**
- Neue **mutable Metadaten-Ebene** (Manifest) neben dem bisher rein content-adressierten Store
  — braucht atomare Writes, Boot-Validierung (fehlende `modelId` → Diagnostik) und
  Konflikt-Regeln (unveränderliche `(name, version)`).
- Die SemVer-**Klassifizierung** bleibt menschliches Urteil (wie ADR-0019); der Server erzwingt
  nur Wohlgeformtheit/Eindeutigkeit.
- **Bewegliche Kanäle** können Nicht-Determinismus einführen, wenn Konsumenten `@latest`
  pinnen → daher das Lockfile-Muster (in Flows/Commands beim Registrieren auflösen & einfrieren).
- **Mehr-Oberflächen-Arbeit** (Store, Resolver, HTTP, MCP, gRPC, Modeler-UI) — als eigene
  Roadmap-Etappe zu schneiden.

**Folgeaufgaben (Roadmap-Etappe „Modell-Releases")**
- **WP-a** Release-Store + Manifest (stdlib, atomar, Boot-Load, Validierung) — Backend-Kern in
  `service`.
- **WP-b** Resolver `name@version|channel|sha256` als geteilter Choke-Point; HTTP
  `POST/GET /v1/models/{name}/releases`, `…/channels` (+ OpenAPI-Sync).
- **WP-c** MCP/gRPC + Flow-Step- & Command-Referenzen auf Releases (mit Auflösen & Einfrieren).
- **WP-d** Modeler-UI: „Veröffentlichen", release-zentrierte Sidebar, Entwurf-Disclosure.
- **WP-e** opt-in GC/Retention (nur Nicht-Releases) — schließt die ADR-0027-Grenze.
- **Doku:** `docs/90-decision-organization.md §Versionierung` konkretisieren; `00-overview.md §3`
  (Decision-Management war MVP-Nicht-Ziel) relativieren, wie schon bei ADR-0022/0027 geschehen.
