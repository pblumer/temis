# Temis — DMN Engine (Go) · Planungspaket

**Repository:** https://github.com/pblumer/temis

> **GitHub-Beschreibung (About):**
> Fast, embeddable DMN 1.5 decision engine in Go with full FEEL support — usable as a library or HTTP/gRPC service.
>
> **Topics:** `dmn` · `dmn-engine` · `feel` · `decision-engine` · `business-rules` · `golang` · `dmn-js` · `decision-tables` · `rules-engine`

**Temis** ist eine schnelle, eingebettete **DMN-1.5**-Engine in Go mit vollem **FEEL**-Support,
betreibbar als **Library** und **HTTP/gRPC-Service**. Modelle werden in **dmn-js** erstellt.

> Der Name spielt auf *Themis* an, die griechische Göttin der Ordnung, Gerechtigkeit und
> des Urteils — passend zu einer Engine, die Entscheidungen trifft. Schreibweise bewusst
> ohne „h": eindeutige Aussprache, sauberer als Binary-/Modulname.

Dieses Repository enthält (zunächst) die **Planung**. Die Implementierung erfolgt durch
einen KI-Coding-Agenten (Claude Code o. ä.) entlang der Arbeitspakete.

## Einstieg

1. `docs/00-overview.md` — Projekt-Charter, harte Entscheidungen, Glossar.
2. `docs/10-architecture.md` — Paketstruktur, Compile/Evaluate-Pipeline.
3. `docs/20-roadmap.md` — MVP / Beta / 1.0 mit Arbeitspaketen & Akzeptanzkriterien.
4. `docs/30-feel-spec.md` — FEEL-Bauplan.
5. `docs/40-api-contract.md` — stabile Go- + Service-API.
6. `docs/50-testing-strategy.md` — Tests, TCK, Benchmarks.
7. `docs/60-ai-agent-guide.md` — **Arbeitsregeln für den KI-Agenten** (zuerst lesen, wenn du Code schreibst).
8. `docs/adr/` — Architecture Decision Records.

## Nächster Schritt für den Agenten

Beginne mit **WP-01** (Projektgerüst) aus `docs/20-roadmap.md` und folge der Arbeitsschleife
in `docs/60-ai-agent-guide.md`.
