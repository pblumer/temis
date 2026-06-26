# Postman — Temis DMN API

Ready-to-import Postman files for the Temis HTTP API.

| File | Import as |
|---|---|
| `temis.postman_collection.json` | Collection |
| `temis.blumer.cloud.postman_environment.json` | Environment (`temis.blumer.cloud`) |

## Verwenden

1. In Postman **Import** → beide Dateien auswählen.
2. Oben rechts das Environment **temis.blumer.cloud** wählen
   (`baseUrl = https://temis.blumer.cloud`).
3. Falls der Server mit Token läuft (`temisd -token …`), die Variable **token**
   im Environment setzen. Die Collection sendet `Authorization: Bearer {{token}}`
   auf den `/v1`-Endpunkten automatisch; bei offenem Server bleibt `token` leer.

Ohne Environment funktioniert die Collection auch lokal: ihre Default-Variable
`baseUrl` zeigt auf `http://localhost:8080`.

## Inhalt

- **Discovery & Health** — `GET /docs` (Swagger UI), `GET /openapi.yaml`,
  `GET /healthz`, `GET /readyz` (öffentlich, ohne Token).
- **Models — Upload & Evaluate (stateful)** — `POST /v1/models` (lädt das
  Dish-Modell hoch und speichert die `modelId` in eine Collection-Variable) →
  `GET /v1/models/{{modelId}}` → `POST /v1/models/{{modelId}}/evaluate`. Die drei
  Requests der Reihe nach ausführen.
- **Stateless Evaluate — Examples** — je ein eigenständiger Request pro
  Beispiel­modell (`POST /v1/evaluate`, DMN-XML inline). Einfach **Send**:

  | Request | Beispiel-Input | Output |
  |---|---|---|
  | Dish | Season=Winter, Guest Count=4 | `Dish = Roastbeef` |
  | Discount | Customer Type=Business, Order Total=1500 | `Discount = 0.15` |
  | Eligibility | Applicant Age=20 | `Eligibility = ELIGIBLE` |
  | Loan Approval | Credit Score=800, Annual Income=60000 | `Approved`, Rate `0.035` |
  | Risk Score | Has Debt=true, Is New Customer=true | `Risk Score = 55` |
  | Net Total | Unit Price=19.99, Quantity=3 | `Net Total = 59.97` (exakt) |

Die Beispiele spiegeln `dmn/testdata/models` und die Golden-Tests der Engine;
jeder Request enthält einen Test, der den erwarteten Output prüft.

> Generiert aus den Modellen unter `dmn/testdata/models` — bei Änderungen an den
> Beispielmodellen die Dateien neu erzeugen (siehe Commit-Historie).
