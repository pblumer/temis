# DMN-TCK Vendor-Submission

Dieser Ordner enthält die **Ergebnis-Artefakte** für eine offizielle
Einreichung bei [`dmn-tck/tck`](https://github.com/dmn-tck/tck) — dem
Vendor-Results-Mechanismus, über den Produkte ihre gemessene DMN-Konformität
öffentlich listen lassen.

## Was hier liegt

```
Temis/<version>/
  tck_results.csv         # eine Zeile je Test-Case: SUCCESS | FAILURE | NOT_TESTABLE
  tck_results.properties  # Vendor-/Produkt-Deskriptor (Format wie andere Vendoren)
```

Erzeugt mit dem reproduzierbaren Exporter:

```sh
make tck-results                       # nutzt den gepinnten Korpus (0dbcaf9)
# oder mit expliziter Version/Datum:
make tck-results TCK_RESULT_VERSION=v0.1.0 TCK_RESULT_DATE=2026-07-05
```

Der Exporter (`cmd/temis-tck-results`) fährt jeden Case des offiziellen Korpus
durch die Temis-Engine und schreibt das Ergebnis im exakten `dmn-tck/tck`-Format.

## Aktueller Stand

| Status | Cases |
|---|---|
| `SUCCESS` | **3430** |
| `FAILURE` | 47 |
| `NOT_TESTABLE` | 18 (externe Java-Funktionen — kein JVM in einer reinen Go-Engine) |
| **Summe** | 3495 |

**98,14 %** über alle Cases bzw. **98,65 %** über die anwendbaren (testbaren)
Cases. Die `FAILURE`- und `NOT_TESTABLE`-Kategorien sind in
[`../tck-exceptions.md`](../tck-exceptions.md) prinzipiell dokumentiert
(externe Java-Funktionen, RE2-Regex-Grenzen, float64-Präzisions-Orakelwerte,
sowie einzelne Fälle, die tiefe Typsystem-/Lexer-Eingriffe bräuchten).

## Einreichen (manuell)

1. `dmn-tck/tck` **forken**.
2. Diesen Ordner nach `TestResults/Temis/<version>/` des Forks kopieren:
   ```sh
   cp -r docs/tck-submission/Temis/<version> <fork>/TestResults/Temis/<version>
   ```
3. Commit + Push im Fork, dann **PR gegen `dmn-tck/tck`** öffnen.

> Vor einer öffentlichen Einreichung ggf. ein echtes Release taggen und die
> Artefakte mit der Release-Version neu erzeugen (`TCK_RESULT_VERSION=vX.Y.Z`),
> statt `0.0.0-dev`.
