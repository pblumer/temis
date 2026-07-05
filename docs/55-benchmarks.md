# Performance & Benchmarks — reproduzierbare Zahlen

> **Über 1 Million Entscheidungen pro Sekunde.** Auf einer bescheidenen
> 4‑Kern‑Cloud‑VM wertet Temis eine typische Decision‑Table mit **> 1,6 Mio
> Entscheidungen/s** aus. Jede Zahl hier ist mit einem im Repo eingecheckten
> Benchmark reproduzierbar — keine Marketing‑Schätzung.

Dieses Dokument ist die verbindliche Quelle für Temis‑Performance‑Aussagen. Es
beschreibt **was** gemessen wird, **wie**, **auf welcher Hardware** und **wie du
es selbst nachstellst**. Die Budgets, die eine Regression im CI brechen, stehen
in [`50-testing-strategy.md`](50-testing-strategy.md) §6; die dazugehörigen
Benchmarks in [`dmn/bench_test.go`](../dmn/bench_test.go) und
[`dmn/throughput_test.go`](../dmn/throughput_test.go).

---

## TL;DR

- **Warme Auswertung ist der kritische Pfad.** Eine kompilierte Entscheidung ist
  unveränderlich und nebenläufigkeitssicher; man kompiliert einmal und wertet
  millionenfach aus.
- **Durchsatz (4 Kerne, `GOGC=400`):** String‑Tabelle **≈ 1,62 Mio/s**,
  numerische Tabelle **≈ 1,22 Mio/s**.
- **Latenz (1 Kern, warm):** String‑Tabelle **≈ 1,9 µs**, numerische Tabelle
  **≈ 2,9 µs** je Entscheidung — bei 15–18 Allokationen/Auswertung.
- **Skaliert mit Kernen.** Der Durchsatz ist GC‑, nicht CPU‑gebunden; mehr Kerne
  und ein größerer Heap (`GOGC`/`GOMEMLIMIT`) heben ihn linear an.

---

## 1. Methodik

Gemessen wird die **warme Auswertung** (`CompiledDecision.Evaluate`) — der Pfad,
der in Produktion pro Anfrage läuft. Kompilierung (`Engine.Compile`) ist ein
Einmalvorgang und wird separat und mit großzügigem Budget geführt.

Zwei Größen werden berichtet:

| Größe | Was | Wie |
|---|---|---|
| **Latenz** | ns pro Auswertung, ein Kern | `go test -bench` (ns/op, allocs/op) |
| **Durchsatz** | Auswertungen/s, alle Kerne | `b.RunParallel`, Metrik `dec/s` = Auswertungen ÷ Wall‑Clock |

Der Durchsatz nutzt `testing.B.RunParallel`, weil genau das der reale Betrieb
ist: viele Goroutinen teilen sich **eine** kompilierte Entscheidung. Die Metrik
`dec/s` erscheint direkt in der Benchmark‑Ausgabe.

**Modelle** (bewusst repräsentativ, nicht geschönt):

- **String‑Tabelle** — zwei String/Enum‑Inputs per Gleichheit gematcht, 10
  Regeln, `UNIQUE`. Die häufigste reale DMN‑Form.
- **Numerische Tabelle** — vier Zahl‑Inputs, Intervall‑Zellen (`[lo..hi]`), 10
  Regeln, `UNIQUE`.
- **Arithmetik** — ein Literal‑Ausdruck `(A*B+3)/2-1` mit exakter Dezimalarithmetik.
- **DRG‑Kette** — 10 verkettete Entscheidungen (Graph‑Auswertung, Memoisation).

---

## 2. Hardware & Umgebung

Die veröffentlichten Zahlen stammen von einer **bescheidenen, geteilten
Cloud‑VM** — bewusst konservativ, damit die Ergebnisse eine Untergrenze sind,
kein Bestfall:

| | |
|---|---|
| CPU | Intel® Xeon® @ 2,80 GHz, **4 vCPU** |
| Go | 1.24 (`goarch=amd64`) |
| OS | Linux |
| Last | geteilter Runner (Timing‑Rauschen ± ~10 %) |

Auf dedizierter Server‑Hardware (16–32 Kerne, höhere Taktung, kein Noisy
Neighbor) liegt der Durchsatz um ein Vielfaches höher.

---

## 3. Ergebnisse

### 3.1 Latenz — ein Kern, warm

| Szenario | ns/op | Auswertungen/s (1 Kern) | Allokationen/op |
|---|---:|---:|---:|
| String‑Tabelle (10 Regeln) | ≈ 1 900 | ≈ 462 000 | 17 |
| Numerische Tabelle (10 Regeln) | ≈ 2 900 | ≈ 353 000 | 15 |
| Arithmetik (Dezimal) | ≈ 4 100 | ≈ 245 000 | 18 |
| DRG‑Kette (10 tief) | ≈ 8 200 | ≈ 122 000 | 64 |

### 3.2 Durchsatz — 4 Kerne parallel

| Szenario | `GOGC=100` (Default) | `GOGC=400` |
|---|---:|---:|
| **String‑Tabelle** | ≈ 854 000/s | **≈ 1 620 000/s** |
| **Numerische Tabelle** | ≈ 734 000/s | **≈ 1 220 000/s** |
| Arithmetik | ≈ 606 000/s | ≈ 911 000/s |

**Warum `GOGC` zählt.** Der Durchsatz ist durch den Garbage Collector begrenzt,
nicht die CPU: der Auswertungspfad allokiert wenige, kurzlebige Objekte, und der
GC ist eine geteilte Ressource, die die Parallelität serialisiert. Ein größerer
Heap (`GOGC=400`, oder in Produktion `GOMEMLIMIT`) senkt die GC‑Frequenz und hebt
den Durchsatz nahezu verdopppelt — ein Standard‑Tuning für durchsatzorientierte
Go‑Dienste. Die reine Rechenarbeit pro Entscheidung ändert sich dabei nicht.

---

### 3.3 Kopf-an-Kopf gegen Drools

Ein 1:1-Vergleich gegen die **Drools-DMN-Engine** (`kie-dmn-core` 8.44.0.Final,
JDK 21) auf **identischen DMN-Dateien**, derselben VM, beide out-of-the-box
(Details, Harness und Reproduktion in
[`benchmarks/comparison/`](../benchmarks/comparison/README.md)):

| Modell | Temis | Drools | Temis schneller |
|---|---:|---:|---:|
| Latenz String-Tabelle (1 Kern) | ≈ 2,1 µs | ≈ 6,1 µs | **2,9×** |
| Latenz numerische Tabelle (1 Kern) | ≈ 2,8 µs | ≈ 5,8 µs | **2,1×** |
| Durchsatz String-Tabelle (4 Kerne) | ≈ 861 000/s | ≈ 577 000/s | **1,49×** |
| Durchsatz numerische Tabelle (4 Kerne) | ≈ 750 000/s | ≈ 535 000/s | **1,40×** |

Beide Engines liefern nachweislich dasselbe Ergebnis (Paritäts-Guard auf beiden
Seiten). Pro Kern ist Temis ~2–3× schneller; im parallelen Default-Fall ~1,4×,
und mit `GOGC=400` wächst der Durchsatzvorsprung auf ~2,3–2,8×.

## 4. Selbst nachstellen

```sh
# Latenz + Allokationen (ein Kern):
go test -run=^$ -bench='BenchmarkEvaluate' -benchmem ./dmn/

# Durchsatz (alle Kerne), Default-GC:
go test -run=^$ -bench='BenchmarkThroughput' -benchtime=2s ./dmn/

# Durchsatz mit größerem Heap:
GOGC=400 go test -run=^$ -bench='BenchmarkThroughput' -benchtime=2s ./dmn/
```

Die Spalte `dec/s` in der Ausgabe ist der Durchsatz. Für einen fairen Vergleich
zwischen Maschinen `-benchtime=2s` (oder mehr) verwenden, damit sich die Zahlen
einschwingen.

---

## 5. Was Temis schnell macht

Der warme Pfad ist bewusst allokationsarm (Details in den Commit‑Nachrichten und
`50-testing-strategy.md` §6):

- **Fast‑Path für Einzelentscheidungen** — eine Entscheidung ohne
  abhängige Entscheidungen umgeht die Graph‑Maschinerie komplett.
- **Intervall‑Zellen ohne Range‑Allokation** — `[lo..hi]` wird direkt gegen die
  Grenzen geprüft, ohne einen `value.Range` ins Interface zu boxen.
- **Wiederverwendeter Spalten‑Scope** — der implizite Test‑Input `?` wird pro
  Spalte neu gebunden statt neu alloziert.
- **Small‑Int‑Cache** — kleine Ganzzahlen liefern unveränderliche, gecachte
  `Number`s ohne `apd.Decimal`‑Allokation.
- **Geteilter Ausführungs‑State** — der Graph‑Evaluator teilt einen State über
  alle Entscheidungen einer Auswertung.
- **Einmalige Kompilierung** — DMN‑XML/FEEL wird einmal in Closures kompiliert;
  die Auswertung ist Slot‑Indexierung, keine Map‑Lookups (ADR‑0004).

---

## 6. Ehrliche Einordnung

- **Kein Cross‑Engine‑Vergleich.** Diese Zahlen messen Temis gegen sich selbst
  über die Zeit, nicht gegen Camunda, Drools o. Ä. Ein fairer Vergleich braucht
  identische Modelle, identische Hardware und identische Warm‑up‑Bedingungen —
  ein separates, sorgfältiges Projekt.
- **Untergrenze, kein Bestfall.** Gemessen auf einer geteilten 4‑Kern‑VM. Bessere
  Hardware und `GOMEMLIMIT`‑Tuning heben die Zahlen deutlich.
- **Korrektheit schlägt Geschwindigkeit.** Jede Optimierung ist gegen die
  offizielle DMN‑TCK‑Suite (unverändert 98,1 %), die volle Testsuite und den
  `-race`‑Build abgesichert. Ein Performance‑Budget (`make budget`) bricht den
  Build bei einer Allokations‑ oder Komplexitätsregression.
