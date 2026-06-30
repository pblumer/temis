# ADR-0021: MCP-Endpoint in `temisd` ko-lokalisieren (geteilter Modell-Cache, ein Adressraum)

- **Status:** accepted
- **Datum:** 2026-06-30
- **Kontext-WP:** WP-53 (Folge zu ADR-0015)

## Kontext

ADR-0015 hat den Remote-MCP-Transport nativ über das eigenständige Binary
`temis-mcp -http` gelöst — bewusst getrennt vom REST/Modeler-Service `temisd`, „ohne
den REST-Service mit MCP zu vermischen". In der Praxis fallen dadurch jedoch **zwei
getrennte Prozesse mit je eigenem In-Memory-Modell-Cache** an:

- `temisd` lädt beim Start die gebündelten Beispielmodelle in **seinen** Cache
  (`WithExamples`, `service/examples.go`), damit der Modeler nie leer startet.
- `temis-mcp` startet mit einem **leeren** eigenen Cache und füllt ihn nur über
  `load_model`.

Folge: Ein Agent sieht über `list_models` **weder die Beispiele noch die im Modeler
geladenen/bearbeiteten Modelle**, und umgekehrt erscheint ein über MCP geladenes Modell
nicht im Modeler. Beide Oberflächen nutzen zwar dasselbe inhaltsadressierte
`sha256:`-Schema, teilen aber keinen Zustand. Gewünscht ist, dass **alle Oberflächen im
selben Adressraum auf demselben Cache arbeiten**.

## Optionen

1. **Beispiele auch in `temis-mcp` vorladen.** Kleiner Eingriff. — Die Beispiele
   erschienen über MCP, aber die Caches blieben getrennt: zur Laufzeit erzeugte/editierte
   Modelle bleiben gegenseitig unsichtbar, und es bleiben **zwei** Prozesse zu betreiben.
   Löst das eigentliche Problem (geteilter Zustand) nicht.
2. **Geteilter externer Store** (DB/Redis) für beide Prozesse. — Überdimensioniert für
   einen flüchtigen, inhaltsadressierten In-Memory-Cache; bringt eine neue Abhängigkeit
   und Infrastruktur (gegen die schmale Linie des Projekts, ADR-0014) ohne aktuellen
   Bedarf an host-übergreifendem Teilen.
3. **MCP-Endpoint in `temisd` ko-lokalisieren** (diese Entscheidung). `temisd` bedient
   optional `POST/GET /mcp` über den vorhandenen `mcp.Server`, der **denselben** Cache
   des Service nutzt. Ein Binary, ein Adressraum, ein Cache. Das eigenständige
   `temis-mcp` bleibt für reines stdio/lokales Einbetten erhalten.

## Entscheidung

Option 3. Eingeführt wird eine schmale Naht im `mcp`-Paket:

- Ein exportiertes `mcp.Store`-Interface (`Compile`/`Lookup`/`List`) plus
  `mcp.WithStore(...)`. Ohne Option behält der Server seinen bisherigen
  In-Process-Cache (`memStore`) — `temis-mcp` (stdio **und** HTTP) bleibt damit
  unverändert, ADR-0014/0015 bleiben gültig.
- `service.Server` stellt seinen Cache über `ModelStore()` als `mcp.Store` bereit und
  mountet die MCP-Routen, wenn per `AttachMCP(...)` ein MCP-Server hinterlegt ist
  (`mcp.RegisterRoutes`). Die Abhängigkeitsrichtung ist einseitig: `service → mcp`;
  `mcp` bleibt frei von `service`.
- `cmd/temisd` verdrahtet beide auf **einer** Engine und **einem** Cache hinter dem Flag
  `-mcp` (Default an). `/mcp` wird vom **selben** optionalen Bearer-Token geschützt wie
  die `/v1`-Endpunkte.

## Konsequenzen

**Positiv**
- Beispiele und Modeler-Modelle sind über MCP sichtbar; über MCP geladene Modelle
  erscheinen im Modeler — eine `modelId` über alle Oberflächen.
- Nur **ein** Binary/Image (`temisd`) zu deployen; der separate `temis-mcp`-Prozess
  entfällt für das Remote-Szenario.
- **Keine** neue Abhängigkeit: `Store` ist reine Standardbibliothek, konsistent mit
  ADR-0014.
- Standalone `temis-mcp` (stdio/HTTP) bleibt vollständig erhalten (Default-`memStore`).

**Negativ / Kosten**
- Der Service-Cache ist eine begrenzte LRU (Default 256, `WithCacheSize`). Unter einer
  Flut **verschiedener** Modelle könnten Beispiele auch aus der Agentensicht verdrängt
  werden — vertretbar (wenige, zuletzt genutzte Beispiele); bei Bedarf `-cache-size`
  erhöhen.
- `temisd` exponiert nun zusätzlich MCP; derselbe Token bewacht damit eine breitere
  Oberfläche (gewollt, einheitlich).
- `service` importiert `mcp` (einseitig, zyklusfrei).

**Verhältnis zu ADR-0015**
ADR-0015 bleibt gültig: Der native Streamable-HTTP-Transport und das eigenständige
`temis-mcp` bestehen fort. Dieses ADR ergänzt die **ko-lokalisierte** Betriebsart als
Standardweg für „eine Instanz, ein Cache" und löst damit die in ADR-0015 betonte strikte
Trennung bewusst und opt-in auf.
