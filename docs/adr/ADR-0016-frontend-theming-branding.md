# ADR-0016: Theming/Branding des Frontends über CSS-Variablen + Deployment-Hook

- **Status:** accepted
- **Datum:** 2026-06-29
- **Kontext-WP:** F-01

## Kontext
Die Oberflächen von temis — der Einsteiger-Editor (`web/`, ADR-0012) und der vom Service
ausgelieferte DMN-Playground (`service/ui.go`) — sollen sich an die **Corporate Identity
(CI)** einer Firma anpassen lassen — Farben, Logo, Produktname —, damit temis bei
Kunden/Partnern im eigenen Look eingebettet werden kann. Beide steuern ihr Aussehen bereits
vollständig über **CSS-Custom-Properties** auf `:root` (`--bg`, `--accent`, …); die
dmn-js-Zeichenfläche behält bewusst ihr eigenes (helles) Theme.

Naheliegender Reflex wäre, Styles pro Kunde zu kopieren oder das Bundle je Deployment
neu zu bauen. Beides skaliert schlecht und vermischt Auslieferung mit Build.

## Optionen
1. **Pro Kunde Styles/Bundle forken** — Wartungslast, Build je Deployment, Drift
   zwischen Varianten. **Verworfen.**
2. **CSS-Variablen-Themes + Deployment-Hook (ohne Neubau)** — Themes sind reine Sammlungen
   von Custom-Properties, die zur Laufzeit auf `<html>` gesetzt werden; ein optionales
   `window.TEMIS_BRANDING` (ausgelieferte `public/branding.js` oder per Reverse-Proxy
   injiziert) registriert ein Firmen-Theme und setzt Logo/Produktname. **Gewählt.**

## Entscheidung
Option 2. Das Theming bleibt **rein im Frontend** (Go-Modul bleibt frontend-frei,
ADR-0006/0011) und additiv (dmn-js wird nicht angefasst, ADR-0012):

- **Eingebaute Themes** (`temis-dark`, `temis-light`) in `web/src/branding.js`; die
  Nutzerwahl wird in `localStorage` gemerkt und über einen Umschalter in der Kopfzeile
  angeboten.
- **Deployment-Branding** über `window.TEMIS_BRANDING`: setzt Produktname/Untertitel/Logo
  und registriert optional ein **Firmen-Theme**, das von einem eingebauten Theme erben
  (`base`) und nur Abweichendes (`vars`) überschreiben kann. Standard-`public/branding.js`
  ist leer; eine Firma überschreibt sie beim Ausliefern — **kein Neubau des Bundles**.
- **Theme-Auswahl** in Reihenfolge: `?theme=` (URL) > gespeicherte Nutzerwahl >
  `defaultTheme` > Firmen-Theme > `temis-dark`. `allowUserSwitch: false` blendet den
  Umschalter aus (fest gebrandetes Deployment).
- **Beide Oberflächen** teilen dasselbe Modell: `web/` lädt `branding.js` als Asset,
  `service/ui.go` trägt eine inhaltsgleiche, kompakte Variante inline (die Playground-Seite
  bleibt asset-frei/offline, ADR-0006-konform; das Branding-Global kann dort per
  Reverse-Proxy injiziert werden).

## Konsequenzen
- **Positiv:** CI-Anpassung ohne Fork und ohne kundenspezifischen Build; eine einzige
  Code-Basis; Branding ist eine Auslieferungs-, keine Build-Entscheidung.
- **Negativ:** Branding-Logik liegt clientseitig (nicht serverseitig erzwungen); Logos für
  Firmen müssen separat ausgeliefert/gehostet werden.
- **Folgeaufgaben:** keine zwingenden; weitere eingebaute Themes oder ein
  Theme-Editor sind optional nachziehbar.
