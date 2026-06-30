# FEEL-WASM-Spike (ADR-0016, Gate 2)

Beweist den Kern-Mehrwert des eigenen Modelers: **FEEL-Zellen werden live gegen
die echte temis-Engine validiert** — derselbe Parser/Compiler, der das Modell
später auswertet — kompiliert nach WebAssembly, **im Browser, offline, ohne
Server-Roundtrip**. Genau das kann dmn-js prinzipiell nicht.

## Bauen & starten

```sh
./web/feel-spike/build.sh        # erzeugt feel.wasm + wasm_exec.js (git-ignored)
go run ./cmd/feel-spike-serve    # statischer Server auf :8090
# Browser: http://localhost:8090
```

Oder per Make: `make feel-spike` (Build), dann `go run ./cmd/feel-spike-serve`.

## Was die Seite zeigt

- **Eingabe-Zelle** (Unary Test, z. B. `> 10`, `[1..5]`, `"Winter", "Spring"`) —
  validiert über `temisFeelValidateUnary`.
- **Ausgabe-Zelle** (FEEL-Ausdruck, z. B. `Guest Count * 2`,
  `if Season = "Winter" then "Stew" else "Salad"`) — validiert über
  `temisFeelValidate`. Erkennt auch **unbekannte Variablen** (probier
  `Guests * 2`) mit `line:col`-Diagnose.
- Die **Decision-Inputs** oben (kommagetrennt) simulieren das Eingabe-Schema
  einer Decision (wie aus `describe_decision`).

## Exponierte WASM-API (`cmd/feel-wasm`)

```js
window.temisFeelValidate(expr, inputNamesCsv)       // → { ok } | { ok:false, line, col, message }
window.temisFeelValidateUnary(test, inputNamesCsv)  // dito, für Eingabe-Zellen
```

## Erkenntnis / nächste Schritte

Dieser Spike validiert nur die **Architektur** (Go→WASM, synchrone
Per-Tastendruck-Validierung, `line:col`-Diagnostics). Er ist **kein** Editor.
Folgeschritte gemäß ADR-0016: Client-Modell + 1.5-XML-Round-Trip, Command-Stack,
Decision-Table-Editor (der diese Validierung pro Zelle nutzt), dann DRD-Canvas.
