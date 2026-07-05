# Temis vs. Drools — 1:1 DMN-Benchmark

Ein fairer Kopf-an-Kopf-Vergleich der DMN-Auswertung: **beide Engines laden
dieselben DMN-Dateien** (`models/*.dmn`), werten sie mit **denselben Eingaben**
auf **derselben Maschine** aus, und beide prüfen dasselbe erwartete Ergebnis
(`m8` bzw. `g5`), damit nichts unbemerkt auseinanderläuft.

> **Kurzfassung:** Pro Auswertung (ein Kern) ist Temis in **jedem** Szenario
> schneller — **1,2× bis 3,0×**, am deutlichsten bei Decision-Tables (dem
> häufigsten realen DMN-Fall). Im parallelen Durchsatz (4 Kerne, beide Default)
> führt Temis bei Tabellen (bis 1,7×) und liegt bei arithmetik-/graphlastigen
> Modellen gleichauf; mit dem üblichen Go-Heap-Tuning (`GOGC=400`) zieht Temis
> überall davon.

## Ergebnisse

Fünf Szenarien decken die wichtigsten DMN-Feature-Typen ab: String-Gleichheit,
numerische Intervalle, exakte FEEL-Arithmetik, ein DRG-Graph (10 verkettete
Entscheidungen) und eine COLLECT-Tabelle (Listen-Ergebnis).

Hardware: Intel® Xeon® @ 2,80 GHz, **4 vCPU**, geteilte Cloud-VM.
Software: Temis (Go 1.24) · Drools `kie-dmn-core` **8.44.0.Final** (JDK 21).
Beide out-of-the-box konfiguriert (Temis `GOGC=100`, JVM Default-G1GC).

### Latenz — ein Kern, warm (µs/Auswertung, weniger ist besser)

| Szenario | Temis | Drools | Temis schneller |
|---|---:|---:|---:|
| String-Tabelle (Gleichheit) | **2,1** | 6,3 | **3,0×** |
| Numerische Tabelle (Intervalle) | **2,7** | 6,4 | **2,4×** |
| COLLECT-Tabelle (Liste) | **1,8** | 4,0 | **2,3×** |
| DRG-Graph (10 tief) | **7,7** | 10,3 | **1,3×** |
| FEEL-Arithmetik (dezimal) | **3,7** | 4,6 | **1,2×** |

Der Single-Core-Wert ist der sauberste Vergleich — er klammert GC-Skalierung
aus. Temis gewinnt jedes Szenario; bei Tabellen 2,3–3,0×, bei den rechen-/
graphlastigen Modellen knapper (dort dominiert die exakte Dezimalarithmetik bzw.
die Graph-Maschinerie, wo beide Engines ähnlich viel Arbeit leisten).

### Durchsatz — 4 Kerne parallel (Auswertungen/s, mehr ist besser)

| Szenario | Temis (Default) | Drools (Default) | Temis schneller | Temis `GOGC=400` |
|---|---:|---:|---:|---:|
| String-Tabelle | **880 000** | 526 000 | **1,67×** | 1 811 000 |
| Numerische Tabelle | **832 000** | 572 000 | **1,45×** | 1 266 000 |
| COLLECT-Tabelle | **955 000** | 875 000 | **1,09×** | 1 552 000 |
| DRG-Graph | **290 000** | 253 000 | **1,15×** | 405 000 |
| FEEL-Arithmetik | 664 000 | **763 000** | 0,87× | 907 000 |

**Ehrlich benannt:** Im *parallelen Default-Fall* schlägt Drools Temis bei reiner
Arithmetik (763k vs. 664k) — dort ist Temis GC-gebunden, und die JVM skaliert
den GC über Threads reifer. Das ist der **einzige** der zehn Vergleiche, den
Drools gewinnt; mit `GOGC=400` dreht sich auch dieser (907k). Bei Tabellen führt
Temis durchgehend, single-core überall.

## Was gemessen wird

Beide Seiten spiegeln denselben Ablauf: **einmal kompilieren, viele Male
auswerten** — der Produktionspfad.

- **Temis** (`temis/bench_temis_test.go`): `Engine.Compile` einmal, dann pro Op
  eine `Input`-Map bauen und `Evaluate` aufrufen (Go-`testing`-Benchmark,
  `dec/s`-Metrik über `RunParallel`).
- **Drools** (`drools/src/main/java/bench/DmnBench.java`): `DMNRuntime` +
  `DMNModel` einmal bauen, dann pro Op einen `DMNContext` bauen und
  `evaluateAll` aufrufen (JMH, `Throughput`- und `AverageTime`-Modus).

Die fünf Modelle in `models/` sind Standard‑**DMN 1.3** (Namespace
`…/20191111/MODEL/`) — die breiteste gemeinsame Basis: Drools unterstützt sie
voll, Temis liest sie (tolerant 1.3/1.4/1.5). So parst garantiert **dasselbe
Dokument** in beide Engines. Alle werden von `gen_models.go` erzeugt.

## Selbst nachstellen

```sh
# 0) (einmalig) Modelle erzeugen — schon eingecheckt, nur bei Änderungen nötig:
go run gen_models.go

# 1) Temis:
cd temis
go test -run=^$ -bench=Latency    -benchmem ./          # Latenz (ns/op) + Allocs, alle 5
go test -run=^$ -bench=Throughput -benchtime=2s ./       # Durchsatz (dec/s), alle 5
GOGC=400 go test -run=^$ -bench=Throughput -benchtime=2s ./   # Durchsatz, größerer Heap

# 2) Drools (JMH):
cd ../drools
mvn -q package                                                        # baut target/benchmarks.jar
java -jar target/benchmarks.jar -bm avgt  -tu us -f 1 -wi 3 -i 5 -t 1 # Latenz (µs/op)
java -jar target/benchmarks.jar -bm thrpt -tu s  -f 1 -wi 3 -i 5 -t 4 # Durchsatz (ops/s)
```

Die `parity`-Prüfung auf beiden Seiten bricht sofort ab, falls eine Engine nicht
das erwartete Ergebnis liefert (`m8`, `g5`, `21.5`, `10`, `[low, mid, spot]`) —
der Vergleich kann also nie stillschweigend Äpfel mit Birnen messen.

## Ehrliche Einordnung

- **Fünf repräsentative Modelle, kein ganzer Korpus.** Sie decken die häufigsten
  DMN‑Formen ab (Tabellen, Arithmetik, Graph, Collect), aber nicht jedes
  FEEL‑Built‑in oder jede Hit‑Policy. Ein voller Feature‑Sweep ist der nächste
  Schritt.
- **Beide Default‑konfiguriert.** Fairness heißt: keine Seite getunt. Der
  getunte Temis‑Wert steht separat; die JVM ließe sich ebenso GC‑tunen. Der
  Single‑Core‑Latenzwert ist der sauberste Vergleich, weil er GC‑Skalierung
  ausklammert.
- **Geteilte 4‑Kern‑VM, JMH mit 1 Fork.** Untergrenze, kein Bestfall; die
  absoluten Zahlen steigen auf dedizierter Hardware. Für Publikationsqualität
  mehr Forks (`-f 3`) und längere Läufe verwenden.
- **Gleiche Semantik.** Beide Engines liefern nachweislich dasselbe Ergebnis;
  gemessen wird reine Auswertungsleistung, nicht unterschiedliches Verhalten.
