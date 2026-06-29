import Diagram from 'diagram-js'
import MoveModule from 'diagram-js/lib/features/move'
import ModelingModule from 'diagram-js/lib/features/modeling'
import ContextPadModule from 'diagram-js/lib/features/context-pad'
import ConnectModule from 'diagram-js/lib/features/connect'
import MoveCanvasModule from 'diagram-js/lib/navigation/movecanvas'
import ZoomScrollModule from 'diagram-js/lib/navigation/zoomscroll'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type CommandStack from 'diagram-js/lib/command/CommandStack'
import type { Shape } from 'diagram-js/lib/model/Types'
import 'diagram-js/assets/diagram-js.css'
import { dmnRendererModule } from './dmn-renderer'
import { dmnRulesModule } from './dmn-rules'
import { dmnContextPadModule } from './dmn-context-pad'
import { dmnLabelEditingModule } from './dmn-label-editing'
import type { Laid } from './layout'

// Handle to the live diagram: nodes are selectable and draggable, every change
// goes through the command stack, so undo/redo work (ADR-0016, WP-63/65).
export type ModelerHandle = {
  undo: () => void
  redo: () => void
  canUndo: () => boolean
  canRedo: () => boolean
  onChange: (cb: () => void) => void
}

// Build an editable DMN diagram into the container with temis' own renderers on
// the diagram-js MIT core — no dmn-js. A fresh diagram is built per call (the
// container is cleared first), so switching models starts a clean undo history.
export function renderGraph(container: HTMLElement, laid: Laid): ModelerHandle {
  container.innerHTML = ''
  const diagram = new Diagram({
    canvas: { container },
    modules: [
      dmnRendererModule, dmnRulesModule, dmnContextPadModule, dmnLabelEditingModule,
      ModelingModule, MoveModule, ContextPadModule, ConnectModule,
      MoveCanvasModule, ZoomScrollModule,
    ],
  })
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')
  const commandStack = diagram.get<CommandStack>('commandStack')
  const eventBus = diagram.get<EventBus>('eventBus')

  const byId: Record<string, Shape> = {}
  for (const n of laid.nodes) {
    // The /v1 graph uses bare type names ("inputData", …); our renderer keys on
    // the "dmn:" vocabulary. name/type are carried on the element for it to read.
    const shape = factory.createShape({ id: n.id, x: n.x, y: n.y, width: n.w, height: n.h, type: 'dmn:' + n.type, name: n.name, dataType: n.dataType, varName: n.varName } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  for (const e of laid.edges) {
    if (!byId[e.source] || !byId[e.target]) continue
    const conn = factory.createConnection({ id: e.id, type: 'dmn:' + e.type, source: byId[e.source], target: byId[e.target], waypoints: e.waypoints } as never)
    canvas.addConnection(conn)
  }

  canvas.zoom('fit-viewport')

  // The shapes added above must not be undoable — only user edits are. The
  // command stack is empty here because addShape/addConnection bypass it.
  let changeCb = (): void => {}
  eventBus.on('commandStack.changed', () => changeCb())

  return {
    undo: () => commandStack.undo(),
    redo: () => commandStack.redo(),
    canUndo: () => commandStack.canUndo(),
    canRedo: () => commandStack.canRedo(),
    onChange: (cb) => {
      changeCb = cb
    },
  }
}
