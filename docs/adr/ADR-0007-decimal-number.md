# ADR-0007: FEEL-Number als Decimal (nicht float64)

- **Status:** accepted
- **Datum:** 2026-06-11
- **Kontext-WP:** WP-05

## Kontext
FEEL/DMN definiert Number mit Dezimalsemantik (34 signifikante Stellen, definierte Rundung).
float64 verletzt das (z. B. 0.1+0.2) und ist für Geldbeträge unbrauchbar.

## Optionen
1. **float64** — schnell, aber spec-falsch → TCK-Fehler. Ausgeschlossen.
2. **Decimal-Lib** (z. B. cockroachdb/apd) — spec-konform, etwas langsamer/allokationsreicher.
3. **Eigener Decimal-Typ** — volle Kontrolle, aber hoher Aufwand/Fehlerrisiko.

## Entscheidung
Etablierte Decimal-Library mit konfigurierbarer Präzision/Rundung gemäß Spec. Kleine
Konstanten cachen/poolen, um Allokationen im Hot Path zu reduzieren.

## Konsequenzen
- Go⇄FEEL-Mapping konvertiert numerische Eingaben zu Decimal; float64-Eingaben mit
  dokumentiertem Genauigkeitsvorbehalt (API-Contract §1.5).
- Service-Transport überträgt Zahlen als String, um JSON/proto-float-Verlust zu vermeiden.
- Decimal-Lib ist eine bewusst zugelassene externe Abhängigkeit (ADR-pflichtig erfüllt).
