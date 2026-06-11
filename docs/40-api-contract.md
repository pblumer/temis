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
}

func (c *CompiledDecision) Evaluate(ctx context.Context, in Input) (Result, error)
```

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

Regel: **Compile-Fehler** → über `Diagnostics` + ggf. `error`. **Evaluate** gibt nur bei
echten Laufzeitproblemen (Limit überschritten, Kontext-Cancel) ein `error`; spec-konforme
`null`-Ergebnisse sind **kein** Fehler.

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
| `GET` | `/v1/models/{id}` | Index (Decisions/Services/Inputs) |
| `POST` | `/v1/models/{id}/evaluate` | `{ "decision": "...", "input": {...} }` → `Result` |
| `POST` | `/v1/evaluate` | Stateless: XML + Input in einem Request (kein Cache) |
| `GET` | `/healthz`, `/readyz` | Liveness/Readiness |

- Modelle werden serverseitig gecacht (WP-35), Key = Hash des XML.
- Fehlerantworten: RFC-7807 `application/problem+json` mit stabilem `code`.
- Limits (WP-34) gelten pro Request.

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
