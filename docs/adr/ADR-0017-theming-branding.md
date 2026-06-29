# ADR-0017: Theming/Branding der UI über CSS-Variablen + Deployment-Hook

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** F-01

## Kontext
Die interaktive Oberfläche von temis — der vom Service ausgelieferte DMN-Editor/Playground
(`service/ui.go`, `GET /` und `/ui`) — soll sich an die **Corporate Identity (CI)** einer
Firma anpassen lassen: Farben, Logo, Produktname. So lässt sich temis bei Kunden/Partnern im
eigenen Look einbetten. Die Seite steuert ihr Aussehen bereits vollständig über
**CSS-Custom-Properties** auf `:root` (`--bg`, `--accent`, …); die dmn-js-Zeichenfläche
behält bewusst ihr eigenes (helles) Theme.

Die Oberfläche ist eine einzige, in sich geschlossene HTML-Seite in einem Go-Raw-String
(inline CSS/JS, dmn-js per CDN). Naheliegender Reflex wäre, Styles pro Kunde zu kopieren
oder das Binary je Deployment neu zu bauen — beides skaliert schlecht und vermischt
Auslieferung mit Build.

## Optionen
1. **Pro Kunde Styles/Binary forken** — Wartungslast, Build je Deployment, Drift zwischen
   Varianten. **Verworfen.**
2. **CSS-Variablen-Themes + Deployment-Hook (ohne Neubau)** — Themes sind reine Sammlungen
   von Custom-Properties, die zur Laufzeit auf `<html>` gesetzt werden; ein optionales
   `window.TEMIS_BRANDING` (per Reverse-Proxy injiziert oder als kleines vorgeschaltetes
   Script) registriert ein Firmen-Theme und setzt Logo/Produktname. **Gewählt.**

## Entscheidung
Option 2. Das Theming bleibt **rein clientseitig** in der ausgelieferten Seite und ändert
nichts an der Engine (Go-Modul bleibt frontend-frei, ADR-0006/0011):

- **Eingebaute Themes** (`temis-dark`, `temis-light`) inline in `service/ui.go`; die
  Nutzerwahl wird in `localStorage` gemerkt und über einen Umschalter in der Kopfzeile
  angeboten.
- **Deployment-Branding** über `window.TEMIS_BRANDING`: setzt Produktname/Untertitel/Logo
  und registriert optional ein **Firmen-Theme**, das von einem eingebauten Theme erben
  (`base`) und nur Abweichendes (`vars`) überschreiben kann. Ist das Global nicht gesetzt,
  gelten die eingebauten Temis-Themes — **kein Neubau des Binaries**.
- **Theme-Auswahl** in Reihenfolge: `?theme=` (URL) > gespeicherte Nutzerwahl >
  `defaultTheme` > Firmen-Theme > `temis-dark`. `allowUserSwitch: false` blendet den
  Umschalter aus (fest gebrandetes Deployment).
- **Logo-Standard:** ein inline ausgeliefertes SVG-Logo (Raute = Entscheidungssymbol in
  DMN/Flowcharts, Häkchen = ausgewertete Decision), das die Akzentfarbe des aktiven Themes
  über `currentColor` übernimmt; das Favicon nutzt dieselbe Glyphe als Data-URI. Firmen
  überschreiben das Logo per `branding.logo`.
- **Asset-Freiheit bleibt:** die Seite lädt außer dmn-js (CDN, wie Swagger UI) keine
  weiteren Assets; das Branding-Global wird vom Betrieb beigesteuert, nicht vom Binary.

Unabhängig von ADR-0016 (eigener Modeler-Fork): Das Theming sitzt auf der Hüll-UI, nicht im
Modeler, und gilt für die dmn-js-Einbettung wie für einen späteren eigenen Modeler.

## Konsequenzen
- **Positiv:** CI-Anpassung ohne Fork und ohne kundenspezifischen Build; eine einzige
  Code-Basis; Branding ist eine Auslieferungs-, keine Build-Entscheidung.
- **Negativ:** Branding-Logik liegt clientseitig (nicht serverseitig erzwungen); Firmen-Logos
  müssen separat ausgeliefert/gehostet werden, sofern kein Data-URI verwendet wird.
- **Folgeaufgaben:** keine zwingenden; weitere eingebaute Themes oder ein serverseitig
  konfigurierbares Branding sind optional nachziehbar.
