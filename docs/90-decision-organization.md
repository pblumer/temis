# Decision-Organisation im Großen (tausende DMN-Modelle)

> **Bezug:** ADR-0025 (Decision- vs. Prozess-Orchestrierung), ADR-0011 (reine
> Go-Library), ADR-0016 (geteilte Modeler-Toolchain / BPMN-Synergie), ADR-0022
> (Git-gestützte Modelle), ADR-0023 (Entscheidungs-Logbuch clio).

Dieser Leitfaden beantwortet die Frage, die sich in einer Unternehmung mit **tausenden
DMN-Modellen** stellt: zentral oder pro Domäne verwalten, wie widersprechende Regeln
vermeiden, und wie die Schichten geschnitten werden. ADR-0025 trifft die zugrunde
liegende Architekturentscheidung; hier steht das **operative Wie**.

## 1. Weder rein zentral noch rein pro Domäne — föderiert

| | Rein zentral | Rein pro Domäne | **Föderiert (Empfehlung)** |
|---|---|---|---|
| Konsistenz | hoch | niedrig (Duplikate, Drift) | hoch (geteiltes Vokabular) |
| Domänen-Velocity | niedrig (Freigabe-Stau) | hoch | hoch |
| Fachwissen am richtigen Ort | nein | ja | ja |
| Skaliert auf 1000e Files | nein (ein Team = Bottleneck) | ja, aber unkontrolliert | ja, kontrolliert |

**Federated Governance:** Ein zentrales Plattform-/Standards-Team besitzt **Vokabular,
Konventionen und die Engine-Plattform**; die **fachlichen Regeln gehören den Domänen**.
Zentral ist die *Sprache*, nicht die *Logik*.

## 2. Schichtenmodell

Siehe ADR-0025 für die verbindliche Fassung. Jede Decision lebt in **genau einer**
Schicht mit **genau einem** Owner; Abhängigkeiten zeigen nur nach unten.

| Schicht | Inhalt | Owner | Ort |
|---|---|---|---|
| **L0 Foundation** | ItemDefinitions, Enums, geteilte BKM (Vokabular) | Plattform-Team | temis |
| **L1 Domain Decisions** | Kernregeln je Domäne (Pricing, Risk, …) | Domänen-Team | temis |
| **L2a Decision-Flow** | Decision komponiert Decisions, **stateless** (JSON-Flow-Deskriptor, ADR-0026) | Prozess-/Fach-Owner | temis |
| **L2b Prozess** | BPMN: durable state, Zeit, Mensch, Events | Prozess-Owner | chrampfer |
| **L3 Policy/Guardrails** | Regulatorik als Override-Decisions | Compliance | temis |

Kernprinzip: **unten breit & stabil, oben schmal & fallspezifisch.** Kein L1↔L1 —
Cross-Domain-Bedarf wandert nach L2a (stateless) bzw. L2b (mit Zustand).

## 3. Widersprüchliche Regeln vermeiden — drei Konfliktklassen

Widersprüche sind nicht *ein* Problem, sondern **drei** — mit je eigener Gegenmaßnahme:

1. **Intra-Table** (Regeln in *einer* Tabelle überlappen). → Über **Hit Policy** lösen:
   `UNIQUE` als Default (temis meldet Überlappung beim Compile), nur wo bewusst nötig
   explizit `PRIORITY`/`FIRST`/`COLLECT`. `FIRST`, um Overlap zu *verstecken*, ist meist
   ein Modellierungsfehler.

2. **Inter-Decision** (zwei Decisions liefern gegensätzliche Ergebnisse). → Vorrang nicht
   „auf gleicher Ebene ausdiskutieren", sondern als **explizite, aufgerufene
   Vorrang-Decision** in L2a (stateless) bzw. im Prozess über eine Business-Rule-Decision
   (L2b). Vorrang ist eine *Entscheidung*, kein Aufrufreihenfolge-Zufall.

3. **Duplikat/Drift** (dieselbe Regel mehrfach kopiert) — die gefährlichste Klasse bei
   1000en Files. → **Single Source of Truth pro Regel**: kein Copy-Paste, sondern Referenz
   auf die eine ownende Decision. Ownership ist eindeutig; niemand pflegt fremde
   Domänenregeln mit.

Flankierend (genau das, wofür temis gebaut ist):

- **Verträge zwischen Schichten** — `describe_decision` + `strict`-Validierung (WP-52)
  machen Ein-/Ausgabe jeder Decision zum typisierten Kontrakt; Änderungen brechen
  sichtbar statt still.
- **Golden-/Contract-Tests** — je Decision ein Satz Beispielfälle, in CI gegen die echte
  Engine geprüft (`make verify`-Gedanke); ein Vertragsbruch scheitert im PR.
- **Compile-before-write** — `git_propose` / `/v1/git/propose` kompilieren vor dem
  Schreiben (ADR-0022); kaputtes DMN landet nie im Repo.
- **Audit & Drift-Erkennung** — clio-Logbuch + `temis-reaudit` (ADR-0023) rechnen
  produktive Entscheidungen nach; Drift wird sichtbar.
- **Versionierung** — content-addressed `modelId` (ADR-0011); Konsumenten pinnen eine
  Version, Decision Services werden per SemVer weiterentwickelt.

## 4. Repo-Layout & Governance (Git-gestützte Modelle, ADR-0022)

Empfehlung: **Mono-Repo mit Verzeichnis-Ownership** — leichter für Cross-Layer-Refactoring
und globale CI als viele Repos.

```
models/
  foundation/          # L0  – CODEOWNERS: @platform-team
    vocabulary/        #      ItemDefinitions, Enums
    shared-bkm/
  domains/             # L1  – je Domäne ein Owner
    pricing/           #      CODEOWNERS: @pricing-team
    risk/
    eligibility/
  flows/               # L2a – stateless Decision-Flows
    loan-decisioning/  #      CODEOWNERS: @decisioning-owners
  policy/              # L3  – CODEOWNERS: @compliance (override)
# L2b (BPMN-Prozesse) leben in chrampfer, nicht hier.
```

- **CODEOWNERS** setzt Ownership technisch durch (PR braucht Owner-Approval) — der Hebel,
  der „föderiert" real macht.
- **Registry/Katalog:** Metadaten je Decision (Owner, Layer, Version, Konsumenten).
  Inventarbasis liefern `GET /v1/git/models` + das MCP-Tool `list_models`.
- **Zentrales Team besitzt** Vokabular (L0), Namens-/Modellier-Konventionen, CI-Gate,
  Plattformbetrieb — **nicht** die Fachregeln.

### 4a. Runtime-Katalog-Plane — Ordnung liegt auf den Namen, nicht auf den Blobs (ADR-0034)

§4 beschreibt die **Authoring-Plane**: der menschliche Verzeichnisbaum lebt in Git,
Ownership setzt CODEOWNERS durch. Zur Laufzeit muss ein `temisd`, der **tausende** Decisions
hält, dieselbe Ordnung aber *abbilden* können — der Server-Store ist ein flacher,
content-adressierter Blob-Haufen (`<sha256>.dmn`, ADR-0027), und ein Hash ist anonym: kein
Name, kein Ort. **Leitsatz: nicht die Blobs ordnen, sondern die Namen.** Organisation trennt
sich in drei Ebenen:

| Ebene | Was | Eigenschaft | Herkunft |
|---|---|---|---|
| **Content** | `sha256:<hex>`-Blobs | immutable, Audit-Wahrheit | Store (ADR-0027) |
| **Katalog / Identität** | `namespace/name@version → modelId + Metadaten` | mutabel, **autoritativ**, ownbar, abfragbar | **aus Git abgeleitet** |
| **Sicht** | Bookmarks, Tags-Filter, gespeicherte Ansichten | persönlich/Team, **nicht** autoritativ | Client (`localStorage`) |

- **Namespace = Verzeichnispfad** aus §4 (`models/domains/pricing/base-price.dmn` → Namespace
  `domains/pricing`, Name `base-price`). Ein Namensschema über **alle** Planes: Git-Verzeichnis
  = Server-Namespace = API/MCP-Prefix-Filter. Der Namespace kodiert Schicht (L0–L3) und
  Owner-Zuschnitt (CODEOWNERS) — die Governance oben wird zur Laufzeit sichtbar.
- **Namespace ist die Primärachse, Tags sind die Querachse.** Hierarchie beantwortet „*wo lebt
  es / wer besitzt es*"; Tags decken das Quer-Schneidende ab (`status`, `pii`, `jurisdiction`,
  `consumer`). *Ein* Baum plus Labels statt tiefer Ordnerverschachtelung.
- **Aus Git abgeleitet, read-only geladen** — der Server lädt den Katalog beim Start (spiegelt
  `-flows-dir`/ADR-0032) und schreibt **nie** zurück; Änderungen laufen über `git_propose`
  (compile-before-write). **Keine zweite Source of Truth, keine Drift** (§3, Klasse 3).
- **Identität ≠ Inhalt.** Der Katalog pinnt je Namen eine **aktuelle Revision**; der Store
  bleibt append-only, alte Revisionen bleiben für Audit/Replay, aber hinter dem Namen versteckt.
- **Skalierbares Listing.** `list_models` / `GET /v1/models` filtern nach Prefix, Tag, Status
  und paginieren — man rendert nie den ganzen Baum, man fragt ab.
- **Die heutigen Modeler-„Ordner" werden zu Bookmarks** — die Sicht-Ebene über dem
  autoritativen Namespace: persönliche Shortcuts und gespeicherte Filter, orthogonal zu
  Ownership. Das *Zuhause* einer Decision ist ihr Namespace; ein *Bookmark* ist eine Abkürzung.

Das Manifest-/Front-Matter-Format der Metadaten (Owner, Layer, Tags, Status, gepinnte
Revision) legt ADR-0034 als Folgeaufgabe fest; Umsetzung: Etappe „Decision-Katalog" (WP-140–143)
in `docs/20-roadmap.md`.

## 5. Der Decision-Flow (L2a) — konkret

Ein L2a-Flow ist ein **JSON-Deskriptor** (ADR-0026, `*.flow.json` im `flows/`-Verzeichnis),
der mehrere per `modelId` gepinnte Decisions/Services zu **einer** zustandslosen,
deterministischen Auswertung verkettet — Daten-Mapping über FEEL, Vorrang als *aufgerufene*
Auflösungs-Decision:

```jsonc
{
  "flow": "loan-decisioning",
  "steps": [
    { "id": "risk",    "model": "sha256:1a2b…", "decision": "Risk Level",
      "in": { "Credit Score": "Credit Score" } },
    { "id": "pricing", "model": "sha256:7c3d…", "decision": "Base Price",
      "in": { "Amount": "Loan Amount" } },
    { "id": "resolve", "model": "sha256:9f8e…", "decision": "Final Decision",
      "in": { "Risk": "risk.Risk Level", "Price": "pricing.Base Price" } }
  ],
  "output": { "Decision": "resolve.Final Decision" }
}
```

Warum ein Deskriptor und **kein** DMN-`import`: jede DMN-Datei bleibt autark ownbar/testbar,
die Komposition ist ein eigenes reviewbares Artefakt mit eigenem Owner (`flows/` +
CODEOWNERS), und weil jeder Step ein `modelId` pinnt, ist der ganze Flow **re-auditierbar**
(ADR-0023). Umsetzung: Etappe „Decision-Flow" (WP-90–94) in `docs/20-roadmap.md`.

**Wie ein Flow in Betrieb kommt (Source of Truth, ADR-0032).** Ein Flow lebt als
`flows/<name>/*.flow.json` **im Git-Repo** — das ist die durable, reviewbare, versionierte
Fassung (CODEOWNERS). Der Server lädt sie beim Start read-only über **`-flows-dir`**
(`WithFlowStore`, spiegelt `-models-dir` für Modelle): alle `*.flow.json` werden kompiliert
und in den Katalog registriert, **nachdem** die Modelle geladen sind, sodass die Validierung
greift. Ein Flow mit (noch) nicht geladenen Modellen registriert trotzdem und trägt
Diagnostics. Der Server schreibt Flows **nie zurück** — Änderungen laufen über Git +
`git_propose` (compile-before-write), nicht über einen Server-Schreibpfad; so entsteht **keine
zweite Quelle der Wahrheit**. `POST /v1/flows` bleibt der **flüchtige Dev-/Interaktiv-Pfad**
(Flow Studio, Experimente), nicht persistiert.

## 6. Die temis↔chrampfer-Naht (Kurzform)

Vollständig in ADR-0025. Merksätze:

- **chrampfer → temis, in-process, nie zurück** (ADR-0011). Der BPMN *Business Rule Task*
  ruft `package dmn` direkt auf, nicht `temisd`-HTTP.
- **Decisions bleiben rein** (kein Zustand/Uhr/I/O); der **Prozess besitzt die Falldaten**
  und mappt sie auf Decision-Inputs/Outputs.
- **Keine Geschäftslogik in BPMN-Gateways** — das Gateway fragt „welcher Pfad?", die
  Bestimmung ist eine DMN-Decision.
- **Zwei komplementäre Logbücher:** clio/temis (*was entschieden wurde*, nachrechenbar) +
  chrampfer (*was passiert ist*, Prozess-Events).
