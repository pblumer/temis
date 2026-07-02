import type Palette from 'diagram-js/lib/features/palette/Palette'
import type { PaletteEntries } from 'diagram-js/lib/features/palette/PaletteProvider'
import type Create from 'diagram-js/lib/features/create/Create'
import type ElementFactory from 'diagram-js/lib/core/ElementFactory'
import type ElementRegistry from 'diagram-js/lib/core/ElementRegistry'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type Modeling from 'diagram-js/lib/features/modeling/Modeling'
import type HandTool from 'diagram-js/lib/features/hand-tool/HandTool'

// Inline SVG icons as data URIs — same crisp, font-free style as the context pad.
const svg = (inner: string): string =>
  'data:image/svg+xml,' +
  encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="22" height="22" viewBox="0 0 18 18">${inner}</svg>`)
const stroke = 'fill="none" stroke="#3b4150" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"'

// Tools: a pointer to return to selecting and a hand to pan the canvas — the
// bpmn.io palette convention.
const ICON_SELECT = svg('<path d="M4 3 L4 14.5 L7.2 11.3 L9.4 15.8 L11.3 15 L9.1 10.6 L13.5 10.3 Z" fill="#3b4150" stroke="#3b4150" stroke-width="0.6" stroke-linejoin="round"/>')
const ICON_HAND = svg(`<path d="M6.2 10.5 V6.3 a0.95 0.95 0 0 1 1.9 0 V9 V4.8 a0.95 0.95 0 0 1 1.9 0 V9 V5.3 a0.95 0.95 0 0 1 1.9 0 V9.4 a0.95 0.95 0 0 1 1.9 0 v2.3 c0 2.3 -1.6 3.9 -3.9 3.9 -1.5 0 -2.5 -0.6 -3.3 -1.8 L5 11.2 a0.98 0.98 0 0 1 1.6 -1.1 Z" ${stroke}/>`)

// Elements allowed on this DRD — the create tools.
const ICON_INPUT = svg(`<rect x="2" y="6" width="14" height="6" rx="3" ${stroke}/>`)
const ICON_DECISION = svg(`<rect x="3" y="5" width="12" height="8" rx="1" ${stroke}/>`)
const ICON_BKM = svg(`<path d="M6 5h9v6l-2 2H3V7z" ${stroke}/>`)

// A DMN element kind that can be created from scratch via the palette.
type Kind = { type: string; name: string; w: number; h: number; icon: string; title: string }
const INPUT: Kind = { type: 'inputData', name: 'Neue Eingabe', w: 120, h: 50, icon: ICON_INPUT, title: 'Eingabedaten erstellen' }
const DECISION: Kind = { type: 'decision', name: 'Neue Decision', w: 150, h: 70, icon: ICON_DECISION, title: 'Decision erstellen' }
const BKM: Kind = { type: 'businessKnowledgeModel', name: 'Neues BKM', w: 150, h: 64, icon: ICON_BKM, title: 'Business Knowledge Model erstellen' }

// The DMN palette (ADR-0016): the left-edge toolbar, modelled on bpmn.io. A
// "tools" group holds the pointer (back to selecting) and the hand (pan the
// canvas); below a separator, the "create" group shows the elements this diagram
// allows — an InputData, a Decision or a BKM — placed by clicking or dragging
// onto the canvas. The same structure carries over to a BPMN/workflow editor:
// the tools are diagram-agnostic and only the element group changes per notation.
// A created node goes through the command stack (undo/redo) and is persisted by
// the structural save.
class DmnPaletteProvider {
  static $inject = ['palette', 'create', 'elementFactory', 'handTool', 'elementRegistry', 'eventBus', 'canvas', 'modeling']

  private create: Create
  private elementFactory: ElementFactory
  private handTool: HandTool
  private elementRegistry: ElementRegistry
  private canvas: Canvas
  private modeling: Modeling
  // True while a drag-create session is in flight, so a click on the palette while
  // one is already running is ignored rather than starting a second, overlapping
  // session.
  private creating = false
  // Whether the in-flight session was started by a drag, and when the last such
  // drag session ended. Both guard the click action against the "ghost" click a
  // browser fires right after a canceled native drag from the palette: without the
  // guard that trailing click used to start a second, phantom create session that
  // followed the cursor and could only be dismissed with Esc/reload. A short time
  // window is the same technique diagram-js uses for its own post-drag click trap.
  private dragSession = false
  private lastDragEnd = 0

  constructor(
    palette: Palette,
    create: Create,
    elementFactory: ElementFactory,
    handTool: HandTool,
    elementRegistry: ElementRegistry,
    eventBus: EventBus,
    canvas: Canvas,
    modeling: Modeling,
  ) {
    this.create = create
    this.elementFactory = elementFactory
    this.handTool = handTool
    this.elementRegistry = elementRegistry
    this.canvas = canvas
    this.modeling = modeling
    eventBus.on('create.init', () => {
      this.creating = true
    })
    eventBus.on('create.cleanup', () => {
      this.creating = false
      if (this.dragSession) this.lastDragEnd = Date.now()
      this.dragSession = false
    })
    palette.registerProvider(this)
  }

  // uniqueName returns base, or "base 2", "base 3", … so a freshly created element
  // never silently collides with an existing element's name — two decisions named
  // "Neue Decision" are a conflict the user would otherwise have to untangle by
  // hand (and cannot rename until placed).
  private uniqueName(base: string): string {
    const taken = new Set<string>()
    for (const el of this.elementRegistry.getAll()) {
      const name = (el as { name?: string }).name
      if (name) taken.add(name)
    }
    if (!taken.has(base)) return base
    let i = 2
    while (taken.has(`${base} ${i}`)) i++
    return `${base} ${i}`
  }

  getPaletteEntries(): PaletteEntries {
    const newShape = (kind: Kind) => this.elementFactory.createShape({ type: 'dmn:' + kind.type, width: kind.w, height: kind.h, name: this.uniqueName(kind.name) } as never)
    const startDragCreate = (kind: Kind, event: Event): void => {
      this.create.start(event as MouseEvent, newShape(kind))
    }
    const createAtViewportCenter = (kind: Kind): void => {
      const viewbox = (this.canvas as unknown as { viewbox: () => { x: number; y: number; width: number; height: number } }).viewbox()
      const position = { x: viewbox.x + viewbox.width / 2, y: viewbox.y + viewbox.height / 2 }
      this.modeling.createShape(newShape(kind) as never, position, this.canvas.getRootElement() as never)
    }
    // Dragging keeps diagram-js' normal drag/drop behavior. A click, however,
    // creates the element immediately in the visible canvas center instead of
    // entering click-to-place mode. That mode looked like a "sticky" decision
    // following the cursor, was easy to trigger accidentally, and canceling it
    // with Esc correctly left no persisted element. A single palette click now
    // performs a real undoable create, so there is no dangling create session.
    const startOnDrag = (kind: Kind) => (event: Event): void => {
      this.dragSession = true
      startDragCreate(kind, event)
    }
    const startOnClick = (kind: Kind) => (_event: Event): void => {
      if (this.creating || Date.now() - this.lastDragEnd < 300) return
      createAtViewportCenter(kind)
    }

    // Entry keys carry the "-tool" suffix the palette strips to match the active
    // diagram-js tool name (e.g. "hand-tool" → "hand"), so the active tool is
    // highlighted. "select-tool" has no backing tool, so it never highlights — it
    // is the momentary "back to selecting" action.
    const entries: PaletteEntries = {
      'select-tool': {
        group: 'tools',
        className: 'pal-icon',
        title: 'Auswählen',
        imageUrl: ICON_SELECT,
        // Leave any active tool (e.g. the hand) and return to plain selection.
        action: { click: () => { if (this.handTool.isActive()) this.handTool.toggle() } },
      },
      'hand-tool': {
        group: 'tools',
        className: 'pal-icon',
        title: 'Navigieren (Hand)',
        imageUrl: ICON_HAND,
        action: { click: () => this.handTool.toggle() },
      },
      'tool-separator': { group: 'tools', separator: true, action: {} },
    }
    for (const [key, kind] of [['create-input', INPUT], ['create-decision', DECISION], ['create-bkm', BKM]] as [string, Kind][]) {
      entries[key] = {
        group: 'create',
        className: 'pal-icon',
        title: kind.title,
        imageUrl: kind.icon,
        action: { dragstart: startOnDrag(kind), click: startOnClick(kind) },
      }
    }
    return entries
  }
}

export const dmnPaletteModule = {
  __init__: ['dmnPaletteProvider'],
  dmnPaletteProvider: ['type', DmnPaletteProvider],
}
