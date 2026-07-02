# ADR-0032: Flow-Registry — beim Start geladene Verzeichnis-Quelle statt flüchtigem POST-Store

- **Status:** accepted
- **Datum:** 2026-07-02
- **Kontext-WP:** WP-100 (Flow-Store beim Start laden); verfeinert ADR-0026 (Decision-Flow-Deskriptor), spiegelt ADR-0027 (Filesystem-Model-Store)

## Kontext

Mit ADR-0026 wurde der Decision-Flow (L2a) als externes JSON-Artefakt eingeführt und
über HTTP (`POST /v1/flows`), MCP und Git (`git_load_flow`) zugänglich gemacht. Die
Registrierung landet in einem **flüchtigen In-Memory-Store** (`flowStore`, eine
mutex-geschützte Map): **leer beim Start, ohne Disk-Load, ohne Persistenz.** Für Modelle
existiert dieses Muster längst — `WithModelStore`/`-models-dir` lädt beim Start ein
Verzeichnis (ADR-0027), `WithExamples` seedet Demos —, für Flows gab es nichts
Vergleichbares.

Konsequenz: Nach jedem Neustart ist der Flow-Katalog (Flow Studio, WP-97) leer, und ein
produktiver Betrieb müsste sich Flows über einen expliziten Ladeschritt jedes Mal neu
beschaffen. Das ist inkonsistent mit der föderierten Governance (`docs/90`), in der Git
+ CODEOWNERS die Quelle der Wahrheit sind — der Server bildet diese Quelle aber nicht ab.

## Optionen

1. **Flüchtige Registrierung, Git als einzige durable Quelle (Status quo).** `POST` bleibt
   Dev-Pfad; produktiv per `git_load_flow` bei Bedarf. — Minimal, „pure", aber: Neustart
   leert Katalog, expliziter Ladeschritt nötig, Empty-State-UX bleibt schwach, kein „das
   sind unsere Flows" out-of-the-box.

2. **Beim Start geladene Verzeichnis-Quelle (`-flows-dir`), spiegelt `-models-dir`** (diese
   Entscheidung). Ein `WithFlowStore(dir)` scannt beim Start `*.flow.json`, kompiliert und
   registriert alle. `POST` bleibt für Dev/interaktiv. — Katalog gefüllt & neustartfest;
   idiomatisch (spiegelt das Modell-Bootstrapping); **Git/Verzeichnis bleibt Source of
   Truth ohne server-seitige Mutation → keine Drift**. Kosten: eine Config-Fläche + ein
   Startup-Scan.

3. **Write-Through: `POST` schreibt auf Disk, Reload beim Start.** — `POST` würde durable,
   aber führt einen **mutierenden server-seitigen Schreibpfad** und eine **zweite Source of
   Truth** (lokale Disk vs. Git) ein → **Drift**, genau die Gefahrenklasse aus `docs/90` §3.
   Umgeht Git-Review/CODEOWNERS/compile-before-write (`git_propose`). **Verworfen.**

## Entscheidung

**Option 2.** Ein neues `service.WithFlowStore(dir)` (`-flows-dir` / `TEMIS_FLOWS_DIR`)
lädt beim Konstruieren des Servers alle `*.flow.json`-Deskriptoren aus `dir` in den
Flow-Katalog — **nachdem** die Modelle geladen sind (Examples, dann Model-Store), damit die
Flow-Validierung gegen die vorhandenen Modelle greift.

### Semantik

- **Read-only.** Das Verzeichnis ist die Quelle der Wahrheit; der Server schreibt **nie**
  zurück (Gegensatz zu `WithModelStore`, das Uploads persistiert). Änderungen an Flows
  laufen über Git + `git_propose` (compile-before-write), nicht über einen Server-Schreibpfad.
- **Fail-open beim Laden.** Ein Deskriptor, der nicht kompiliert, wird geloggt und
  **übersprungen** — er blockiert den Start nie und bleibt auf Disk, sodass ein späterer Fix
  ihn wiederherstellt. Ein fehlendes Verzeichnis deaktiviert den Store (geloggt), ohne den
  Start zu blockieren.
- **Diagnostics statt Fehler.** Ein Flow, dessen referenzierte Modelle (noch) nicht geladen
  sind, **registriert trotzdem** und trägt Validierungs-Diagnostics (wie `POST /v1/flows`).
  So ist der Katalog robust gegen Modell-/Flow-Ladereihenfolge und Teil-Deployments.
- **Content-addressed, idempotent.** Die `flowId` bleibt der Content-Hash des Deskriptors
  (wie bei `POST`); dasselbe File ergibt dieselbe id.
- **`POST /v1/flows` bleibt** der flüchtige Dev-/Interaktiv-Pfad (Flow Studio, Experimente) —
  unverändert, nicht persistiert.

## Konsequenzen

**Positiv**
- Der Flow-Katalog ist **neustartfest & gefüllt**; das Flow Studio zeigt die Org-Flows sofort.
- **Keine zweite, mutierbare Quelle** — Git/Verzeichnis bleibt Source of Truth, keine Drift;
  passt in die föderierte Governance (`docs/90`, `flows/` + CODEOWNERS).
- **Idiomatisch** — spiegelt `-models-dir`/ADR-0027; ein Betreiber kennt das Muster bereits.
- Rein additiv: ohne `-flows-dir` ist das Verhalten byte-identisch zu vorher.

**Negativ / Kosten**
- Eine neue Config-Fläche (`-flows-dir`) und ein Startup-Scan.
- Modelle und Flows müssen zusammen bereitgestellt werden (`-models-dir` + `-flows-dir`),
  sonst tragen Flows beim Start Diagnostics (bewusst nicht fatal).

**Folgeaufgaben**
- WP-100 in `docs/20-roadmap.md`: `WithFlowStore` + `-flows-dir`, Test, Doku (mit dieser ADR).
- `docs/90-decision-organization.md` §5 um den Lade-/Deploy-Weg ergänzt.
- Später (separat): ob das Flow Studio ein „aus Git laden" direkt anbietet (`git_load_flow`
  über die UI) — außerhalb dieser ADR.
