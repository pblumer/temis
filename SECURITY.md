# Security Policy

## Unterstützte Versionen

Temis ist Vor-1.0 (die Binaries tragen bis zum ersten getaggten Release
`0.0.0-dev`). Sicherheitsfixes landen auf `main`; sobald es getaggte Releases
gibt, wird jeweils die neueste Minor-Version gepflegt. Bis dahin gilt: gegen
`main` bauen.

## Eine Schwachstelle melden

Bitte **keine** öffentlichen Issues für Sicherheitslücken anlegen. Nutze
stattdessen einen der privaten Kanäle:

- GitHub → **Security → Report a vulnerability** (Private Vulnerability
  Reporting) im Repository `pblumer/temis`, oder
- E-Mail an den Maintainer: **patrick@blumer.net** (gerne PGP, Schlüssel auf
  Anfrage).

Bitte beschreibe Angriffspfad, betroffene Version/Commit und – wenn möglich –
einen minimalen Reproduktionsfall. Wir bestätigen den Eingang in der Regel
innerhalb weniger Tage und halten dich bis zur Behebung auf dem Laufenden. Eine
koordinierte Offenlegung nach dem Fix ist ausdrücklich willkommen.

## Sicherheits-Grundhaltung (wichtig für Betreiber)

Temis ist **zero-config** startbar, und dieser Standard ist **bewusst offen**,
damit die Demo ohne Parameter läuft. Für einen produktiven Betrieb sind die
folgenden Punkte betreiberseitig zu härten:

- **Authentifizierung ist standardmäßig aus.** Ohne `-keys-file`/`-keys-dir`
  (bzw. das veraltete `-token`) ist die gesamte `/v1`-, `/mcp`- und gRPC-
  Oberfläche **offen**. Für Produktion scoped API-Keys konfigurieren
  (ADR-0028).
- **Kein TLS im Binary.** `temisd` spricht per Default Klartext-HTTP/h2c;
  entweder TLS extern terminieren oder `-tls-cert`/`-tls-key` setzen. Ohne TLS
  reisen API-Keys sowie Git-/LLM-Token unverschlüsselt.
- **Modellierungs-Assistent.** `POST /v1/chat` ist per Default aktiv (BYOK). Ist
  ein server-seitiger LLM-Key gesetzt **ohne** API-Auth, ist der Endpoint ein
  offener Kosten-Proxy — temisd warnt beim Start. Absichern über API-Keys
  und/oder `-rate-limit`.
- **clio-Audit-Sink** bleibt ohne Token aus; es verlässt nichts den Prozess,
  bis `TEMIS_CLIO_TOKEN` gesetzt ist.

Die vollständige Konfiguration steht in `README.md` und `docs/`; die Auth-Details
in `docs/40-api-contract.md` und ADR-0028.
