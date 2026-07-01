# ADR-0027: Dateisystem-Modell-Store (optionale Persistenz des Modell-Cache)

- **Status:** accepted
- **Datum:** 2026-07-01
- **Kontext-WP:** WP-36 (Etappe „Persistenz", Ausbau von WP-32/WP-35)

## Kontext

`temisd` hält kompilierte Modelle in einem beschränkten, content-adressierten
**In-Memory-LRU-Cache** (`service/cache.go`, WP-35). Das ist bewusst so gebaut — die
Engine ist eine reine Berechnungsfunktion (ADR-0004/0011), ohne globalen Zustand,
thread-safe und embeddable. `docs/00-overview.md` §3 führt „**kein eingebauter
Persistenzlayer im MVP**" explizit als Nicht-Ziel: es hielt den MVP-Scope klein.

Die Kehrseite: **jeder Neustart verliert alles**, was ein Nutzer hochgeladen oder im
Modeler gebaut hat. Die eingebetteten Beispielmodelle kommen per `go:embed` zurück, aber
eigene Modelle sind weg. Für einen containerisierten `temisd`, in dem jemand über die
Web-UI ein paar Entscheidungstabellen pflegt, ist das ein echter Datenverlust bei jedem
Deploy/Restart — genau der Reibungspunkt, der diese Entscheidung auslöst.

Wichtige Beobachtung: Ein `storedModel` enthält zwar `defs`/`index`/`diags`, aber die
sind **allesamt deterministisch aus dem rohen `xml` rekompilierbar** (der Hot-Reload-Pfad
tut das bereits). Zu persistieren ist also nur das rohe DMN-XML — kein abgeleiteter
Zustand, der von der Engine abdriften könnte.

## Optionen

1. **Auf Git-Modelle setzen (WP-70–73, ADR-0022)** — DMN-Modelle versioniert aus einem
   Repo lesen/schreiben existiert bereits. Aber: Token pro Request, Remote nötig, und es
   ist eine *Lifecycle*-Story (PR-Review, Versionierung), keine automatische
   Sitzungspersistenz eines laufenden Servers. Löst „Neustart verliert alles" nicht direkt.
2. **Embedded DB (SQLite/bbolt)** — echte DB mit Metadaten/Queries. Bringt aber einen
   **neuen Dependency** und bricht die durchgängige stdlib-only-Politik (ADR-0011/0014,
   „reine Standardbibliothek, kein neuer Dep, kein Go-Bump"). Für reine XML-Blobs
   überdimensioniert.
3. **Dateisystem-Store (gewählt)** — ein Verzeichnis content-adressierter `.dmn`-Dateien,
   beim Boot in den Cache geladen, bei Upload/Save geschrieben. Reine stdlib
   (`os`/`filepath`), kein neuer Dep. Passt zum distroless-Container (gemountetes Volume)
   und zum batteries-included/opt-in-`TEMIS_*`-Muster.

## Entscheidung

Ein **optionaler, dateisystem-gestützter Modell-Store** (`service/persist.go`,
`diskStore`), aktiviert über `service.WithModelStore(dir)` bzw. `temisd -models-dir` /
`$TEMIS_MODELS_DIR`. **Per Default aus** — ohne gesetztes Verzeichnis bleibt `temisd`
byte-identisch rein in-memory, das MVP-Nicht-Ziel bleibt der Default.

Eigenschaften:

- **Content-adressiert:** Dateiname ist der SHA-256-Hex des XML (`<hex>.dmn`), ohne
  `sha256:`-Präfix (portabel über Dateisysteme). Re-Persistieren unveränderter Modelle ist
  ein No-op; ein editiertes Modell landet unter neuem Namen (überschreibt die alte Revision
  nie) — konsistent mit dem content-addressed Cache-Schlüssel (WP-35).
- **Nur rohes XML:** kein abgeleiteter Zustand auf der Platte. Beim Boot rekompiliert
  `compileAndStore` alles frisch → der Store kann nie von der Engine abdriften.
- **Atomare Writes:** Temp-Datei + `os.Rename`, damit ein Crash mitten im Schreiben nie ein
  halbes, nicht-kompilierbares Modell hinterlässt.
- **Einziger Choke-Point:** Persistenz hängt in `compileAndStore` (Write) und `lookup`
  (Read-Fallback). Da HTTP, Modeler-Saves, MCP, gRPC, Git-Load und Assist-Tools alle
  darüber laufen, wird jeder Schreibpfad automatisch durabel.
- **Cache-Miss-Fallback:** Ein persistiertes, aber aus dem beschränkten Cache verdrängtes
  Modell wird bei `lookup` on-demand von der Platte rekompiliert → der Store ist die
  durable Quelle, der Cache nur das Working-Set.
- **Beispiele nicht persistiert:** die gebündelten Examples laden, solange der Store noch
  nicht offen ist, und werden nie auf die Platte geschrieben (sie re-embedden ohnehin).

## Konsequenzen

**Positiv:** Eigene Modelle überleben Neustart/Deploy; ein Container mit gemountetem Volume
ist ohne externe Infrastruktur zustandsbehaftet. Reine stdlib, kein neuer Dep, kein
Go-Bump. Default-Verhalten unverändert (opt-in). Engine-Kern unberührt (kein Hot-Path).

**Negativ / Grenzen:** Kein verteilter/geteilter Store (ein Prozess, ein Verzeichnis —
Cluster-Betrieb bleibt Nicht-Ziel). Keine Löschung/GC alter Revisionen (append-only auf
der Platte; ein editiertes Modell hinterlässt seine Vorgänger). Listing-Reihenfolge beim
Reload nach mtime — grob Erstellungsreihenfolge, keine exakte Historie.

**Folgeaufgaben:** `docs/00-overview.md` §3 relativiert das Nicht-Ziel entsprechend
(Persistenz jetzt optional verfügbar, Cluster-Betrieb bleibt ausgeschlossen). Optionale
GC/Retention und eine explizite Lösch-API sind spätere WPs, falls der Bedarf entsteht.
