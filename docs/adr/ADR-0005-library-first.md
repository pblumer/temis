# ADR-0005: Library-first, Service als dünner Wrapper

- **Status:** accepted
- **Datum:** 2026-06-11

## Kontext
Anforderung: betreibbar als Library UND als Service.

## Optionen
1. **Service-first** — Engine hinter Netzwerkgrenze, Library nachgelagert.
2. **Library-first** — Kern ist die Go-API; HTTP/gRPC sind dünne Adapter darüber.

## Entscheidung
Library-first. `package dmn` ist die einzige öffentliche API; `service/` und `cmd/temisd`
nutzen ausschließlich diese.

## Konsequenzen
- Saubere Testbarkeit ohne Netzwerk.
- Service erbt automatisch Korrektheit/Performance der Library.
- Klarer Grenzverlauf für KI-Agenten (kein Durchgriff auf `internal/`).
