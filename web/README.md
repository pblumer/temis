# Temis Web — Einsteiger-Editor (F-01, optional)

Ein **optionales** Frontend für Temis: bettet [dmn-js](https://github.com/bpmn-io/dmn-js)
**unverändert** ein (DRD- + Decision-Table-Editor) und wertet das Modell direkt gegen die
laufende temis-HTTP-API (`temisd`) aus.

> **Kein Produktziel der Engine.** Das Go-Modul bleibt frontend-frei (ADR-0006, ADR-0011).
> Dieses Verzeichnis hat eine eigene Toolchain und wird nicht von `go build`/`make` erfasst.
> dmn-js wird **nie geforkt**, nur über additive Module angepasst; das bpmn.io-Logo bleibt
> sichtbar (bpmn.io-Lizenz). Hintergrund: `docs/adr/ADR-0012-einsteiger-editor.md`.

## Voraussetzungen

- Node.js ≥ 18
- Ein laufender temis-Service. Aus dem Repo-Root:

  ```sh
  go run ./cmd/temisd -addr :8080
  ```

## Entwickeln

```sh
cd web
npm install
npm run dev
```

Vite startet auf <http://localhost:5173> und proxyt `/v1`, `/healthz`, `/readyz` an
`http://localhost:8080`. Lauscht `temisd` woanders:

```sh
TEMIS_API=http://localhost:9000 npm run dev
```

Verlangt der Server ein Token (`temisd -token …`), trägt man es im Feld **Bearer-Token** ein.

## Bedienung

1. Diagramm im Editor zeichnen (es öffnet eine kleine Beispiel-Decision).
2. **Modell prüfen** — speichert das DMN-XML und schickt es an `POST /v1/models`;
   erkannte Decisions und Inputs erscheinen rechts.
3. Decision wählen, Eingabewerte setzen (JSON oder Text) und **Auswerten** —
   ruft `POST /v1/models/{id}/evaluate`. Outputs und Diagnostics werden angezeigt.

## Bauen

```sh
npm run build    # statische Dateien nach web/dist/
npm run preview  # gebautes Bundle lokal ansehen
```

Beim Ausliefern hinter denselben Origin wie `temisd` legen (oder einen Reverse-Proxy für
`/v1` einrichten), damit die relativen API-Pfade greifen.

## Aufbau

| Datei | Inhalt |
|---|---|
| `index.html` | Layout (Editor links, Auswerte-Panel rechts) |
| `src/main.js` | dmn-js-Modeler einbetten, Auswerte-Flow, Rendering |
| `src/api.js` | dünner Client für die temis-HTTP-API |
| `src/starter.js` | Start-Diagramm (DRD + Decision Table) |
| `src/style.css` | Styles des Panels (dmn-js behält sein eigenes Theme) |
| `src/branding.js` | Theme-/Branding-System (CI-Anpassung, ADR-0016) |
| `public/branding.js` | Deployment-Branding-Vorlage (leer = Standard-Themes) |
| `vite.config.js` | Dev-Server + Proxy auf `temisd` |

## Theming / Branding (CI-Anpassung)

Die Oberfläche steuert ihr Aussehen über CSS-Variablen (`src/style.css`). `src/branding.js`
bietet darauf aufbauend zwei eingebaute Themes (**Temis Dunkel**, **Temis Hell**); der
Umschalter sitzt rechts in der Kopfzeile, die Wahl wird in `localStorage` gemerkt. Hintergrund:
`docs/adr/ADR-0016-frontend-theming-branding.md`.

Für die **Corporate Identity einer Firma** lässt sich die Oberfläche ohne Neubau des Bundles
anpassen: beim Ausliefern `public/branding.js` (wird nach `dist/branding.js` kopiert) durch eine
eigene Version ersetzen und `window.TEMIS_BRANDING` setzen — Produktname, Logo und ein eigenes
Theme (erbt per `base` von einem eingebauten Theme, überschreibt nur Abweichendes über `vars`):

```js
window.TEMIS_BRANDING = {
  brand: 'ACME AG',
  logo: '/branding/acme-logo.svg',
  theme: { id: 'acme', label: 'ACME', base: 'temis-light', vars: { '--accent': '#e4002b' } },
  defaultTheme: 'acme',
  allowUserSwitch: true, // false = fest gebrandetes Deployment ohne Umschalter
};
```

Die Vorlage `public/branding.js` enthält ein kommentiertes Beispiel. Theme-Auswahl in Reihenfolge:
`?theme=` (URL) > gespeicherte Nutzerwahl > `defaultTheme` > Firmen-Theme > `temis-dark`.

## Nächste Schritte (F-02, siehe Roadmap)

- Einsteiger-Module: reduzierte Palette, Decision-Table-Vorlagen, Inline-FEEL-Hilfe.
- **Diagnostics-Overlay**, das temis-Diagnostics (`line/col`) auf die betroffenen
  Tabellenzellen mappt.
