import { APP_NAME, SCAFFOLD_WP } from './build-info'
import './style.css'

// WP-60 scaffold: proves the toolchain end-to-end — TS is type-checked, bundled
// by Vite, the output is committed + embedded via go:embed, and temisd serves it
// at /app/ with no CDN (works offline). The real modeler (DRD canvas, decision
// table editor) lands on top of this in WP-61+ (ADR-0016).
function render(root: HTMLElement): void {
  const probe = document.createElement('p')
  probe.className = 'probe'
  probe.textContent = 'OK'

  root.innerHTML = `
    <main>
      <h1>${APP_NAME}</h1>
      <p class="sub">Eigener DMN-Modeler · embedded build · kein CDN, offline (ADR-0016)</p>
      <p class="wp">${SCAFFOLD_WP}</p>
      <p class="hint">
        Gerüst steht. Hier entsteht der eigene Modeler auf dem geforkten MIT-Kern
        (diagram-js/table-js/dmn-moddle): DRD-Canvas, Decision-Table-Editor und
        FEEL-Validierung gegen die echte temis-Engine.
      </p>
    </main>`
  root.querySelector('main')?.appendChild(probe)
}

const root = document.getElementById('app')
if (root) render(root)
