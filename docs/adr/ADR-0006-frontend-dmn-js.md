# ADR-0006: dmn-js als Editor/Viewer, Standard-DMN-XML als Schnittstelle

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
Frontend/Editor ist dmn-js (bpmn.io). Es liest/schreibt Standard-DMN-XML inkl. DMNDI.

## Optionen
1. **Eigener Editor** — hoher Aufwand, kein Mehrwert.
2. **dmn-js unverändert** — Industriestandard, erzeugt konformes XML.

## Entscheidung
dmn-js unverändert nutzen. Integrationsvertrag = Standard-DMN-XML. Engine muss dieses XML
verlustfrei laden und round-trip-fähig serialisieren (DMNDI bewahren).

## Konsequenzen
- Kein Frontend-Code im Engine-Repo nötig (optionales Demo-Frontend = F-01, kein Produktziel).
- WP-02 muss exakt dmn-js-Exporte abdecken (Golden-Files aus echten Exporten).
