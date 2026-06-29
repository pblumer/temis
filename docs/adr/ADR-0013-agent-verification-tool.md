# ADR-0013: temis als Laufzeit-Verifikationswerkzeug für KI-Agenten (Agent-First-Schnittstelle)

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-50 / WP-51 / WP-52 (neue Etappe „Agent-First")

## Kontext

Ein KI-Agent (LLM) ist von Natur aus **probabilistisch**: Er erzeugt plausible
Antworten, ohne dass die Korrektheit garantiert ist. Für eine ganze Klasse von
Entscheidungen ist das genau das falsche Werkzeug — nämlich überall dort, wo es
eine *richtige* Antwort gibt, die durch **Regeln** definiert ist: Förderfähigkeit,
Preis-/Rabattstaffeln, Risiko-/Bonitätsklassen, Compliance-Prüfungen, Routing,
Berechtigungslogik. Lässt man den Agenten solche Entscheidungen „aus dem Bauch"
treffen, sind die Resultate nicht reproduzierbar, nicht prüfbar und gelegentlich
schlicht falsch — ohne dass es jemandem auffällt.

temis ist das natürliche Gegenstück dazu: eine **deterministische**, **spec-konforme**
(DMN 1.5 / FEEL / TCK) und **auditierbare** Entscheidungsmaschine. Dieselbe Eingabe
liefert *immer* dieselbe Ausgabe, und es lässt sich exakt nachvollziehen, *welche*
Regel *warum* gefeuert hat. Damit kann temis für einen Agenten zum
**Verifikations-Orakel / Grounding-Tool** werden:

> Statt eine regelbasierte Geschäftsentscheidung selbst zu „erfinden", **delegiert**
> der Agent sie an temis und erhält eine reproduzierbare, begründete Antwort zurück —
> eine verlässliche Grundwahrheit, gegen die er sich absichern und mit der er sein
> Handeln rechtfertigen kann.

**Wichtige Unterscheidung.** `docs/60-ai-agent-guide.md` adressiert bislang
ausschließlich Agenten, die temis **bauen** (Coding-Agenten als Contributor). Dieses
ADR eröffnet eine zweite, orthogonale Achse: Agenten, die temis zur **Laufzeit
konsumieren**. Genau das ist der Unterschied zwischen „ein HTTP-Service, den ein Agent
zufällig auch aufrufen kann" und „ein Werkzeug, das *für* Agenten gebaut ist". Bislang
ist Letzteres kein ausgesprochenes Produktziel — dieses ADR macht es zu einem.

Diese Entscheidung steht auf ADR-0011 (Core Engine als reine Go-Library, Service nur
Adapter): Die Agenten-Schnittstelle ist **kein** neues Primärartefakt und erhält keine
Sonderrechte gegenüber `package dmn`. Sie ist ein weiterer dünner Konsument derselben
Library — wie `service/` und `cmd/temisd` — und zieht **keine** Transport-/Protokoll-
Importe in den Engine-Kern.

## Optionen

1. **Status quo: Agenten nutzen die bestehende HTTP-`/v1`-API + OpenAPI.** Technisch
   bereits aufrufbar. — Aber nicht agenten-nativ: kein Tool-Discovery-Mechanismus, den
   ein Agent-Runtime out-of-the-box versteht; die Antwort liefert nur Outputs, keine
   nachvollziehbare Entscheidungsspur (das Kernbedürfnis „warum"); die Selbstbeschreibung
   der erwarteten Inputs/Typen ist nur lose. Der Agent kann temis benutzen, aber sich
   nicht wirklich *absichern*.
2. **Agent-First-Oberfläche als dünner Adapter über `package dmn`** (diese Entscheidung).
   Drei Säulen: ein **MCP-Server** als nativer Tool-Einstieg, eine strukturierte
   **Entscheidungsspur** (welche Regeln warum), und ein **scharfes Eingabe-Schema mit
   strenger Validierung**. — Bedient Agenten nativ, ohne den Engine-Kern zu verbiegen.
   Kosten: MCP ist ein neues, zu pflegendes Artefakt (inkl. Abhängigkeit); die Spur wird
   Teil des öffentlichen Verhaltens.
3. **Agenten-Affordanzen in den Library-Kern oder ein proprietäres Agenten-Protokoll
   einbacken.** — Verletzt ADR-0011 (Transport-/Protokoll-Importe im Engine-Package,
   „null Transport-Importe"), koppelt Korrektheit an ein Protokoll und schließt die
   In-Process-Einbettung von der Spur aus. Verworfen.

## Entscheidung

Option 2. temis bekommt eine **Agent-First-Schnittstelle** als dünnen Adapter über die
bestehende Library, getragen von drei Säulen:

1. **MCP-Server (größter Hebel, Punkt 1 / WP-50).** Ein eigenes Binary (z. B.
   `cmd/temis-mcp`), das `package dmn` exakt wie `service/` konsumiert und temis als
   natives Werkzeug über das Model Context Protocol anbietet: Modelle laden/auflisten,
   eine Decision samt Eingabe-Schema beschreiben, auswerten. Damit *entdeckt* und
   *benutzt* ein Agent-Runtime temis ohne handgeschriebenen HTTP-Klebstoff. Der
   MCP-Server importiert **kein** `internal/` und erhält keine Sonderrechte gegenüber
   anderen Adaptern (konsistent mit ADR-0005 / ADR-0011).
2. **Entscheidungsspur / Erklärbarkeit (WP-51).** Eine Auswertung liefert nicht nur den
   Output, sondern eine strukturierte, deterministische Begründung: welche Regel(n)
   gefeuert haben, mit welcher Hit Policy aggregiert wurde, welche Eingaben welche
   Bedingungen erfüllt/verfehlt haben. Das ist der Kern von „sich absichern": Der Agent
   (und der Mensch dahinter) kann die Entscheidung *rechtfertigen*, nicht nur ablesen.
   Die Spur ist eine **Ableitung der tatsächlichen Auswertung durch die Engine** — kein
   nachträglich vom LLM erzeugtes Rationalisat. Sie gehört deshalb in die Library
   (Erweiterung von `Result`/`Evaluate`), damit Library, Service *und* MCP-Server sie
   gleichermaßen erben.
3. **Agent-Schema & strenge Validierung (WP-52).** Selbstbeschreibung der erwarteten
   Inputs samt Typen sowie präzise, maschinenlesbare Validierungsfehler. Eine Fehleingabe
   führt zu einer *exakten* Rückmeldung („Feld X erwartet Number, erhielt String"), nie
   zu einem stillschweigend falschen Default. Der Agent bekommt so eine Grundwahrheit,
   gegen die er seine Eingaben prüfen kann, bevor er auf das Ergebnis vertraut.

**Produktversprechen an Agenten.** Determinismus und Reproduzierbarkeit sind die Zusage,
die temis vertrauenswürdig macht: gleiche Eingabe → gleiche Ausgabe → belastbare
Grundwahrheit. Die Erklärbarkeit macht diese Grundwahrheit *prüfbar*.

**Performance-Vorbehalt (aus ADR-0011).** Die Entscheidungsspur darf das
µs-Performance-Budget des Hot Path nicht ruinieren. Sie wird deshalb **opt-in**: Die
allokationsarme `Evaluate`-Pfad ohne Spur bleibt der Default; die Spur wird nur erzeugt,
wenn sie explizit angefordert wird (Agenten/Service tun das, hochfrequente
In-Process-Batches nicht).

## Konsequenzen

**Positiv**
- Agenten erhalten ein vertrauenswürdiges Orakel für regelbasierte Entscheidungen,
  statt sie zu „raten" — der eigentliche Produktzweck dieses ADR.
- Die Entscheidungsspur macht jede Auswertung auch für **Menschen** auditierbar
  (Compliance, Debugging, Nachvollziehbarkeit) — ein Wert über Agenten hinaus.
- Eine einzige Spur-Implementierung in der Library kommt Lib, Service *und* MCP zugute.
- MCP als nativer Einstieg senkt die Integrationshürde für Agent-Runtimes drastisch.
- Konsistent mit ADR-0005 / ADR-0011: kein neues Primärartefakt, kein
  Transport-Import im Kern.

**Negativ / Kosten**
- Die Entscheidungsspur wird Teil des öffentlichen Verhaltens/Vertrags (`Result`) und
  unterliegt damit der SemVer-Disziplin (`docs/40-api-contract.md`).
- Der MCP-Server ist ein neues, zu pflegendes Artefakt **und** bringt eine neue externe
  Abhängigkeit (MCP-SDK) — nach Goldener Regel 6 (`60-ai-agent-guide.md`)
  begründungs- und ADR-pflichtig (Folge-ADR zur SDK-/Transportwahl).
- Spur-Erzeugung kostet Allokationen; der opt-in-Pfad muss diszipliniert gehalten
  werden, damit der Default das Performance-Budget hält.

**Folgeaufgaben**
- Neue Roadmap-Etappe „Agent-First" mit **WP-50** (MCP-Server), **WP-51**
  (Entscheidungsspur in `Result`) und **WP-52** (Agent-Schema & strenge Validierung).
- `docs/60-ai-agent-guide.md` um die Unterscheidung **Agent-als-Contributor** (baut
  temis) vs. **Agent-als-Konsument** (nutzt temis zur Laufzeit) ergänzen.
- ~~Folge-ADR: Wahl von MCP-SDK und Transport (stdio vs. HTTP/SSE) inkl.
  Abhängigkeitsbegründung.~~ → **erledigt in ADR-0014** (Eigenbau über Standard­
  bibliothek, stdio; kein SDK).
- `docs/40-api-contract.md`: Entscheidungsspur als Teil der `Result`-Oberfläche
  spezifizieren (Form, Stabilität, opt-in-Schalter).
