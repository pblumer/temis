# ADR-0011: Core Engine als reine Go-Library (`Compile`/`Evaluate`), Service nur Adapter

- **Status:** accepted
- **Datum:** 2026-06-27
- **Kontext-WP:** WP-04 / WP-05 (verfeinert ADR-0005)

## Kontext

ADR-0005 hat „Library-first vs. Service-first" auf Architektur-Ebene entschieden:
der Kern ist die Go-API, `service/` und `cmd/temisd` sind dünne Adapter. Dieses ADR
setzt eine Ebene tiefer an und hält die daraus folgende **Produktentscheidung** fest:
Die Core Engine ist das *Primärartefakt* — eine reine Go-Library mit dem schmalen
Vertrag `Compile` + `Evaluate` — und der HTTP/gRPC-Service ist bewusst nur ein
nachgelagerter Konsument dieser Library, kein gleichrangiges Gegenstück.

Auslöser ist die Frage, ob ein HTTP/gRPC-Server „alles erschlägt". Er tut es nicht:
ein Server bedient ausschließlich den Out-of-Process-Fall (zentraler Decision-Service,
polyglotte Clients, unabhängige Skalierung) und erzwingt dafür pro Auswertung
Serialisierung, einen Netzwerk-Hop und Latenz. Eine ganze Klasse legitimer
DMN-Nutzungen wird davon prinzipiell nicht oder schlecht bedient:

- **In-Process-Einbettung in Go-Programme.** Pricing, Eligibility, Routing inline im
  Request-Pfad — ein RPC zu einem externen DMN-Server wäre eine sinnlose Abhängigkeit
  und ein zusätzlicher Failure-Mode.
- **Hochfrequente Auswertung.** Batch über Millionen Records oder pro Event in einem
  Stream. Hier zählt jede Allokation; ein RPC pro Decision ist disqualifiziert. Das
  Performance-Budget (`docs/50-testing-strategy.md`: warmes Evaluate im niedrigen
  einstelligen µs-Bereich) ist nur in-process erreichbar.
- **Embedded / CLI / Edge** ohne Server-Infrastruktur.
- **Einbettung durch andere Engines.** Eine BPMN-Engine (z. B. `pblumer/chrampfer`)
  ruft DMN aus Business Rule Tasks auf. Das muss in-process sein, sonst koppelt eine
  durable Workflow-Engine ihre Korrektheit an die Verfügbarkeit eines externen
  HTTP-Dienstes — genau die Fragilität, die solche Architekturen vermeiden.

Daraus folgt: Die Library-Oberfläche, nicht die Service-API, ist der stabil zu
haltende, SemVer-versionierte Vertrag, an den sich externe Nutzer binden.

## Optionen

1. **Service als primäre Schnittstelle, Library als Nebenprodukt.** Ein Deployment,
   einheitlicher Zugang. — Schließt alle In-Process-Fälle aus, erzwingt Netzwerk-Hop
   und Serialisierung pro Decision, verfehlt das µs-Performance-Budget, macht Einbetter
   (Chrampfer) von einem externen Dienst abhängig.
2. **Library als Primärartefakt, Service als dünner Adapter** (diese Entscheidung).
   `package dmn` ist der stabile Vertrag; `service/` + `cmd/temisd` übersetzen nur
   Transport ⇆ Engine-Typen. — Bedient In-Process *und* Out-of-Process, Server erbt
   automatisch Korrektheit/Performance. Kosten: die Lib-API ist eine
   Breaking-Change-Oberfläche und braucht echte SemVer-Disziplin.
3. **Zwei getrennte Repos (Engine-Lib + eigenständiger Service).** Maximale Trennung. —
   Unnötiger Overhead (separate CI, Release-Koordination, ADR-Verwaltung) ohne
   Mehrwert gegenüber Option 2, solange der Service keine eigene Roadmap hat. Der
   `internal/`-Grenzverlauf liefert die Entkopplung bereits innerhalb eines Moduls.

## Entscheidung

Option 2. Die Core Engine wird als reine Go-Library im selben Modul
(`github.com/pblumer/temis`, Package `dmn`) geführt und ist das Primärartefakt.

**Scope des Lib-Kerns:** ausschließlich `Compile` und `Evaluate` — über *alle
freigegebenen* DMN-Versionen (Ziel 1.5, lesend 1.3/1.4; siehe ADR-0002) und mit
*vollständigem* FEEL (ADR-0003). Die Versionserkennung liegt in `Compile` (aus
Namespace/Version des Modells), nicht in der API-Signatur: ein neuer DMN-Jahrgang ist
interner Zuwachs, kein API-Bruch.

**Stabiler Vertrag:** `Engine`, `Compile`, `Definitions`, `Decision`/`Service`,
`CompiledDecision`, `Evaluate`, `Input`, `Result`, `Diagnostics` (siehe
`docs/40-api-contract.md`). Diese Oberfläche denkt ausschließlich in Go-Typen und zieht
**null Transport-Importe** (kein `net/http`, kein gRPC im Engine-Package). Änderungen an
ihr sind Breaking Changes und brauchen ein eigenes ADR.

**Service als Adapter:** `service/` und `cmd/temisd` konsumieren `package dmn` wie jeder
andere Client und erhalten keine Sonderrechte. Wer den Dienst will, nimmt `temisd`; wer
die Engine will, importiert `temis`.

Ein eigenes Repo (Option 3) wird *nicht* gewählt, bleibt aber eine Option für später,
falls der Service jemals eine eigenständige, unabhängig versionierte Roadmap bekommt.

## Konsequenzen

**Positiv**
- Beide Welten werden bedient: In-Process-Einbettung *und* Netzwerkdienst.
- Einbetter wie Chrampfer binden DMN ohne Netzwerk-Failure-Mode in-process ein.
- Das µs-Performance-Budget bleibt erreichbar (kein erzwungener RPC/Serialisierung).
- Der Server erbt automatisch Korrektheit (TCK) und Performance der Library.
- Schmale, in Go-Typen gedachte Oberfläche → klares SemVer-Versprechen für externe Nutzer.

**Negativ / Kosten**
- Die Lib-API ist jetzt eine harte Breaking-Change-Grenze und verlangt SemVer-Disziplin.
- Die Compile-/Eval-Fehlergrenze (was ist Compile-Fehler vs. Laufzeit-Fehler nach
  FEEL-Null-Semantik) wird Teil des öffentlichen Verhaltens und ist später nur noch
  unter Breaking-Change-Vorbehalt verschiebbar — muss früh bewusst gezogen werden.

**Folgeaufgaben**
- API-Stabilitätszusage in `docs/40-api-contract.md` explizit auf „alle freigegebenen
  Versionen, voller FEEL, Versionswahl in `Compile`" festschreiben.
- Compile- vs. Eval-Fehlergrenze dokumentieren (eigener Abschnitt im API-Vertrag).
- ADR-0005 bleibt gültig; dieses ADR verfeinert es und ersetzt es nicht.
