# Temis — DMN Engine in Go · Projektübersicht

> **Zweck dieses Dokuments:** Single Source of Truth für Menschen *und* KI-Coding-Agenten
> (Claude Code o.ä.). Alle anderen Dokumente hängen hier dran. Wenn ein Agent nur *ein*
> Dokument liest, ist es dieses.
>
> **Projektname:** **Temis** (Modulpfad `github.com/pblumer/temis`, Service-Binary `temisd`).
> Repository: https://github.com/pblumer/temis · Anspielung auf *Themis*, die griechische
> Göttin von Ordnung und Urteil — ohne „h" für
> eindeutige Aussprache und saubere Tool-Namen.

## 1. Vision (ein Satz)

**Temis** ist eine eingebettete, sehr schnelle DMN-1.5-Engine in Go mit vollständigem
FEEL-Support, betreibbar als Go-Library **und** als HTTP/gRPC-Service, deren Modelle im
[dmn-js](https://bpmn.io/toolkit/dmn-js/)-Editor erstellt und visualisiert werden.

## 2. Harte Rahmenentscheidungen (NICHT ohne ADR ändern)

| # | Entscheidung | Wert | ADR |
|---|---|---|---|
| D1 | Sprache | Go (≥ 1.23) | ADR-0001 |
| D2 | DMN-Zielversion | 1.5 (formal OMG), abwärtskompatibel 1.3/1.4 lesen | ADR-0002 |
| D3 | FEEL-Scope | **Voll** (alle Boxed Expressions, volle Grammatik) | ADR-0003 |
| D4 | Ausführungsmodell | **Compile-to-Closures** (kein Tree-Walking im Hot Path) | ADR-0004 |
| D5 | Betriebsform | **Library-first**, Service als dünner Wrapper | ADR-0005 |
| D6 | Editor/Frontend | dmn-js (read/write Standard-DMN-XML) | ADR-0006 |
| D7 | Keine externen FEEL-Abhängigkeiten | eigener Lexer/Parser/Compiler | ADR-0004 |
| D8 | Zielgruppe Agenten | **Agent-First**: temis als Laufzeit-Verifikationswerkzeug (MCP, Entscheidungsspur, Agent-Schema) | ADR-0012 |

## 3. Nicht-Ziele (explizit ausgeschlossen, verhindert Scope-Creep)

- **Kein** eigener Web-Editor — dmn-js wird unverändert eingebunden.
- **Kein** Decision-Management-UI (Versionierung, Deployment-Konsole) im MVP.
- **Kein** PMML/ONNX-Import (DMN 1.5 erlaubt es; bewusst Post-1.0).
- **Kein** verteilter Cluster-Betrieb / kein eingebauter Persistenzlayer im MVP.
- **Keine** BPMN/CMMN-Integration (nur DMN).

## 4. Qualitätsziele (priorisiert)

1. **Korrektheit** — TCK-Konformität (DMN Technology Compatibility Kit) ist das
   oberste Maß. Eine schnelle, falsche Engine ist wertlos.
2. **Performance** — Evaluierung im einstelligen Mikrosekundenbereich für typische
   Decision Tables; Compile einmalig, Evaluate allokationsarm.
3. **Einbettbarkeit** — saubere, kleine Go-API; keine globalen Zustände; thread-safe.
4. **Wartbarkeit durch KI** — kleine Pakete, klare Grenzen, hohe Testabdeckung,
   sprechende Fehler.

## 5. Glossar (für eindeutige Agenten-Kommunikation)

| Begriff | Bedeutung |
|---|---|
| DRD | Decision Requirements Diagram — Graph aus Decisions/Inputs/BKM |
| DRG | Decision Requirements Graph — das ausführbare Modell hinter dem DRD |
| FEEL | Friendly Enough Expression Language — Ausdruckssprache der DMN |
| BKM | Business Knowledge Model — wiederverwendbare Logik (Funktion) |
| Boxed Expression | Tabellarische FEEL-Darstellung (Decision Table, Context, Invocation, List, Function, Relation, Conditional, Iterator, Filter) |
| Hit Policy | Regel zur Auswahl/Aggregation von Treffern in einer Decision Table (U,A,P,F,R,O,C,C+,C<,C>,C#) |
| TCK | DMN Technology Compatibility Kit — offizielle Konformitäts-Testsuite |
| Unary Test | Eingabe-Bedingung in einer Decision-Table-Zelle (z. B. `< 18`, `[1..10]`) |

## 6. Etappen (Details in 20-roadmap.md)

- **MVP** — Parser + Decision Tables + FEEL-Kern + Library-API. Lädt reale dmn-js-Dateien.
- **Beta** — Voller FEEL, alle Boxed Expressions, DRD-Verkettung, Service-Wrapper.
- **1.0** — TCK grün (Zielquote definiert in 20-roadmap.md), Performance-Budget erfüllt,
  stabile API, Doku.

## 7. Dokumentenkarte

| Datei | Inhalt |
|---|---|
| `00-overview.md` | *dieses Dokument* |
| `10-architecture.md` | Paketstruktur, Datenfluss, Schnittstellen, Pipeline |
| `20-roadmap.md` | MVP / Beta / 1.0 mit Akzeptanzkriterien & Arbeitspaketen |
| `30-feel-spec.md` | FEEL-Implementierungsplan (Grammatik, Typen, Built-ins) |
| `40-api-contract.md` | Go-API + HTTP/gRPC-Vertrag (stabil zu haltende Oberfläche) |
| `50-testing-strategy.md` | Test-Pyramide, TCK-Anbindung, Benchmarks, Fuzzing |
| `60-ai-agent-guide.md` | Arbeitsregeln für KI-Coding-Agenten (Konventionen, Definition of Done) |
| `adr/ADR-XXXX-*.md` | Architecture Decision Records |

## 8. Wie ein KI-Agent dieses Projekt bearbeitet (Kurzfassung)

1. Lies `00`, `10`, `60` vollständig. Lies das ADR-Verzeichnis.
2. Wähle das **nächste offene Arbeitspaket** aus `20-roadmap.md` (oberstes mit erfüllten
   Abhängigkeiten).
3. Schreibe **Tests zuerst** gegen das Akzeptanzkriterium des Pakets.
4. Implementiere bis Tests grün + `make verify` (lint, vet, race, bench-smoke) sauber.
5. Aktualisiere den Status im Arbeitspaket und ergänze ggf. ein ADR bei
   Architektur-relevanten Entscheidungen.
