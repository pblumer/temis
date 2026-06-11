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
  Go-Closures übersetzen, Decision-Graph topologisch sortieren.
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
│   ├── feel/                   // FEEL: Lexer→Parser→AST→Typecheck→Compiler→Closure
│   │   ├── token.go lexer.go
│   │   ├── ast.go parser.go
│   │   ├── types.go            //   FEEL-Typsystem
│   │   ├── typecheck.go
│   │   ├── compile.go          //   AST → CompiledExpr (func(*Scope) Value)
│   │   ├── value.go            //   FEEL-Werte (number=big.Decimal, date/time, list, context…)
│   │   ├── builtins/           //   alle FEEL-Built-in-Funktionen, je Kategorie 1 Datei
│   │   └── scope.go            //   Variablenauflösung zur Laufzeit
│   ├── boxed/                  // Boxed Expressions → Compiler (nutzt feel/)
│   │   ├── decisiontable.go    //   Unary Tests, Hit Policies, Aggregation
│   │   ├── context.go invocation.go list.go relation.go
│   │   ├── function.go         //   Boxed Function / BKM
│   │   ├── conditional.go iterator.go filter.go   // DMN 1.4+/1.5
│   ├── drg/                    // Decision-Graph: Topo-Sort, Dependency-Resolution, Eval-Plan
│   └── tck/                    // TCK-Runner (liest offizielle .dmn + Testcases)
├── service/                    // HTTP- & gRPC-Handler (von cmd/temisd genutzt)
│   ├── http.go openapi.yaml
│   └── grpc.go dmn.proto
└── docs/                       // diese Planungsdokumente
```

**Regel für KI-Agenten:** `dmn/` ist die einzige öffentliche API. Alles unter `internal/`
darf sich frei ändern. `service/` und `cmd/` dürfen **nur** über `dmn/` auf die Engine zugreifen
— niemals direkt auf `internal/`.

## 3. Datenfluss im Detail

### 3.1 Parse (`internal/xml` → `internal/model`)
- Namespace-tolerant: DMN 1.3/1.4/1.5-Namespaces werden auf dasselbe interne Modell
  abgebildet. Unbekannte Elemente → Diagnose, kein harter Abbruch (Forward-Compat).
- dmn-js schreibt Standard-DMN-XML inkl. `DMNDI` (Diagramm-Layout). Layout wird
  **bewahrt** (round-trip-fähig), aber für die Ausführung ignoriert.

### 3.2 Compile (`internal/feel` + `internal/boxed` + `internal/drg`)
- Jede Decision besitzt eine Logik-Form (Literal Expression, Decision Table, oder andere
  Boxed Expression). Diese wird in eine `CompiledExpr` übersetzt.
- `drg/` baut aus den Information Requirements einen DAG, prüft auf Zyklen, erzeugt eine
  topologische Auswertungsreihenfolge (Eval-Plan).
- Typecheck wo möglich statisch (FEEL ist teils dynamisch typisiert → Rest zur Laufzeit).

### 3.3 Evaluate (`dmn` + `internal/drg`)
- Eingabe-Context (Input Data) → Scope. Decisions werden in Eval-Plan-Reihenfolge
  ausgeführt, Zwischenergebnisse fließen als Variablen weiter.
- **Decision Services** (DMN-Konzept): definierter Satz Output-Decisions + Input-Decisions
  → erlauben gezielte Teilauswertung.

## 4. Zentrale interne Schnittstelle (stabilisierend)

```go
// internal/feel
type Value any            // konkret: nil, bool, *Number, string, time-typen, []Value, *Context, *Function
type Scope interface {    // Variablenauflösung zur Laufzeit
    Get(name string) (Value, bool)
}
type CompiledExpr func(s Scope) (Value, error)   // das ist der "kompilierte" FEEL-Ausdruck

// internal/boxed — jede Boxed Expression kompiliert zu genau dieser Signatur
type Compiler interface {
    Compile(node model.Expression, env *TypeEnv) (feel.CompiledExpr, []Diagnostic, error)
}
```

Diese drei Typen sind das Rückgrat. Performance entsteht dadurch, dass `CompiledExpr`
eine reine Go-Closure ist — kein AST-Walk, keine Reflection im Hot Path.

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
