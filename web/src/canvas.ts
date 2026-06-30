import Diagram from 'diagram-js'
import MoveModule from 'diagram-js/lib/features/move'
import ModelingModule from 'diagram-js/lib/features/modeling'
import ContextPadModule from 'diagram-js/lib/features/context-pad'
import ConnectModule from 'diagram-js/lib/features/connect'
import PaletteModule from 'diagram-js/lib/features/palette'
import CreateModule from 'diagram-js/lib/features/create'
import HandToolModule from 'diagram-js/lib/features/hand-tool'
import MoveCanvasModule from 'diagram-js/lib/navigation/movecanvas'
import ZoomScrollModule from 'diagram-js/lib/navigation/zoomscroll'
import OverlaysModule from 'diagram-js/lib/features/overlays'
import type Overlays from 'diagram-js/lib/features/overlays/Overlays'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type CommandStack from 'diagram-js/lib/command/CommandStack'
import type ElementRegistry from 'diagram-js/lib/core/ElementRegistry'
import type { Shape, Connection } from 'diagram-js/lib/model/Types'
import 'diagram-js/assets/diagram-js.css'
import { dmnRendererModule } from './dmn-renderer'
import { dmnRulesModule } from './dmn-rules'
import { dmnContextPadModule } from './dmn-context-pad'
import { dmnLabelEditingModule } from './dmn-label-editing'
import { dmnLayouterModule } from './dmn-layouter'
import { dmnPaletteModule } from './dmn-palette'
import type { Laid } from './layout'

// What the toolbar needs to know about the current selection to offer the type
// editor: the selected InputData's id and current type, or null otherwise.
export type Selected = { id: string; dataType?: string } | null

// NodeState is the current persistable state of one diagram node, read back from
// the live shapes for the structural save (ADR-0016): id and type plus the
// editable name/type and the bounds. x/y are the shape's top-left (DMNDI bounds).
export type NodeState = {
  id: string
  type: string
  name?: string
  dataType?: string
  x: number
  y: number
  width: number
  height: number
}

// EdgeState is one requirement edge, directed from the required (source) to the
// requiring (target) element — the DMN arrow direction.
export type EdgeState = { type: string; source: string; target: string }

// GraphState is the live decision requirements graph read off the canvas, the
// payload for a structural save (persists added/removed nodes and edges).
export type GraphState = { nodes: NodeState[]; edges: EdgeState[] }

// Handle to the live diagram: nodes are selectable and draggable, every change
// goes through the command stack, so undo/redo work (ADR-0016, WP-63/65).
export type ModelerHandle = {
  undo: () => void
  redo: () => void
  canUndo: () => boolean
  canRedo: () => boolean
  onChange: (cb: () => void) => void
  // onSelect reports the selected InputData (for the type editor) or null.
  onSelect: (cb: (sel: Selected) => void) => void
  // setSelectedType sets the selected InputData's FEEL type (undoable); "" clears it.
  setSelectedType: (dataType: string) => void
  // graph returns the live decision requirements graph (nodes + edges) for a
  // structural save — reflecting nodes/edges added or removed on the canvas.
  graph: () => GraphState
  // onOpenTable fires with a decision's id when the user double-clicks a decision
  // whose logic is a decision table (to open its table view).
  onOpenTable: (cb: (decisionId: string) => void) => void
  // onCreateTable fires with a decision's id when the user asks (via the context
  // pad) to give a table-less decision a fresh decision table.
  onCreateTable: (cb: (decisionId: string) => void) => void
  // onOpenLiteral fires with a decision's id when the user double-clicks a
  // decision whose logic is a literal FEEL expression (to open the expression
  // editor).
  onOpenLiteral: (cb: (decisionId: string) => void) => void
  // onCreateLiteral fires with a decision's id when the user asks (via the context
  // pad) to give an undecided decision a literal expression.
  onCreateLiteral: (cb: (decisionId: string) => void) => void
  // onOpenBKM fires with a business knowledge model's id when the user asks (via
  // the context pad) to edit its function.
  onOpenBKM: (cb: (bkmId: string) => void) => void
  // onBoxed fires with a decision's id when the user tries to open a decision
  // whose logic is a boxed expression the modeler cannot edit yet (WP-66), so the
  // app shell can give an honest hint instead of a silent no-op.
  onBoxed: (cb: (decisionId: string) => void) => void
  // zoom adjusts the canvas zoom: step in/out, or fit the whole diagram.
  zoom: (dir: 'in' | 'out' | 'fit') => void
  // showDiagnostics marks each decision node that has compile/eval problems with a
  // severity badge (error/warning) carrying the messages as a tooltip, so issues
  // are visible on the diagram. Diagnostics without a decision id are model-level
  // and handled by the caller. An empty list clears the markers.
  showDiagnostics: (diags: { severity: string; message: string; decisionId?: string }[]) => void
  // showResults overlays each decision's evaluated value on its node (keyed by
  // decision name), so the whole graph's results are visible on the diagram. When
  // hitRules is given (decision name → matched rule numbers, 1-based), the badge
  // also shows which rule(s) fired — the Operate view's hit-rule highlight. An
  // empty values map clears the overlays.
  showResults: (values: Record<string, unknown>, hitRules?: Record<string, number[]>) => void
}

// A Canvas with the viewbox getter/setter we need (not in the bundled types).
type ViewBox = { x: number; y: number; width: number; height: number; scale: number; inner: { x: number; y: number } }
type FitCanvas = {
  zoom: (mode: string) => number
  viewbox: { (): ViewBox; (box: { x: number; y: number; width: number; height: number }): void }
}

// The width reserved on the left for the palette toolbox, so fitting the diagram
// does not tuck the left-most element behind it.
const PALETTE_INSET = 88

// fitViewport fits the whole diagram, then — if the standard fit left the diagram
// flush to the left edge — nudges it right so the palette toolbox does not hide
// the left-most element.
function fitViewport(canvas: unknown): void {
  const c = canvas as FitCanvas
  c.zoom('fit-viewport')
  const vb = c.viewbox()
  const leftOnScreen = (vb.inner.x - vb.x) * vb.scale
  if (leftOnScreen < PALETTE_INSET) {
    c.viewbox({ x: vb.inner.x - PALETTE_INSET / vb.scale, y: vb.y, width: vb.width, height: vb.height })
  }
}

// Undoable type change on an InputData; redraws the pill via the returned element.
class UpdateTypeHandler {
  execute(ctx: { element: Shape & { dataType?: string }; dataType: string; old?: string }): Shape[] {
    ctx.old = ctx.element.dataType
    ctx.element.dataType = ctx.dataType || undefined
    return [ctx.element]
  }
  revert(ctx: { element: Shape & { dataType?: string }; old?: string }): Shape[] {
    ctx.element.dataType = ctx.old
    return [ctx.element]
  }
}

type SelectionService = { get: () => Shape[] }
const isInputData = (el: Shape | undefined): boolean => !!el && el.type === 'dmn:inputData'

// The element types that are DRG nodes vs requirement edges, for reading the
// live graph back off the canvas.
const NODE_TYPES = new Set(['dmn:inputData', 'dmn:decision', 'dmn:businessKnowledgeModel'])
const EDGE_TYPES = new Set(['dmn:informationRequirement', 'dmn:knowledgeRequirement'])

// Build an editable DMN diagram into the container with temis' own renderers on
// the diagram-js MIT core — no dmn-js. A fresh diagram is built per call (the
// container is cleared first), so switching models starts a clean undo history.
// The currently mounted diagram, destroyed when the next one is built so its
// command stack and DOM/document listeners don't linger and fire stray commands
// (e.g. re-adding a shape) into the new diagram after a model switch.
let current: Diagram | null = null

export function renderGraph(container: HTMLElement, laid: Laid): ModelerHandle {
  if (current) current.destroy()
  container.innerHTML = ''
  const diagram = new Diagram({
    canvas: { container },
    modules: [
      dmnRendererModule, dmnRulesModule, dmnContextPadModule, dmnLabelEditingModule,
      dmnPaletteModule, ModelingModule, MoveModule, ContextPadModule, ConnectModule,
      PaletteModule, CreateModule, HandToolModule, MoveCanvasModule, ZoomScrollModule, OverlaysModule, dmnLayouterModule,
    ],
  })
  current = diagram
  const canvas = diagram.get<Canvas>('canvas')
  const factory = diagram.get<ElementFactory>('elementFactory')
  const commandStack = diagram.get<CommandStack>('commandStack')
  const eventBus = diagram.get<EventBus>('eventBus')
  const elementRegistry = diagram.get<ElementRegistry>('elementRegistry')
  const selection = diagram.get<SelectionService>('selection')
  const overlays = diagram.get<Overlays>('overlays')
  commandStack.registerHandler('element.updateType', UpdateTypeHandler)

  const byId: Record<string, Shape> = {}
  for (const n of laid.nodes) {
    // The /v1 graph uses bare type names ("inputData", …); our renderer keys on
    // the "dmn:" vocabulary. name/type are carried on the element for it to read.
    const shape = factory.createShape({ id: n.id, x: n.x, y: n.y, width: n.w, height: n.h, type: 'dmn:' + n.type, name: n.name, dataType: n.dataType, varName: n.varName, hasTable: n.hasTable, hasLiteral: n.hasLiteral, hasLogic: n.hasLogic } as never)
    canvas.addShape(shape)
    byId[n.id] = shape
  }
  for (const e of laid.edges) {
    if (!byId[e.source] || !byId[e.target]) continue
    const conn = factory.createConnection({ id: e.id, type: 'dmn:' + e.type, source: byId[e.source], target: byId[e.target], waypoints: e.waypoints } as never)
    canvas.addConnection(conn)
  }

  fitViewport(canvas)

  // The shapes added above must not be undoable — only user edits are. The
  // command stack is empty here because addShape/addConnection bypass it.
  let changeCb = (): void => {}
  eventBus.on('commandStack.changed', () => changeCb())

  // Double-clicking a decision that has a decision table opens its table view.
  // Such shapes are not inline-renamed (see dmn-label-editing), so the gestures
  // don't collide.
  let openTableCb = (_decisionId: string): void => {}
  let openLiteralCb = (_decisionId: string): void => {}
  let openBoxedCb = (_decisionId: string): void => {}
  eventBus.on('element.dblclick', (e: { element?: Shape & { hasTable?: boolean; hasLiteral?: boolean } }) => {
    const el = e.element
    if (!el || el.type !== 'dmn:decision') return
    if (el.hasTable) openTableCb(el.id)
    else if (el.hasLiteral) openLiteralCb(el.id)
    // A boxed-expression decision has neither; double-click inline-renames it
    // (see dmn-label-editing). The "not editable" hint comes from the context
    // pad's boxed-info icon instead, so it doesn't clash with the rename.
  })
  // The context pad's boxed-info icon fires this for a boxed-expression decision.
  eventBus.on('dmn.boxedInfo', (e: { element?: Shape }) => {
    if (e.element) openBoxedCb(e.element.id)
  })
  // The context pad's open icons fire these so the table/expression opens with a
  // single click (the same handlers as the double-click above).
  eventBus.on('dmn.openTable', (e: { element?: Shape }) => {
    if (e.element) openTableCb(e.element.id)
  })
  eventBus.on('dmn.openLiteral', (e: { element?: Shape }) => {
    if (e.element) openLiteralCb(e.element.id)
  })

  let createTableCb = (_decisionId: string): void => {}
  let createLiteralCb = (_decisionId: string): void => {}
  eventBus.on('dmn.createTable', (e: { element?: Shape }) => {
    if (e.element) createTableCb(e.element.id)
  })
  eventBus.on('dmn.createLiteral', (e: { element?: Shape }) => {
    if (e.element) createLiteralCb(e.element.id)
  })
  let openBKMCb = (_bkmId: string): void => {}
  eventBus.on('dmn.openBKM', (e: { element?: Shape }) => {
    if (e.element) openBKMCb(e.element.id)
  })

  let selectCb = (_sel: Selected): void => {}
  const reportSelection = (): void => {
    const sel = selection.get()
    const one = sel.length === 1 ? sel[0] : undefined
    selectCb(isInputData(one) ? { id: one!.id, dataType: (one as Shape & { dataType?: string }).dataType } : null)
  }
  eventBus.on('selection.changed', reportSelection)
  // A type change keeps the same element selected; refresh the editor's value.
  eventBus.on('commandStack.changed', reportSelection)

  return {
    undo: () => commandStack.undo(),
    redo: () => commandStack.redo(),
    canUndo: () => commandStack.canUndo(),
    canRedo: () => commandStack.canRedo(),
    onChange: (cb) => {
      changeCb = cb
    },
    onSelect: (cb) => {
      selectCb = cb
    },
    setSelectedType: (dataType) => {
      const sel = selection.get()
      const one = sel.length === 1 ? sel[0] : undefined
      if (isInputData(one)) commandStack.execute('element.updateType', { element: one, dataType })
    },
    graph: () => {
      const nodes: NodeState[] = []
      const edges: EdgeState[] = []
      for (const el of elementRegistry.getAll()) {
        const type = (el as { type?: string }).type ?? ''
        if (NODE_TYPES.has(type)) {
          const s = el as Shape & { name?: string; dataType?: string }
          nodes.push({ id: s.id, type: type.replace(/^dmn:/, ''), name: s.name, dataType: s.dataType, x: s.x ?? 0, y: s.y ?? 0, width: s.width ?? 0, height: s.height ?? 0 })
        } else if (EDGE_TYPES.has(type)) {
          const c = el as Connection
          if (c.source && c.target) edges.push({ type: type.replace(/^dmn:/, ''), source: c.source.id, target: c.target.id })
        }
      }
      return { nodes, edges }
    },
    onOpenTable: (cb) => {
      openTableCb = cb
    },
    onCreateTable: (cb) => {
      createTableCb = cb
    },
    onOpenLiteral: (cb) => {
      openLiteralCb = cb
    },
    onCreateLiteral: (cb) => {
      createLiteralCb = cb
    },
    onOpenBKM: (cb) => {
      openBKMCb = cb
    },
    onBoxed: (cb) => {
      openBoxedCb = cb
    },
    zoom: (dir) => {
      if (dir === 'fit') fitViewport(canvas)
      else canvas.zoom(canvas.zoom() * (dir === 'in' ? 1.18 : 0.85))
    },
    showDiagnostics: (diags) => {
      overlays.remove({ type: 'diagnostic' })
      const byDec = new Map<string, { severity: string; message: string }[]>()
      for (const d of diags) {
        if (!d.decisionId) continue
        const list = byDec.get(d.decisionId) ?? []
        list.push(d)
        byDec.set(d.decisionId, list)
      }
      for (const [id, ds] of byDec) {
        if (!elementRegistry.get(id)) continue
        const worst = ds.some((d) => d.severity === 'error') ? 'error' : ds.some((d) => d.severity === 'warning') ? 'warning' : 'info'
        const badge = document.createElement('div')
        badge.className = 'node-diag node-diag-' + worst
        badge.textContent = worst === 'info' ? 'i' : '!'
        badge.title = ds.map((d) => d.message).join('\n')
        overlays.add(id, 'diagnostic', { position: { top: -9, right: -9 }, html: badge })
      }
    },
    showResults: (values, hitRules) => {
      overlays.remove({ type: 'eval-result' })
      for (const el of elementRegistry.getAll()) {
        const s = el as Shape & { name?: string; type?: string }
        if (s.type !== 'dmn:decision' || !s.name || !(s.name in values)) continue
        const text = fmtResult(values[s.name])
        const rules = hitRules?.[s.name] ?? []
        const badge = document.createElement('div')
        badge.className = 'node-result'
        badge.append(Object.assign(document.createElement('span'), { className: 'node-result-val', textContent: text }))
        if (rules.length) {
          badge.append(Object.assign(document.createElement('span'), { className: 'node-result-rule', textContent: 'R' + rules.join(',') }))
          badge.title = s.name + ' = ' + text + ' · Regel ' + rules.join(', ')
        } else {
          badge.title = s.name + ' = ' + text
        }
        overlays.add(s.id, 'eval-result', { position: { bottom: -4, left: 6 }, html: badge })
      }
    },
  }
}

// fmtResult renders a decision's evaluated value for the on-node badge: strings
// as-is, null explicit, everything else as compact JSON.
function fmtResult(v: unknown): string {
  if (v === null || v === undefined) return 'null'
  if (typeof v === 'string') return v
  return JSON.stringify(v)
}
