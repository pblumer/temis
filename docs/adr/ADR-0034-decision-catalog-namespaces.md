# ADR-0034: Decision-Katalog & Namespaces — Ordnung liegt auf den Namen, nicht auf den Blobs

- **Status:** proposed
- **Datum:** 2026-07-03
- **Kontext-WP:** WP-140 (Decision-Katalog); baut auf ADR-0027 (Filesystem-Model-Store) und ADR-0032 (Flow-Registry als Verzeichnis-Quelle), setzt `docs/90-decision-organization.md` in die Runtime um

## Kontext

Ein produktiver `temisd` muss damit rechnen, **tausende oder mehr** Decisions zu
beherbergen. Heute findet „Organisation" an drei unverbundenen Orten statt — keiner davon
skaliert als Navigations- und Governance-Fläche:

1. **Server-Store (ADR-0027).** Ein *flacher*, content-adressierter Haufen `<sha256>.dmn`,
   append-only, nach mtime sortiert. Kein Namensraum, keine Hierarchie, kein GC.
   `GET /v1/models` bzw. das MCP-Tool `list_models` liefern eine **flache Liste**
   (`ModelID, Name, Decisions, Inputs, Seq`) — **ohne Owner, Layer, Tag oder Status.** Bei
   tausenden Files × Revisionen ist die Liste unnavigierbar.
2. **„Ordner" im Modeler (`web/src/main.ts`).** Reine **Browser-`localStorage`**
   (`temis.modeler.folders`), zugeordnet über den *Namen*, **pro Browser**, nicht geteilt,
   nicht autoritativ.
3. **Die föderierte Governance (`docs/90-decision-organization.md`).** Schichten L0–L3,
   Mono-Repo mit Verzeichnis-Ownership + CODEOWNERS — aber das lebt **nur in Git**
   (Authoring-/Lifecycle-Plane). Der laufende Server kennt diese Ordnung zur Laufzeit nicht.

Kernbeobachtung: Der content-adressierte Store ist **richtig so** (ADR-0011/0027 —
reproduzierbar, audit-/replay-fest). Aber ein Hash ist **anonym**: kein Name, kein Ort.
Jede Hierarchie *auf den Blobs* wäre sinnlos, weil jede Bearbeitung den Hash ändert und das
Modell neu einsortiert werden müsste. **Ordnung gehört auf eine Identitäts-/Katalog-Ebene
darüber**, nicht auf den Speicher darunter.

## Optionen

1. **Status quo — flacher Store + private localStorage-Ordner.** Minimal, aber skaliert
   nicht: keine geteilte Ordnung, keine serverseitige Governance-Sicht, Liste unnavigierbar
   bei tausenden Files.
2. **Hierarchie in den Store legen** (Blobs in Verzeichnisse `domains/pricing/…`). — Bricht
   die Content-Adressierung: der Ort wäre an den *Namen* gebunden, aber der Dateiname ist der
   *Inhalts-Hash*; jede Edit-Operation verschöbe das Modell. Vermischt zwei Identitäten
   (Inhalt vs. Platz). **Verworfen.**
3. **Eigenständige Embedded-DB als Katalog** (SQLite/bbolt mit Metadaten/Queries). — Bringt
   einen neuen Dependency und einen mutierenden serverseitigen Schreibpfad (zweite Source of
   Truth neben Git → Drift, die Gefahrenklasse aus `docs/90` §3). Widerspricht der
   stdlib-only-Politik (ADR-0011/0014). **Verworfen** (wie in ADR-0027 für Blobs).
4. **Katalog-Ebene über den Blobs, aus Git abgeleitet, read-only geladen** (diese
   Entscheidung). Drei Ebenen sauber trennen; die Ordnung lebt im Katalog (autoritativ, aus
   Git) und in Bookmarks (persönlich). Spiegelt exakt das `-flows-dir`-Muster (ADR-0032).

## Entscheidung

**Option 4.** Wir führen einen **Decision-Katalog** als erstklassige, aus Git abgeleitete,
read-only geladene Ebene ein und trennen Organisation in drei Planes:

| Ebene | Was | Eigenschaft | Herkunft |
|---|---|---|---|
| **Content** | `sha256:<hex>`-Blobs | immutable, Audit-Wahrheit | Store (ADR-0027), unverändert |
| **Katalog / Identität** | `namespace/name@version → modelId + Metadaten` | mutabel, **autoritativ**, ownbar, abfragbar | **aus Git abgeleitet** |
| **Sicht** | Bookmarks, Tags-Filter, gespeicherte Ansichten | persönlich/Team, **nicht** autoritativ | Client (`localStorage`), später opt-in geteilt |

Leitsatz: **Nicht die Blobs ordnen, sondern die Namen.** Der menschliche Verzeichnisbaum ist
Git; der Server *indexiert* ihn, statt ihn zu duplizieren.

### Semantik

- **Quelle der Wahrheit = Git (ADR-0022/0032).** Der Namespace ist der **Verzeichnispfad im
  Modell-Repo** (`models/domains/pricing/base-price.dmn` → Namespace `domains/pricing`,
  Name `base-price`); Metadaten (Owner, Layer, Tags, Status, aktuelle Revision) stammen aus
  einem Manifest bzw. Front-Matter neben dem Modell. Der Server lädt den Katalog **read-only**
  und schreibt **nie** zurück — Änderungen laufen über Git + `git_propose`
  (compile-before-write). **Keine zweite Source of Truth, keine Drift.**
- **Ein Namensschema über alle Planes.** Derselbe Koordinat `namespace/name` funktioniert in
  Git (Verzeichnis), im Server-Katalog (Namespace) und in API/MCP (Prefix-Filter). Der
  Namespace kodiert zugleich die Schicht (L0–L3) und den Owner-Zuschnitt (CODEOWNERS) aus
  `docs/90` §4 — die dokumentierte Governance wird damit zur Laufzeit sichtbar.
- **Namespace = Primärachse, Tags = Querachse.** Hierarchie beantwortet „*wo lebt es / wer
  besitzt es*"; **Tags/Labels** decken das Quer-Schneidende ab (`status=active|deprecated|
  archived`, `pii`, `jurisdiction=CH`, `consumer=loan-app`). Das verhindert die
  „Deep-Folder-Hölle", die einen einzelnen Baum bei tausenden Files erschlägt.
- **Identität ≠ Inhalt.** Der Katalog bindet einen **stabilen Namen** an eine **aktuelle,
  gepinnte Revision** (`modelId`). Der Store bleibt append-only content-adressiert; alte
  Revisionen bleiben für Audit/Replay erhalten, aber **hinter dem Namen versteckt**. Die UI
  zeigt *benannte aktuelle Decisions*, nicht den rohen Hash-Haufen.
- **Skalierbares Listing.** `list_models` / `GET /v1/models` werden um **Prefix-Filter,
  Tag-Filter, Status und Pagination** erweitert; `ModelInfo`/`modelSummary` um
  `Namespace, Owner, Layer, Tags, Status`. Man rendert nie den ganzen Baum — man fragt ab.
- **Fail-open, additiv (wie ADR-0032).** Ohne Katalog-Quelle verhält sich der Server
  **byte-identisch** zu heute: eine flache, unbenannte Liste. Ein Katalogeintrag, dessen
  Modell (noch) nicht geladen ist, wird geloggt und trägt Diagnostics, blockiert aber nie den
  Start. Ein fehlendes Katalog-Verzeichnis deaktiviert die Ebene (geloggt).
- **Ordner → Bookmarks.** Die heutigen `localStorage`-Ordner werden zur **Sicht-Ebene** über
  dem autoritativen Namespace: Shortcuts, gespeicherte Filter, persönliche Kuratierung —
  orthogonal zu Ownership, nicht autoritativ. Das *Zuhause* einer Decision ist ihr Namespace;
  ein *Bookmark* ist eine Abkürzung. Bestehende Ordner-Zuweisungen (nach Name) können
  Namespace/Tags initial seeden.

## Konsequenzen

**Positiv**
- Der Server bildet die föderierte Governance aus `docs/90` **zur Laufzeit** ab (Namespace =
  Layer + Owner-Zuschnitt); „das sind unsere Decisions, so geordnet" out-of-the-box.
- Skaliert auf tausende Decisions: Abfrage statt Vollrender; Hierarchie *und* Tags.
- **Keine zweite, mutierbare Quelle** — Git bleibt Source of Truth, keine Drift; konsistent
  mit ADR-0022/0027/0032 und `docs/90` §3.
- Rein **additiv & stdlib-only**: kein neuer Dependency, kein Hot-Path berührt; ohne
  Katalog-Quelle unverändertes Verhalten.
- Deine Intuition wird tragend: die heutigen Ordner bekommen ihre **richtige Rolle**
  (Bookmarks), statt als Pseudo-Ordnung zu überladen.

**Negativ / Kosten**
- Eine neue Config-Fläche (Katalog-Verzeichnis, spiegelt `-flows-dir`) und ein Startup-Scan.
- Das Namespace-/Metadaten-Format (Manifest bzw. Front-Matter) muss festgelegt und in `docs/90`
  §4 verbindlich gemacht werden.
- Katalog und Modelle müssen zusammen bereitgestellt werden; sonst tragen Einträge beim Start
  Diagnostics (bewusst nicht fatal).

**Folgeaufgaben**
- WP-140–143 in `docs/20-roadmap.md`: Katalog-Loader (`WithCatalog(dir)` / `-catalog-dir`),
  `list_models`-Filter (Prefix/Tag/Status/Pagination), `ModelInfo`-Felder, Modeler-Sidebar
  (Namespace-Baum + Tag-Filter, Ordner→Bookmarks).
- `docs/90-decision-organization.md`: neuer Abschnitt „Runtime-Katalog-Plane" (siehe unten),
  §4 um das Manifest-/Front-Matter-Format ergänzen.
- Später (separat): opt-in geteilte Bookmarks/Views (serverseitig), GC/Retention alter
  Revisionen (offen aus ADR-0027), Katalog auch über MCP/Git-Tools abfragbar.
