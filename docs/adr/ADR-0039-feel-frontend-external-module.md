# ADR-0039: FEEL-Front-end als externes Modul (`github.com/pblumer/feel`)

- **Status:** accepted
- **Datum:** 2026-07-22
- **Kontext-WP:** Feel-Engine-Integration; berührt ADR-0003 (FEEL-Scope), ADR-0005
  (Paketgrenzen), ADR-0007 (Decimal), ADR-0029 (`dmn.CompileExpression`)

## Kontext

Das FEEL-Front-end — Lexer, Parser, AST, Typsystem/Typecheck, Compiler zu
allokations-leichten Go-Closures — lag bisher unter `internal/feel`, das Wertemodell
unter `internal/value`, die Built-ins unter `internal/feel/builtins`. Diese Pakete sind
in sich geschlossen (nur `apd/v3` als externe Abhängigkeit) und **nicht** temis-spezifisch:
FEEL ist eine allgemeine Ausdruckssprache, die auch außerhalb einer DMN-Engine nützlich ist.

Genau dafür wurde das Front-end nach **`github.com/pblumer/feel`** extrahiert (Pakete
`feel`, `feel/value`, `feel/builtins`) — ein eigenständig nutzbares, Apache-2.0-lizenziertes
Modul. Zum Extraktionszeitpunkt war der Quellcode **byte-identisch** zu temis' internen
Kopien (nur die Import-Pfade unterscheiden sich).

Damit gab es die Engine an **zwei** Stellen: einmal intern in temis, einmal im
Standalone-Repo. Ohne Sync-Vereinbarung driften beide zwangsläufig auseinander. Diese
Entscheidung löst die Doppelpflege auf.

## Optionen

1. **temis konsumiert `github.com/pblumer/feel` als Modul-Abhängigkeit** (diese Entscheidung).
   `internal/feel` und `internal/value` werden gelöscht; `dmn` und `internal/boxed`
   importieren das externe Modul. Eine Quelle der Wahrheit.

2. **Beide Kopien behalten, manuell synchron halten.** — Dauerhafte, fehleranfällige
   Doppelpflege (jeder FEEL-Fix an zwei Orten). Verworfen.

3. **Extraktion rückgängig machen, `pblumer/feel` verwerfen.** — Verwirft den Nutzen der
   eigenständigen Wiederverwendung, die der Grund für die Extraktion war. Verworfen.

## Entscheidung

**Option 1.**

- `go.mod` bekommt `require github.com/pblumer/feel <pseudo-version>` (keine transitive
  Neu-Abhängigkeit außer dem bereits vorhandenen `apd/v3`).
- Import-Umschreibung (reine Pfad-Ersetzung, keine Bezeichner-Änderung, da Paketnamen
  `feel`/`value`/`builtins` gleich bleiben):
  - `internal/feel/builtins` → `github.com/pblumer/feel/builtins`
  - `internal/feel` → `github.com/pblumer/feel`
  - `internal/value` → `github.com/pblumer/feel/value`
- `internal/feel/` und `internal/value/` werden aus temis entfernt. Betroffene Importeure:
  `dmn`, `internal/boxed`, `internal/tck`, `cmd/feel-wasm`.
- Die FEEL-/Wert-**Fuzz- und Coverage-Ziele** wandern in das externe Repo; die
  `make cover`- und `make fuzz`-Listen in temis führen nur noch temis-eigene Pakete
  (`dmn`, `internal/boxed`, `internal/model` bzw. `dmn:FuzzCompile`, `internal/xml:FuzzDecode`).

`dmn` bleibt die einzige öffentliche Engine-API; `dmn.CompileExpression` (ADR-0029) und die
gesamte v1-Surface bleiben unverändert. Die Extraktion ist rein interne Umverdrahtung — die
`dmn`-Surface, die Golden-Tests und das Laufzeitverhalten ändern sich nicht.

## Konsequenzen

**Positiv**
- Eine Quelle der Wahrheit für das FEEL-Front-end; keine Doppelpflege mehr.
- FEEL-Fixes im `pblumer/feel`-Repo landen über ein Versions-Bump in temis.
- Kleinere temis-Codebasis; die Engine ist unabhängig testbar und wiederverwendbar.

**Negativ / Kosten**
- **Kapselung nicht mehr compiler-erzwungen:** `internal/feel` war privat — nur temis-Pakete
  konnten es importieren, und ADR-0005 erlaubte den Zugriff nur `dmn`. Das nun **öffentliche**
  Modul `github.com/pblumer/feel` ist von jedem Paket importierbar. Die Konvention „Nicht-Engine-
  Pakete erreichen FEEL nur über `dmn`" bleibt bestehen, ruht für das FEEL-Paket aber auf
  Disziplin (nur `dmn` und `internal/boxed` importieren es), nicht auf dem `internal/`-Mechanismus.
- **Externe Version pflegen:** temis hängt jetzt an einer `pblumer/feel`-Version; FEEL-Änderungen
  brauchen Release-dort-dann-Bump-hier statt einer einzelnen Edit. (Aktuell eine
  Pseudo-Version des Default-Branch, bis das Modul getaggt wird.)
- Das Wertemodell (`feel/value`) wird nun auch von `internal/boxed`, `internal/tck` und `dmn`
  aus dem externen Modul bezogen — unverändert im Verhalten, aber die Abhängigkeitsrichtung
  zeigt nach außen.

## Folgeaufgaben

- `pblumer/feel` mit SemVer-Tags versehen und in temis die Pseudo-Version durch einen
  getaggten Release ersetzen.
- Beim nächsten FEEL-Feature den Zwei-Repo-Fluss (Fix in `pblumer/feel` → Release → Bump in
  temis) in `CONTRIBUTING.md` festhalten, falls er sich einspielt.
