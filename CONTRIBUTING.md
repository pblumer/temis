# Mitwirken an Temis

Temis wird überwiegend von KI-Coding-Agenten entlang klar geschnittener
Arbeitspakete (WPs) gebaut. Die verbindlichen Arbeitsregeln stehen in
**[`docs/60-ai-agent-guide.md`](docs/60-ai-agent-guide.md)** — dieses Dokument
ist die Kurzfassung für Menschen wie Agenten.

## Bevor du Code schreibst

Lies in dieser Reihenfolge:

1. [`docs/00-overview.md`](docs/00-overview.md) — Ziele, Nicht-Ziele, Rahmenentscheidungen
2. [`docs/10-architecture.md`](docs/10-architecture.md) — Aufbau der Pakete
3. [`docs/60-ai-agent-guide.md`](docs/60-ai-agent-guide.md) — Arbeitsschleife & goldene Regeln
4. die relevanten ADRs unter [`docs/adr/`](docs/adr/); für FEEL zusätzlich
   [`docs/30-feel-spec.md`](docs/30-feel-spec.md), für die API
   [`docs/40-api-contract.md`](docs/40-api-contract.md)

## Arbeitsschleife (Kurzform)

1. **WP wählen:** das oberste offene Arbeitspaket aus
   [`docs/20-roadmap.md`](docs/20-roadmap.md), dessen Abhängigkeiten `done` sind.
2. **Tests zuerst:** die Teststrategie ist verbindlich
   ([`docs/50-testing-strategy.md`](docs/50-testing-strategy.md)).
3. **Implementieren**, dann `make verify` (formatting, vet, lint, race-Tests,
   bench, budget, tck) **grün** halten — plus `make cover` für die
   korrektheitskritischen Pakete. Frontend: `make web-check`.
4. **Kleine, fokussierte Commits** mit aussagekräftiger Message; ein WP je PR,
   wo möglich.

## Goldene Regeln (Auszug)

- **Korrektheit schlägt Geschwindigkeit.** Die DMN-Spec und das TCK sind die
  oberste Autorität.
- **Keine neuen Abhängigkeiten** ohne ADR — der Kern ist reine Standardbibliothek
  plus wenige, bewusst gewählte Module (siehe `go.mod`).
- **`package dmn` ist als v1 zugesagt** (ADR-0019). Änderungen an der
  exportierten Oberfläche brechen den Golden-Surface-Test bewusst; nur mit
  `-update-api` und im Rahmen der SemVer-/Deprecation-Policy.
- **`internal/` bleibt frei beweglich.**

## Pull Requests

- CI muss grün sein (`verify`, `security`, `docker`, `web`, `web-e2e`, `proto`).
- Beschreibe **was** sich ändert und **warum**; verweise auf das WP bzw. den
  ADR. Für Frontend-Änderungen den neu gebauten `web/dist` mitcommitten (der
  Drift-Check erzwingt das).

## Sicherheitslücken

Nicht als öffentliches Issue — siehe [`SECURITY.md`](SECURITY.md).

## Lizenz

Mit einem Beitrag stimmst du zu, dass er unter der Projekt-Lizenz
([Apache-2.0](LICENSE)) veröffentlicht wird.
