import RuleProvider from 'diagram-js/lib/features/rules/RuleProvider'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type { Shape, Connection } from 'diagram-js/lib/model/Types'

// The DMN modeling rules (ADR-0016, WP-65): what may be moved, created and —
// the focus here — which requirement edge may connect which elements. A
// requirement arrow points from the required (upstream) element to the element
// that requires it, so the rule decides both validity and the concrete edge
// type from the source/target kinds:
//   • information requirement: inputData|decision  →  decision
//   • knowledge requirement:   BKM                 →  decision|BKM
// Anything else (an arrow into an inputData, a decision feeding a BKM, a
// self-loop, a duplicate, or an edge that would close a cycle) is rejected.

type Req = { type: string }

// requirement returns the edge attrs for a legal source→target requirement, or
// false. It encodes only the kind-based legality; duplicates and cycles are
// checked separately so the auto-reverse in Connect still gets a clean answer.
function requirement(source: Shape, target: Shape): Req | false {
  if (!source || !target || source === target) return false
  // Nothing requires *into* input data — it is a leaf source.
  if (target.type === 'dmn:inputData') return false
  if (source.type === 'dmn:businessKnowledgeModel') {
    // A BKM lends its knowledge to a decision or to another BKM.
    if (target.type === 'dmn:decision' || target.type === 'dmn:businessKnowledgeModel') {
      return { type: 'dmn:knowledgeRequirement' }
    }
    return false
  }
  // An inputData or decision feeds data into a decision.
  if (target.type === 'dmn:decision') return { type: 'dmn:informationRequirement' }
  return false
}

// duplicate reports whether source already connects to target (ignoring the
// connection currently being reconnected, if any).
function duplicate(source: Shape, target: Shape, ignore?: Connection): boolean {
  return (source.outgoing ?? []).some((c) => c.target === target && c !== ignore)
}

// wouldCycle reports whether adding source→target would close a requirement
// cycle — i.e. source already (transitively) depends on target. Dependencies
// follow incoming requirement edges (a node depends on the sources of its
// incoming edges). The edge being reconnected is ignored.
function wouldCycle(source: Shape, target: Shape, ignore?: Connection): boolean {
  const stack: Shape[] = [source]
  const seen = new Set<string>([source.id])
  while (stack.length) {
    const n = stack.pop() as Shape
    for (const e of n.incoming ?? []) {
      if (e === ignore) continue
      const src = e.source as Shape | null
      if (!src) continue
      if (src === target) return true
      if (!seen.has(src.id)) {
        seen.add(src.id)
        stack.push(src)
      }
    }
  }
  return false
}

function canConnect(source: Shape, target: Shape, ignore?: Connection): Req | false {
  const req = requirement(source, target)
  if (!req) return false
  if (duplicate(source, target, ignore)) return false
  if (wouldCycle(source, target, ignore)) return false
  return req
}

class DmnRules extends RuleProvider {
  static $inject = ['eventBus']

  constructor(eventBus: EventBus) {
    super(eventBus)
  }

  init(): void {
    this.addRule('shape.move', () => true)
    this.addRule('elements.move', () => true)
    // Any DMN node may be resized; the minimum size is enforced in the app shell
    // (resize.start), and the new bounds are persisted to the DMNDI by the
    // structural save. Requirement edges are not shapes, so they never resize.
    this.addRule('shape.resize', () => true)
    // Permit creating a fresh element from the palette (drag/click to place);
    // the new node is undoable and persisted by the structural save.
    this.addRule('shape.create', () => true)
    // Any element (node or requirement edge) may be deleted — the editor-actions
    // removeSelection (Delete/Backspace) consults this rule, and the structural
    // save then drops the element and its DMNDI on the server.
    this.addRule('elements.delete', () => true)
    // A requirement may start from any node; Connect figures out the valid
    // direction from the hovered target via connection.create below.
    this.addRule('connection.start', () => true)
    // Returning the edge attrs (or false) lets Connect both reject illegal
    // requirements and auto-orient a legal one (it tries the reverse when the
    // dragged direction isn't allowed), so the DMN arrow always ends up
    // pointing required → requiring with the right type.
    this.addRule('connection.create', (ctx: { source: Shape; target: Shape }) => canConnect(ctx.source, ctx.target))
    this.addRule('connection.reconnect', (ctx: { connection: Connection; source: Shape; target: Shape }) =>
      canConnect(ctx.source, ctx.target, ctx.connection),
    )
  }
}

export const dmnRulesModule = {
  __init__: ['dmnRules'],
  dmnRules: ['type', DmnRules],
}
