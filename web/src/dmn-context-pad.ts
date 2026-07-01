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
const ICON_CONTEXT = svg(`<path d="M6 3.5C4.7 3.5 4.7 6 4.7 7.2c0 1.1-1 1.8-1.7 1.8.7 0 1.7.7 1.7 1.8 0 1.2 0 3.7 1.3 3.7M12 3.5c1.3 0 1.3 2.5 1.3 3.7 0 1.1 1 1.8 1.7 1.8-.7 0-1.7.7-1.7 1.8 0 1.2 0 3.7-1.3 3.7" ${stroke}/>`)
const ICON_CONDITIONAL = svg(`<path d="M9 2.5v13M9 6l4-3M9 10l-4-3" ${stroke}/>`)
const ICON_LIST = svg(`<rect x="3" y="3.5" width="12" height="11" rx="1" ${stroke}/><path d="M6 7h6M6 9.5h6M6 12h4" ${stroke}/>`)
const ICON_RELATION = svg(`<rect x="2.5" y="3.5" width="13" height="11" rx="1" ${stroke}/><path d="M2.5 7.5h13M7 3.5v11M11 3.5v11" ${stroke}/>`)
const ICON_FILTER = svg(`<path d="M3 4h12l-4.5 5.5V14l-3 1.5V9.5L3 4Z" ${stroke}/>`)

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
    // The connection is laid out border-to-border by the DMN layouter (see
    // dmn-layouter.ts), so its line and hit area dock at the node edges and don't
    // sit over the new node.
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
      // A decided decision: open its logic with a single click on the icon —
      // the table view or the FEEL-expression editor (also reachable by
      // double-click). The handlers live in the app shell, so fire events.
      const decided = (element as { hasTable?: boolean; hasLiteral?: boolean; hasContext?: boolean; hasConditional?: boolean; hasList?: boolean; hasRelation?: boolean; hasFilter?: boolean; hasLogic?: boolean })
      if (decided.hasTable) {
        entries['open-table'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Decision Table anzeigen',
          imageUrl: ICON_TABLE,
          action: { click: () => this.eventBus.fire('dmn.openTable', { element }) },
        }
      } else if (decided.hasLiteral) {
        entries['open-literal'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'FEEL-Ausdruck anzeigen',
          imageUrl: ICON_LITERAL,
          action: { click: () => this.eventBus.fire('dmn.openLiteral', { element }) },
        }
      } else if (decided.hasContext) {
        entries['open-context'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Boxed Context bearbeiten',
          imageUrl: ICON_CONTEXT,
          action: { click: () => this.eventBus.fire('dmn.openContext', { element }) },
        }
      } else if (decided.hasConditional) {
        entries['open-conditional'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Conditional (if/then/else) bearbeiten',
          imageUrl: ICON_CONDITIONAL,
          action: { click: () => this.eventBus.fire('dmn.openConditional', { element }) },
        }
      } else if (decided.hasList) {
        entries['open-list'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Liste bearbeiten',
          imageUrl: ICON_LIST,
          action: { click: () => this.eventBus.fire('dmn.openList', { element }) },
        }
      } else if (decided.hasFilter) {
        entries['open-filter'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Filter bearbeiten',
          imageUrl: ICON_FILTER,
          action: { click: () => this.eventBus.fire('dmn.openFilter', { element }) },
        }
      } else if (decided.hasRelation) {
        entries['open-relation'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Relation bearbeiten',
          imageUrl: ICON_RELATION,
          action: { click: () => this.eventBus.fire('dmn.openRelation', { element }) },
        }
      } else if (decided.hasLogic) {
        // A decided decision whose logic is another boxed expression (invocation,
        // for/every/some) the modeler cannot edit yet (WP-66). Offer an honest
        // hint rather than a "create" that the server rejects because the decision
        // already has logic.
        entries['boxed-info'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Boxed-Ausdruck — im Modeler noch nicht editierbar',
          imageUrl: ICON_LITERAL,
          action: { click: () => this.eventBus.fire('dmn.boxedInfo', { element }) },
        }
      } else {
        // A truly undecided decision (no logic at all) can get a fresh decision
        // table, a literal expression or a boxed context. Handlers in the app shell.
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
        entries['create-context'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Boxed Context anlegen',
          imageUrl: ICON_CONTEXT,
          action: { click: () => this.eventBus.fire('dmn.createContext', { element }) },
        }
        entries['create-conditional'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Conditional (if/then/else) anlegen',
          imageUrl: ICON_CONDITIONAL,
          action: { click: () => this.eventBus.fire('dmn.createConditional', { element }) },
        }
        entries['create-list'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Liste anlegen',
          imageUrl: ICON_LIST,
          action: { click: () => this.eventBus.fire('dmn.createList', { element }) },
        }
        entries['create-relation'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Relation anlegen',
          imageUrl: ICON_RELATION,
          action: { click: () => this.eventBus.fire('dmn.createRelation', { element }) },
        }
        entries['create-filter'] = {
          group: 'add',
          className: 'cp-icon',
          title: 'Filter anlegen',
          imageUrl: ICON_FILTER,
          action: { click: () => this.eventBus.fire('dmn.createFilter', { element }) },
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

export const dmnContextPadModule = {
  __init__: ['dmnContextPadProvider'],
  dmnContextPadProvider: ['type', DmnContextPadProvider],
}
