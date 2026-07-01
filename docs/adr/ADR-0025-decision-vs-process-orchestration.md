# ADR-0025: Decision- vs. Prozess-Orchestrierung — Schichtenmodell und die temis↔chrampfer-Naht

- **Status:** proposed
- **Datum:** 2026-07-01
- **Kontext-WP:** übergreifend (Governance/Architektur); Bezug WP-70–73 (Git-Modelle), verfeinert ADR-0011, Bezug ADR-0016

## Kontext

In einer Unternehmung entstehen schnell **tausende DMN-Modelle**. Damit stellt sich
die Organisationsfrage: zentral oder pro Domäne verwalten, wie widersprechende Regeln
vermeiden, und **welche Schicht orchestriert** mehrere Entscheidungen zu einem
Geschäftsfall. Die naheliegende Antwort — „eine Orchestrierungs-Schicht darüber" —
verbirgt eine Zweideutigkeit, die über Erfolg oder Chaos entscheidet: **„Orchestrierung"
meint zwei grundverschiedene Dinge.**

1. **Decision-Orchestrierung (Decisioning).** „Was ist die Antwort, *jetzt*?" Mehrere
   Entscheidungen werden **zustandslos** zu einer Antwort komponiert — im Kern ein DRG,
   das über Modellgrenzen hinweg andere Decisions referenziert. Rein, deterministisch,
   ohne Zeit/Warten/Mensch — und damit **re-auditierbar** (`temis-reaudit`, ADR-0023).

2. **Prozess-Orchestrierung (Workflow).** „Was passiert *als Nächstes*, über die Zeit?"
   Langlebiger **durable state** (Sekunden bis Monate), Timer, Events, Human Tasks,
   Retries, Kompensation/Saga. Nichtdeterministisch (I/O, Uhr, Seiteneffekte).

Diese Trennung ist keine Feinheit, sondern die tragende Grenze der geplanten
Produktfamilie: **temis ist die erste fachliche Schicht (DMN), `pblumer/chrampfer` die
zweite (BPMN)** — so schon in `docs/20-roadmap.md` (BPMN-Synergie) und ADR-0016
angelegt. **ADR-0011** hält bereits fest, dass temis eine reine Go-Library bleibt,
*damit* eine durable BPMN-Engine DMN **in-process** aus einem *Business Rule Task*
aufrufen kann, ohne ihre Korrektheit an die Verfügbarkeit eines externen HTTP-Dienstes
zu koppeln.

Ohne eine dokumentierte Grenze droht die Vermischung: Geschäftsregeln wandern in
BPMN-Gateways (verstreut, untestbar, nicht ownership-fähig), oder temis bekommt
schleichend Zustand/Zeit/Retry hineingebaut und verliert Determinismus und
Re-Auditierbarkeit. Diese ADR zieht die Grenze **bevor** chrampfer Code produziert
und sie faktisch — womöglich falsch — zementiert.

Zur Klarstellung gegenüber `docs/00-overview.md` §3: das Nicht-Ziel **„Keine
BPMN/CMMN-Integration (nur DMN)"** bleibt gültig. temis orchestriert **keine Prozesse**.
Diese ADR *bestätigt* die Grenze und definiert nur die schmale Naht, an der ein externer
BPMN-Konsument (chrampfer) temis aufruft.

## Optionen

1. **Eine gemeinsame Orchestrierungs-Schicht** (Decision- und Prozess-Orchestrierung
   vermengt), z. B. temis um Zustand/Zeit/Human-Tasks erweitern. — Bricht ADR-0011
   (temis verliert Reinheit/Determinismus), macht `temis-reaudit` wirkungslos, dupliziert
   später BPMN-Funktionalität schlecht. Verworfen.

2. **Nur BPMN, DMN als bloße Ausdruckssprache in Gateways.** Keine eigenständige
   Decision-Schicht; Regeln leben in Prozess-Gateways. — Der klassische Anti-Pattern:
   Geschäftslogik verstreut über Diagramme, nicht testbar, nicht einzeln
   ownership-/versionierbar, nicht re-auditierbar. Verworfen.

3. **Zwei getrennte Orchestrierungs-Schichten, die ineinander stecken** (diese
   Entscheidung). Eine **stateless Decision-Orchestrierung in temis** (eine Decision, die
   andere Decisions komponiert) und eine **durable Prozess-Orchestrierung in chrampfer**,
   die temis über den Business Rule Task **in-process** aufruft. Klare, gerichtete
   Abhängigkeit chrampfer → temis. — Kosten: die Naht (Datenmapping, Determinismus-Grenze)
   muss diszipliniert eingehalten werden; genau das leistet diese ADR.

## Entscheidung

**Option 3.** Es gibt **zwei** Orchestrierungs-Schichten, nie eine. Sie nisten
ineinander; die Abhängigkeit zeigt ausschließlich **chrampfer → temis**, in-process,
nie zurück.

### Schichtenmodell

```
L3   Policy / Guardrails      DMN-Decisions, als Override aufgerufen        temis
L2b  Prozess-Orchestrierung   BPMN: durable state, Zeit, Mensch, Events     chrampfer
      └─ Business Rule Task ───────────────── in-process (ADR-0011) ───────┐
L2a  Decision-Flow            Decision komponiert Decisions, stateless      temis
L1   Domain Decisions         Pricing, Risk, Eligibility, …                 temis
L0   Foundation / Vocabulary  ItemDefinitions, Enums, geteilte BKM          temis
```

Alles **unterhalb** der Naht ist deterministisch und re-auditierbar; alles **oberhalb**
ist durable und beobachtbar. Jede Decision lebt in **genau einer** Schicht mit **genau
einem** Owner; Abhängigkeiten zeigen nur nach unten (L2a→L1→L0, kein L1↔L1).

### Litmus-Test (welche Schicht?)

Muss ein Schritt sich zwischen zwei Aufrufen etwas **merken**, **warten**, **wiederholen**
oder auf **Mensch/Event** reagieren → **Prozess (chrampfer/L2b)**. Ist es „gegeben diese
Inputs, berechne die Antwort jetzt" über mehrere Decisions hinweg → **Decisioning
(temis/L2a)**.

### Regeln für die Naht

1. **Decisions bleiben rein.** Kein I/O, keine Uhr (außer injiziert), kein Zustand.
   Alles Nichtdeterministische lebt im Prozess. Nur so bleibt die
   `clio`+`temis-reaudit`-Replaybarkeit (ADR-0023) je Einzel-Decision intakt.
2. **Der Prozess besitzt die Falldaten.** Er mappt Case-Variablen → Decision-Inputs →
   Outputs zurück in Case-Variablen. Eine Decision greift **nie** in Prozess-State. Der
   Kontrakt ist der bestehende (`describe_decision` + `strict`-Validierung, WP-52).
3. **Keine Geschäftslogik in BPMN-Gateways.** Ein Gateway fragt „welcher Pfad?"; die
   *Bestimmung* ist eine DMN-Decision, das Gateway verzweigt nur auf deren Output. So
   bleiben Regeln in DMN — testbar, auditierbar, ownership-fähig.
4. **Zwei komplementäre Logbücher.** clio/temis protokolliert *was entschieden wurde*
   (deterministisch nachrechenbar); chrampfer protokolliert *was passiert ist* (Prozess-
   Event-Stream). Sie ersetzen einander nicht.
5. **Gerichtete Abhängigkeit.** chrampfer importiert temis (`package dmn`, in-process);
   temis kennt keine Prozesse und bleibt reine Library (ADR-0011, `null` Transport-/
   Prozess-Importe).

### L2a — der explizite Decision-Flow

Die stateless Decision-Orchestrierung wird als **Decision Service modelliert, der andere
Decision Services referenziert** — ein DRG über Modellgrenzen. Er deklariert die
komponierten Modelle (per content-addressed `modelId`, ADR-0011/WP-70) und den Datenfluss
zwischen ihnen; er dehnt den DMN-Standard **nicht** und führt **keinen** Zustand ein.
Hier — und nur hier auf der Decisioning-Seite — wird stateless Vorrang zwischen
konkurrierenden Domänen-Ergebnissen aufgelöst (als *aufgerufene* Vorrang-Decision, nicht
als impliziter Aufrufreihenfolge-Zufall). Die konkrete Umsetzung ist in **ADR-0026**
entschieden: ein externer **JSON-Flow-Deskriptor** (statt DMN-`import`), ausgewertet von
einem eigenen `flow`-Package; diese ADR legt nur das Prinzip fest.

Der praktische Leitfaden (Schichten-Ownership, föderierte Governance, Repo-Layout,
Klassen von Regelkonflikten und ihre Gegenmaßnahmen) steht in
`docs/90-decision-organization.md`.

## Konsequenzen

**Positiv**
- Determinismus und Re-Auditierbarkeit (ADR-0023) bleiben auf der gesamten Decisioning-
  Seite intakt; Zustand/Zeit/Seiteneffekte sind sauber im Prozess isoliert.
- Geschäftsregeln bleiben in DMN (einzeln testbar, versionierbar, ownership-fähig) statt
  über BPMN-Diagramme verstreut.
- Die Grenze ist dokumentiert, **bevor** chrampfer sie faktisch zementiert; die geteilte
  Modeler-Toolchain (ADR-0016) und die In-Process-Einbettung (ADR-0011) fügen sich
  bruchfrei ein.
- Skaliert auf tausende Modelle über föderierte Governance
  (`docs/90-decision-organization.md`), ohne dass ein zentrales Team zum Flaschenhals wird.

**Negativ / Kosten**
- Die Naht (Datenmapping, Reinheits-/Determinismus-Grenze) verlangt Disziplin; ein „mal
  eben" in temis eingebauter Zustand oder eine Uhr bräche ADR-0011 und diese ADR.
- L2a (Decision-Flow über Modellgrenzen) ist noch **nicht** implementiert; bis dahin
  erfolgt Cross-Modell-Komposition durch externe Orchestrierung (mehrere `evaluate`).

**Folgeaufgaben**
- `docs/90-decision-organization.md` als praktischen Leitfaden pflegen (mit dieser ADR
  angelegt).
- Scope-Entscheidung für L2a getroffen in **ADR-0026**: externer JSON-**Flow-Deskriptor**
  (statt DMN-`import`), Umsetzung als Etappe „Decision-Flow" (WP-90–94).
- Bei Start von chrampfer: den Business-Rule-Task-Adapter gegen `package dmn` in-process
  bauen (nicht gegen `temisd`-HTTP), konform zu ADR-0011.
- ADR-0011 und ADR-0016 bleiben gültig; diese ADR verfeinert sie und ersetzt sie nicht.
