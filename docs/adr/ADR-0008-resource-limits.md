# ADR-0008: Ressourcenlimits & Sandboxing für nicht vertrauenswürdige Eingaben

- **Status:** accepted
- **Datum:** 2026-06-11
- **Kontext-WP:** WP-34

## Kontext
Der Service nimmt fremdes DMN-XML + fremde FEEL-Ausdrücke entgegen. FEEL erlaubt
Rekursion, Comprehensions, große Listen → DoS-Risiko (CPU, Speicher, Endlosschleifen).

## Optionen
1. **Keine Limits** — einfach, aber unsicher. Ausgeschlossen für Service.
2. **Konfigurierbare harte Limits** — Rekursionstiefe, Iterationsanzahl, Listengröße,
   Compile-Timeout, Eval-Timeout (via context.Context).

## Entscheidung
Konfigurierbare harte Limits (`WithLimits`), per Default sicher gesetzt. Überschreitung →
sauberer Fehler (kein Crash). `context.Context` erzwingt Timeouts/Cancel in Compile & Eval.

## Konsequenzen
- `Limits`-Typ in der Public API; Service setzt strengere Defaults als die Bibliothek.
- Fuzzing (WP-44) verifiziert, dass Limits nicht umgehbar sind.
- Built-ins/Comprehensions/Rekursion müssen Limits kooperativ prüfen.
