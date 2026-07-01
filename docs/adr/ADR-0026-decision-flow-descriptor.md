# ADR-0026: Decision-Flow-Deskriptor (L2a) — externe JSON-Komposition statt DMN-`import`

- **Status:** proposed
- **Datum:** 2026-07-01
- **Kontext-WP:** WP-90–94 (neue Etappe „Decision-Flow"); baut auf WP-28/29/52/70, verfeinert ADR-0025

## Kontext

ADR-0025 hat die **L2a-Schicht** eingeführt — eine *stateless, deterministische*
Komposition mehrerer Entscheidungen über Modellgrenzen hinweg — und ihre konkrete
Umsetzung bewusst offengelassen: **nativer DMN-`import`/`includedModels`** vs. **leichter
Flow-Deskriptor**. Diese ADR trifft die Wahl und spezifiziert sie.

Der Bedarf ist real: In einer Unternehmung mit tausenden Modellen (siehe
`docs/90-decision-organization.md`) muss eine Journey mehrere Domänen-Decisions (Pricing,
Risk, Eligibility) zu **einer** Antwort verketten und dabei **stateless-Vorrang** zwischen
konkurrierenden Ergebnissen auflösen — ohne bereits eine Prozess-Engine (L2b/chrampfer) zu
bemühen. Heute geht das nur durch **externe Orchestrierung** (mehrere `evaluate`-Aufrufe von
Hand verdrahtet), was weder ein wiederverwendbares Artefakt noch eine re-auditierbare Einheit
ist.

Bestehende Bausteine, an die L2a andockt:

- **DRG-Verkettung** (WP-28, `dmn/graph.go`) und **Decision Services** (WP-29,
  `dmn/eval.go` — gemeinsamer memoisierter `evaluator`) komponieren *innerhalb* **eines**
  Modells. L2a komponiert *über* Modelle hinweg — die fehlende Ebene.
- **Content-addressed `modelId`** (`sha256:`-Schema; Service-Cache und `audit.DirModelSource`
  hashen identisch, WP-55) macht ein Modell eindeutig referenzierbar.
- **`InputSchema`/`ValidateInput`** (WP-52) beschreiben je Decision den typisierten
  Ein-/Ausgabe-Kontrakt — die Grundlage, um die Verdrahtung zweier Steps zu prüfen.
- **`package vcs`** (WP-70) löst Modelle aus Git auf; **clio + `audit.ReAudit`** (WP-54/55)
  protokollieren und rechnen Einzel-Decisions nach.

## Optionen

1. **Nativer DMN-`import`/`includedModels`.** Cross-Modell-Referenzen im DMN-XML;
   `internal/model` bekäme `includedModels`, ein Resolver löst `href`→Modell auf. — Bleibt
   „im Standard", aber: dehnt die namespace-tolerante XML-Schicht erheblich, verwebt die
   Modelle (eine Datei ist ohne ihre Importe nicht mehr autark ownbar/testbar), und die
   DMN-`import`-Semantik (Namespaces, `locationURI`) ist schwergewichtig für den eigentlichen
   Zweck. Widerspricht der föderierten „ein Modell = ein Owner"-Governance (`docs/90`).

2. **Externer JSON-Deskriptor, ausgewertet von einem neuen `flow`-Package** (diese
   Entscheidung). Ein kleines, deklaratives Artefakt verdrahtet mehrere **per `modelId`
   gepinnte** Decisions; Daten-Mapping über **FEEL**; ausgewertet **in-process** durch ein
   eigenes Package über der stabilen `dmn`-API. — Jede DMN-Datei bleibt autark; die
   Komposition ist ein eigenes, reviewbares, ownbares Artefakt; deterministisch und
   re-auditierbar. Kosten: ein neues Artefaktformat + Package (eigene SemVer-Spur).

3. **Nur im Service/Orchestrator, kein First-class-Artefakt.** Ein Ad-hoc-Endpunkt kettet
   Auswertungen. — Nicht in-process einbettbar (chrampfer könnte L2a nicht ohne
   Netzwerk-Hop nutzen, gegen ADR-0011), kein versionierbares Artefakt, nicht
   re-auditierbar als Einheit. Verworfen.

## Entscheidung

**Option 2.** Ein **Decision-Flow-Deskriptor**: ein externes JSON-Artefakt, ausgewertet von
einem neuen, zustandslosen **`flow`-Package**, das die öffentliche `dmn`-API konsumiert.

### 1. Format: JSON, nicht in DMN-XML

Der Deskriptor ist ein eigenständiges Artefakt (`*.flow.json`, Media-Type
`application/vnd.temis.flow+json`), das neben den Modellen im Repo lebt (`flows/`-Verzeichnis,
`docs/90`). **JSON, nicht YAML** — konsistent mit dem JSON-Wire-Vertrag (HTTP/MCP) und
**ohne neue Dependency** (Goldene Regel 6; `encoding/json` der stdlib, wie `assist`/`vcs`).

```jsonc
{
  "flow": "loan-decisioning",
  "version": "1",
  "inputs": [                                  // optionales, deklariertes Flow-Eingabeschema
    { "name": "Applicant Age", "type": "number" },
    { "name": "Credit Score",  "type": "number" }
  ],
  "steps": [
    {
      "id": "risk",
      "model": "sha256:1a2b…",                 // content-addressed modelId (Pinning)
      "decision": "Risk Level",                //   Decision ODER Decision Service
      "in": {                                  // FEEL-Ausdrücke über {Flow-Inputs + frühere Steps}
        "Credit Score": "Credit Score",        //   Namens-Durchreichung (häufigster Fall)
        "Adjusted Score": "Credit Score - 50"  //   oder jeder FEEL-Ausdruck
      }
    },
    {
      "id": "resolve",
      "model": "sha256:9f8e…",
      "decision": "Final Decision",
      "in": { "Risk": "risk.Risk Level", "Age": "Applicant Age" }   // frühere Step-Outputs referenzierbar
    }
  ],
  "output": { "Decision": "resolve.Final Decision" }  // Flow-Ergebnis aus Step-Outputs assemblieren
}
```

### 2. Eigenes `flow`-Package (nicht in `package dmn`)

L2a lebt in einem **neuen Top-Level-Package `flow/`**, das `package dmn` importiert — **nicht**
in `package dmn` selbst. Damit bleibt die als v1 eingefrorene `dmn`-Golden-Surface
(`dmn/apisurface_test.go`, ADR-0019) **unberührt**, L2a bekommt eine **eigene SemVer-Spur**,
und die physische Paketgrenze spiegelt die Schichtung aus ADR-0025 (L1 = `dmn`, L2a = `flow`).
Präzedenz: `assist`, `audit`, `vcs` sind ebenfalls eigenständige Pakete über der Engine.

```go
package flow

// Ein geparster, validierter Deskriptor. Immutable, thread-safe.
type Flow struct { /* opaque */ }

func Compile(descriptor []byte) (*Flow, Diagnostics, error)   // parst + validiert die Struktur

// Resolver liefert kompilierte Modelle zu einem modelId — die Abstraktion,
// über die Cache-, Git- (vcs) und Inline-Quellen eingehängt werden.
type Resolver interface {
    Resolve(ctx context.Context, modelID string) (*dmn.Definitions, error)
}

// Evaluate wertet den ganzen Flow zustandslos aus: topologische Reihenfolge der Steps,
// jeder Step über den bestehenden dmn-Evaluator, Ergebnisse als Kontext weitergereicht.
func (f *Flow) Evaluate(ctx context.Context, in dmn.Input, r Resolver, opts ...Option) (dmn.Result, error)
```

`Evaluate` liefert eine **`dmn.Result`** (gleiche Outputs/Trace-Typen wie eine
Einzel-Decision), damit Flows und Decisions über alle Oberflächen uniform behandelt werden.

### 3. Semantik

- **Modelle per `modelId` gepinnt.** Ein Flow referenziert Modelle über ihre
  content-addressed `modelId`. Damit ist der Flow **deterministisch**: gleiche `modelId`s +
  gleiche Eingabe → byte-identische Ausgabe (ADR-0007). Das ist die Voraussetzung für
  Re-Audit (Punkt 6). Git-Referenzen (`{owner,repo,ref,path}`) werden vom Resolver *zu einem
  `modelId` aufgelöst* und dann gepinnt.
- **Daten-Mapping als FEEL.** Jeder `in`-Eintrag ist ein FEEL-Ausdruck, ausgewertet gegen
  einen Kontext aus **Flow-Inputs + Outputs aller früheren Steps** (letztere unter ihrer
  `step.id` als Kontext-Key). Reine Namens-Durchreichung ist der Normalfall; die volle
  FEEL-Mächtigkeit steht für Umbenennung/Ableitung bereit. Wiederverwendung des bestehenden
  FEEL-Compilers — kein neuer Ausdrucks-Dialekt.
- **DAG, azyklisch.** Steps bilden einen gerichteten azyklischen Graphen (ein Step darf nur
  frühere referenzieren). Der Compiler prüft Azyklizität (wie `DECISION_CYCLE` in WP-28);
  Auswertung memoisiert (Diamond → einmal). Parallel-unabhängige Steps *dürfen* nebenläufig
  ausgewertet werden (rein → deterministisch), zunächst aber sequenziell.
- **Vorrang als aufgerufene Decision.** Konkurrierende Domänen-Ergebnisse werden **nicht**
  durch Step-Reihenfolge aufgelöst, sondern indem ihre Outputs in eine **eigene
  Auflösungs-Decision** (ein weiterer Step) verdrahtet werden — genau das ADR-0025-Prinzip
  „Vorrang ist eine Entscheidung, kein Aufrufreihenfolge-Zufall".

### 4. Validierung (compile-before-write)

Wie bei Modellen (`Save`/`Propose` kompilieren vor dem Schreiben, WP-71/72) wird ein Flow
**vor** Persistierung/Ausführung validiert: alle `modelId`s resolvebar, jede referenzierte
`decision`/Service existiert im Zielmodell, die `in`-Verdrahtung passt gegen das
**`InputSchema`** (WP-52) des Ziel-Decisions (Typ-Check), der Step-Graph ist azyklisch, alle
Mapping-FEEL-Ausdrücke kompilieren. So landet ein kaputter Flow **nie** im Repo und schlägt
**vor** der Auswertung mit präzisen Diagnostics fehl, nicht still mit `null`.

### 5. Ressourcenlimits (ADR-0008)

Ein Flow wertet N Decisions aus. Das per-Evaluation-Budget (WP-34) wird **über alle Steps
geteilt** (nicht pro Step zurückgesetzt), plus ein **`MaxSteps`-Guard** — ein Flow darf kein
Amplifikations-Vektor werden. Die Step-Liste ist endlich und azyklisch; das begrenzt die
Fan-out-Fläche strukturell.

### 6. Auditierbarkeit (ADR-0023)

Weil jeder Step ein `modelId` pinnt und Mappings reines FEEL sind, ist die **gesamte
Flow-Auswertung deterministisch nachrechenbar**. Der clio-Sink protokolliert (a) die
Einzel-Step-Decisions wie bisher (`com.temis.decision.evaluated.v1`) **plus** (b) ein
**Flow-Event** (`com.temis.flow.evaluated.v1`), das Flow-`id`/`version`, die geordnete Liste
der Step-`modelId`s sowie Flow-Ein-/Ausgabe festhält. `audit.ReAudit` bekommt einen
Flow-Replay: Event → Flow neu auswerten → kanonisch vergleichen (Folge-WP).

### 7. Oberflächen

- **Library (`package flow`)** — primär, in-process; **chrampfer** (L2b) bettet L2a hierüber
  ein, konform zu ADR-0011 (kein Netzwerk-Hop).
- **HTTP** — `POST /v1/flows` (Deskriptor registrieren/validieren → `flowId`),
  `POST /v1/flows/{id}/evaluate`, `POST /v1/flow/evaluate` (stateless inline). RFC-7807-Fehler
  (`FLOW_*`-Codes), OpenAPI-Sync.
- **MCP** — `load_flow`/`describe_flow`/`evaluate_flow`, damit ein Agent Flows komponiert und
  ausführt (Agent-First, ADR-0013).
- **Git** — Flows liegen im Repo (`flows/`, `docs/90`); `vcs` listet/lädt sie (Folge-WP).
- **Modeler** — ein visueller Flow-Editor ist **außer Scope** dieser ADR (später; die
  BPMN-Editor-Synergie aus ADR-0016 ist davon getrennt).

## Konsequenzen

**Positiv**
- L2a wird ein **wiederverwendbares, ownbares, versionierbares** Artefakt statt Ad-hoc-Glue;
  passt direkt in die föderierte Governance (`docs/90`, `flows/`-Verzeichnis + CODEOWNERS).
- **Determinismus & Re-Audit** bleiben über die ganze Komposition erhalten (ADR-0023).
- Jede DMN-Datei bleibt **autark** (kein XML-Verweben); die `dmn`-v1-Surface bleibt
  **eingefroren** (neues Package, eigene SemVer-Spur).
- **In-process einbettbar** — chrampfer nutzt L2a ohne Netzwerk-Failure-Mode (ADR-0011).
- Kein neuer Ausdrucks-Dialekt (FEEL fürs Mapping) und **keine neue Dependency** (stdlib-JSON).

**Negativ / Kosten**
- Ein zweites Artefaktformat neben DMN-XML (Deskriptor-Schema muss gepflegt/versioniert
  werden — daher `version` im Deskriptor).
- Der Flow ist **nicht** DMN-Standard; ein DMN-natives Tooling sieht ihn nicht (bewusst, um
  den Standard nicht zu dehnen — Option 1 verworfen).
- Neue Oberflächenfläche (HTTP/MCP/Git-Wrapper) mit eigenem OpenAPI-/Test-Aufwand.

**Folgeaufgaben**
- Etappe „Decision-Flow" in `docs/20-roadmap.md`: WP-90 (Kern-Package, ✅ umgesetzt),
  WP-91 (HTTP), WP-92 (MCP), WP-93 (Audit/Re-Audit), WP-94 (Git) — mit dieser ADR angelegt.
- **Mapping-Ausbau:** WP-90 liefert **referenzbasierte** Mappings (Flow-Input-Name oder
  `stepID.output`) — den in §3 genannten Normalfall. Die **vollen FEEL-Mapping-Ausdrücke**
  (`"Credit Score - 50"`) sind ein Folge-WP: sie brauchen ein öffentliches FEEL-Eval-Primitive
  in `package dmn` (eigene additive-Surface-Entscheidung, damit `flow` nicht `internal/feel`
  importiert).
- Deskriptor-Schema formal (JSON-Schema) unter `docs/` festschreiben, sobald WP-90 steht.
- `docs/90-decision-organization.md` um einen konkreten Flow-Beispielabschnitt ergänzen.
- ADR-0025 bleibt gültig; diese ADR ist ihre konkrete L2a-Umsetzung und ersetzt sie nicht.
