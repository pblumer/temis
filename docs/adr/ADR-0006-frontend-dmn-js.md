# ADR-0006: dmn-js als Editor/Viewer, Standard-DMN-XML als Schnittstelle

- **Status:** superseded by ADR-0016
- **Datum:** 2026-06-11

> **Überholt durch ADR-0016:** Statt dmn-js unverändert einzubetten, baut temis einen eigenen
> DMN-Modeler auf einem Fork des MIT-Kerns (diagram-js/table-js/dmn-moddle) — u. a. für
> 1.5-Authoring und ohne bpmn.io-Logo-Pflicht. Der Integrationsvertrag *Standard-DMN-XML*
> bleibt gültig.

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
