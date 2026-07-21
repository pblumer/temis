# Business Case: Benutzer-Onboarding & Email-Konto-Provisionierung

> **Status:** Iteration 2 (verifiziert gegen die `dmn`-Engine und live über MCP) · **Modell:** [`onboarding-email.dmn`](./onboarding-email.dmn)
> **Methode:** Roundtrip-Engineering — Mensch im Modeler, Agent über MCP, ein geteilter Modell-Cache.

Dieses Dokument beschreibt einen konkreten, ausführbaren Business Case für temis:
Wenn ein neuer Benutzer in der **Benutzerverwaltung** angelegt wird, soll das
**Email-Konto** (Postfach, Kontingent, Verteiler, Freigabe) **regelbasiert und
nachvollziehbar** bestimmt werden — statt manuell und uneinheitlich pro Ticket.

---

## 1. Problem & Nutzen

**Problem.** Beim Onboarding entscheiden heute Menschen ad hoc, welches Postfach ein
neuer Nutzer bekommt, wie groß es ist, in welche Verteiler er kommt und ob eine
Freigabe nötig ist. Das ist uneinheitlich, schlecht auditierbar und skaliert nicht.

**Nutzen mit temis.**

| Hebel | Wirkung |
|---|---|
| **Deterministisch** | Gleiche Merkmale → gleiches Konto. Kein „kommt drauf an". |
| **Nachvollziehbar** | Jede Auswertung liefert eine Explain-Spur: *welche Regel warum* gegriffen hat. |
| **Auditierbar** | Optionale clio-Anbindung protokolliert jede Entscheidung (ADR-0023). |
| **Reviewbar** | Regeln sind DMN-XML in git — Änderungen laufen über PRs (ADR-0022). |
| **Automatisierbar** | Das Onboarding-System ruft die Decision über REST/gRPC/MCP; kein Copy-Paste. |
| **Agentenfähig** | Ein Agent kann das Modell über MCP auswerten *und* weiterentwickeln. |

---

## 2. Scope (Iteration 1)

Fokus: **Onboarding / Provisionierung**. Eingang sind die Stammdaten des neuen
Nutzers; Ergebnis ist ein vollständiger **Provisionierungssatz** fürs Email-Konto.

Bewusst **noch nicht** enthalten (siehe [§7 Ausbaustufen](#7-ausbaustufen)):
Lifecycle (Mover/Leaver), Namens-/Adressgenerierung, Lizenz-/Kostenstellen-Logik,
Anbindung an ein reales Verzeichnis (AD/Entra/Keycloak).

---

## 3. Das Entscheidungsmodell (DRG)

```
   Rolle        Abteilung     Vertragsart
     │              │            │
     ├─────┐        │            │
     ▼     ▼        ▼            ▼
 Postfachtyp     Verteiler     Freigabe
     │              │            │
     └──────┬───────┴─────┬──────┘
            ▼             (+ BKM Postfachkontingent(Rolle))
     Email-Provisionierung   ◄─── Top-Decision (Boxed Context)
```

### 3.1 Eingaben (Enumerationen)

| Eingabe | Typ | Zulässige Werte |
|---|---|---|
| `Rolle` | string | `Fuehrungskraft`, `Manager`, `Mitarbeiter`, `Praktikant`, `Extern`, `Dienstkonto` |
| `Abteilung` | string | `Vertrieb`, `IT`, `HR`, `Finanzen`, `Marketing`, `Geschaeftsleitung` |
| `Vertragsart` | string | `Unbefristet`, `Befristet`, `Werkstudent`, `Extern` |
| `Vorname` / `Nachname` | string | freier Text (für die Adressgenerierung, Iteration 2) |
| `BelegteAdressen` | list&lt;string&gt; | bereits vergebene Adressen (optional; für die Kollisionsregel) |

> Rollen/Abteilungen/Vertragsarten bewusst umlautfrei (`Fuehrungskraft`,
> `Geschaeftsleitung`), damit die Eingaben aus Fremdsystemen stabil vergleichbar
> bleiben. Vor-/Nachname dürfen Umlaute haben — das BKM `Normalisiere` wandelt sie
> für die Adresse um.

### 3.2 Regeln

**`Postfachtyp`** (Hit Policy FIRST, je `Rolle`)

| Rolle | → Postfachtyp |
|---|---|
| Fuehrungskraft, Manager | Premium |
| Mitarbeiter, Praktikant | Standard |
| Extern | Gast |
| Dienstkonto | Funktionspostfach |
| *(sonst)* | Standard |

**`Postfachkontingent`** — wiederverwendbares **BKM** `Postfachkontingent(rolle)` → GB

| Rolle | → GB |
|---|---|
| Fuehrungskraft | 100 |
| Manager | 75 |
| Mitarbeiter | 50 |
| Praktikant, Extern | 25 |
| *(sonst)* | 10 |

**`Verteiler`** (FIRST, je `Abteilung`) → Liste von Mail-Adressen
(jede Abteilung erbt zusätzlich `alle@firma.de`; Fallback nur `alle@firma.de`).

**`Freigabe`** (FIRST, `Rolle` × `Vertragsart`) → boolean

| Bedingung | → Freigabe nötig |
|---|---|
| Rolle = Fuehrungskraft | true |
| Rolle = Extern | true |
| Vertragsart = Extern | true |
| *(sonst)* | false |

**`Primaeradresse`** (Literal, Iteration 2) → string — mit **BKM** `Normalisiere(text)`
(Kleinschreibung + Umlaute `ä→ae, ö→oe, ü→ue, ß→ss`, Leerzeichen entfernt). Basis:
`vorname.nachname@firma.de`; **Kollisionsregel:** steht diese schon in `BelegteAdressen`,
Fallback auf `initial.nachname@firma.de`. FEEL-Verkettung über `string join([...], "")`
(nicht `+`, sonst `TYPE_ERROR`).

### 3.3 Ergebnis: `Email-Provisionierung`

Top-Decision als **getypter** Boxed Context (Iteration 1: benanntes ItemDefinition
`EmailProvisionierung`) — bündelt alles zu einem Satz:

```feel
// variable typeRef = EmailProvisionierung
{
  primaerAdresse:       Primaeradresse,
  postfachTyp:          Postfachtyp,
  kontingentGB:         Postfachkontingent(Rolle),
  verteiler:            Verteiler,
  freigabeErforderlich: Freigabe
}
```

---

## 4. Verifikation

Kompiliert und ausgewertet gegen `package dmn` (deterministisch, ohne Raten) und
zusätzlich live über MCP (`evaluate`):

| Rolle / Abt. / Vertrag · Person | primaerAdresse | postfachTyp | kontingentGB | verteiler | freigabe |
|---|---|---|---|---|---|
| Führungskraft / GL / Unbefr. · Anna Müller | `anna.mueller@` | Premium | 100 | `gl@`, `alle@` | **true** |
| Mitarbeiter / IT / Unbefr. · Jörg Weiß | `joerg.weiss@` | Standard | 50 | `it@`, `alle@` | false |
| Mitarbeiter / IT / Unbefr. · Anna Müller *(`anna.mueller@` belegt)* | `a.mueller@` | Standard | 50 | `it@`, `alle@` | false |
| Extern / Vertrieb / Extern · — | — | Gast | 25 | `vertrieb@`, `alle@` | **true** |
| Dienstkonto / IT / Befristet · — | — | Funktionspostfach | 10 | `it@`, `alle@` | false |

> Umlaute werden normalisiert (`Müller→mueller`, `Weiß→weiss`); ist `vorname.nachname@`
> bereits in `BelegteAdressen`, greift der Alias-Fallback `initial.nachname@`.

---

## 5. Auswerten

**MCP (Agent):**
```
evaluate(model="Email-Onboarding", decision="Email-Provisionierung",
         inputs={"Rolle":"Manager","Abteilung":"IT","Vertragsart":"Unbefristet"},
         explain=true)
```

**REST (Onboarding-System):**
```sh
curl -s $TEMIS/api/decisions/Email-Provisionierung/evaluate \
  -H 'content-type: application/json' \
  -d '{"inputs":{"Rolle":"Manager","Abteilung":"IT","Vertragsart":"Unbefristet"}}'
```

> `explain=true` bzw. der Trace zeigt je Teil-Decision, welche Regel gegriffen hat —
> die Begründung fürs Ticket/Audit.

---

## 6. Roundtrip-Workflow (Mensch ⇄ Agent)

Beide arbeiten auf **einem** Modell-Cache (co-located `temisd`, ADR-0021):

1. **Agent** lädt dieses Modell (`load_model` mit dem XML, Name `Email-Onboarding`)
   oder git-basiert (`git_load_model`). Es erscheint im Modeler nach **⟳ Aktualisieren**.
2. **Mensch** passt Regeln im Modeler an (z. B. neue Abteilung), **speichert** →
   neue `modelId` (Chip in der Toolbar).
3. **Mensch** gibt dem Agenten die `modelId`; **Agent** liest mit `get_model_xml`
   das *echte* FEEL zurück, prüft mit `evaluate`/`explain`, korrigiert additiv und
   gibt die neue `modelId` zurück.
4. Reviewfähig festhalten über `git_propose` (Branch/PR).

> Übergabe-Etikette: `modelId` **in beide Richtungen** nennen, nichts still
> überschreiben — jede Version ist additiv.

---

## 7. Ausbaustufen

- ✅ **Strukturierter Ergebnistyp** *(Iteration 1 — umgesetzt)*. `Email-Provisionierung`
  als benanntes ItemDefinition `EmailProvisionierung` statt offenem Context → getypte Ausgabe.
- ✅ **Primäradresse & Alias-Generierung** *(Iteration 2 — umgesetzt)*.
  `vorname.nachname@firma.de` inkl. Umlaut-Normalisierung und Kollisionsregel.
- **Lifecycle.** Mover (Abteilungswechsel → Verteiler-Umzug) und Leaver
  (Sperre, Weiterleitung, Aufbewahrung) als eigener Decision-Flow (ADR-0026).
- **Lizenz & Kosten.** Postfachtyp → Lizenzstufe & Kostenstelle.
- **temis-eigene Zugriffe.** Rolle → Admin-Key-Scope / Public-Decision-Zugriff
  (ADR-0028/0035/0036) — falls der Case auf temis selbst gemünzt werden soll.
- **Regression-Test.** Die Szenarien aus §4 als Go-Test einfrieren.

---

## 8. Annahmen

- Mail-Domain `firma.de` ist Platzhalter.
- Rollen/Abteilungen/Vertragsarten sind geschlossene Enumerationen; unbekannte
  Werte fallen bewusst auf sichere Defaults (Standard-Postfach, 10 GB, nur `alle@`).
- Hit Policy FIRST mit Auffangregel → das Modell liefert **nie** `null`, auch bei
  unerwarteten Eingaben (bewusst gewählt, um die häufigste `null`-Falle zu vermeiden).
