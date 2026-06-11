# ADR-0004: Ausführung via Compile-to-Closures (kein Tree-Walking)

- **Status:** accepted
- **Datum:** 2026-06-11
- **Kontext-WP:** WP-06

## Kontext
Ziel „sehr schnell". Auswertung passiert oft (heißer Pfad), Kompilierung selten.

## Optionen
1. **Tree-Walking-Interpreter** — einfach, aber AST-Walk + Typ-Dispatch pro Eval = langsam, allokationsreich.
2. **Compile-to-Go-Closures** — AST wird einmalig in `func(Scope)(Value,error)` übersetzt; Variablen als Slot-Indizes, Built-ins direkt gebunden.
3. **Bytecode-VM** — sehr schnell, aber deutlich komplexer; Closures erreichen ~90 % des Nutzens bei Bruchteil der Komplexität.

## Entscheidung
Compile-to-Closures. Eigener Lexer/Parser/Compiler (keine Fremd-FEEL-Lib), damit volle
Kontrolle über Performance und Spec-Konformität besteht.

## Konsequenzen
- Strikte Zweiphasigkeit Compile/Evaluate (Architektur §1).
- `CompiledExpr` ist die zentrale interne Schnittstelle; immutable, thread-safe.
- Spätere Bytecode-VM bleibt als Option offen, falls Profiling es rechtfertigt (neues ADR).
