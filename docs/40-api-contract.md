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

#### Entscheidungsspur (`Trace`) — opt-in, ADR-0012/WP-51

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

#### Eingabe-Schema & strenge Validierung — ADR-0012/WP-52

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
| `GET` | `/v1/models/{id}` | Index (Decisions/Services/Inputs) |
| `POST` | `/v1/models/{id}/evaluate` | `{ "decision", "input", "explain"?, "strict"? }` → `Result` (+ `trace` bei `explain`; `422 INVALID_INPUT` + `problems` bei `strict`) |
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
- **Optionaler Token-Schutz:** Mit `temisd -token <token>` (oder `TEMIS_API_TOKEN`)
  verlangen die `/v1`-Endpunkte `Authorization: Bearer <token>`; fehlt/falsch →
  `401` mit `code: UNAUTHORIZED` (`WWW-Authenticate: Bearer`). Ohne Token ist die
  API offen. `/docs`, `/openapi.yaml` und die Health-Probes sind nie gegated. Das
  OpenAPI-Dokument beschreibt das `bearerAuth`-Schema (Swagger-UI-**Authorize**).

## 3. gRPC-Service (`service/dmn.proto`)

```proto
service DmnEngine {
  rpc Compile(CompileRequest) returns (CompileResponse);
  rpc Evaluate(EvaluateRequest) returns (EvaluateResponse);
  rpc EvaluateBatch(stream EvaluateRequest) returns (stream EvaluateResponse);
}
```

- `Input`/`Output` als `google.protobuf.Struct` (deckt das Go⇄FEEL-Mapping ab).
- Decimal-genaue Zahlen als String-Feld transportieren, um JSON-/proto-float-Verlust zu
  vermeiden (ADR-0007-Konsequenz).

## 4. Versionierung

- Go-Modulpfad endet bei Major ≥ 2 auf `/v2` (Go-Konvention).
- HTTP-Pfade tragen `/v1`. gRPC-Package `dmn.v1`.
- Stabilität ab WP-43. Davor „experimental", in README markiert.
