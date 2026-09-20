# Architektur

## 1. Leitprinzip: zweiphasig (Compile / Evaluate)

Die gesamte Engine trennt strikt zwischen einer **teuren, einmaligen Compile-Phase**
und einer **billigen, oft wiederholten Evaluate-Phase**. Diese Trennung ist die Grundlage
für „sehr schnell".

```
DMN-XML  ──parse──▶  DMN-Modell  ──compile──▶  CompiledDecision  ──evaluate(ctx)──▶  Result
(I/O,     (struct,    (FEEL→Closures,         (immutable, thread-     (allokationsarm,
 selten)   1×)         Typprüfung, 1×)         safe, wiederverwendbar)  heißer Pfad)
```

- **Compile** darf gründlich sein: Parsen, Typinferenz, Validierung, FEEL-Ausdrücke in
  Go-Closures übersetzen, Decision-Graph verdrahten und auf Zyklen prüfen.
- **Evaluate** darf (idealerweise) **keine** Parserarbeit und **minimale** Allokation tun.
  Ergebnis von Compile ist unveränderlich und beliebig oft parallel nutzbar.

## 2. Paketstruktur (Go-Module)

```
temis/
├── go.mod                      // module github.com/pblumer/temis
├── Makefile                    // verify, test, bench, tck, lint
├── cmd/
│   └── temisd/                   // Service-Binary (HTTP+gRPC), dünner Wrapper
├── dmn/                        // PUBLIC: Engine-Einstieg (Compile/Evaluate-API)
│   ├── engine.go               //   Engine, Compile(), CompiledDecision
│   ├── context.go              //   Eingabe-Context / Variablen
│   └── result.go               //   Ergebnis-, Diagnose-, Fehlertypen
├── internal/
│   ├── xml/                    // DMN-XML ⇄ Modell (encoding/xml-Strukturen, 1.3/1.4/1.5)
│   │   ├── schema.go           //   Go-Structs gemäß DMN-XSD
│   │   └── decode.go           //   Namespace-tolerantes Decoding
│   ├── model/                  // versionsneutrales Domänenmodell (DRG, Decision, Table…)
│   │                          // FEEL-Front-end (Lexer→Parser→AST→Typecheck→Compiler→Closure),
│   │                          // das Wertemodell und die Built-ins liegen extern in
│   │                          // github.com/pblumer/feel (+/value, +/builtins) — extrahiert aus
│   │                          // temis, hier als Modul-Abhängigkeit konsumiert (ADR-0039).
│   ├── boxed/                  // Boxed Expressions → Compiler (nutzt feel/ + feel/value)
│   │   ├── decisiontable.go    //   Unary Tests, Hit Policies, Aggregation
│   │   ├── context.go invocation.go list.go relation.go
│   │   ├── function.go         //   Boxed Function / BKM
│   │   ├── conditional.go iterator.go filter.go   // DMN 1.4+/1.5
│   ├── drg/                    // Scaffold (leer). Die Decision-Graph-Logik (DAG, Zyklencheck,
│   │                           //   memoisierte Auswertung) liegt real in dmn/graph.go+eval.go.
│   └── tck/                    // TCK-Runner (liest offizielle .dmn + Testcases)
├── service/                    // HTTP- & gRPC-Handler (von cmd/temisd genutzt)
│   ├── http.go openapi.go openapi.yaml   // Routen, Bearer-Token, Swagger-UI/Spec
│   └── grpc.go dmn.proto
└── docs/                       // diese Planungsdokumente
```

**Regel für KI-Agenten:** `dmn/` ist die einzige öffentliche API. Alles unter `internal/`
darf sich frei ändern. `service/` und `cmd/` dürfen **nur** über `dmn/` auf die Engine zugreifen
— niemals direkt auf `internal/`. Das FEEL-Front-end (`github.com/pblumer/feel`) ist seit
ADR-0039 ein **externes** Modul; die Regel „Engine nur über `dmn/`" gilt weiterhin, ist für
das nun öffentliche FEEL-Paket aber nicht mehr compiler-erzwungen (es lag zuvor unter
`internal/`) — nur `dmn` und `internal/boxed` sollen es importieren.

> **Hinweis (WP-05, aktualisiert ADR-0039):** Das Wertemodell liegt in einem **eigenen Paket**
> (nicht im FEEL-Paket), damit Wert-Namen wie `Number`/`Kind` nicht mit den Token-Kinds des
> Lexers (`feel.Number`, `feel.Kind`) kollidieren. Seit ADR-0039 kommen FEEL-Front-end und
> Wertemodell aus dem externen Modul `github.com/pblumer/feel` bzw. `github.com/pblumer/feel/value`;
> `dmn` und `internal/boxed` importieren sie von dort.

## 3. Datenfluss im Detail

### 3.1 Parse (`internal/xml` → `internal/model`)
- Namespace-tolerant: DMN 1.3/1.4/1.5-Namespaces werden auf dasselbe interne Modell
  abgebildet. Unbekannte Elemente → Diagnose, kein harter Abbruch (Forward-Compat).
- dmn-js schreibt Standard-DMN-XML inkl. `DMNDI` (Diagramm-Layout). Layout wird
  **bewahrt** (round-trip-fähig), aber für die Ausführung ignoriert.

### 3.2 Compile (`github.com/pblumer/feel` + `internal/boxed` + `dmn/graph.go`)
- Jede Decision besitzt eine Logik-Form (Literal Expression, Decision Table, oder andere
  Boxed Expression). Diese wird in eine `CompiledExpr` übersetzt.
- `dmn/graph.go` verdrahtet aus den Information Requirements einen DAG und prüft ihn zur
  **Compile-Zeit** per DFS (3-Färbung) auf Zyklen (`DECISION_CYCLE`-Diagnostic). Es wird
  **kein** vorab materialisierter topologischer Plan erzeugt — die Reihenfolge ergibt sich zur
  Laufzeit aus der memoisierten Tiefensuche (§3.3). (`internal/drg` ist ein leeres Scaffold.)
- Typecheck wo möglich statisch (FEEL ist teils dynamisch typisiert → Rest zur Laufzeit).

### 3.3 Evaluate (`dmn/eval.go`)
- Eingabe-Context (Input Data) → Scope. Benötigte Decisions werden **rekursiv und memoisiert**
  ausgewertet (Diamond → einmal), Zwischenergebnisse fließen als Variablen weiter; ein
  Laufzeit-Guard fängt Zyklen zusätzlich ab.
- **Decision Services** (DMN-Konzept): definierter Satz Output-Decisions + Input-Decisions
  → erlauben gezielte Teilauswertung.

## 4. Zentrale interne Schnittstelle (stabilisierend)

```go
// github.com/pblumer/feel/value — das Wertemodell (eigenes Paket, s. §2-Hinweis)
type Value interface{ Kind() Kind; String() string /* … */ }  // null/bool/number/string/temporal/list/context/range/function

// github.com/pblumer/feel — Compiler & Hot-Path-Schnittstelle (WP-06)
type Scope struct{ /* vars []value.Value */ }                 // Slot-Array, keine Map im Hot Path (§5.2)
type Env struct{ /* name→Slot-Index */ }                      // Compile-Zeit-Layout; Env.NewScope(map) ist die einzige map→Slots-Grenze
type CompiledExpr func(*Scope) (value.Value, error)            // reine Go-Closure, immutable, thread-safe

// internal/boxed — jede Boxed Expression kompiliert zu genau dieser Signatur
type Compiler interface {
    Compile(node model.Expression, env *Env) (feel.CompiledExpr, []Diagnostic, error)
}
```

Diese Typen sind das Rückgrat. Performance entsteht dadurch, dass `CompiledExpr`
eine reine Go-Closure ist — kein AST-Walk, keine Reflection im Hot Path. **Variablen
sind zur Compile-Zeit auf Slot-Indizes aufgelöst** (`Scope` ist ein konkretes
Slot-Array, kein `Get(name)`-Interface — verfeinert gem. §5.2, beschlossen in WP-06).

## 5. Performance-Architektur (konkrete Regeln)

1. **Number = Festkomma-Decimal**, nicht float64. DMN/FEEL verlangt Dezimalsemantik
   (Geldbeträge!). Implementierung über `cockroachdb/apd` o.ä. — aber Pooling/Caching
   kleiner Konstanten, um Allokationen zu sparen. (Siehe ADR-0007.)
2. **Scope ohne map im Hot Path**: kompilierte Variablenzugriffe werden zu Slot-Indizes
   aufgelöst (Array-Lookup statt Map-Lookup). Map nur am Rand (externe Eingabe).
3. **CompiledDecision ist immutable** → einmal kompiliert, beliebig parallel evaluierbar
   ohne Locks.
4. **Allocation-Budget pro Evaluate** wird in Benchmarks überwacht (`-benchmem`, Ziel:
   Größenordnung niedrige zweistellige Allokationen für eine mittlere Decision Table;
   exakte Schwelle in 50-testing-strategy.md festgelegt).
5. **Built-ins als direkte Go-Funktionen** in einer Lookup-Tabelle, zur Compile-Zeit
   gebunden (kein String-Dispatch zur Laufzeit).

## 6. Fehler- und Diagnosemodell

- **Compile-Fehler** (Syntax, Typ, Zyklus) → strukturierte `Diagnostic{Severity, Message,
  Source, Line, Col, DecisionID}`. Niemals panic für Nutzerfehler.
- **Evaluate-Fehler** → FEEL kennt einen `null`-Propagationsmechanismus; viele Fehler
  führen spec-konform zu `null`, nicht zum Abbruch. Echte Laufzeitfehler (z. B. erschöpfte
  Limits) als Go-`error`.
- **Sicherheit:** Eingaben sind nicht vertrauenswürdig (Service nimmt fremde DMN entgegen).
  → Limits für Rekursionstiefe, Listengröße, Schleifeniterationen, Compile-Zeit.
  Siehe ADR-0008.

## 7. Nebenläufigkeit

- `Engine.Compile` ist re-entrant.
- `CompiledDecision.Evaluate` ist vollständig thread-safe und seiteneffektfrei.
- Kein globaler Zustand. Keine `init()`-Magie außer statischen Built-in-Tabellen.
