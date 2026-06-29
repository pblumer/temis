import type ContextPad from 'diagram-js/lib/features/context-pad/ContextPad'
import type { ContextPadEntries } from 'diagram-js/lib/features/context-pad/ContextPadProvider'
import type Connect from 'diagram-js/lib/features/connect/Connect'
import type Modeling from 'diagram-js/lib/features/modeling/Modeling'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type EventBus from 'diagram-js/lib/core/EventBus'
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
const ICON_TABLE = svg(`<rect x="2.5" y="3.5" width="13" height="11" rx="1" ${stroke}/><path d="M2.5 7h13M7 7v7.5" ${stroke}/>`)
const ICON_LITERAL = svg(`<path d="M6 4 3 9l3 5M12 4l3 5-3 5" ${stroke}/>`)

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
  static $inject = ['contextPad', 'connect', 'modeling', 'canvas', 'eventBus']

  private connect: Connect
  private modeling: Modeling
  private canvas: Canvas
  private eventBus: EventBus

  constructor(contextPad: ContextPad, connect: Connect, modeling: Modeling, canvas: Canvas, eventBus: EventBus) {
    this.connect = connect
    this.modeling = modeling
    this.canvas = canvas
    this.eventBus = eventBus
    contextPad.registerProvider(this)
  }

  // append creates a new upstream element of `kind` below `source` and wires its
  // requirement edge (new → source), all as one undoable step.
  private append(source: Shape, kind: Kind): void {
    const root = this.canvas.getRootElement()
    // Place the new requirement below the source, fanned out by how many it
    // already has so siblings don't stack. The connecting edge is cropped to the
    // node borders by the connection docking (see canvas.ts).
    const n = source.incoming?.length ?? 0
    const cx = (source.x ?? 0) + (source.width ?? 0) / 2 + (n - 1) * (kind.w + 30)
    const cy = (source.y ?? 0) + (source.height ?? 0) + 80 + kind.h / 2
    const shape = this.modeling.createShape(
      { type: 'dmn:' + kind.type, width: kind.w, height: kind.h, name: kind.name } as never,
      { x: cx, y: cy },
      root as never,
    )
    const conn = this.modeling.createConnection(shape, source as never, { type: 'dmn:' + kind.req } as never, root as never)
    // diagram-js routes new connections centre-to-centre; dock them at the node
    // borders (like the loaded edges) so the edge — and its hit area — doesn't sit
    // over the new node, which would make it unselectable.
    this.modeling.updateWaypoints(conn as never, [borderPoint(shape, source), borderPoint(source, shape)] as never)
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
      // An undecided decision (no logic yet) can get a fresh decision table or a
      // literal expression. The handlers live in the app shell, so fire events.
      const decided = (element as { hasTable?: boolean; hasLiteral?: boolean })
      if (!decided.hasTable && !decided.hasLiteral) {
        entries['create-table'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Decision Table anlegen',
          imageUrl: ICON_TABLE,
          action: { click: () => this.eventBus.fire('dmn.createTable', { element }) },
        }
        entries['create-literal'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'FEEL-Ausdruck anlegen',
          imageUrl: ICON_LITERAL,
          action: { click: () => this.eventBus.fire('dmn.createLiteral', { element }) },
        }
      }
    }

    // A business knowledge model: edit its encapsulated function (parameters +
    // FEEL body). The handler lives in the app shell, so fire an event.
    if (element.type === 'dmn:businessKnowledgeModel') {
      entries['edit-bkm'] = {
        group: 'add',
        className: 'cp-icon',
        title: 'Funktion bearbeiten',
        imageUrl: ICON_LITERAL,
        action: { click: () => this.eventBus.fire('dmn.openBKM', { element }) },
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

// borderPoint returns the point on node's border on the line from its centre
// toward other's centre — used to dock requirement edges at the node edges.
function borderPoint(node: Shape, other: Shape): { x: number; y: number } {
  const cx = (node.x ?? 0) + (node.width ?? 0) / 2
  const cy = (node.y ?? 0) + (node.height ?? 0) / 2
  const ox = (other.x ?? 0) + (other.width ?? 0) / 2
  const oy = (other.y ?? 0) + (other.height ?? 0) / 2
  const dx = ox - cx
  const dy = oy - cy
  if (dx === 0 && dy === 0) return { x: cx, y: cy }
  const t = 1 / Math.max(Math.abs(dx) / ((node.width ?? 0) / 2), Math.abs(dy) / ((node.height ?? 0) / 2))
  return { x: cx + dx * t, y: cy + dy * t }
}

export const dmnContextPadModule = {
  __init__: ['dmnContextPadProvider'],
  dmnContextPadProvider: ['type', DmnContextPadProvider],
}
