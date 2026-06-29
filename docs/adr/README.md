# Architecture Decision Records (ADR)

Index der getroffenen Architekturentscheidungen. Neue Entscheidungen: `ADR-template.md`
kopieren, fortlaufend nummerieren, im `00-overview.md` referenzieren falls rahmenrelevant.

| ADR | Titel | Status |
|---|---|---|
| 0001 | Implementierungssprache Go | accepted |
| 0002 | Ziel-DMN-Version 1.5 (lesend 1.3/1.4) | accepted |
| 0003 | Voller FEEL-Scope inkl. aller Boxed Expressions | accepted |
| 0004 | Ausführung via Compile-to-Closures | accepted |
| 0005 | Library-first, Service als Wrapper | accepted |
| 0006 | dmn-js als Editor, Standard-DMN-XML als Schnittstelle | accepted |
| 0007 | FEEL-Number als Decimal (nicht float64) | accepted |
| 0008 | Ressourcenlimits & Sandboxing | accepted |
| 0009 | Projektname „Temis" | accepted |
| 0010 | DMNDI-Round-trip über verbatim Token-Stream | accepted |
| 0011 | Core Engine als reine Go-Library (`Compile`/`Evaluate`), Service nur Adapter | accepted |
| 0012 | F-01 als Einsteiger-Editor (separates Frontend, dmn-js unverändert) | accepted |
| 0013 | temis als Laufzeit-Verifikationswerkzeug für KI-Agenten (Agent-First-Schnittstelle) | accepted |
| 0014 | MCP über Standardbibliothek implementieren (kein offizielles Go-MCP-SDK) | accepted |
| 0015 | Remote-MCP über nativen Streamable-HTTP-Transport (`temis-mcp -http`) | accepted |
