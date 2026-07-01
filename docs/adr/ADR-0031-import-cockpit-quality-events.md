# ADR-0031: Import-Cockpit — Batch-Auswertung und Quality-Events auf Entitäten (entkoppelte Queue)

- **Status:** accepted
- **Datum:** 2026-07-01
- **Kontext-WP:** Modeler / Import-Cockpit (ADR-0016), clio-Kopplung (ADR-0023)

## Kontext

Das **Import-Cockpit** (dritter Modeler-Modus neben Design/Operate) lädt Testfälle aus
einer CSV/JSON-Vorlage und wertet sie als „Fließband" gegen die Engine aus. Zwei Kräfte
trafen aufeinander:

1. **Durchsatz.** Ein realistischer Stapel hat **tausende** Fälle. Die erste Fassung
   feuerte einen HTTP-Request **pro Fall**, streng sequenziell, mit künstlichen
   Animations-Pausen und einem Voll-Re-Render pro Datensatz — 5000 Fälle brauchten
   Minuten und froren den Browser ein. Die Engine selbst wertet 5000-mal in **~50 ms**
   aus; der Flaschenhals war ausschließlich das Zusammenspiel UI ↔ HTTP.
2. **Nachweisbarkeit.** Ein **Produktivlauf** soll pro Fall ein Ereignis ins
   revisionssichere Logbuch **[clio](https://github.com/pblumer/clio)** schreiben — aber
   **entkoppelt gequeuet**, damit die schnelle Batch-Antwort nicht an clios Durchsatz
   hängt. Ein **Testlauf** hingegen darf **nichts** schreiben. Die Events sollen als
   **Quality-Events auf Entitäten** liegen, damit Reports über **Verletzungen** je Entität
   möglich werden.

## Optionen

1. **Client-seitige Nebenläufigkeit** (Pool paralleler Fetches) — kein neuer Endpoint, aber
   5000 Round-Trips bleiben 5000 Round-Trips; „unter einer Sekunde" nicht erreichbar, und
   das Audit müsste der Browser gegen clio fahren (Token im Browser — unerwünscht).
2. **Batch-Endpoint, Auswertung server-seitig geschleift** — ein Round-Trip; das Audit lebt
   dort, wo Sink und clio-Token schon sind (`temisd`). Gewählt.
3. **Audit synchron im Request** — einfach, aber koppelt die Batch-Antwort an clios
   Durchsatz und Verfügbarkeit; ein 5000er-Lauf würde auf 5000 clio-Writes warten.
4. **Audit über eine entkoppelte, garantierte Queue** — Antwort kehrt sofort zurück,
   Hintergrund-Worker liefern mit Retry/Idempotenz; Backpressure statt Verlust. Gewählt.

## Entscheidung

- **Batch-Endpoint** `POST /v1/models/{id}/evaluate-graph-batch`: viele Eingabezeilen in
  **einem** Request, die Engine schleift **in-memory** (ohne Traces, um die Antwort klein
  zu halten). Jede Zeile wird **unabhängig** ausgewertet — eine abgelehnte (strict) oder
  fehlgeschlagene Zeile wird als deren `problem` gemeldet und bricht den Batch **nicht** ab;
  die Antwort ist **1:1** zur Eingabe. Cap von 100000 Zeilen, respektiert ctx-Cancellation.
- **Cockpit**: ruft den Batch **einmal** statt einer Schleife auf, ohne künstliche Pausen;
  jede Lane **begrenzt** die gezeichneten Karten (Zähler + Overflow-Hinweis), und die
  Animation ist bewusst nur *angedeutet* (gestaffelte CSS-Kaskade) — so bleibt ein
  5000-Fälle-Lauf sofort, statt tausende DOM-Knoten einfrieren zu lassen.
- **Test- vs. Produktivlauf**: Ein **Testlauf** (Default) wertet nur lokal aus und schreibt
  **nichts**. Ein **Produktivlauf** (`record: true`) schreibt pro **ausgewertetem** Fall ein
  **Quality-Event** `com.temis.quality.evaluated.v1` — Subject ist die **Entität**
  (`/quality/<entity>`), Daten tragen Modell, Entität, Fall, Eingabe, Ergebnisse, erwartete
  Werte und ein **`violation`-Flag** (true/false, bzw. weggelassen ohne Erwartung). Die
  Entität kommt aus einer **`entity`-Spalte** der Vorlage, sonst einem konfigurierbaren
  **Eingabefeld** (SubjectKey), sonst dem Fall-Label. Ist keine clio-Queue konfiguriert,
  wird ein Produktivlauf klar mit **`409 CLIO_NOT_CONFIGURED`** abgelehnt.
- **Entkoppelte, garantierte Queue** (`QualityQueue`): der Request-Pfad **enqueued**
  (blockiert bei vollem Puffer = **Backpressure**), Hintergrund-Worker liefern an clio und
  **retrien mit Backoff, bis clio annimmt**. Zustellung ist damit **garantiert für die
  Prozesslaufzeit**; clios idempotente Precondition (Subject + `inputHash`) macht Retries
  sicher. `temisd` fährt einen **Graceful-Shutdown**, der die Queue vor dem Beenden unter
  Deadline drainiert.

Die Kopplung bleibt wie bei ADR-0023 **rein über clios HTTP-API** — kein Go-Import, reine
Standardbibliothek, opt-in (ohne `TEMIS_CLIO_TOKEN` verlässt kein Event den Prozess).

## Konsequenzen

- **Positiv:** 5000 Fälle laufen in **~50 ms** statt Minuten; das UI friert nie ein.
  Produktivläufe erzeugen **entitätsbezogene Quality-Events** für Verletzungs-Reports, ohne
  die schnelle Antwort zu bremsen und ohne bei transientem clio-Ausfall zu verlieren.
  Testlauf und Produktivlauf sind sauber getrennt; das Produktiv-Log bleibt vom bloßen
  Ausprobieren unberührt.
- **Negativ / Grenzen:** „Garantiert" gilt **prozesslebenslang** — ein harter Crash (kein
  Graceful-Shutdown) verliert noch nicht zugestellte Events; volle Persistenz bräuchte eine
  Platten-gestützte Queue (Folgeaufgabe). Der Batch verzichtet bewusst auf Traces; ein
  einzeln erklärter Lauf geht weiter über `evaluate-graph`. Nur **ausgewertete** Fälle
  werden als Quality-Event geschrieben; abgelehnte/fehlgeschlagene Zeilen erscheinen in der
  Antwort, aber nicht im Log.
- **Folgeaufgaben:** optional persistente Queue (Crash-Sicherheit), Reporting-Views/Queries
  über `violation`-Events in clio, sowie Testfall-Erzeugung durch den Assistenten.
