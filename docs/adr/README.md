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
| 0006 | dmn-js als Editor, Standard-DMN-XML als Schnittstelle | superseded by 0016 |
| 0007 | FEEL-Number als Decimal (nicht float64) | accepted |
| 0008 | Ressourcenlimits & Sandboxing | accepted |
| 0009 | Projektname „Temis" | accepted |
| 0010 | DMNDI-Round-trip über verbatim Token-Stream | accepted |
| 0011 | Core Engine als reine Go-Library (`Compile`/`Evaluate`), Service nur Adapter | accepted |
| 0012 | F-01 als Einsteiger-Editor (separates Frontend, dmn-js unverändert) | superseded by 0016 |
| 0013 | temis als Laufzeit-Verifikationswerkzeug für KI-Agenten (Agent-First-Schnittstelle) | accepted |
| 0014 | MCP über Standardbibliothek implementieren (kein offizielles Go-MCP-SDK) | accepted |
| 0015 | Remote-MCP über nativen Streamable-HTTP-Transport (`temis-mcp -http`) | accepted |
| 0016 | Eigener DMN-Modeler durch Fork des MIT-Kerns (Loslösung von dmn-js, 1.5, BPMN-Synergie) | accepted |
| 0017 | Statische Typprüfung ist advisory (Warnung), FEEL bleibt dynamisch | accepted |
| 0018 | Theming/Branding der UI über CSS-Variablen + Deployment-Hook | accepted |
| 0019 | API-Stabilität, SemVer und Deprecation-Policy für `package dmn` | accepted |
| 0020 | gRPC-Schnittstelle über ConnectRPC (nicht grpc-go) | accepted |
| 0021 | MCP-Endpoint in `temisd` ko-lokalisieren (geteilter Modell-Cache, ein Adressraum) | accepted |
| 0022 | DMN-Modelle aus einem Git-Repository lesen/bearbeiten (Provider-Interface, GitHub zuerst) | proposed |
| 0023 | Entscheidungs-Logbuch über clio (revisionssicherer Event-Sink, opt-in) | proposed |
| 0024 | Eingebauter Modellierungs-Assistent über LLM-Provider (Standardbibliothek, kein SDK; Anthropic/OpenAI; opt-in) | accepted |
| 0025 | Decision- vs. Prozess-Orchestrierung: Schichtenmodell und die temis↔chrampfer-Naht | proposed |
| 0026 | Decision-Flow-Deskriptor (L2a): externe JSON-Komposition statt DMN-`import` | proposed |
| 0027 | Dateisystem-Modell-Store (optionale Persistenz des Modell-Cache, content-adressiert, stdlib, opt-in) | accepted |
| 0028 | Scoped API-Key-Authentifizierung (`kid.secret`, Keystore, Scopes) — angeglichen an clio | proposed |
