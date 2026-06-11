# ADR-0009: Projektname „Temis"

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
Das Projekt braucht einen einprägsamen, eindeutigen Namen für Repository, Go-Modulpfad,
Service-Binary und Außenkommunikation.

## Optionen
1. **Themis** — griech. Göttin von Ordnung/Recht/Urteil; thematisch perfekt, aber `th`
   tippt sich umständlich und kollidiert mit bestehenden Projekten (u. a. Krypto-Lib von
   Cossack Labs).
2. **Temis** — gleiche Anspielung, Schreibweise ohne „h": eindeutige Aussprache, sauberer
   als CLI/Binary, bessere Abgrenzung von Namensgleichen.
3. Gavel / Verdict / Krino — verworfen zugunsten der mythologischen Linie.

## Entscheidung
**Temis.** Modulpfad `github.com/pblumer/temis`,
Service-Binary `temisd`, öffentliches Go-Package bleibt `dmn` (beschreibt die Domäne, nicht
die Marke — idiomatischer Import: `import "github.com/pblumer/temis/dmn"`).

## Konsequenzen
- `cmd/temisd` als Service-Binary; Container-Image gleichnamig.
- Public-API-Package heißt weiterhin `dmn` (siehe `40-api-contract.md`) — der Markenname
  steckt im Modulpfad, nicht im Package-Identifier.
