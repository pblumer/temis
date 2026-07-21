# Changelog

Alle nennenswerten Änderungen an Temis werden hier dokumentiert.

Das Format orientiert sich an [Keep a Changelog](https://keepachangelog.com/de/1.1.0/),
die Versionierung an [Semantic Versioning](https://semver.org/lang/de/). Der SemVer-Vertrag
gilt für die öffentliche Go-API (`package dmn`) und die HTTP-API (ADR-0019,
`docs/40-api-contract.md §4`); `internal/` ist ausgenommen.

> **Pflege:** Neue Einträge unter `[Unreleased]` sammeln. Beim Release den Abschnitt in
> eine Versionsüberschrift `[x.y.z] - JJJJ-MM-TT` umbenennen, einen neuen leeren
> `[Unreleased]` anlegen und den Tag `vx.y.z` setzen — die Release-Pipeline
> (`.github/workflows/release.yml`) zieht die Notizen dieses Abschnitts in den
> GitHub-Release.

## [Unreleased]

Vor-1.0-Entwicklung. Bis zum ersten getaggten Release tragen die Binaries die Version
`0.0.0-dev`. Bisher umgesetzt (Auszug, voller Stand in `docs/20-roadmap.md`):

### Security

- **Härtungs-Etappe H2 (WP-137–139, aus dem Code-Qualitäts-Audit).** CI-Härtung: neuer
  `govulncheck`-Job, Docker-Image-Smoke-Build je PR, durchgesetztes Coverage-Gate
  (`make cover`, ≥ 90 % auf den korrektheitskritischen Paketen), `go-version-file: go.mod`
  statt hart codierter Version, Dependabot (gomod/npm/actions) und ein Nightly-Fuzz-Sweep.
  Neue Governance-Dateien `SECURITY.md` (Meldeweg + dokumentierte Default-Posture),
  `CONTRIBUTING.md` und `CODEOWNERS`.
- **Mindest-Go-Version auf 1.24 angehoben.** Mehrere vom `govulncheck`-Gate gemeldete
  stdlib-CVEs (u. a. GO-2025-4007, quadratische `crypto/x509`-Name-Constraint-Prüfung,
  erreichbar über `ListenAndServeTLS`) sind ausschließlich in Go 1.24.9+ gefixt und werden
  nicht in die EOL-1.23-Linie zurückportiert. Bauen auf 1.24 ist daher zur echten Behebung
  nötig (nicht nur, um den Scan grün zu bekommen); die Security-CI-Lane scannt mit dem
  jeweils aktuellen Stable-Go. `go.mod` (`go 1.24.0`), Dockerfile und Doku entsprechend.
- **Härtungs-Etappe H1 (WP-130–135, aus dem Code-Qualitäts-Audit `docs/audits/`).** Behebt die
  im Audit verifizierten kritischen/hohen Befunde:
  - **Kein Prozess-Crash mehr durch Eingaben (K1):** FEEL-Parser und DMN-XML-Decoder hatten kein
    Rekursionstiefen-Limit; eine tiefe Eingabe (innerhalb des HTTP-Body-Limits) löste
    `fatal error: stack overflow` aus und riss den ganzen Prozess mit. Jetzt begrenzt
    (`DefaultMaxParseDepth`, `DefaultMaxElementDepth`) → Diagnostic statt Absturz (ADR-0008).
  - **Kein HTML-Injection/Stored-XSS im Modeler (H1/H2):** ein einheitlicher `escapeHtml()`
    (inkl. Anführungszeichen) ersetzt drei uneinheitliche Escaper; Typ-Dropdown baut über den
    DOM. BYOK-LLM-Key default nur in `sessionStorage`.
  - **Timeouts & TLS-Transparenz (H4/H5/M1):** LLM-/GitHub-Aufrufe mit Client-Timeout,
    HTTP-Server mit `ReadHeaderTimeout`/`IdleTimeout`, optionales `-tls-cert`/`-tls-key`,
    expliziter Klartext-Hinweis beim Start.
  - **Missbrauchs-Schutz (H6/M2):** opt-in Rate-Limit (`-rate-limit`) pro Client-IP; Startup-
    Warnung, wenn ein LLM-Server-Key ohne API-Auth einen offenen Kosten-Proxy ergäbe.
  - **First-Run repariert (H3):** der Modeler auf einem leeren Server verdrahtet jetzt alle
    Aktionen (kein früher `boot()`-Abbruch mehr).

### Fixed

- **`DELETE /v1/models/{id}` ist mit `-models-dir` dauerhaft (M3):** löschte bisher nur den
  Cache, sodass ein persistiertes Modell beim nächsten Zugriff zurückkehrte.
- **Testsuite offline vollständig grün (M5):** die Scope-Autorisierungs-Tests rufen nicht mehr
  die echte GitHub-API, sondern ein Fake-Backend.
- **GitHub-Pfad-Traversal abgewiesen (N6)**, `AuthKid` auch bei Whole-Graph-Evals gestempelt
  (N7), begrenzte Dedupe-Menge im clio-Worker (M4).

### Added

- **MCP: Typ-Werkzeuge `list_types`, `save_type`, `delete_type`.** Ein Agent kann die
  eigenen Item-Definitionen eines gecachten Modells lesen (`list_types`, Scope
  `models:read`) sowie einen einfachen Typ (Basistyp + optional Collection + Allowed-Values)
  anlegen/ändern (`save_type`) oder entfernen (`delete_type`, beide `models:write`). Sie
  spiegeln die HTTP-Endpunkte `…/types`; eine Änderung liefert eine neue content-adressierte
  `modelId` und erscheint in der Typen-Verwaltung des Modelers. Strukturierte Typen bleiben
  dem vollen `load_model`-XML vorbehalten.
- **Agent-Zusammenarbeit: Co-Modeling-Vertrag als MCP-`instructions` + Repo-Skill.** Der
  MCP-Server liefert beim `initialize` jetzt ein `instructions`-Feld, das jedem verbundenen
  Agenten den Vertrag für gemeinsames Modellieren mit einem Menschen mitgibt (Agent via MCP,
  Mensch via Modeler, geteilter Cache): Modell per `modelId`/Toolbar-Chip finden (Name nicht
  eindeutig), `get_model_xml` vor dem Ändern lesen, mit `evaluate`/`explain` diagnostizieren,
  als neue Version zurückgeben — inklusive der häufigsten `null`-Fallen (Unary-Test in
  Tabellen-Eingabezellen, leeres BKM, `typeRef`=leer→Any). Die ausführliche Fassung liegt als
  Skill unter `.claude/skills/temis-decision-modeling/` (mit ausgearbeiteten Vorher/Nachher-
  Beispielen in `references/dmn-feel-traps.md`).
- **MCP: Modellname in `list_models` und neues Tool `get_model_xml`.** `list_models`
  liefert je Modell zusätzlich den Anzeigenamen (den DMN-`definitions`-Namen, wie im Modeler),
  sodass ein Agent ein ihm bekanntes Modell wiederfindet — der Name ist kein eindeutiger
  Schlüssel, da jede gespeicherte Revision eine eigene content-adressierte `modelId` ist. Das
  neue Tool **`get_model_xml`** liest das rohe DMN/FEEL eines gecachten Modells zurück (Scope
  `models:read`), analog zum HTTP-Endpunkt `GET /v1/models/{id}/xml` — ein Agent kann so die
  FEEL-Ausdrücke inspizieren, nicht nur auswerten.
- **DMN-TCK-Konformität: über 98 % (WP-41.28, 98,0 % → 98,1 %).**
  Vier weitere Fixes: ein `some`/`every` mit echt nicht-boolescher `satisfies`-Klausel ergibt null
  (dieselbe Regel wie beim Boxed-Filter, jetzt auch für Quantoren); `list replace(match: …)` bindet
  die benannte Match-Funktions-Form; `string join` ist auf seine DMN-Formen mit 1–2 Argumenten
  beschränkt (das nicht-standardisierte Prefix/Suffix entfällt, ein 3. Argument ergibt null); und
  ein Kontext-Literal mit doppelten Schlüsseln (`{foo: 1, foo: 2}`) wertet zu null aus statt
  Last-Wins. +4 Cases (1153, 1155, 1140, 0057); Floor 98,1 %.
- **DMN-TCK-Konformität: Weg zu 98 % (WP-41.27, 97,5 % → 98,0 %). 🎯**
  Ein gemischtes Bündel über die 98-%-Marke: die Temporal-Konstruktoren binden ihre benannten
  Komponentenformen (`date(year:…, month:…, day:…)`, `time(hour:…, minute:…, second:…, offset:…)`,
  `date and time(date:…, time:…)`) über eine alternative Signatur (`Builtin.AltParams`) neben `from:`;
  Dauer-Literale erlauben fraktionale Sekunden (`duration("PT0.5S")`, `PT0.S`); Dauer × Zahl trunkiert
  Richtung Null (`-2.5 * @"P1Y11M"` → `-P4Y9M`); `xsd:string`-Erwartungswerte werden nicht mehr
  getrimmt (`upper case("xyZ ")` → `"XYZ "`); ein ungültiges `@`-Literal (`@"foo"`) wertet zu null aus
  statt Compile-Fehler; `context merge(contexts: …)` und `context put(… , keys: […], …)` binden benannt;
  und ein Boxed-Filter mit echt nicht-boolescher Bedingung ergibt null. +14 Cases
  (1115–1117, 1120, 0100, 1103–1109, 0093, 1146/1147, 1151); Floor 98,0 %.
- **DMN-TCK-Konformität: Named-Arg-Arity, decimal-Skala & Conditional-Semantik (WP-41.26, 97,4 % → 97,5 %).**
  Ein benannter Funktionsaufruf darf optionale Parameter weglassen (sie defaulten auf null) statt an der
  Arity zu scheitern (`is(value1: x)` → false); `decimal`/`floor`/`ceiling` floorn eine fraktionale Skala
  (`decimal(1/3, 2.5)` → 0,33); und eine echte nicht-boolesche `if`-Bedingung ergibt null (false/null
  nehmen weiter den Else-Zweig). +4 Cases (0103, 1100, 1150); Floor 97,4 %.
- **DMN-TCK-Konformität: Unary-Test-Membership & Punkt-Namen (WP-41.25, 97,3 % → 97,4 %).**
  Ein Decision-Table-Unary-Test, dessen Wert eine Liste ist, ist jetzt ein Membership-Test (`? in e`)
  statt Gleichheit (ein Intervall testet Containment, ein Skalar reduziert auf Gleichheit); und FEEL-Namen
  dürfen einen Punkt enthalten (`Person.Gender`), sodass BKMs mit qualifizierten Formal-Parametern
  kompilieren — normaler Pfad-Zugriff `a.b` navigiert unverändert. +4 Cases (0039, 0037); Floor 97,3 %.
- **DMN-TCK-Konformität: Aggregat-/Builtin-Randfälle (WP-41.24, 96,9 % → 97,3 %).**
  Ein Bündel Funktions-Randfälle: die variadischen Aggregate (`all`/`any`/`sum`/`count`/`mean`/`min`/
  `max`/`median`/`stddev`/`mode`/`product`) akzeptieren jetzt das Einzel-Kollektions-Argument benannt
  (`fn(list: […])`); `mode(null)` ist `null`; `substring` akzeptiert fraktionale Position/Länge (gefloort);
  und ein bloßer Built-in-Name (`abs`, `sqrt`) hebt sich zu einem First-Class-Funktionswert, der an eine
  BKM oder Higher-Order-Funktion übergeben werden kann. +11 Cases; der Ratchet-Floor steigt auf 97,2 %.
- **DMN-TCK-Konformität: `instance of` mit generischen & benutzerdefinierten Typen (WP-41.23, 96,5 % → 96,9 %).**
  `instance of` prüft jetzt das volle Typsystem: parametrisierte Generics (`list<T>`, `context<a: T, …>`,
  verschachtelt und leer) werden geparst und strukturell verglichen, benutzerdefinierte Item-Typen
  (`t255`, `t_context_013`, …) werden aufgelöst (die Typen fließen über ein neues `types`-Feld an der
  internen `feel.Env`), und `null instance of X` ist für jeden Typ `false`. +15 Cases (0070 vollständig
  127→142); der Ratchet-Floor steigt auf 96,9 %.
- **DMN-TCK-Konformität: Zahl-Vergleich mit der TCK-Präzision (WP-41.22, 96,1 % → 96,5 %).**
  Die Engine rechnet spec-konform in decimal128 (34 Stellen, ADR-0007) und liefert für transzendente/
  irrationale Ergebnisse (`exp`/`log`/`sqrt`/`**`/Statistik/Zinseszins) mehr Stellen als die auf endliche
  Präzision gerundeten TCK-Erwartungswerte. Der TCK-Runner vergleicht zwei Zahlen jetzt, indem er das
  Ist-Ergebnis auf die Dezimalstellen-Zahl des Erwartungswerts rundet — additiv (exakte Arithmetik und
  ganzzahlige Erwartungen bleiben streng, echte Abweichungen scheitern weiterhin). Reine Test-Harness-
  Änderung, die Engine bleibt unberührt. +16 Cases; der Ratchet-Floor steigt auf 96,5 %. Wenige
  Zinseszins-Fälle bleiben offen, weil der TCK-Referenzwert selbst in float64-Genauigkeit erzeugt wurde
  (dokumentiert in `docs/tck-exceptions.md`).
- **DMN-TCK-Konformität: Typ-Koerzierung an Aufruf-Grenzen (WP-41.21, 95,8 % → 96,1 %).**
  Die FEEL-Item-Definition-Koerzierung (DMN §10.3.2.9.4) greift jetzt auch an Funktions- und
  Service-Aufruf-Grenzen, nicht nur an Decision-Outputs: ein Argument, das nicht zum deklarierten
  Parametertyp passt (auch nach Singleton-Unwrap), macht den ganzen Aufruf `null` („Funktion nicht
  invoziert"); der Rückgabewert wird auf den deklarierten Typ koerziert. Gilt für BKMs und für
  Decision-Service-Aufrufe (neue `ParamTypes`/`ResultType` an der internen `feel.Func`); zusätzlich
  koerziert `Service.Evaluate` seine Single-Output-Ausgabe auf den Service-Typ. Die Koerzierungs-Logik
  wohnt jetzt geteilt in `internal/feel`. +10 Cases (0082 23→31, 0085 16→18); der Ratchet-Floor des
  CI-Gates steigt auf 96,0 %.
- **DMN-TCK-Konformität: Decision Services aus FEEL aufrufbar (WP-41.20, 95,6 % → 95,8 %).**
  Ein Decision Service kann jetzt aus dem FEEL-Ausdruck einer Decision heraus per Namen aufgerufen
  werden (`Svc("bar")`, `Svc(inputData_x: …, decision_y: …)`) — DMN §10.4. Die Parameter sind die
  Input-Data gefolgt von den Input-Decisions des Service (in Deklarationsreihenfolge, positional oder
  benannt); eine einzelne Output-Decision liefert deren Wert, mehrere einen Kontext. Umgesetzt über ein
  neues optionales `Native`-Feld an der internen `feel.Func`. +5 Cases (0085); der Ratchet-Floor des
  CI-Gates steigt auf 95,7 %.
- **DMN-TCK-Konformität: Rundungs-Skala, `**`-Präzedenz & Time-Rendering (WP-41.19, 95,1 % → 95,6 %).**
  Numerische/temporale Randfälle: (1) die Rundungsfunktionen (`round …`, `decimal`, `floor`,
  `ceiling`) verlangen eine Skala in `[-6111, 6176]`, außerhalb → `null`, und eine große gültige Skala
  lässt den Wert unverändert statt zu überlaufen; (2) `**` ist links-assoziativ und bindet loser als
  unäres Minus (`3 ** 4 ** 5` = `(3**4)**5`, `-5 ** 2` = 25, beide per TCK); (3) Zeit-Offsets mit
  Sekunden-Anteil rendern als `±HH:MM:SS`, und `time(date)` ergibt `00:00:00Z`. +19 Cases
  (1141–1144 je +3, 0100 +2, 1116 +3); der Ratchet-Floor des CI-Gates steigt auf 95,6 %.
- **DMN-TCK-Konformität: 95 % erreicht — number()-Validierung, range()-Konstruktoren & Regex (WP-41.18, 94,5 % → 95,1 %).**
  Die Etappe, die das WP-41-Endziel (≥ 95 % der Cases) knackt. Vier Fixes: (1) `number()` validiert
  seine Separatoren (grouping/decimal müssen gültig, verschieden und Strings sein, sonst `null`);
  (2) `range()` parst Konstruktor-Endpunkte wie `date("…")`/`duration("…")` gleichwertig zu `@"…"`;
  (3) `replace()` bildet FEEL-`$N`-Gruppenreferenzen auf Gos `${N}` ab und setzt das `x`-Flag durch
  Whitespace-Strippen um (RE2 kennt `(?x)` nicht); (4) unbekannte String-Escapes (`\d`, `\.`, `\s`)
  werden verbatim durchgereicht, sodass Regex-Muster als FEEL-String-Literale kompilieren. +21 Cases
  (1111 +9, 0058 +4, 1156 +4, 1109 +3); der Ratchet-Floor des CI-Gates steigt auf 95,0 %.
- **DMN-TCK-Konformität: Invocation-Null, Zahl-Wort-Namen & Default-Output (WP-41.17, 93,6 % → 94,5 %).**
  Drei kaskadierende Fixes: (1) der Aufruf eines unbekannten Namens oder eines Nicht-Funktions-Werts
  (`123()`, `"x"()`, `true()`) ergibt `null` statt die Decision nicht-ausführbar zu machen
  (Total-Funktions-Semantik); (2) Name-Referenzen assemblieren über Zahl-Wörter (`Extra days case 1`)
  und `-`+Zahl (`K-MatchesFunc-1`) hinweg, wenn das Orakel den Namen kennt; (3) Entscheidungstabellen
  werten `defaultOutputEntry` aus — trifft keine Regel, liefert die Tabelle den Default-Output statt
  `null` (Single-Hit-Policies und Collect-mit-Aggregation). +30 Cases (1131 8→0, 0020 0→7, 0034 u. a.);
  der Ratchet-Floor des CI-Gates steigt auf 94,4 %.
- **DMN-TCK-Konformität: `in`/Range mit null-Endpunkten (WP-41.16, 93,4 % → 93,6 %).**
  Der `in`-Operator ist eine 3-wertige Disjunktion: ein null-Testwert gegen eine Range oder ein
  expliziter null-Endpunkt ergibt `null` statt `false` (`null in [1..10]`, `5 in [null..10]` → `null`);
  ein weggelassener (unbounded) Endpunkt bleibt unberührt. Range-Gleichheit unterscheidet jetzt
  einen unbounded von einem expliziten null-Endpunkt (`(< 10)` ≠ `(null..10)`). +9 Cases (0072 5→0,
  0068 4→0); der Ratchet-Floor des CI-Gates steigt auf 93,6 %.
- **DMN-TCK-Konformität: Bindestrich-Namen & fraktionale `time`-Sekunden (WP-41.15, 92,1 % → 93,4 %).**
  FEEL-Namen dürfen einen Bindestrich enthalten (`Date-Time`, `Pre-bureauRiskCategory`): Der
  Parser assembliert eine Referenz über den `-` hinweg zu einem Namen, sobald das Namens-Orakel
  ihn kennt — dafür fließen die Umgebungs-Variablennamen einer Decision ins Orakel ein. Ein bloßes
  `a - b` ohne gleichnamige Variable bleibt Subtraktion. Zusätzlich akzeptiert die
  `time(h, m, s, offset?)`-Konstruktorform eine fraktionale Sekunde (`time(12,59,1.3,-PT1H)` →
  `12:59:01.3-01:00`). +43 Cases (0007 15→0, 0004-lending 7→0, 0087 7→0 u. a.); der Ratchet-Floor
  des CI-Gates steigt auf 93,3 %.
- **DMN-TCK-Konformität: Kontext-Eintrags-Referenzen & string join (WP-41.14, 92,0 % → 92,1 %).**
  Ein Kontext-Eintrag kann jetzt die vor ihm deklarierten Einträge referenzieren
  (`{a: 1+2, b: a+3}` → `{a:3, b:6}`); und `string join(null)` ergibt `null` statt `""`.
  +4 Cases; der Ratchet-Floor des CI-Gates steigt auf 92,1 %.
- **DMN-TCK-Konformität: FEEL-Kommentare (WP-41.13, 91,9 % → 92,0 %).** Der Lexer
  überspringt jetzt `// …`-Zeilen- und `/* … */`-Block-Kommentare. +3 Cases (0073: 3→0);
  der Ratchet-Floor des CI-Gates steigt auf 92,0 %.
- **DMN-TCK-Konformität: `for`/Quantifier über Ranges (WP-41.12, 91,6 % → 91,9 %).**
  `for i in a..b` (und `some`/`every`) enumeriert jetzt neben Zahlen-Ranges auch
  Date-Ranges tageweise (auf-/absteigend); Ranges anderer Typen (String, date-and-time,
  time, Dauer, unbounded) sind nicht iterierbar → das Ergebnis ist `null` statt einer
  leeren Liste. +10 Cases (0084: 13→3, 0016: 5→2); Ratchet-Floor auf 91,9 %.
- **DMN-TCK-Konformität: Unicode-String-Escapes (WP-41.11, 91,4 % → 91,6 %).** Der
  String-Lexer dekodiert jetzt `\U`-Escapes (6-Hex-Codepoint) und kombiniert UTF-16-
  Surrogatpaare (`\uD83D\uDCA9` → ein Codepoint), sodass `string length` Codepoints
  korrekt zählt. +7 Cases (0083: 9→2); der Ratchet-Floor des CI-Gates steigt auf 91,6 %.
- **DMN-TCK-Konformität: `is()` auf Temporalen (WP-41.10, 91,2 % → 91,4 %).** `is(v1, v2)`
  vergleicht für `date`/`time`/`date and time` jetzt die Repräsentation statt des Instants:
  `is(@"23:00:50", @"23:00:50Z")` → false (gleicher Instant, andere Darstellung). +9 Cases
  (0103: 11→2); der Ratchet-Floor des CI-Gates steigt auf 91,4 %.
- **DMN-TCK-Konformität: `range()`-Validierung (WP-41.9, 90,8 % → 91,2 %).** Die
  `range(from)`-Builtin weist malformte Range-Strings jetzt als `null` ab: unbounded
  Endpunkt mit geschlossener Klammer (`[1..]`), Typ-Mismatch (`[1.."b"]`, date/dateTime)
  und umgekehrte Grenzen (`[3..1]`, `["z".."a"]`). +12 Cases (1156: 16→4); der
  Ratchet-Floor des CI-Gates steigt auf 91,0 %.
- **DMN-TCK-Konformität: Range-Literale aus Vergleichen (WP-41.8, 90,6 % → 90,8 %).**
  `(< v)`, `(<= v)`, `(> v)`, `(>= v)`, `(= v)` parsen jetzt als halb-/geschlossene
  Range-Literale (`(<10)` → `(..10)`; `(>=10)` → `[10..)`; `(=10)` → `[10..10]`), inkl.
  unbounded Grenzen und Range-Membership (`5 in (>3)`). +7 Cases (0074 komplett grün);
  der Ratchet-Floor des CI-Gates steigt auf 90,8 %.
- **DMN-TCK-Konformität: Cross-Typ-Gleichheit → null (WP-41.7, 90,3 % → 90,6 %).** `=` und
  `!=` zwischen zwei nicht-null-Werten unterschiedlichen Typs ergeben jetzt `null` statt
  `false` (`100 = "100"`, `[] = 0`, `{} = []` → null; DMN §10.3.2.7). Chirurgisch nur an den
  `=`/`!=`-Operatoren; das interne Gleichheits-Prädikat für Decision-Tables/`in`/`contains`
  bleibt boolesch. +12 Cases; der Ratchet-Floor des CI-Gates steigt auf 90,6 %.
- **DMN-TCK-Konformität: `instance of` Funktionstypen (WP-41.6, 90,0 % → 90,3 %).** Der
  Parser akzeptiert jetzt Funktionstyp-Ausdrücke `function<…> -> ReturnType` in
  `instance of` (`function` ist ein Keyword); Signatur wird verworfen, gematcht wird die
  Funktions-Art. +10 Cases; der Ratchet-Floor des CI-Gates steigt auf 90,3 %.
- **DMN-TCK-Konformität: Collection-Funktionen — 90 % erreicht (WP-41.5, 89,6 % → 90,0 %).**
  Drei Builtins vervollständigt (+16 Cases): `context put` mit Pfad-Liste für
  verschachtelte Updates (`context put({x:1,y:{a:0}}, ["y","a"], 2)` → `{x:1,y:{a:2}}`);
  `context(entries)` akzeptiert einen einzelnen Entry unverpackt und liefert bei
  Duplikat-Keys `null`; `list replace` mit Singleton-Koerzierung, Positions-Truncation
  und null bei Match-Funktion falscher Arity/Nicht-Boolean-Ergebnis. Damit ist die
  **90-%-Marke** der offiziellen DMN-TCK-Konformität erreicht; der Ratchet-Floor des
  CI-Gates steigt auf 90,0 %.
- **DMN-TCK-Konformität: `in`-Operator & `abs` (WP-41.4, 89,0 % → 89,6 %).** `X in (= Y)`
  und `X in (!= Y)` — ein parenthesierter Operator-Unary-Test ohne Komma — parsen jetzt
  (`10 in (=10)` → true); und `abs` liefert auch für beide Dauer-Typen den Betrag. +20
  Cases; der Ratchet-Floor des CI-Gates steigt auf 89,5 %.
- **DMN-TCK-Konformität: Property-Zugriff auf Temporale & Ranges (WP-41.3, 88,7 % → 89,0 %).**
  FEEL-Member-Namen mit Leerzeichen (`x.time offset`, `[1..10].start included`) parsen
  jetzt — der Parser assembliert den ganzen Namens-Lauf nach `.`. `value.Member`
  exponiert zudem Range-Properties (`start`, `end`, `start included`, `end included`).
  +9 Cases; der Ratchet-Floor des CI-Gates steigt auf 88,9 %.
- **DMN-TCK-Konformität: Runner dekodiert item-verpackte Listen (WP-41.2, 85,6 % → 88,7 %).**
  Der TCK-Runner las erwartete Listen bisher nur als `<list><value>…`; das offizielle
  Korpus nutzt breit auch `<list><item><value>…` (inkl. verschachtelter Listen und
  Kontext-Items), was als leere Liste fehlgelesen wurde und viele korrekte Engine-
  Ergebnisse fälschlich als Fehlschlag zählte. Reiner Harness-Fix (keine Engine-
  Änderung): +108 Cases; der Ratchet-Floor des CI-Gates steigt auf 88,5 %.
- **DMN-TCK-Konformität: FEEL-Invocation-Fehlersemantik (WP-41.1, 82,1 % → 85,6 %).**
  Ein syntaktisch gültiger Funktionsaufruf mit falscher Argument-Anzahl oder
  unbekanntem·gemischtem benanntem Parameter ergibt jetzt zur Laufzeit `null` und
  lässt die Decision ausführbar (FEEL-Total-Funktions-Semantik), statt sie als „nicht
  ausführbar" abzubrechen (`round up()`, `modulo(4)`, `floor(n:1.5, scal:1)` → null).
  Echte Fehler (unbekannte Funktion, Nicht-Funktions-Callee, Syntaxfehler) bleiben
  unverändert. Der mit Abstand größte Konformitäts-Hebel: **+123 Cases** quer über
  fast alle Builtin-Suiten; der Ratchet-Floor des CI-Gates steigt auf 85,5 %.
- **DMN-TCK-Konformität: Typ-Koerzierung am Decision-Output (WP-41, 81,7 % → 82,1 %).**
  Das Ergebnis einer Decision wird jetzt an den deklarierten `typeRef` ihrer Variable
  angepasst (DMN §10.3.2.9.4), bevor es zurückgegeben und nachgelagerten Decisions
  zugewiesen wird (+16 Cases, Suite `0082` von 28 auf 13 Fails): eine Singleton-Liste
  wird zum Skalar entpackt (`["foo"]` bei Ziel `string` → `"foo"`), und ein Wert, der
  nicht zum deklarierten Typ passt, wird `null`. Listen und Kontexte werden element-
  bzw. feldweise geprüft; `null` ist Mitglied jedes Typs, ein fehlender `typeRef`
  (`Any`) erzwingt nichts. Der Ratchet-Floor des CI-Gates steigt auf 82,0 %.
- **DMN-TCK-Konformität: strikte Temporal-Lexik (WP-41, 81,2 % → 81,7 %).** Die
  FEEL-Konstruktoren (`date`/`time`/`date and time`) und `@"…"`-Literale weisen
  lexikalisch malformte Datums-/Zeit-Strings jetzt korrekt als `null` ab, statt sie
  tolerant zu akzeptieren (+15 Cases über die Suiten `1115`/`1116`/`1117`): Jahre mit
  weniger als 4 oder mehr als 9 Ziffern, 5+-stellige Jahre mit führender Null,
  führendes `+`, einstellige Stunden (`T7:00:00`) und Zonen-Offsets jenseits ±18:00
  (`+19:00`). Reale Zonen (≤ ±14:00) bleiben gültig. Der Ratchet-Floor des CI-Gates
  steigt auf 81,5 %.
- **DMN-TCK-Konformität: `date and time`-Konstruktor & Rendering (WP-41, 80,3 % → 81,2 %).**
  Vier FEEL-Engine-Fixes am offiziellen DMN-TCK (Level 2+3, +32 Cases, `1117` von 35
  auf 10 Fails): der Zwei-Argument-Konstruktor `date and time(date, time)` akzeptiert
  als erstes Argument nun auch ein `date and time` (dessen Datums-Teil); ein
  date-only-String promoviert zum Tagesbeginn (`date and time("2012-12-24")` →
  `2012-12-24T00:00:00`); Sekundenbruchteile überleben Parse und Rendering
  (`…:30.987@Europe/Paris`); und Jahre mit 1–9 Ziffern (bis `999999999`) parsen jetzt.
  Der Ratchet-Floor des CI-Gates steigt auf 81,0 %; Details in `docs/tck-exceptions.md`.
- **DMN-TCK-Konformität: Arithmetik & Temporal (WP-41, 77,4 % → 80,3 %).** Fünf
  FEEL-Engine-Fixes, gemessen am offiziellen DMN-TCK (Level 2+3, +103 Cases):
  negative (BCE-/astronomische) Jahre in Datums-/Zeit-Literalen inkl. IANA-Zonen
  (`@"-2021-01-01T10:10:10@Australia/Melbourne"`); `date ± duration` bleibt ein
  `date` (Zeit-Anteil abgeschnitten); gemischte `date`/`date and time`-Subtraktion
  ergibt eine Dauer, mit korrektem `null` bei unterschiedlicher Zonen-Kennzeichnung;
  ISO-`24:00:00` (Ende-des-Tages-Mitternacht); und `string + string`-Konkatenation.
  Der Ratchet-Floor des CI-Gates steigt entsprechend auf 80,0 %. `0100-arithmetic`
  fällt von 96 auf 5 Fails; Details in `docs/tck-exceptions.md`.
- **Modeler: Der Graph pulsiert beim Auswerten (Stage 3 — „Juice").** Eine frische
  Auswertung spielt die Illumination jetzt als tiefen-gestaffelte Welle: Die Leitungen
  streamen (fließende Striche), jede Decision pulsiert mit einem Partikel-Burst, sobald ihre
  Eingaben ankommen — die finale Decision am kräftigsten, in Magenta —, und aufeinanderfolgende
  schnelle Läufe bauen einen **Combo**-Streak auf, den der Endknoten feiert. Eine transiente
  Partikelschicht liegt über dem Diagramm (Screen-Space-Bursts an der Live-Position des Knotens,
  ohne Pan/Zoom-Tracking); Stream und Puls sind reines SVG/CSS und bleiben unter Pan und Zoom
  ausgerichtet. Alles ist per **⚡-Toolbar-Toggle** abschaltbar und unter `prefers-reduced-motion`
  von vornherein aus — die statische Illumination (History-Navigation, ruhig) bleibt davon
  unberührt. Reines Frontend, kein neuer Endpunkt. Dritter Schritt, den „Auswerten"-Bereich ins
  Diagramm aufzulösen.
- **Modeler: Eingaben direkt am Knoten (Operate) — der „Auswerten"-Bereich wandert ins
  Diagramm.** In Operate trägt jeder Blatt-Eingabeknoten jetzt eine editierbare Pille direkt am
  inputData-Knoten: eine Auswahl-Liste bei geschlossener Enumeration, sonst ein JSON-coercedes
  Textfeld. Jede Änderung wertet (entprellt) den ganzen Graphen live aus und lässt Ergebnisse
  und Kanten-Illumination sofort auf dem Diagramm nachziehen — man füllt die Eingaben am
  Graphen selbst statt nur im Seitenpanel. Das Blättern durch die Lauf-Historie spiegelt die
  Eingaben des aktiven Laufs in die Pillen. Panel und Pillen teilen sich eine Widget-Fabrik
  (`inputform.ts`, aus dem Panel herausgelöst), sodass beide Oberflächen dieselben Typ-Regeln,
  Enum-Dropdowns und JSON-Coercion verwenden. Reines Frontend über die bestehende
  Whole-Graph-Auswertung; zweiter Schritt, den „Auswerten"-Bereich ins Diagramm aufzulösen.
- **Modeler: Live-Graph — der Datenfluss leuchtet auf dem Diagramm auf.** Nach einer
  Auswertung illuminieren sich jetzt die Anforderungskanten direkt im Diagramm: jede Kante,
  die einen Wert trägt, färbt sich im Operate-Blau und lässt den Wert, der durch sie floss,
  an ihrem Mittelpunkt schweben — die Abhängigkeit zwischen Eingaben und Decisions wird auf
  dem Graphen selbst sichtbar, nicht nur im „Auswerten"-Panel. Die Kanten leuchten gestaffelt
  nach Graphentiefe auf (Eingaben zuerst, finale Decision zuletzt), sodass die Entscheidung
  sichtbar von den Blättern nach oben propagiert. Reines Frontend über die bestehende
  Whole-Graph-Auswertung; spiegelt das Illuminate-Muster des Flow Studios (WP-98) in den
  DMN-Modeler. Erster Schritt, den „Auswerten"-Bereich ins Diagramm aufzulösen.
- **Offizielle DMN-TCK-Konformität — Messung & Gate (WP-41, in Arbeit):** Temis wird jetzt
  gegen das offizielle DMN Technology Compatibility Kit (github.com/dmn-tck/tck) an einem
  gepinnten Commit geprüft. Neu: CI-Lane `tck` + `make tck-conformance` +
  `internal/tck.TestOfficialTCKConformance` mit **Ratchet-Floor** (skippt offline ohne
  `TCK_CORPUS`). Der Runner bewertet jetzt **pro Case** die Ziel-Decision statt die ganze
  Suite bei einem Compile-Fehler abzubrechen. Erste Engine-Fixes: Builtins `is`,
  `list replace` (Positions- und Match-Funktions-Form), `number(from, grouping, decimal)`
  der **vollständige `in`-Operator** (operator-präfixierte Unary-Tests, Komma-Test-Listen,
  Listen-Mitgliedschaft inkl. Range-Elementen — TCK 0072, 224→21 Fails) sowie das
  **`range(from)`-Builtin** (Range-String-Parsing inkl. unbeschränkter Enden und Temporal-
  Endpunkte + `instance of range<T>`).
  **Stand: 77,4 % der Level-2/3-Cases** (2704/3495); Kategorien & Ausnahmen in
  `docs/tck-exceptions.md`, Ziel ≥ 95 %.
- **Betriebs-Observability abgeschlossen (WP-113/114, ADR-0030):** opt-in Metriken-Export —
  `GET /debug/vars` (expvar) und `GET /metrics` (Prometheus-Textformat, stdlib-Encoder, kein
  Client) hinter dem `audit`-Scope, standardmäßig aus (`temisd -metrics`/`$TEMIS_METRICS`);
  Zähler für Evaluations, LLM, clio, Cache, Modelle, Uptime. Strukturierte Logs über `log/slog`
  (`-log-format text|json`, `-log-level`); der clio-Best-Effort-Fehler erscheint als
  strukturierter Record (`system=clio`).
- **Modeler: Modelle in der Seitenleiste durchsuchen:** Über der Modell-Liste sitzt jetzt ein
  Suchfeld („Modelle suchen…"). Je mehr Modelle auf dem Server liegen, desto wichtiger — die
  Suche filtert live, ist diakritik-unempfindlich (`begru` findet `Begrüßung`) und
  term-basiert (Leerzeichen trennt Begriffe, die in beliebiger Reihenfolge alle vorkommen
  müssen, z. B. `demo alter` → „Alterskette (Demo)"). Sie greift auf Modell- **und**
  Ordnernamen (ein Ordnername holt seinen Inhalt hervor), klappt passende Ordner automatisch
  auf, hebt die Treffer im Namen hervor und zeigt einen Hinweis, wenn nichts passt. Rein
  clientseitig, keine API-Änderung.
- **Operate: clio-Events einlesen & nachspielen (ADR-0033, Read-Side):** Die Operate-Ansicht
  bekommt ein Panel „Aus clio nachspielen". Man **definiert das Mapping** — clio-**Subject**-
  Teilbaum + **Event-Typ** (`com.temis.decision.evaluated.v1` u. a.) + Limit — liest die dort
  protokollierten Entscheidungen ein und **spielt jede aufgezeichnete Eingabe** erneut durch
  das offene Modell; jeder Replay erscheint als normaler Lauf oben in der History und auf dem
  Diagramm. Das Mapping wird pro Modell (nach Name) im `localStorage` gemerkt und aus der
  Subject-Konfiguration des Sinks vorbefüllt. Serverseitig neu: **`GET /v1/clio/events`**
  (Audit-Scope, secret-frei — der Server liest über die Sink-Verbindung, der Browser sieht
  den clio-Token nie) und `ClioSink.Query` (clio-`run-query` lesen). `GET /v1/status` meldet
  zusätzlich `subjectPrefix`/`subjectKey` zum Vorbefüllen.

- **Quality-Report – welcher Datensatz welche Regel verletzt (ADR-0034):** Die **Lese-Seite** der
  Produktivläufe (ADR-0031) und die Antwort auf „ich lasse ein ganzes Regelset über 70 000 Server
  laufen und will am Schluss die Auswertung". Neues, read-only **`package quality`** aggregiert die
  `com.temis.quality.evaluated.v1`-Events (aus clios `run-query`-NDJSON) zu einem Report: distinct
  Entitäten, wie viele bestanden, je **verletzender** Entität die sortierte Liste der Regel-IDs und
  eine **Rangliste je Regel**. Drei Kanäle über denselben Kern: **CLI `temis-quality-report`**
  (Text/JSON, `-fail-on-violation` als CI-Gate), **`GET /v1/quality/report`** (Scope `audit`; der
  Server fragt clio selbst ab, kein Token im Browser; `409 CLIO_NOT_CONFIGURED` ohne Sink) und ein
  **Report-Panel im Import-Cockpit** (Tabelle „Entität × verletzte Regeln"). Dazu das gebündelte
  Beispiel-Regelset **`server_compliance`** (COLLECT-Tabelle mit unabhängigen Server-Checks, die die
  verletzten Regel-IDs als Liste ausgibt). Ein End-to-End-Test streamt 70 000 synthetische Server
  durch den Batch und prüft die aggregierten Tallies. Reine Standardbibliothek, kein neuer Dependency.

- **clio-Command-Consumer – Entscheidungen per Event auslösen (WP-120/121, ADR-0033):** Die
  **Gegenrichtung** zum Logbuch. Ein in clio geschriebenes **Command-Event**
  `com.temis.decision.requested.v1` löst eine Auswertung aus — Einzel-Decision (`modelId`+
  `decision`), ganzer Modell-Graph (`modelId`) oder Decision-Flow/DRG (`flowId`) —, und das
  Ergebnis fliesst **korreliert** (`data.requestId`, gleicher `subject`) als bestehendes
  `com.temis.decision.evaluated.v1`/`com.temis.flow.evaluated.v1` zurück; nicht auswertbar →
  `com.temis.decision.failed.v1`, sodass **jedes** Command eine Antwort bekommt. Neues
  `package consume` (über `dmn`/`flow`/`audit`, **kein** `internal/`-/`service`-Import,
  symmetrisch zu `package audit`) + Binary **`temis-clio-worker`**: beobachtet Commands über
  clios **`observe`**-Stream (mit `run-query`-Backfill je Reconnect; `-poll`/`-once`-Modi),
  wertet über die öffentliche Engine-API aus und schreibt idempotent zurück (Precondition auf
  `requestId`, `409` = No-op). **Zustandslos** — clio hält den gesamten Zustand; damit bleibt
  der Consumer Decisioning und wird **nicht** zur Prozess-Engine (Grenze aus ADR-0025 gewahrt).
  Kopplung nur über clios HTTP-Vertrag, Kern unberührt, reine stdlib (ADR-0011/0014). Die
  `data`-Verträge liegen maschinenlesbar als **JSON Schema** in `docs/schemas/` (Command +
  Ergebnis-Events); das Command-Schema lässt sich in clio (`register-event-schema`)
  hinterlegen, sodass fehlerhafte Commands **beim Schreiben** abgewiesen werden. Ein
  `consume/schema_test.go` bindet die Schemas dependency-frei an die erzeugten Events.
- **Engine-Kern (WP-01–11):** DMN-1.5-XML-Decoding (tolerant 1.3/1.4) mit `DMNDI`-Round-Trip;
  vollständige FEEL-Pipeline (Lexer → Parser → Compile-to-Closures); Decimal-Numbers (`apd`);
  Decision Tables mit Hit Policies; öffentliche Library-API `package dmn` (`Compile`/`Evaluate`).
- **FEEL vollständig (WP-20–22):** Comprehensions/Filter/Pfad-Projektion, alle nicht-temporalen
  Built-ins, Date/Time/Duration samt temporaler Built-ins und `@`-Literalen.
- **Boxed Expressions & DRG (WP-23–26):** Boxed Context/Invocation/Function, BKM, DRG-Verkettung,
  Decision Services.
- **Hit Policies & Typsystem (WP-27, WP-30–31):** alle Hit Policies (inkl. PRIORITY/OUTPUT ORDER);
  Typsystem, `instance of`, advisory statische Typprüfung (ADR-0017), Item-Definition-Constraints.
- **Robustheit & Betrieb (WP-34–35, WP-42, WP-44):** Ressourcenlimits/Sandboxing (ADR-0008),
  LRU-Modell-Cache, Performance-Budget-CI-Gate, Fuzzing über jede untrusted-Input-Schicht.
- **Service & Agenten (WP-32, WP-50–53):** HTTP-Service `temisd` (REST, OpenAPI, `/ui`-Playground);
  MCP-Server `temis-mcp` (stdio + HTTP), Entscheidungsspur, striktes Eingabe-Schema.
- **Modellierungs-Assistent (WP-80, ADR-0024):** eingebauter LLM-Chat im Modeler, der beim Bauen
  von Decisions hilft und seine Vorschläge mit `evaluate` gegen die echte Engine verifiziert.
  Anbieter-agnostisch (Anthropic/OpenAI) über das neue Paket `assist` — reine Standardbibliothek,
  kein SDK, keine neue Dependency (konsistent mit ADR-0014). Endpunkt `POST /v1/chat` (opt-in,
  Default aus), aktiviert über `temisd -llm-provider/-llm-token/-llm-model/-llm-base-url`; Token
  server-seitig **plus** optionaler Browser-Key (`X-LLM-Token`, `-llm-allow-byok`, nie persistiert),
  vom selben `-token` bewacht. Der Agent-Loop läuft server-seitig auf dem geteilten Modell-Cache mit
  sieben Werkzeugen (inspizieren/auswerten/bauen); Frontend: angedocktes Chat-Panel mit
  Tool-Schritt-Anzeige und automatischem Reload bei Modelländerungen.
- **Ko-lokalisierter MCP-Endpoint (ADR-0021):** `temisd` bedient optional `POST /mcp` (Flag `-mcp`,
  Default an) auf **demselben Modell-Cache und Flow-Katalog** wie Modeler und `/v1`-API — vorgeladene
  Beispiele und Modeler-Modelle sind über MCP sichtbar und umgekehrt, eine `modelId` über alle
  Oberflächen; ebenso erscheint ein über MCP `load_flow`/`git_load_flow` registrierter Flow im
  Flow-Katalog des Modelers (`GET /v1/flows`) und umgekehrt (`mcp.WithFlowStore` /
  `Server.FlowStore()`); das eigenständige `temis-mcp` (stdio/HTTP) bleibt unverändert.
- **Decision-Flow – transitive Step-Inputs (ADR-0026, L2a):** Ein Flow-Step auf eine
  **zusammengesetzte** Decision darf jetzt deren **transitiv benötigte** Blatt-Inputs
  verdrahten — Inputs, die die Ziel-Decision nur über eine Sub-Decision desselben Modells
  bezieht (z. B. `FinalPremium`, das `VehicleValue` allein über `BasePremium` braucht). Zuvor
  waren solche Decisions in Flows faktisch unbenutzbar: der transitive Input wurde als
  `FLOW_UNKNOWN_INPUT` abgelehnt bzw. lief bei Weglassen still auf `null`. Wiring-Validierung
  und Auswertung eines Decision-Steps arbeiten nun gegen die **Requirements-Cone** der
  Ziel-Decision statt nur gegen ihre direkt deklarierten Inputs; die transitiven Werte werden
  bis in die Sub-Decisions durchgereicht (inkl. Typ-Koerzierung numerischer Inputs). Echte
  unbekannte Inputs (`FLOW_UNKNOWN_INPUT`) und fehlende required-Inputs (`FLOW_INPUT_UNWIRED`)
  werden weiterhin präzise gemeldet. Neue additive `dmn`-API `ReachableInputSchema` /
  `ValidateReachableInput` (cone-gescopt, analog zu `ModelInputSchema`/`ValidateModelInput`);
  MCP `describe_decision` weist die Menge additiv als `reachableInputs` neben `inputs` aus.
- **Modeler – Deluxe-JSON-Editor an jedem JSON-Eingabefeld (ADR-0016):** Überall, wo ein Feld
  seinen Wert als FEEL/JSON entgegennimmt — die **Auswerten**-Eingaben (Operate), das
  **Flow-auswerten**-Panel und das **Testen**-Formular des Flow-Designers — steht jetzt neben dem
  einzeiligen Feld ein **`{ }`-Icon**, das einen **großzügigen JSON-Editor** als Modal öffnet.
  Der Editor gibt eine Monospace-Textfläche mit viel Platz, **Live-Validierung** (gültiges JSON ✓
  bzw. die Parser-Meldung), **Formatieren**/**Kompakt**/**Kopieren**-Werkzeuge, Tab-Einrückung und
  Tastatur-Shortcuts (Strg/Cmd+Enter = Übernehmen, Esc = Abbrechen). Beim Öffnen wird vorhandenes
  JSON eingerückt dargestellt, beim „Übernehmen" kompakt ins Feld zurückgeschrieben. Geschlossene
  Aufzählungsfelder (`<select>`) bekommen kein Icon. Rein additiv, keine Backend-Änderung.
- **Flow-Designer – Flows via UI erstellen & designen (WP-116, ADR-0026):** Decision-Flows lassen
  sich jetzt **im Modeler visuell erstellen, designen und testen**, nicht nur ansehen & ausführen.
  Ein neuer **Flow-Designer** (betretbar über das **+** in der FLOWS-Sidebar oder **„✎ Bearbeiten"**
  im Studio) bietet einen **strukturierten Inspector** — Flow-Name/Version, deklarierte Inputs
  (Name + FEEL-Typ), Steps mit **Modell- + Decision-Picker** und **FEEL-Input-Verdrahtung**
  (Vorschläge aus Flow-Inputs + Step-IDs; **Auto-Wiring** übernimmt die Inputs der gewählten
  Decision und referenziert gleichnamige Flow-Inputs) sowie Output-Mapping — neben einer
  **Live-Graph-Preview**, die den Cross-Model-DRG beim Tippen neu zeichnet. **„Testen"** wertet den
  Entwurf inline aus (`POST /v1/flow/evaluate`, ohne Registrierung) und *illuminiert* die Preview;
  **„Prüfen"** validiert gegen die geladenen Modelle; **„Registrieren & Öffnen"** legt den Flow im
  Katalog ab und öffnet ihn im Studio; **„Export"** lädt den `*.flow.json`-Deskriptor herunter.
  **Git bleibt die dauerhafte Quelle (ADR-0032):** die Registrierung ist der flüchtige Dev-Pfad,
  der Export der Weg in den Repo (`flows/` + `git_propose`) — kein neuer server-seitiger
  Schreibpfad. Rein additiv, keine Backend-Änderung.
- **Modeler – Auto-Layout mit orthogonalem Routing & Orientierungs-Umschalter (ADR-0016):** Modelle ohne
  authorede `DMNDI`-Bounds werden nicht länger als diagonaler „Spaghetti" gezeichnet. Das Auto-Layout
  richtet die Knoten spaltenweise aus (jeder Knoten wird über/unter seine Nachbarn gezogen) und führt
  jede Requirement-Kante als **rechtwinkligen Konnektor**: die Eingänge eines Hubs laufen als sauberer
  Kamm zusammen, und lange „Skip"-Kanten werden durch eine freie Bahn zwischen den Spalten gefädelt, statt
  durch Knoten hindurchzulaufen. Ein Toolbar-Knopf **Bottom-up / Top-down** schaltet um, ob die Eingabe-
  Pillen die Decisions von unten (Pfeile nach oben, Default) oder von oben (Pfeile nach unten) speisen, und
  ordnet das ganze Diagramm entsprechend neu an. Authorede `DMNDI`-Layouts bleiben unangetastet (bis der
  Umschalter ein Neu-Anordnen erzwingt); der Decision-Flow-Canvas ist unverändert.
- **Modeler – Modelle verwalten (ADR-0016):** Im Modeler lässt sich ein Modell jetzt komplett neu
  (leer) anlegen statt nur eine `.dmn`-Datei hochzuladen, sowie **umbenennen** und **löschen**
  (inkl. des gesamten Revisions-Verlaufs). Zwei neue HTTP-Endpunkte: `POST /v1/models/{id}/rename`
  (setzt den Definitions-Namen, legt eine neue Revision an) und `DELETE /v1/models/{id}` (entfernt
  eine Revision aus dem Cache); neue Library-Funktion `dmn.SetModelName`. Anlegen, Umbenennen und
  Löschen laufen über eigene In-App-Dialoge (kein `window.prompt`), mit Hinweis bei Namensdopplung.
- **Modeler – Operate-Cockpit (ADR-0016):** Die **Operate**-Sicht (Auswerten/Betreiben) ist jetzt klar
  von der **Design**-Sicht abgegrenzt — eigener, kühler „Cockpit"-Look (blaue Chrome-Farbwelt, getönter
  Canvas) und read-only Graph. Sie besteht aus drei Bausteinen: (1) eine **Läufe-Historie oben** über
  dem Diagramm, rein per Tastatur blätterbar (↑/↓/←/→/j/k, Pos1/Ende, Enter) als ARIA-`listbox` mit
  `aria-activedescendant`/`aria-selected`; der Wechsel des aktiven Laufs aktualisiert Diagramm und
  Overlays. (2) **Halbtransparente, schwebende Overlays** (frosted/Backdrop-Blur, ein-/ausblendbar)
  direkt über dem Diagramm fassen Eingangsdaten (links/oben) und Ergebnisse (rechts/unten) zusammen,
  während die grünen Ergebnis-Pills an den Knoten erhalten bleiben. (3) **Hover-Grafik**: über einer
  Ergebniszeile erscheint die Entscheidungstabelle als Matrix mit hervorgehobener getroffener Regel,
  numerische Werte als Mini-Bars. Reines Frontend, baut auf derselben Auswerte-Logik auf (kein neuer
  Endpunkt, keine neue Dependency).
- **Modeler – FEEL-Editor-Assistenz überall (ADR-0016):** Syntax-Highlighting (Funktionen, Variablen,
  Schlüsselwörter, Strings, Zahlen als farbige Token hinter dem transparenten Feld) und
  Code-Completion (In-Scope-Variablen + Engine-Built-ins, aufklappend unter dem Cursor beim Tippen
  oder per Ctrl/Cmd+Leertaste) stehen jetzt in **allen** FEEL-Eingabefeldern zur Verfügung — nicht
  mehr nur im Literal-, Decision-Table- und BKM-Editor, sondern auch in den Boxed-Editoren
  **Conditional** (Wenn/Dann/Sonst), **Filter**, **Iteration**, **Liste**, **Relation** und
  **Boxed Context**. Alle Felder laufen über eine gemeinsame Primitive (`attachFeelField`), sodass
  Highlighting und Completion nicht mehr auseinanderlaufen oder bei neuen Editoren vergessen werden
  können. Der Funktionskatalog kommt weiterhin direkt aus der echten Engine (WASM), reines Frontend.
- **Operate – Entscheidungs-Pfad in der Tabelle:** Ein Doppelklick auf eine Decision mit Tabelle zeigt
  im Operate-Modus jetzt den **genommenen Weg** grafisch: eine Chip-und-Pfeil-Leiste
  (Eingabewert → getroffene Regel → Ergebnis), der getestete Eingabewert je Spaltenkopf und eine
  **Pass/Fail-Heatmap** über alle Regeln mit leuchtend hervorgehobener getroffener Regel. Das
  Hover-Popover der Ergebnis-Overlays wurde korrekt im Viewport positioniert (lag zuvor außerhalb des
  Sichtbereichs) und hoverbare Zeilen sind mit einem ⊞-Marker gekennzeichnet.
- **Modeler – Import-Cockpit (ADR-0016):** Ein dritter Modus **Import** neben Design/Operate — ein
  Testfall-Stapellauf als **Fließband**. Man lädt eine **Vorlage** (CSV **oder** JSON) herunter, die
  exakt zu den Leaf-Inputs des Modells passt (dieselbe autoritative Eingabemenge wie das Auswerte-
  Formular, `leafInputs`), füllt sie mit Testdaten — von Hand, in der Tabellenkalkulation oder von
  einem **KI-Agenten** (dokumentiertes, agentenfreundliches Format) — und importiert sie (Datei-
  Auswahl oder Drag & Drop). Optionale `→Decision`-Spalten machen aus einer Zeile eine **Pass/Fail-
  Erwartung**. „Durchlaufen lassen" wertet den **ganzen Stapel in EINEM Batch-Request** aus und lässt
  die Datensätze von links (**Eingang**) durch die **Evaluation** nach rechts in den **clio Store**
  fliegen — samt berechneter Ergebnisse und Pass/Fail-Badges. Eigene kühle Cyan-Chrome-Farbwelt
  (`--imp`), respektiert `prefers-reduced-motion`.
  **Durchsatz (Folge-Fix):** Neuer Endpunkt **`POST /v1/models/{id}/evaluate-graph-batch`** wertet
  viele Eingabezeilen in einem Round-Trip aus (die Engine schleift in-memory, ohne Traces; jede Zeile
  unabhängig — eine abgelehnte Zeile bricht den Batch nicht ab). Damit laufen **5000 Testfälle in
  ~50 ms** statt tausender Einzel-Requests. Das Cockpit ruft den Batch statt einer Schleife auf,
  verzichtet auf künstliche Pro-Datensatz-Pausen und **begrenzt die gezeichneten Karten pro Lane**
  (Zähler + Overflow-Hinweis zeigen die echte Menge) — die Animation ist bewusst nur *angedeutet* als
  gestaffelte CSS-Kaskade, statt tausende DOM-Knoten einfrieren zu lassen.
  **Test- vs. Produktivlauf & clio-Quality-Events (ADR-0031):** Das Cockpit unterscheidet einen
  **Testlauf** (Default, schreibt **nichts**) von einem **Produktivlauf**, der pro ausgewertetem Fall
  ein **Quality-Event** `com.temis.quality.evaluated.v1` **auf der Entität** nach clio schreibt
  (Subject `/quality/<entity>`, mit `violation`-Flag aus den erwarteten Werten) — so werden Reports
  über Verletzungen je Entität möglich. Die Entität kommt aus einer **`entity`-Vorlagenspalte**, sonst
  einem wählbaren Eingabefeld, sonst dem Fall-Label. Die Zustellung läuft **entkoppelt über eine
  garantierte Queue mit Backpressure** (`QualityQueue`): der Batch-Response kehrt sofort zurück,
  Hintergrund-Worker liefern mit Retry & Idempotenz (clio-Precondition), `temisd` drainiert die Queue
  beim **Graceful-Shutdown**. Ohne konfiguriertes clio wird ein Produktivlauf klar mit
  **`409 CLIO_NOT_CONFIGURED`** abgelehnt (opt-in, Default aus — kein Datenabfluss).
  **Ergebnis-CSV:** Nach einem Lauf schreibt das Cockpit die **berechneten Decision-Ausgaben** je
  Fall in eine CSV (Fall/Entität/Eingaben + eine Spalte je Decision, plus `status`-Spalte
  OK/Abweichung/Fehler bei Erwartungen) und bietet sie als **„Ergebnisse · CSV ↓"** zum Download an —
  das ausgefüllte Testblatt mit den Outputs.
- **clio-Audit auch für Whole-Graph-Auswertung:** Der „Auswerten"-Pfad des Modelers
  (`POST /v1/models/{id}/evaluate-graph`) wird jetzt ebenfalls protokolliert — **ein
  `com.temis.decision.evaluated.v1`-Event je ausgewerteter Decision** (best-effort, bzw. `502` bei
  `-clio-strict`; idempotent per `(modelId, decision, input)`). Zuvor auditierte der Sink nur
  Einzel-Decision- und Flow-Auswertungen, sodass genau die interaktive Graph-Auswertung nicht im
  Logbuch landete.
- **clio-Entscheidungs-Logbuch (WP-54, ADR-0023):** `temisd` protokolliert optional jede
  Einzel-Decision-Auswertung als manipulationssicheres `com.temis.decision.evaluated.v1`-CloudEvent
  in einer [clio](https://github.com/pblumer/clio)-Instanz — Flags `-clio-url`/`-clio-token`/
  `-clio-source`/`-clio-subject-prefix`/`-clio-subject-key`/`-clio-strict` (`$TEMIS_CLIO_*`), Default
  **aus** (byte-identisch). Idempotent per clio-Precondition (`inputHash`); `-clio-strict` macht den
  Sink fail-closed (`502 AUDIT_WRITE_FAILED`), sonst best-effort. Reine stdlib, kein Go-Import von
  clio (Kopplung nur über dessen HTTP-API, ADR-0011/0014).
- **Dateisystem-Modell-Store (ADR-0027):** `temisd` persistiert seinen Modell-Cache optional
  auf das Dateisystem — Flag `-models-dir` (`$TEMIS_MODELS_DIR`), Default **aus** (byte-identisch
  rein in-memory). Hochgeladene und im Modeler editierte Modelle werden content-adressiert als rohes
  DMN-XML (`<sha256>.dmn`, atomarer Write) abgelegt und beim Start wieder in den Cache geladen, sodass
  sie einen Neustart überleben. Nur das rohe XML liegt auf der Platte (Kompilat/Index/Diagnostik werden
  deterministisch neu erzeugt); ein aus dem LRU-Cache verdrängtes, aber persistiertes Modell wird
  on-demand von der Platte rekompiliert. Die gebündelten Beispiele werden nie persistiert (re-embed per
  `go:embed`). Reine stdlib, kein neuer Dependency; Persistenz hängt am einzigen Choke-Point
  `compileAndStore`/`lookup`, greift also auch für Modeler-Saves, MCP, gRPC und Git-Load.
- **Re-Audit-/Replay-Tool `temis-reaudit` (WP-55, ADR-0023):** `package audit` + Binary
  `cmd/temis-reaudit` lesen die Decision-Events aus clio (`run-query`), rechnen jede Entscheidung
  `input`@`modelId` über die `dmn`-API erneut nach und vergleichen kanonisch mit den protokollierten
  `outputs` — Verdikt je Event (reproduced/discrepancy/model_unavailable/eval_error), Exit-Code
  (0/1) wie clios `verify`. Modelle werden über ein DMN-Verzeichnis (`-models`) per `sha256:`-`modelId`
  aufgelöst. Read-only; ergänzt clios *Unverändert*-Beweis um den *Regelkonformitäts*-Beweis.
- **Nullkonfiguration & Env-Opt-out (`temisd`):** Ein nackter Start (`temisd`, keine Flags,
  keine Env-Variablen) bringt sofort einen voll ausgestatteten Server — Modeler, Swagger-UI,
  Beispiele, Modell-Listing, MCP-Endpunkt und der **Modellier-Assistent** sind ab Start aktiv.
  Der Assistent ist damit **standardmäßig an** (zuvor opt-in): ohne serverseitigen Schlüssel
  läuft er im **BYOK-Modus** (Endpunkt live, antwortet sobald ein Aufrufer `X-LLM-Token`
  mitschickt), mit `TEMIS_LLM_TOKEN` nutzt der Server den eigenen Key; Abschalten via
  `-assist=false`/`TEMIS_ASSIST=false`. Für den Profi lässt sich **jedes** Feature allein über
  Umgebungsvariablen ab-/umschalten (`TEMIS_ADDR`, `TEMIS_EXAMPLES`, `TEMIS_MCP`,
  `TEMIS_LIST_MODELS`, `TEMIS_ASSIST`, `TEMIS_LLM_ALLOW_BYOK`, `TEMIS_CACHE_SIZE`,
  `TEMIS_MAX_*`, `TEMIS_CLIO_*` u. a.) — kein Flag nötig (container-freundlich); ein explizites
  Flag hat weiterhin Vorrang vor der Env-Variable. Das clio-Audit-Logbuch zeigt jetzt
  standardmäßig auf die gehostete clio (`https://clio.blumer.cloud`), bleibt aber **aus, bis
  ein `TEMIS_CLIO_TOKEN` gesetzt ist** — kein Datenabfluss im Default, Anschalten ist ein
  einziger Schritt (Token setzen oder `-clio-url` auf die eigene clio zeigen); der Start-Banner
  weist auf die Verfügbarkeit hin.
- **Betriebs-Observability (WP-110–112, ADR-0030):** `temisd` ist jetzt *observierbar*.
  `/healthz` (Liveness) und `/readyz` (echte Readiness) sind **ehrlich getrennt** — `/readyz`
  liefert `503`, wenn eine harte Startbedingung fehlt (z. B. ein fail-closed `-clio-strict`
  clio unerreichbar ist); ein best-effort-clio-Ausfall lässt es bewusst bei `200`. Neu:
  **`GET /v1/status`** zeigt den Zustand der Umsysteme (clio/LLM/Git) und die Last der Engine
  — clio `writesOk`/`writesFailed`/`idempotentSkips`, `lastOk`/`lastError`, `reachable`, dazu
  Version/Uptime/Cache-Zähler; **secret-frei** (kein Token/Key im Body) und hinter dem
  `audit`-Scope (ADR-0028; `admin`-Keys lesen ebenfalls, offen ohne Auth-Konfig).
  clio-Erreichbarkeit standardmäßig **passiv** aus echten Writes;
  `-clio-active-probe`/`TEMIS_CLIO_ACTIVE_PROBE` schaltet einen aktiven Health-Ping zu. Reine
  stdlib (`sync/atomic`), Zähler allokationsfrei im Hot Path, Engine-Kern unberührt (ADR-0011).
- **API-Stabilisierung (WP-43):** `package dmn` als v1 zugesagt; SemVer-/Deprecation-Policy;
  Golden-Surface-Test gegen unbeabsichtigte Brüche.
- **Doku & Release (WP-45–46):** godoc-Beispiele, Integrations-/Quickstart-Leitfaden; versionierte
  Release-Pipeline, Container-Image für `temisd`, dieses Changelog.

### Changed

- **Doppelklick wechselt durchgängig in den Inhalt, Umbenennen nur noch bewusst:**
  Ein Doppelklick auf ein Element öffnet jetzt **immer** dessen Inhalt statt es zu
  benennen — eine Decision ihre Logik (Tabelle/FEEL/Boxed-Ausdruck), ein Business
  Knowledge Model seine gekapselte Funktion; eine noch undefinierte Decision (ohne
  Logik) hat keinen Inhalt und öffnet nichts. **Umbenennen** läuft ausschließlich
  über das **Bleistift-Symbol** im Context-Pad und die **F2-Taste** auf dem
  selektierten Element. Damit kollidieren die beiden Gesten nie mehr (bisher
  benannte der Doppelklick logiklose Decisions/BKMs inline). Betroffen sind
  `web/src/dmn-label-editing.ts` (Doppelklick-Rename entfernt, F2-Handler ergänzt),
  `web/src/canvas.ts` (Doppelklick auf BKM öffnet die Funktion) und der
  Context-Pad-Hinweis. Headless (Chromium) verifiziert.
- **Flow-Studio-Autolayout auf dagre (WP-97/98):** Die read-only Flow-Ansicht
  ordnet ihre Schritte jetzt mit **dagre** (`@dagrejs/dagre`) statt der
  handgeschriebenen Barycentre-Sweeps an. Ränge, Kreuzungs-Minimierung und
  gebogene Kanten-Waypoints kommen in einem Schritt — Flows mit geteilten Quellen
  (mehrere Steps, die dieselben Flow-Inputs konsumieren, z. B. `kfz-antrag`)
  rendern mit klar getrennten Ebenen (Inputs unten, finale Decision oben), ohne
  überlappende Boxen und mit deutlich weniger überlagerten Kantenlinien.
  Betroffen ist **nur** der Auto-Layout-Pfad der Flow-Ansicht; authored **DMNDI**
  wird weiterhin verbatim genutzt und der DRD-Modeler-Pfad (ortho) bleibt
  unverändert. `Laid`-Interface und Renderer bleiben stabil.

### Fixed

- **Modeler – fehlende Input-Spalte in der Decision Table (ADR-0016):** Die Eingabespalten
  einer Decision Table werden nur bei der **Erstellung** aus den Informationsanforderungen
  des Knotens abgeleitet. Eine Eingabe, die *nachträglich* an die Decision verdrahtet wird
  (die Input-Pille ist im Graphen sichtbar), bekam dadurch keine Spalte — der Tabellen-Editor
  zeigte den Input gar nicht an. Der Editor gleicht jetzt beim Öffnen mit den aktuell
  verdrahteten Eingaben des Knotens ab und blendet jede noch spaltenlose Anforderung als
  Input-Spalte ein (mit ihrem Namen als Ausdruck vorbelegt); überflüssige Spalten lassen sich
  wie gewohnt entfernen. Read-only-/Trace-Ansichten (Operate) bleiben unverändert.

- **Modeler – Palette „klebendes" Element (ADR-0016):** Ein aus der Design-Palette gezogenes
  Element blieb am Cursor „kleben" und ließ sich nur per Esc/Neuladen lösen. Zwei Ursachen:
  (1) der Geister-Klick, den der Browser nach einem abgebrochenen nativen Drag noch auf den
  Palette-Eintrag feuert — er startete eine zweite, verwaiste Erstell-Sitzung; die Klick-Aktion
  ignoriert diesen Nachzügler jetzt (und einen Klick, während schon eine Sitzung läuft).
  (2) Eine Ausnahme in einem Listener, der auf das frisch erstellte Element reagiert, entkam
  `create.end`, sodass diagram-js' Aufräumen ausblieb und die Drag-Sitzung hängen blieb. Die
  Palette fängt solche Ausnahmen jetzt während einer laufenden Erstellung ab (sie werden weiterhin
  in der Konsole protokolliert), lässt die Erstellung zu Ende laufen — das Element wird platziert —
  und gibt den Cursor frei. Zusätzlich bekommen neu erstellte Elemente eindeutige Vorgabenamen
  („Neue Decision", „Neue Decision 2", …), damit zwei gleichnamige Knoten nicht stumm
  kollidieren.

### Docs

- **OpenAPI & API-Vertrag mit dem Modeler synchronisiert:** Die 13 Modeler-Endpunkte
  (ADR-0016 — Graph, Item Definitions, Decision-Tables, Literal-Expressions, BKM, Save)
  sowie `GET /v1/models/{id}/xml` und `POST /v1/models/{id}/evaluate-graph` sind jetzt in
  `service/openapi.yaml` (Pfade + Schemas) und `docs/40-api-contract.md` §2.1 dokumentiert;
  README entsprechend ergänzt. Ein neuer Test (`TestOpenAPICoversDataRoutes`) gleicht die
  registrierten `/v1`-Routen gegen die OpenAPI-Pfade ab, sodass die Spec nicht mehr stillschweigend
  von der Implementierung abdriften kann.
- **Entscheidungs-Logbuch via clio (ADR-0023, WP-54–56 komplett):** ADR-0023 und
  `docs/80-clio-decision-log.md` beschreiben ein revisionssicheres Entscheidungs-Logbuch über das
  Schwesterprojekt [clio](https://github.com/pblumer/clio) — versionierter
  `com.temis.decision.evaluated.v1`-CloudEvent-Vertrag, opt-in-Sink in `temisd` (WP-54, siehe oben)
  und Re-Audit-Tool `temis-reaudit` (WP-55, siehe oben). WP-56 ergänzt das **Agent-Muster
  „delegieren → protokollieren"** (`docs/80` §5 mit lauffähigem Beispiel, `docs/60-ai-agent-guide.md`
  §8) — ein Agent gibt die Entscheidung an temis (`evaluate`) und schreibt sie selbst nach clio
  (`write-events`), ganz ohne neuen temis-Code.

[Unreleased]: https://github.com/pblumer/temis/commits/main
