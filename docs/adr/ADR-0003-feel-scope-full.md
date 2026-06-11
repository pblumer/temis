# ADR-0003: Voller FEEL-Scope inkl. aller Boxed Expressions

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
Anforderung: vollständige DMN-1.5-Unterstützung. FEEL ist der aufwändigste Teil.

## Optionen
1. **FEEL-Subset** (nur Decision Tables) — schnell, aber nicht spec-vollständig.
2. **Voller FEEL** + alle Boxed Expressions — aufwändig, aber spec-konform und TCK-fähig.

## Entscheidung
Voller FEEL. Schrittweise Umsetzung: Kern im MVP, Vollständigkeit in Beta (WP-20..WP-26).

## Konsequenzen
- Hoher Testaufwand → Spec-Beispieltabellen + TCK sind Pflicht.
- Rechtfertigt eigenen Compiler (ADR-0004) statt eingeschränkter Fremdlösung.
