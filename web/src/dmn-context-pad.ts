import type ContextPad from 'diagram-js/lib/features/context-pad/ContextPad'
import type { ContextPadEntries } from 'diagram-js/lib/features/context-pad/ContextPadProvider'
import type Connect from 'diagram-js/lib/features/connect/Connect'
import type Modeling from 'diagram-js/lib/features/modeling/Modeling'
import type { Element } from 'diagram-js/lib/model/Types'

// Inline SVG icons as data URIs — no icon font needed, crisp at any zoom.
const svg = (inner: string): string =>
  'data:image/svg+xml,' +
  encodeURIComponent(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 18 18">${inner}</svg>`)
const ICON_CONNECT = svg('<path d="M3 9h8M9 5l4 4-4 4" fill="none" stroke="#3b4150" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"/>')
const ICON_DELETE = svg('<path d="M4 5h10M7 5V3.5h4V5M5.5 5l.8 9h5.4l.8-9" fill="none" stroke="#c0392b" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round"/>')

// The DMN context pad (ADR-0016, WP-65): the popup of actions to the right of a
// selected element, matching the dmn-js feel. Starts with connect + delete;
// append actions (add decision/input/BKM/knowledge-source) build on this.
class DmnContextPadProvider {
  static $inject = ['contextPad', 'connect', 'modeling']

  private connect: Connect
  private modeling: Modeling

  constructor(contextPad: ContextPad, connect: Connect, modeling: Modeling) {
    this.connect = connect
    this.modeling = modeling
    contextPad.registerProvider(this)
  }

  getContextPadEntries(element: Element): ContextPadEntries {
    const connect = this.connect
    const modeling = this.modeling
    return {
      connect: {
        group: 'connect',
        className: 'cp-icon',
        title: 'Verbinden (Requirement ziehen)',
        imageUrl: ICON_CONNECT,
        action: {
          click: (event: Event) => connect.start(event as MouseEvent, element),
          dragstart: (event: Event) => connect.start(event as MouseEvent, element),
        },
      },
      delete: {
        group: 'edit',
        className: 'cp-icon',
        title: 'Löschen',
        imageUrl: ICON_DELETE,
        action: {
          click: () => modeling.removeElements([element as never]),
        },
      },
    }
  }
}

export const dmnContextPadModule = {
  __init__: ['dmnContextPadProvider'],
  dmnContextPadProvider: ['type', DmnContextPadProvider],
}
