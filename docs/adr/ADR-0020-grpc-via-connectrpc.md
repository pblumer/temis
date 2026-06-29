# ADR-0020: gRPC-Schnittstelle über ConnectRPC (nicht grpc-go)

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** WP-33 (gRPC-Service-Wrapper)

## Kontext

WP-33 fordert eine gRPC-Schnittstelle (`Compile`/`Evaluate`-RPCs, Streaming für
Batch; docs/40-api-contract.md §3). Bisher hatte das Projekt **genau eine**
Laufzeit-Abhängigkeit (`apd/v3` für Decimal, ADR-0007); ADR-0014 verlangt für
jede neue Abhängigkeit einen ADR und eine bewusste Entscheidung. gRPC lässt sich
in Go nicht sinnvoll ohne Protobuf-Codegen umsetzen — die Frage ist **welcher
Stack**, nicht ob überhaupt eine Abhängigkeit dazukommt.

## Optionen

1. **`google.golang.org/grpc` + `protobuf` (kanonisch).** Der Standard-Stack.
   Reif, vollständig, aber großer transitiver Footprint (`x/net`, `x/sys`,
   `x/text`, `genproto`, eigener HTTP/2-Stack, Balancer/Resolver/…). Bringt viel
   Maschinerie, die ein dünner Engine-Wrapper nicht braucht.
2. **ConnectRPC (`connectrpc.com/connect`) + `protobuf`.** Spricht **gRPC,
   gRPC-Web und das Connect-Protokoll** über das Standard-`net/http`. Die
   Bibliothek selbst hat **null** Abhängigkeiten außer der Protobuf-Runtime;
   Handler sind gewöhnliche `http.Handler` und mounten auf denselben Mux wie der
   REST-Service. Für **Klartext**-gRPC und den **bidirektionalen** Batch-Stream
   wird HTTP/2 ohne TLS (h2c) benötigt → `golang.org/x/net/http2/h2c`.
3. **Eigenes Protobuf/gRPC über stdlib** (analog ADR-0014 für MCP). — Für JSON-RPC
   (MCP) tragbar; ein konformer gRPC-/HTTP-2-Server inkl. Framing/Flow-Control von
   Hand ist unverhältnismäßig und fehleranfällig.

## Entscheidung

**Option 2: ConnectRPC.** Es passt zur Linie des Projekts „so nah an der
Standardbibliothek wie möglich" (vgl. ADR-0014: MCP über stdlib statt SDK): die
Handler sind `net/http`-Handler, teilen sich Engine **und** Modell-Cache mit dem
HTTP-Service und laufen auf demselben Port. Der Dependency-Zuwachs bleibt minimal
und gut abgrenzbar:

- `connectrpc.com/connect` — RPC-Runtime, selbst ohne Fremd-Deps.
- `google.golang.org/protobuf` — Protobuf-Runtime (für `Compile`/`Evaluate`-
  Nachrichten und `google.protobuf.Struct` als Input/Output-Träger).
- `golang.org/x/net` — nur für **h2c** (Klartext-HTTP/2), damit voller gRPC und
  der bidi-Batch-Stream auch ohne TLS funktionieren. `x/text` kommt transitiv mit.

Das ist deutlich weniger als der grpc-go-Footprint (der `x/net` ohnehin
mitbrächte) und liefert als Bonus gRPC-Web und ein HTTP/JSON-Fallback.

### Konsequenzen

- **Generierter Code wird committet** (`internal/gen/dmnv1/…`), erzeugt via
  `buf generate` (lokale Plugins `protoc-gen-go`, `protoc-gen-connect-go`), pinbar
  über `buf.yaml`/`buf.gen.yaml`. So baut `go build` **ohne** protoc/buf — dieselbe
  Politik wie `web/dist` (committetes Frontend, ADR-0016/WP-60). `make proto`
  regeneriert; `make verify` enthält eine Drift-Prüfung des generierten Codes.
- Die Proto-Quelle (`proto/dmn/v1/engine.proto`) ist die Vertrags-Quelle für §3;
  das gRPC-Package `dmn.v1` ist additiv-stabil (docs/40-api-contract.md §4).
- `internal/gen/` ist privat (kein Teil des `package dmn`-SemVer-Vertrags); der
  Wire-Vertrag wird über die `.proto` und einen `buf breaking`-Lauf geschützt.
- Decimal-Zahlen werden als String transportiert (ADR-0007-Konsequenz), Input/
  Output als `google.protobuf.Struct`.
- Die Pins berücksichtigen Go 1.23: `connect@v1.18.1` (jüngere Releases verlangen
  Go ≥ 1.25), `protobuf@v1.36.6`, `x/net@v0.41.0`.

## Status der Anti-Dependency-Regel

Bewusst akzeptierter Zuwachs von **einer** auf **vier** direkte Abhängigkeiten,
ausschließlich im Service-/Adapter-Layer (`service/`, `internal/gen/`). Der
Engine-Kern (`package dmn`, `internal/feel`, …) bleibt frei von Transport-/RPC-
Abhängigkeiten (ADR-0005/0011).
