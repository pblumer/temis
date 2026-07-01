# API-Vertrag

> Dies ist die **stabil zu haltende Oberfläche**. Änderungen hier sind breaking changes
> und brauchen ein ADR. Interna (`internal/`) sind frei.

## 1. Go-Library-API (`package dmn`)

> Import: `import "github.com/pblumer/temis/dmn"` (Modul `github.com/pblumer/temis`). Der Markenname „Temis" steckt im Modulpfad; das Package
> heißt domänenbeschreibend `dmn`.

### 1.1 Typen

```go
package dmn

// Engine kompiliert DMN-Modelle. Re-entrant, kann global geteilt werden.
type Engine struct { /* opts */ }

func New(opts ...Option) *Engine

type Option func(*config)
// Beispiele:
func WithLimits(l Limits) Option
func WithClock(now func() time.Time) Option   // für deterministische now()/today()
func WithLocale(loc string) Option

// CompiledDecision ist immutable und thread-safe. Beliebig oft parallel evaluierbar.
type CompiledDecision struct { /* opaque */ }

// Definitions repräsentiert ein geladenes DMN-Modell (alle Decisions/BKM/Services).
type Definitions struct { /* opaque */ }
```

### 1.2 Compile

```go
// Lädt + kompiliert ein komplettes DMN-XML-Dokument.
func (e *Engine) Compile(ctx context.Context, xml []byte) (*Definitions, Diagnostics, error)

// Liefert eine kompilierte, evaluierbare Decision per ID oder Name.
func (d *Definitions) Decision(idOrName string) (*CompiledDecision, error)

// Liefert einen kompilierten Decision Service.
func (d *Definitions) Service(idOrName string) (*CompiledDecision, error)

// Listet verfügbare Decisions/Services/Inputs für Tooling.
func (d *Definitions) Index() ModelIndex
```

### 1.3 Evaluate

```go
// Input-Context: Schlüssel = Input-Data-/Variablen-Name, Wert = Go-Wert.
// Akzeptierte Go-Typen werden in FEEL-Werte konvertiert (siehe Mapping unten).
type Input map[string]any

type Result struct {
    Outputs   map[string]any   // Name → Ergebnis (FEEL→Go zurückkonvertiert)
    Decisions map[string]any   // alle ausgewerteten Zwischen-Decisions
    Diags     Diagnostics      // Laufzeit-Warnungen (z.B. null aus Fehler)
    Trace     *Trace           // strukturierte Erklärung; nur bei WithTrace, sonst nil
}

// Evaluate ist variadisch erweiterbar; ohne Optionen unverändert (abwärtskompatibel).
func (c *CompiledDecision) Evaluate(ctx context.Context, in Input, opts ...EvalOption) (Result, error)

type EvalOption func(*evalConfig)
func WithTrace() EvalOption   // opt-in: füllt Result.Trace
```

#### Standalone-FEEL-Ausdruck (`CompileExpression`) — ADR-0029

Der FEEL-Evaluator ist auch **ohne umgebende Decision** nutzbar: ein Ausdruck über einem
Namenskontext. Genutzt u. a. von `package flow` für volle FEEL-Step-Mappings (ADR-0026).

```go
type CompiledExpression struct { /* opaque */ }

// Kompiliert einen FEEL-Ausdruck, der die genannten Namen referenzieren darf.
// Ein benutzter, nicht deklarierter Name ist ein Compile-Fehler.
func CompileExpression(expr string, names ...string) (*CompiledExpression, error)
func (c *CompiledExpression) References() []string                 // referenzierte Namen (sortiert)
func (c *CompiledExpression) Evaluate(ctx context.Context, in Input) (any, error)
```

Default-Config (volle Built-ins, Decimal; `now()`/`today()` lesen die Prozess-Uhr →
nicht-deterministisch, ggf. Fixwert als Input geben). Ein deklarierter, aber fehlender Name
wertet zu `null`; ein spec-konformes FEEL-`null` ist ein `nil`-Wert, kein `error`.

#### Entscheidungsspur (`Trace`) — opt-in, ADR-0013/WP-51

`WithTrace()` lässt `Evaluate` eine **strukturierte, aus der echten Auswertung
abgeleitete** Erklärung anhängen (kein nachträgliches Rationalisat). Der Default-Pfad
ohne Option bleibt allokationsarm (Performance-Budget, ADR-0011). Die `Trace`-Typen
sind die einzige `dmn`-Oberfläche mit JSON-Tags: HTTP- und MCP-Adapter serialisieren sie
verbatim, die Feldnamen sind damit Teil des Wire-Vertrags.

```go
type Trace struct {
    Tables []TableTrace            // je ausgewerteter Decision Table ein Eintrag
}
type TableTrace struct {
    HitPolicy   string             // U/A/F/R/C
    Aggregation string             // SUM/MIN/MAX/COUNT oder "" (kein/Plain-Collect)
    Inputs      []TraceInput       // Eingabespalten + ausgewertete Werte
    Rules       []TraceRule        // jede Regel mit ihren Bedingungsergebnissen
    Matched     []int              // Indizes (0-basiert) der getroffenen Regeln
}
type TraceInput     struct { Expression string; Value any }
type TraceRule      struct {
    Index int; ID string; Matched bool
    Conditions []TraceCondition    // bis einschließlich der ersten verfehlten (Short-Circuit)
    Outputs    []any               // nur gesetzt, wenn die Regel zum Ergebnis beigetragen hat
}
type TraceCondition struct { Input, Entry string; Matched bool }
```

HTTP/MCP: das Auswerten akzeptiert ein optionales `"explain": true`; die Antwort trägt
dann zusätzlich `"trace"` (gleiche Struktur, `omitempty`, camelCase-Feldnamen).

#### Eingabe-Schema & strenge Validierung — ADR-0013/WP-52

Selbstbeschreibung der erwarteten Inputs samt Typen, plus präzise, maschinenlesbare
Validierungsfehler statt stillschweigend falscher Defaults.

```go
type InputField struct { Name string; Type string; Required bool }   // Type "" = undeklariert/Custom
func (c *CompiledDecision) InputSchema() []InputField
func (d *Definitions)      InputSchema(idOrName string) ([]InputField, error)

type InputProblem struct { Input, Code, Message, Expected, Got string }  // Code: TYPE_MISMATCH | UNKNOWN_INPUT | MISSING_INPUT
func (c *CompiledDecision) ValidateInput(in Input) []InputProblem        // leeres Ergebnis = gültig

func WithStrictInput() EvalOption   // Evaluate validiert zuerst; bei Verstoß → *InputError{Problems}
type InputError struct { Problems []InputProblem }
```

Der Typ wird aus dem `typeRef` der InputData-Variablen abgeleitet, ersatzweise aus dem
`typeRef` der Decision-Table-Input-Clause gleichen Namens (dmn-js-Stil). Kanonische
FEEL-Typen: `string`, `number`, `boolean`, `date`, `time`, `date and time`, `duration`;
unbekannte/Custom-Typen (Item Definitions, WP-31) erzeugen `""` und damit keine
Constraint. `null` ist nie ein Typkonflikt (Abwesenheit ist `MISSING_INPUT`).

HTTP: Auswerten akzeptiert `"strict": true`; bei Verstoß `422` mit
`code: INVALID_INPUT` und der Liste unter `problems`. Die Modell-Antwort
(`POST /v1/models`, `GET /v1/models/{id}`) trägt zusätzlich `schema` (Decision-Name →
`InputField[]`). MCP: `describe_decision` liefert das typisierte Schema; `evaluate`
akzeptiert `"strict": true`.

### 1.4 Diagnostics & Fehler

```go
type Severity int
const ( SevError Severity = iota; SevWarning; SevInfo )

type Diagnostic struct {
    Severity   Severity
    Code       string   // stabiler Code, z.B. "FEEL_TYPE_MISMATCH"
    Message    string
    DecisionID string
    Line, Col  int      // 0 wenn nicht zutreffend
}
type Diagnostics []Diagnostic
func (d Diagnostics) HasErrors() bool
```

#### Compile- vs. Evaluate-Fehlergrenze (verbindlich)

Diese Grenze ist **öffentliches Verhalten** und damit Teil des SemVer-Vertrags. Sie nach
1.0 zu verschieben ist ein Breaking Change. Drei Mechanismen, klar getrennt:

1. **`error`** — der Aufruf konnte nicht durchgeführt werden (kein verwertbares Ergebnis).
2. **`Diagnostic` mit `SevError`** — ein Modell-/Logikproblem an einer *bestimmten Stelle*;
   der umgebende Aufruf liefert trotzdem ein verwertbares (Teil-)Ergebnis.
3. **FEEL-`null`** (+ optional `SevWarning`/`SevInfo`) — spec-konformes Ergebnis, **nie**
   ein Fehler.

**`Compile` (best effort, sammelnd):**

| Situation | Mechanismus |
|---|---|
| Malformed XML, nicht dekodierbar | `error` (Abbruch, kein `Definitions`) |
| Unbekannte FEEL-Variable, Typkonflikt, nicht unterstütztes Konstrukt in *einer* Decision | `Diagnostic{SevError}`; übrige Decisions kompilieren weiter |
| Unbekannter/nicht unterstützter Namespace an einer Decision | `Diagnostic{SevError}`; Decision nicht ausführbar |
| Decision kompiliert, aber mit Hinweis (z. B. ungenutzte Eingabe) | `Diagnostic{SevWarning/SevInfo}` |

`Compile` gibt also **nur bei nicht-dekodierbarem Dokument** ein `error`. Alles andere
landet in `Diagnostics`; `diags.HasErrors()` zeigt, ob ausführbare Decisions fehlen.
Eine Decision mit `SevError`-Diagnostic ist im `Definitions` präsent, aber **nicht
ausführbar**.

**`Evaluate` (hart, fail-fast):** gibt ein `error` zurück, wenn —

| Situation | Mechanismus |
|---|---|
| Angeforderte Decision wegen Compile-Fehler **nicht ausführbar** | `error` |
| **Pflicht-Eingabe** (vom Modell referenziertes Input Data) fehlt | `error` |
| Kontext gecancelt / Deadline | `error` (`ctx.Err()`) |
| Ressourcenlimit erschöpft (ADR-0008) | `error` |
| `UNIQUE`-Hit-Policy mit Mehrfachtreffer | `error` |
| FEEL-Ausdruck ergibt spec-konform `null` (Typkonflikt zur Laufzeit, Division durch 0, …) | **kein** `error` → `null` + ggf. `Diags` |

> **Verbindlich:** Der Aufrufer prüft nach `Compile` `diags.HasErrors()`, *bevor* er
> `Decision`/`Evaluate` ruft. `Evaluate` wiederholt diese Prüfung nicht kulant, sondern
> verweigert hart — eine nicht-ausführbare Decision oder eine fehlende Pflicht-Eingabe ist
> ein Programmierfehler des Aufrufers, kein Datenfall, und soll nicht als stilles `null`
> maskiert werden. (Spätere Promotion zu Decision-Graph-Chaining, WP-28, ändert daran
> nichts: fehlt eine *erforderliche* Sub-Decision, ist das `error`, nicht `null`.)

#### Stabilität von `Diagnostic.Code`

`Code` ist die **maschinenlesbare, stabile** Fehlerklasse (z. B. `FEEL_TYPE_MISMATCH`,
`UNKNOWN_VARIABLE`, `UNIQUE_MULTIPLE_MATCH`, `XML_MALFORMED`). Er kodiert die
*Fehlerklasse*, **nicht** die Severity. Codes werden in einer Registry geführt und nur
additiv erweitert; Umbenennung/Entfernung ist ein Breaking Change. `Message` ist
menschenlesbar und **nicht** stabil — Aufrufer dürfen nur gegen `Code` programmieren.

> **Folgeaufgabe (Bug):** `fromModelDiagnostics` erzeugt heute `Code = "MODEL_" + severity`
> und verletzt damit diese Zusage (Severity statt Fehlerklasse). Vor 1.0 auf echte
> Klassen-Codes umstellen.

### 1.5 Go ⇄ FEEL Typ-Mapping

| Go-Eingabe | FEEL-Typ |
|---|---|
| `nil` | Null |
| `bool` | Boolean |
| `int, int64, float64, *big.Rat, string-decimal?` | Number (zu Decimal konvertiert) |
| `string` | String |
| `time.Time` | Date / Date-Time (je nach Flag) |
| `[]any` | List |
| `map[string]any` | Context |

> **Achtung Number:** `float64`-Eingaben werden nach Decimal konvertiert — der Aufrufer ist
> für Genauigkeit am Rand verantwortlich. Für exakte Beträge `string` oder ein Decimal-Typ
> bevorzugen. (Dokumentations-Pflicht in WP-45.)

### 1.6 Minimalbeispiel (Vertrag)

```go
eng := dmn.New()
defs, diags, err := eng.Compile(ctx, xmlBytes)
if err != nil || diags.HasErrors() { /* ... */ }
dec, _ := defs.Decision("Loan Approval")
res, _ := dec.Evaluate(ctx, dmn.Input{
    "Applicant Age": 35,
    "Credit Score":  720,
})
fmt.Println(res.Outputs["Loan Approval"])
```

## 2. HTTP-Service (`service/http.go`, `cmd/temisd`)

OpenAPI in `service/openapi.yaml`. Endpunkte:

| Methode | Pfad | Zweck |
|---|---|---|
| `POST` | `/v1/models` | DMN-XML hochladen → kompilieren, gibt `modelId` + Diagnostics |
| `GET` | `/v1/models` | Liste aller gecachten Modelle (`modelId`, Decisions, Inputs) — abschaltbar |
| `GET` | `/v1/models/{id}` | Index (Decisions/Services/Inputs) + `schema` |
| `GET` | `/v1/models/{id}/xml` | Rohes DMN-XML wie hochgeladen (Editor-Reopen) |
| `POST` | `/v1/models/{id}/evaluate` | `{ "decision", "input", "explain"?, "strict"? }` → `Result` (+ `trace` bei `explain`; `422 INVALID_INPUT` + `problems` bei `strict`) |
| `POST` | `/v1/models/{id}/evaluate-graph` | Ganzes Modell: Leaf-Inputs einmal füllen → Wert (und `trace`) jeder Decision + `inputSchema` |
| `POST` | `/v1/evaluate` | Stateless: XML + Input in einem Request (kein Cache); `explain`/`strict` analog |
| `GET` | `/docs` | Interaktive Swagger-UI-Testseite (lädt `/openapi.yaml`) |
| `GET` | `/openapi.yaml` | Eingebettetes OpenAPI-3-Dokument |
| `GET` | `/healthz`, `/readyz` | Liveness/Readiness |

- Modelle werden serverseitig gecacht (WP-35), Key = Hash des XML.
- **Modell-Listing abschaltbar:** `GET /v1/models` listet alle gecachten Modelle.
  Mit `temisd -list-models=false` (oder `service.WithModelListing(false)`) lässt sich
  der Endpunkt deaktivieren, damit niemand die hinterlegten Decisions einsehen kann; er
  antwortet dann `404` mit `code: NOT_FOUND`. Standard: aktiviert.
- Fehlerantworten: RFC-7807 `application/problem+json` mit stabilem `code`.
- Limits (WP-34) gelten pro Request.
- **Scoped API-Keys (ADR-0028, WP-100–102):** siehe §2.2 für den Scope-Vertrag.
  Mit `temisd -keys-file <datei>` (oder `TEMIS_KEYS_FILE`) verlangen `/v1`, `/mcp`
  und gRPC einen `Authorization: Bearer <kid>.<secret>`-Key; fehlt/ungültig/
  abgelaufen/widerrufen → `401` mit `code: UNAUTHORIZED` (`WWW-Authenticate: Bearer`),
  gültig aber ohne den nötigen Scope → `403` mit `code: FORBIDDEN`. Der deprecated
  `-token`/`TEMIS_API_TOKEN` läuft als Legacy-Admin-Key weiter (deckt alle Routen).
  Ohne Keys **und** ohne Legacy-Token ist die API offen. `/docs`, `/openapi.yaml`
  und die Health-Probes sind nie gegated. Das OpenAPI-Dokument beschreibt das
  `bearerAuth`-Schema (Swagger-UI-**Authorize**).
- **Optionales Audit-Logbuch (clio, ADR-0023, WP-54):** Mit `temisd -clio-url …`
  protokolliert der Server jede Einzel-Decision-Auswertung (`/v1/evaluate`,
  `/v1/models/{id}/evaluate`) als `com.temis.decision.evaluated.v1`-CloudEvent in einer
  clio-Instanz (`service.WithClioSink`). **Default aus** ⇒ Antworten byte-identisch. Im
  best-effort-Default verändert der Sink die Antwort nie; mit `-clio-strict` (fail-closed)
  kann eine fehlgeschlagene Audit-Schreibung den Request mit `502` und
  `code: AUDIT_WRITE_FAILED` beenden. Vertrag & Betrieb: `docs/80-clio-decision-log.md`.

### 2.2 Scope-Vertrag (ADR-0028)

Der Auth-Vertrag spiegelt clios `kid.secret`-Modell und ist Teil der stabilen
Oberfläche (unterliegt SemVer): eine neue Route braucht eine bewusste Scope-Zuordnung.

- **Token-Format:** `Authorization: Bearer <kid>.<secret>`. Der `kid` ist öffentlich
  (loggbar), das `secret` geheim. Gespeichert wird je Key nur
  `{kid, sha256(secret), scopes[], owner, expiresAt?, revoked}` — **kein Klartext**,
  Vergleich in Konstantzeit (`crypto/subtle`).
- **Semantik:** kein/unbekannter `kid`/falsches Secret/abgelaufen/widerrufen → `401`
  (`UNAUTHORIZED`, `WWW-Authenticate: Bearer`); gültig, aber Scope fehlt → `403`
  (`FORBIDDEN`, RFC-7807). Ohne Keys **und** ohne Legacy-Token bleibt die API offen.
- **`admin` ist ein Super-Scope:** ein Admin-Key erfüllt jede Scope-Anforderung; der
  deprecated Legacy-Token ist ein synthetischer Admin-Key (byte-identisches Verhalten).

| Scope | Deckt ab |
|---|---|
| `evaluate` | `POST /v1/evaluate`, `/v1/models/{id}/evaluate`, `/v1/models/{id}/evaluate-graph`; gRPC `Evaluate`/`EvaluateBatch`; MCP `evaluate` |
| `models:read` | `GET /v1/models` (Listing), `/{id}`, `/xml`, `/graph`, `/types`, alle `GET …/decisions/*`, `…/bkm/*`; MCP `list_models`/`load_model`/`describe_decision` |
| `models:write` | `POST /v1/models`, `save`/`rename`/`create-*`, alle Modeler-Edits (auch `DELETE …/types/{name}`); gRPC `Compile` |
| `git` | alle `/v1/git/*` und MCP `git_*` (Provider-Token bleibt per-Request `X-Git-Token`) |
| `assist` | `POST /v1/chat` (LLM-Assistent, kostenverursachend) |
| `flow` | `POST /v1/flows`, `/v1/flows/{id}/evaluate`, `/v1/flow/evaluate`; MCP `load_flow`/`describe_flow`/`evaluate_flow` |
| `admin` | `DELETE /v1/models/{id}` (Modell-Löschung), Key-Management (Phase 2), Betriebs-/Dev-Routen; Super-Scope |
| `audit` | read-only Auth-/Audit-Log (Phase 3) |

**Statische Konfiguration (WP-102):** Keys kommen aus einer JSON-Datei
(`-keys-file`/`$TEMIS_KEYS_FILE`; je Eintrag bevorzugt `secretHash` (hex `sha256`),
alternativ Klartext-`secret`, plus `scopes[]`, `owner?`, `expiresAt?` RFC-3339,
`revoked?`) und/oder einem Bootstrap-Admin-Key (`$TEMIS_BOOTSTRAP_ADMIN_KEY` = das
Secret; der abgeleitete `kid` wird beim Start geloggt, das Secret nie). Persistenz
und Lifecycle-API (`/v1/keys*`) folgen in Phase 2 (WP-103/104).

### 2.1 Modeler-Endpunkte (ADR-0016)

Diese `/v1`-Endpunkte bedienen den **eingebauten DMN-Modeler** (ausgeliefert unter `/`):
Modellstruktur und Decision-Logik lesen und editieren. Sie liegen auf **derselben
token-geschützten `/v1`-Oberfläche** wie die Kern-API und gehören damit zum Vertrag.
Jeder **mutierende** Aufruf rekompiliert das gepatchte DMN-XML und antwortet `201` mit der
gespeicherten Modell-Antwort (`ModelResponse`) unter ihrer neuen content-addressierten
`modelId` — der Client wechselt damit auf die persistierte Revision. Vollständige
Schemas in `service/openapi.yaml`.

| Methode | Pfad | Zweck |
|---|---|---|
| `GET` | `/v1/models/{id}/graph` | Decision Requirements Graph (Knoten + Kanten) zum Zeichnen |
| `POST` | `/v1/models/{id}/graph` | Graph abgleichen (Knoten/Kanten hinzufügen/entfernen/verschieben) → `201` |
| `GET` | `/v1/models/{id}/types` | Benannte Item Definitions des Modells |
| `POST` | `/v1/models/{id}/types` | Einfache Item Definition anlegen/ändern → `201` |
| `DELETE` | `/v1/models/{id}/types/{name}` | Item Definition entfernen → `201` |
| `GET` | `/v1/models/{id}/decisions/{decision}/table` | Decision-Table-Ansicht (Hit Policy, Spalten, Regeln) |
| `POST` | `/v1/models/{id}/decisions/{decision}/table` | Regeln einer Decision Table neu schreiben → `201` |
| `POST` | `/v1/models/{id}/decisions/{decision}/create-table` | Unentschiedener Decision eine frische Tabelle geben → `201` |
| `GET` | `/v1/models/{id}/decisions/{decision}/literal` | Literal-Expression-Ansicht einer Decision |
| `POST` | `/v1/models/{id}/decisions/{decision}/literal` | Literal-Expression-Logik setzen → `201` |
| `GET` | `/v1/models/{id}/bkm/{bkm}` | BKM-Ansicht (Parameter + Body) |
| `POST` | `/v1/models/{id}/bkm/{bkm}` | BKM-Funktion setzen → `201` |
| `POST` | `/v1/models/{id}/save` | Knoten-Edits (Position/Name/Typ) ins XML patchen → `201` |

## 3. gRPC-Service (`proto/dmn/v1/engine.proto`, WP-33)

Implementiert über **ConnectRPC** (ADR-0020): die Handler sprechen gRPC, gRPC-Web und
das Connect-Protokoll und laufen im selben `service.Server`/Mux **auf demselben Port**
wie der REST-Service (`temisd`); Engine und Modell-Cache sind geteilt. Klartext-HTTP/2
(h2c) ist aktiv, sodass voller gRPC und der bidi-Stream auch ohne TLS funktionieren.

```proto
package dmn.v1;
service DmnEngine {
  rpc Compile(CompileRequest) returns (CompileResponse);
  rpc Evaluate(EvaluateRequest) returns (EvaluateResponse);
  rpc EvaluateBatch(stream EvaluateRequest) returns (stream EvaluateResponse);
}
```

- `Evaluate`/`EvaluateBatch` wählen das Modell per `model_id` (zuvor kompiliert) **oder**
  inline `xml` (stateless, wird gecacht); `decision`, `explain`, `strict` analog zu §2.
- `EvaluateBatch` ist ein **bidirektionaler Stream**: je Request genau eine Response, in
  Reihenfolge — Pipelining vieler Auswertungen über eine Verbindung.
- `Input`/`Output`/`trace` als `google.protobuf.Struct` (deckt das Go⇄FEEL-Mapping ab).
- Decimal-genaue Zahlen als String transportieren, um JSON-/proto-float-Verlust zu
  vermeiden (ADR-0007-Konsequenz).
- Der optionale Bearer-Token (§2) gilt per Interceptor für **jeden** RPC (sonst
  `CodeUnauthenticated`). Fehler-Mapping: fehlendes Modell/Decision → `NotFound`,
  Schema-Verletzung (`strict`) → `InvalidArgument`, sonstige Auswertungsfehler →
  `FailedPrecondition`.
- Generierter Code (`internal/gen/dmnv1/`) ist committet; `make proto` regeneriert,
  eine CI-Lane prüft auf Drift (ADR-0020).

## 4. Versionierung & Stabilität (SemVer, WP-43)

Ab WP-43 ist `package dmn` als **stabile v1-Oberfläche** zugesagt (ADR-0011,
ADR-0019). Die Engine folgt [Semantic Versioning](https://semver.org/lang/de/):

- **Stabiler Vertrag (v1):** die exportierten Symbole von `package dmn` —
  `Engine`/`New`/`Option` (+ `WithLimits`), `Compile`, `Definitions`
  (+ `Decision`/`Service`/`InputSchema`/`Index`/`ModelName`), `CompiledDecision`
  (+ `Evaluate`/`EvalOption`/`WithTrace`/`WithStrictInput`/`ValidateInput`),
  `CompiledService`, `Input`/`Result`/`Trace`, `Diagnostics`/`Diagnostic`/`Sev*`,
  die `Code*`-Konstanten, `EvalError`/`InputError`, `InputField`/`InputProblem`,
  `Limits`. Diese Menge ist durch den **API-Surface-Golden-Test**
  (`dmn/apisurface_test.go` → `testdata/api/dmn.api`) eingefroren: jede Änderung
  der exportierten Oberfläche bricht CI und erzwingt eine bewusste Entscheidung.
- **Additive Änderungen** (neue Funktion/Typ/Feld/`Code*`) → Minor. Golden mit
  `-update-api` aktualisieren.
- **Breaking Changes** (Umbenennen/Entfernen/Signaturänderung; auch das
  Verschieben der Compile-/Eval-Fehlergrenze aus §1.4 oder das Umbenennen eines
  `Diagnostic.Code`) → **Major**. Go-Modulpfad endet bei Major ≥ 2 auf `/vN`.
- **Deprecation-Policy:** ein zu entfernendes Symbol wird zuerst mit
  `// Deprecated: <Grund/Ersatz>` markiert (bleibt voll funktionsfähig),
  frühestens im nächsten Major entfernt.
- **`internal/` ist ausgenommen** (privat, jederzeit änderbar) — nur `package dmn`
  ist der SemVer-Vertrag (ADR-0011).
- **HTTP/gRPC:** HTTP-Pfade tragen `/v1`, gRPC-Package `dmn.v1`; RFC-7807-`code`
  und `Diagnostic.Code` sind additiv stabil (§1.4/§2).
