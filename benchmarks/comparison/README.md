# Temis vs. Drools — 1:1 DMN-Benchmark

Ein fairer Kopf-an-Kopf-Vergleich der DMN-Auswertung: **beide Engines laden
dieselben DMN-Dateien** (`models/*.dmn`), werten sie mit **denselben Eingaben**
auf **derselben Maschine** aus, und beide prüfen dasselbe erwartete Ergebnis
(`m8` bzw. `g5`), damit nichts unbemerkt auseinanderläuft.

> **Kurzfassung:** Auf identischen Modellen ist Temis pro Auswertung **~2–3×
> schneller** als Drools (ein Kern) und liefert **~1,4–1,5× mehr Durchsatz**
> (4 Kerne, beide mit Default-GC). Mit dem üblichen Go-Durchsatz-Tuning
> (`GOGC=400`) wächst der Durchsatzvorsprung auf **~2,3–2,8×**.

## Ergebnisse

Hardware: Intel® Xeon® @ 2,80 GHz, **4 vCPU**, geteilte Cloud-VM.
Software: Temis (Go 1.24) · Drools `kie-dmn-core` **8.44.0.Final** (JDK 21).
Beide out-of-the-box konfiguriert (Temis `GOGC=100`, JVM Default-G1GC).

### Latenz — ein Kern, warm (weniger ist besser)

| Modell | Temis | Drools | Temis schneller |
|---|---:|---:|---:|
| String-Tabelle (Gleichheit) | **≈ 2,1 µs** | ≈ 6,1 µs | **2,9×** |
| Numerische Tabelle (Intervalle) | **≈ 2,8 µs** | ≈ 5,8 µs | **2,1×** |

### Durchsatz — 4 Kerne parallel (mehr ist besser)

| Modell | Temis (Default) | Drools (Default) | Temis schneller | Temis `GOGC=400` |
|---|---:|---:|---:|---:|
| String-Tabelle | **≈ 861 000/s** | ≈ 577 000/s | **1,49×** | ≈ 1 620 000/s |
| Numerische Tabelle | **≈ 750 000/s** | ≈ 535 000/s | **1,40×** | ≈ 1 220 000/s |

Drools skaliert über Threads etwas besser (die JVM ist reifer im parallelen GC),
Temis ist pro Kern deutlich schneller und im parallelen Default‑Fall trotzdem
vorn; mit größerem Go‑Heap (`GOGC`/`GOMEMLIMIT`) zieht Temis klar davon.

## Was gemessen wird

Beide Seiten spiegeln denselben Ablauf: **einmal kompilieren, viele Male
auswerten** — der Produktionspfad.

- **Temis** (`temis/bench_temis_test.go`): `Engine.Compile` einmal, dann pro Op
  eine `Input`-Map bauen und `Evaluate` aufrufen (Go-`testing`-Benchmark,
  `dec/s`-Metrik über `RunParallel`).
- **Drools** (`drools/src/main/java/bench/DmnBench.java`): `DMNRuntime` +
  `DMNModel` einmal bauen, dann pro Op einen `DMNContext` bauen und
  `evaluateAll` aufrufen (JMH, `Throughput`- und `AverageTime`-Modus).

Die Modelle (`models/string-table.dmn`, `models/numeric-table.dmn`) sind
Standard‑**DMN 1.3** (Namespace `…/20191111/MODEL/`) — die breiteste gemeinsame
Basis: Drools unterstützt sie voll, Temis liest sie (tolerant 1.3/1.4/1.5). So
parst garantiert **dasselbe Dokument** in beide Engines.

## Selbst nachstellen

```sh
# 0) (einmalig) Modelle erzeugen — schon eingecheckt, nur bei Änderungen nötig:
go run gen_models.go

# 1) Temis:
cd temis
go test -run=^$ -bench='BenchmarkStringTable$|BenchmarkNumericTable$' -benchmem ./   # Latenz + Allocs
go test -run=^$ -bench=Throughput -benchtime=2s ./                                   # Durchsatz (dec/s)
GOGC=400 go test -run=^$ -bench=Throughput -benchtime=2s ./                          # Durchsatz, größerer Heap

# 2) Drools (JMH):
cd ../drools
mvn -q package                                                                       # baut target/benchmarks.jar
java -jar target/benchmarks.jar -bm avgt -tu us -f 1 -wi 3 -i 5 -t 1                 # Latenz (µs/op)
java -jar target/benchmarks.jar -bm thrpt -tu s  -f 1 -wi 3 -i 5 -t 4               # Durchsatz (ops/s)
```

Die `parity`-Prüfung auf beiden Seiten bricht sofort ab, falls eine Engine nicht
`m8`/`g5` liefert — der Vergleich kann also nie stillschweigend Äpfel mit Birnen
messen.

## Ehrliche Einordnung

- **Zwei repräsentative Tabellen, kein ganzer Korpus.** String‑Gleichheit und
  numerische Intervalle sind die häufigsten realen DMN‑Formen, decken aber nicht
  jedes Feature ab. Ein breiterer Vergleich (FEEL‑Arithmetik, DRG‑Graphen,
  Kollektionen) ist der nächste sinnvolle Schritt.
- **Beide Default‑konfiguriert.** Fairness heißt: keine Seite getunt. Der
  getunte Temis‑Wert steht separat; die JVM ließe sich ebenso GC‑tunen. Der
  Single‑Core‑Latenzwert ist der sauberste Vergleich, weil er GC‑Skalierung
  ausklammert.
- **Geteilte 4‑Kern‑VM, JMH mit 1 Fork.** Untergrenze, kein Bestfall; die
  absoluten Zahlen steigen auf dedizierter Hardware. Für Publikationsqualität
  mehr Forks (`-f 3`) und längere Läufe verwenden.
- **Gleiche Semantik.** Beide Engines liefern nachweislich dasselbe Ergebnis;
  gemessen wird reine Auswertungsleistung, nicht unterschiedliches Verhalten.
