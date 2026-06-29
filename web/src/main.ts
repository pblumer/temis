import { APP_NAME } from './build-info'
import { mountCanvas } from './canvas'
import './style.css'

// WP-61: the page now renders a real diagram-js canvas from the forked MIT core
// (diagram-js + its MIT deps), bundled and embedded — no dmn-js, no bpmn.io
// logo, no CDN, offline. The decision-table editor, real DMN renderers and the
// modeling interactions build on this core in WP-64/65 (ADR-0016).
function render(root: HTMLElement): void {
  root.innerHTML = `
    <main>
      <h1>${APP_NAME}</h1>
      <p class="sub">Eigener DMN-Modeler · diagram-js (MIT) + eigene DMN-Renderer · kein dmn-js, kein CDN, offline (ADR-0016)</p>
      <div id="canvas" class="canvas"></div>
      <p class="hint">
        WP-65 (Vorgriff): Eigene Renderer zeichnen die DMN-Vokabel auf dem
        diagram-js-Kern — Decision (Rechteck), InputData (Oval),
        BusinessKnowledgeModel (geknickte Ecken) und Requirement-Kanten
        (Information = durchgezogener Pfeil, Knowledge = gestrichelt). Noch ein
        fester Beispiel-Graph; echtes Modell-Laden + DMNDI-Layout und
        Modellier-Interaktionen folgen (WP-62-JS/65).
      </p>
    </main>`
  const c = root.querySelector<HTMLElement>('#canvas')
  if (c) mountCanvas(c)
}

const root = document.getElementById('app')
if (root) render(root)
