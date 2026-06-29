import type ContextPad from 'diagram-js/lib/features/context-pad/ContextPad'
import type { ContextPadEntries } from 'diagram-js/lib/features/context-pad/ContextPadProvider'
import type Connect from 'diagram-js/lib/features/connect/Connect'
import type Modeling from 'diagram-js/lib/features/modeling/Modeling'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type { Element, Shape } from 'diagram-js/lib/model/Types'

// Inline SVG icons as data URIs — no icon font needed, crisp at any zoom.
const svg = (inner: string): string =>
  'data:image/svg+xml,' +
  encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18">${inner}</svg>`)
const stroke = 'fill="none" stroke="#3b4150" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"'
const ICON_CONNECT = svg(`<path d="M3 9h8M9 5l4 4-4 4" ${stroke}/>`)
const ICON_DELETE = svg('<path d="M4 5h10M7 5V3.5h4V5M5.5 5l.8 9h5.4l.8-9" fill="none" stroke="#c0392b" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>')
const ICON_INPUT = svg(`<rect x="2" y="6" width="14" height="6" rx="3" ${stroke}/>`)
const ICON_DECISION = svg(`<rect x="3" y="5" width="12" height="8" rx="1" ${stroke}/>`)
const ICON_BKM = svg(`<path d="M6 5h9v6l-2 2H3V7z" ${stroke}/>`)

// A DMN element kind that can be appended as an upstream requirement.
type Kind = { type: string; name: string; w: number; h: number; req: string; icon: string; title: string }
const INPUT: Kind = { type: 'inputData', name: 'Neue Eingabe', w: 120, h: 50, req: 'informationRequirement', icon: ICON_INPUT, title: 'Eingabedaten anhängen' }
const DECISION: Kind = { type: 'decision', name: 'Neue Decision', w: 150, h: 70, req: 'informationRequirement', icon: ICON_DECISION, title: 'Decision anhängen' }
const BKM: Kind = { type: 'businessKnowledgeModel', name: 'Neues BKM', w: 150, h: 64, req: 'knowledgeRequirement', icon: ICON_BKM, title: 'Business Knowledge Model anhängen' }

// The DMN context pad (ADR-0016, WP-65): the popup of actions to the right of a
// selected element, matching the dmn-js feel. A decision can append upstream
// requirements (input/decision/BKM); every element can connect or be deleted.
// All edits run through the command stack, so they undo/redo.
class DmnContextPadProvider {
  static $inject = ['contextPad', 'connect', 'modeling', 'canvas']

  private connect: Connect
  private modeling: Modeling
  private canvas: Canvas

  constructor(contextPad: ContextPad, connect: Connect, modeling: Modeling, canvas: Canvas) {
    this.connect = connect
    this.modeling = modeling
    this.canvas = canvas
    contextPad.registerProvider(this)
  }

  // append creates a new upstream element of `kind` below `source` and wires its
  // requirement edge (new → source), all as one undoable step.
  private append(source: Shape, kind: Kind): void {
    const root = this.canvas.getRootElement()
    const cx = (source.x ?? 0) + (source.width ?? 0) / 2
    const cy = (source.y ?? 0) + (source.height ?? 0) + 60 + kind.h / 2
    const shape = this.modeling.createShape(
      { type: 'dmn:' + kind.type, width: kind.w, height: kind.h, name: kind.name } as never,
      { x: cx, y: cy },
      root as never,
    )
    this.modeling.createConnection(shape, source as never, { type: 'dmn:' + kind.req } as never, root as never)
  }

  getContextPadEntries(element: Element): ContextPadEntries {
    const connect = this.connect
    const modeling = this.modeling
    const entries: ContextPadEntries = {}

    // Append upstream requirements — only meaningful on a decision.
    if (element.type === 'dmn:decision') {
      for (const [key, kind] of [['append-input', INPUT], ['append-decision', DECISION], ['append-bkm', BKM]] as [string, Kind][]) {
        entries[key] = {
          group: 'add',
          className: 'cp-icon',
          title: kind.title,
          imageUrl: kind.icon,
          action: { click: () => this.append(element as Shape, kind) },
        }
      }
    }

    entries['connect'] = {
      group: 'connect',
      className: 'cp-icon',
      title: 'Verbinden (Requirement ziehen)',
      imageUrl: ICON_CONNECT,
      action: {
        click: (event: Event) => connect.start(event as MouseEvent, element),
        dragstart: (event: Event) => connect.start(event as MouseEvent, element),
      },
    }
    entries['delete'] = {
      group: 'edit',
      className: 'cp-icon',
      title: 'Löschen',
      imageUrl: ICON_DELETE,
      action: { click: () => modeling.removeElements([element as never]) },
    }
    return entries
  }
}

export const dmnContextPadModule = {
  __init__: ['dmnContextPadProvider'],
  dmnContextPadProvider: ['type', DmnContextPadProvider],
}
