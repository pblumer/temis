# ADR-0001: Implementierungssprache Go

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
Engine soll sehr schnell, gut einbettbar und einfach als Service deploybar sein.

## Optionen
1. **Go** — schnelle Kompilate, einfache statische Binaries, gute Concurrency, leichtes Deployment, große Verbreitung in BPM/Cloud-Umfeld.
2. **Rust** — maximale Performance/Kontrolle, aber höhere Implementierungskomplexität und langsamere Iteration für KI-Agenten.
3. **JVM (Kotlin/Java)** — Nähe zu existierenden DMN-Engines, aber schwergewichtiger Betrieb.

## Entscheidung
Go (≥ 1.23). Bestes Verhältnis aus Performance, Einbettbarkeit, Deployment-Einfachheit und Agenten-Produktivität.

## Konsequenzen
- Decimal-Arithmetik braucht externe Lib (siehe ADR-0007), da kein nativer Decimal-Typ.
- GC vorhanden → Hot Path muss allokationsarm gebaut werden (Architektur §5).
