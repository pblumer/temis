# Temis — DMN-Engine (Go)

**Repository:** https://github.com/pblumer/temis

> **GitHub-Beschreibung (About):**
> Fast, embeddable DMN 1.5 decision engine in Go with full FEEL support — usable as a library or HTTP/gRPC service.
>
> **Topics:** `dmn` · `dmn-engine` · `feel` · `decision-engine` · `business-rules` · `golang` · `dmn-js` · `decision-tables` · `rules-engine`

**Temis** ist eine schnelle, eingebettete **DMN-1.5**-Engine in Go mit vollem **FEEL**-Support,
betreibbar als **Library** und **HTTP/gRPC-Service**. Modelle werden im **dmn-js**-Editor erstellt
und als Standard-DMN-XML geladen.

> Der Name spielt auf *Themis* an, die griechische Göttin der Ordnung, Gerechtigkeit und
> des Urteils — passend zu einer Engine, die Entscheidungen trifft. Schreibweise bewusst
> ohne „h": eindeutige Aussprache, sauberer als Binary-/Modulname.

## Status

**Aktiv in Entwicklung.** Das Fundament der Engine steht; das MVP (lauffähige Library, die
reale dmn-js-Dateien auswertet) wird entlang der Arbeitspakete in `docs/20-roadmap.md` gebaut.
Jedes Arbeitspaket landet als eigener, CI-grüner Pull Request (`make verify`: fmt, vet, lint,
`-race`, Benchmarks).

| Arbeitspaket | Inhalt | Stand |
|---|---|---|
| WP-01 | Projektgerüst, Makefile, CI | ✅ |
| WP-02 | DMN-XML-Decoding (1.5, tolerant 1.3/1.4) → Modell, `DMNDI`-Round-trip | ✅ |
| WP-03 | FEEL-Lexer | ✅ |
| WP-04 | FEEL-Parser → AST | ✅ |
| WP-05 | FEEL-Wertemodell, Number als Decimal (`apd`), Temporaltypen | ✅ |
| WP-06 | FEEL-Compiler-Kern (AST → Closure, Slot-Index-Variablen) | ✅ |
| WP-07 | FEEL-Built-ins (Kern) | 🚧 in Review |
| WP-08 | Unary Tests | offen |
| WP-09 | Decision-Table-Compiler + Hit Policies U/A/F/R/C | offen |
| WP-10 | Öffentliche Library-API (`dmn.Engine`, Compile/Evaluate) | offen |
| WP-11 | MVP-Beispiele & Golden-Tests | offen |

> Die **öffentliche `dmn/`-API** entsteht in WP-10. Bis dahin liegt die gesamte Logik unter
> `internal/` und kann sich frei ändern. Die maßgebliche, fortlaufend gepflegte Statusquelle
> ist `docs/20-roadmap.md`.

### Was heute funktioniert

- **DMN-XML laden:** namespace-tolerantes Decoding (1.3/1.4/1.5) in ein versionsneutrales
  Modell; das `DMNDI`-Diagramm-Layout übersteht einen Round-trip.
- **FEEL auswerten (intern):** Lexer → Parser → Compiler liefert eine `CompiledExpr`-Closure.
  Unterstützt u. a. Arithmetik (Decimal, `0.1 + 0.2 = 0.3`), Vergleiche, dreiwertige
  Boolesche Logik, `if`, `between`/`in`, Listen/Contexts/Ranges, Pfadzugriff, `@`-Temporal­literale
  und Funktionsaufrufe gegen die Built-in-Registry — alles mit FEEL-`null`-Propagation.

## Entwicklung

Voraussetzung: **Go ≥ 1.23**.

```sh
go test ./...      # alle Tests
make verify        # fmt-check, vet, lint, test -race, bench-smoke, tck  (CI-Gate)
make help          # alle Make-Targets
```

### Projektstruktur (Auszug)

```
dmn/                 # öffentliche API (entsteht in WP-10)
internal/
  xml/               # DMN-XML ⇄ Modell (namespace-tolerant)
  model/             # versionsneutrales Domänenmodell
  value/             # FEEL-Wertemodell (Decimal-Number, Temporaltypen, …)
  feel/              # FEEL: Lexer, Parser/AST, Compiler, builtins/
  …                  # boxed/, drg/, tck/ folgen gemäß Roadmap
docs/                # Planung, Architektur, ADRs (Single Source of Truth)
```

## Dokumentation

| Datei | Inhalt |
|---|---|
| `docs/00-overview.md` | Projekt-Charter, harte Entscheidungen, Glossar |
| `docs/10-architecture.md` | Paketstruktur, Compile/Evaluate-Pipeline, interne Schnittstellen |
| `docs/20-roadmap.md` | MVP / Beta / 1.0 mit Arbeitspaketen & Akzeptanzkriterien **(Live-Status)** |
| `docs/30-feel-spec.md` | FEEL-Bauplan (Grammatik, Typen, Built-ins) |
| `docs/40-api-contract.md` | stabile Go- + HTTP/gRPC-API |
| `docs/50-testing-strategy.md` | Test-Pyramide, TCK, Benchmarks |
| `docs/60-ai-agent-guide.md` | Arbeitsregeln für KI-Coding-Agenten |
| `docs/adr/` | Architecture Decision Records |

## Mitwirken

Die Implementierung erfolgt durch einen KI-Coding-Agenten entlang der Arbeitspakete. Wer Code
beiträgt, liest zuerst `docs/00-overview.md`, `docs/10-architecture.md` und
`docs/60-ai-agent-guide.md`, wählt das nächste offene Arbeitspaket aus `docs/20-roadmap.md`,
schreibt Tests zuerst und hält `make verify` grün.

## Lizenz

Siehe [LICENSE](LICENSE).
