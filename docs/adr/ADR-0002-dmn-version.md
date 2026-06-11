# ADR-0002: Ziel-DMN-Version 1.5 (lesend 1.3/1.4)

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
DMN existiert in 1.3/1.4/1.5 (formal) und 1.6 (Beta). dmn-js erzeugt Standard-DMN-XML.
Bestehende Modelle können ältere Namespaces tragen.

## Optionen
1. **Nur 1.5** — sauber, aber bricht ältere Dateien.
2. **1.5 als Ziel, 1.3/1.4 tolerant lesen** — maximale Kompatibilität, geringe Mehrkosten.
3. **Direkt 1.6** — Beta, instabil, Tooling/TCK-Reife geringer.

## Entscheidung
Ziel ist 1.5. Der XML-Decoder liest 1.3/1.4-Namespaces tolerant auf dasselbe interne Modell.
1.6-spezifische Features sind Post-1.0.

## Konsequenzen
- `internal/xml` ist namespace-tolerant; unbekannte Elemente → Diagnose statt Abbruch.
- Round-trip bewahrt das Original-XML/DMNDI so weit wie möglich.
