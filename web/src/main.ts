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
      <p class="sub">Eigener DMN-Modeler · diagram-js (MIT) · kein dmn-js, kein CDN, offline (ADR-0016)</p>
      <div id="canvas" class="canvas"></div>
      <p class="hint">
        WP-61: Der geforkte MIT-Kern (diagram-js) rendert den Canvas — ohne dmn-js
        und ohne bpmn.io-Logo. Oben ein Platzhalter-Graph als Beweis; echte
        DMN-Formen (Decision/InputData/Requirements) und Modellier-Interaktionen
        folgen in WP-64/65.
      </p>
    </main>`
  const c = root.querySelector<HTMLElement>('#canvas')
  if (c) mountCanvas(c)
}

const root = document.getElementById('app')
if (root) render(root)
