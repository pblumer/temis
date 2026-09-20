import { buildFieldControl, type FieldControl } from './inputform'
import { leafInputs } from './evaluate'
import type { ModelDetail } from './api'

// InputPills is the set of editable input controls placed directly on the diagram
// (Operate): one per leaf input, mounted on its inputData node. It carries the
// overlays for the canvas to place, plus collect() (the current inputs, for a
// whole-graph evaluation) and setValues() (to reflect the active run's inputs).
export type InputPills = {
  items: { nodeId: string; html: HTMLElement }[]
  collect: () => Record<string, unknown>
  setValues: (inputs: Record<string, unknown>) => void
}

// buildInputPills builds an editable pill for every leaf input that maps to an
// inputData node on the canvas. Editing any pill fires onChange (the caller
// debounces it into a whole-graph evaluation). Clicks/drags on a pill are kept
// from reaching the diagram so editing never selects or drags the node.
export function buildInputPills(model: ModelDetail, nodeIdByName: Map<string, string>, onChange: () => void): InputPills {
  const controls: { name: string; ctrl: FieldControl }[] = []
  const items: { nodeId: string; html: HTMLElement }[] = []
  leafInputs(model).forEach((field, idx) => {
    const nodeId = nodeIdByName.get(field.name)
    if (!nodeId) return
    const ctrl = buildFieldControl(field, { idx, className: 'pill-field' })
    const pill = document.createElement('div')
    pill.className = 'node-input'
    pill.append(ctrl.input, ...ctrl.extras)
    // The pill sits on a node in a diagram-js canvas that still has move/selection
    // active in Operate; swallow the pointer/click gestures so editing an input
    // never starts a node drag or changes the selection.
    for (const ev of ['mousedown', 'pointerdown', 'click', 'dblclick']) {
      pill.addEventListener(ev, (e) => e.stopPropagation())
    }
    ctrl.input.addEventListener('input', onChange)
    ctrl.input.addEventListener('change', onChange)
    controls.push({ name: field.name, ctrl })
    items.push({ nodeId, html: pill })
  })
  return {
    items,
    collect: () => {
      const out: Record<string, unknown> = {}
      for (const { name, ctrl } of controls) {
        const v = ctrl.read()
        if (v !== undefined) out[name] = v
      }
      return out
    },
    setValues: (inputs) => {
      for (const { name, ctrl } of controls) ctrl.setValue(inputs[name])
    },
  }
}
