import type ContextPad from 'diagram-js/lib/features/context-pad/ContextPad'
import type { ContextPadEntries } from 'diagram-js/lib/features/context-pad/ContextPadProvider'
import type Connect from 'diagram-js/lib/features/connect/Connect'
import type Modeling from 'diagram-js/lib/features/modeling/Modeling'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type ElementRegistry from 'diagram-js/lib/core/ElementRegistry'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type { Element, Shape } from 'diagram-js/lib/model/Types'
import { BOXED_TYPES, boxedTypeFor } from './boxededitors'

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
const ICON_ITERATOR = svg(`<path d="M4 6a4 4 0 1 1 0 6h6.5" ${stroke}/><path d="M9 9.5l2 2.5-2 2.5" ${stroke}/>`)
const ICON_RENAME = svg(`<path d="M11.5 3.5l3 3L6 15H3v-3z" ${stroke}/><path d="M10 5l3 3" ${stroke}/>`)
// A tag glyph for editing the FEEL identifier (variable name) — the italic "x"
// evokes a variable, distinct from the pencil's free-form rename.
const ICON_VARNAME = svg(`<path d="M3 8.5l4.2-4.2a1.4 1.4 0 0 1 1-.4H14v5.7a1.4 1.4 0 0 1-.4 1L9.4 15z" ${stroke}/><circle cx="11.3" cy="6.7" r="1" fill="#3b4150"/>`)
const ICON_INVOCATION = svg(`<rect x="2.5" y="4" width="13" height="10" rx="1.5" ${stroke}/><path d="M6.5 9h5M9 6.5v5" ${stroke}/>`)
// ICONS maps a boxed kind to its context-pad icon, so the registry-driven
// open/create entries pick the right glyph by kind (WP-142).
const ICONS: Record<string, string> = {
  table: ICON_TABLE,
  literal: ICON_LITERAL,
  context: ICON_CONTEXT,
  conditional: ICON_CONDITIONAL,
  list: ICON_LIST,
  relation: ICON_RELATION,
  filter: ICON_FILTER,
  iterator: ICON_ITERATOR,
  invocation: ICON_INVOCATION,
}
// Edge-shape icons: a right-angle L (eckig), a rounded bend (gerundet) and a
// straight diagonal (direkt) — matching the routes the renderer draws.
const ICON_EDGE_ORTHO = svg(`<path d="M4 3.5V13h10" ${stroke}/>`)
const ICON_EDGE_CURVED = svg(`<path d="M4 3.5v5a5 5 0 0 0 5 5h5" ${stroke}/>`)
const ICON_EDGE_DIRECT = svg(`<path d="M3.5 14.5 14.5 3.5" ${stroke}/>`)

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
  static $inject = ['contextPad', 'connect', 'modeling', 'canvas', 'eventBus', 'elementRegistry']

  private connect: Connect
  private modeling: Modeling
  private canvas: Canvas
  private eventBus: EventBus
  private elementRegistry: ElementRegistry

  constructor(contextPad: ContextPad, connect: Connect, modeling: Modeling, canvas: Canvas, eventBus: EventBus, elementRegistry: ElementRegistry) {
    this.connect = connect
    this.modeling = modeling
    this.canvas = canvas
    this.eventBus = eventBus
    this.elementRegistry = elementRegistry
    contextPad.registerProvider(this)
  }

  // uniqueName returns base, or "base 2", "base 3", … so an appended element never
  // silently collides with an existing element's name (see dmn-palette.ts).
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
      { type: 'dmn:' + kind.type, width: kind.w, height: kind.h, name: this.uniqueName(kind.name) } as never,
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

    // A requirement edge: let the modeler pick its shape — eckig (right-angle),
    // gerundet (rounded corners) or direkt (straight line). Each fires an event
    // the app shell turns into an undoable style change (see canvas.ts).
    if (element.type === 'dmn:informationRequirement' || element.type === 'dmn:knowledgeRequirement') {
      const styles: [string, string, string, string][] = [
        ['edge-ortho', 'ortho', ICON_EDGE_ORTHO, 'Eckige Verbindung'],
        ['edge-curved', 'curved', ICON_EDGE_CURVED, 'Gerundete Verbindung'],
        ['edge-direct', 'direct', ICON_EDGE_DIRECT, 'Direkte Verbindung'],
      ]
      for (const [key, style, icon, title] of styles) {
        entries[key] = {
          group: 'edge-style',
          className: 'cp-icon',
          title,
          imageUrl: icon,
          action: { click: () => this.eventBus.fire('dmn.setEdgeStyle', { element, style }) },
        }
      }
    }

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
      const decided = element as unknown as Record<string, unknown>
      const bt = boxedTypeFor(decided)
      if (bt) {
        // The decision carries a known boxed kind: one edit entry, from the
        // registry (WP-142). Fires dmn.openLogic with the kind.
        entries['open-' + bt.kind] = {
          group: 'edit',
          className: 'cp-icon',
          title: bt.editTitle,
          imageUrl: ICONS[bt.kind],
          action: { click: () => this.eventBus.fire('dmn.openLogic', { element, kind: bt.kind }) },
        }
      } else if (decided.hasLogic) {
        // A decided decision whose logic is a boxed form the modeler doesn't edit
        // (e.g. a bare function definition). Offer an honest hint rather than a
        // "create" that the server rejects because the decision already has logic.
        entries['boxed-info'] = {
          group: 'edit',
          className: 'cp-icon',
          title: 'Boxed-Ausdruck — im Modeler noch nicht editierbar',
          imageUrl: ICON_LITERAL,
          action: { click: () => this.eventBus.fire('dmn.boxedInfo', { element }) },
        }
      } else {
        // A truly undecided decision (no logic yet): a create entry per boxed kind,
        // generated from the registry (WP-142). Each fires dmn.createLogic.
        for (const t of BOXED_TYPES) {
          entries['create-' + t.kind] = {
            group: 'add',
            className: 'cp-icon',
            title: t.createTitle,
            imageUrl: ICONS[t.kind],
            action: { click: () => this.eventBus.fire('dmn.createLogic', { element, kind: t.kind }) },
          }
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

    // Rename: an explicit, deliberate gesture to inline-edit the element's name.
    // This pencil icon and the Enter/F2 keys are the ONLY ways to rename an
    // existing element — double-click is reserved throughout the editor for
    // switching to an element's content (see dmn-label-editing and canvas.ts).
    // The keys are named in the tooltip so they are discoverable. Requirement
    // edges carry no name.
    if (element.type !== 'dmn:informationRequirement' && element.type !== 'dmn:knowledgeRequirement') {
      entries['rename'] = {
        group: 'edit',
        className: 'cp-icon',
        title: 'Umbenennen (Enter / F2)',
        imageUrl: ICON_RENAME,
        action: { click: () => this.eventBus.fire('dmn.renameElement', { element }) },
      }
    }

    // Edit the FEEL identifier (variable name) separately from the display name —
    // only meaningful for a decision or input data, whose result/value is
    // referenced in FEEL by that identifier. A BKM is referenced by its own name,
    // so it has no separate one. This lets the display name be a free-form label
    // (spaces, hyphens) while the FEEL reference stays a valid identifier.
    if (element.type === 'dmn:decision' || element.type === 'dmn:inputData') {
      entries['rename-var'] = {
        group: 'edit',
        className: 'cp-icon',
        title: 'FEEL-Name (Variablenname)',
        imageUrl: ICON_VARNAME,
        action: { click: () => this.eventBus.fire('dmn.renameVariable', { element }) },
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
