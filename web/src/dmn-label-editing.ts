import DirectEditingModule from 'diagram-js-direct-editing'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type CommandStack from 'diagram-js/lib/command/CommandStack'
import type { Element, Shape } from 'diagram-js/lib/model/Types'
import { ensureFeel, validateName } from './feel'

// A node is inline-renamable unless it is a requirement edge or a decision whose
// logic is a decision table or a literal expression — for those, double-click
// opens the respective editor instead (see canvas.ts), so the gestures do not
// collide.
// A named DMN shape (decision, input data, BKM) — anything but the requirement
// edges, which carry no name. This gates whether a name can be edited at all.
const isNameable = (el: (Element & { hasLogic?: boolean }) | undefined): el is Shape =>
  !!el &&
  typeof el.type === 'string' &&
  el.type.indexOf('dmn:') === 0 &&
  el.type !== 'dmn:informationRequirement' &&
  el.type !== 'dmn:knowledgeRequirement'

// Whether DOUBLE-CLICK should inline-rename. A decision that carries ANY logic
// (table, literal, or any boxed expression) opens its editor on double-click, so
// it must NOT also inline-rename — else both gestures fire at once. hasLogic is
// the aggregate flag, so this stays correct as new boxed kinds are added. Such a
// decision is still renamable on demand via the context pad's rename action,
// which drives activate() directly (bypassing this double-click guard).
const isRenamable = (el: (Element & { hasLogic?: boolean }) | undefined): el is Shape =>
  isNameable(el) && !(el.type === 'dmn:decision' && el.hasLogic)

// Undoable rename: change the element's name and report it changed so the
// renderer redraws. diagram-js core has no label command, so we add one.
class UpdateNameHandler {
  execute(ctx: { element: Shape & { name?: string }; newName: string; old?: string }): Element[] {
    ctx.old = ctx.element.name
    ctx.element.name = ctx.newName
    return [ctx.element]
  }
  revert(ctx: { element: Shape & { name?: string }; old?: string }): Element[] {
    ctx.element.name = ctx.old
    return [ctx.element]
  }
}

// Double-click a node to rename it inline; the name is validated live against
// the FEEL engine (spaces ok, operator characters not) and only applied when
// valid — through the command stack, so it undoes/redoes (ADR-0016, WP-65).
// DirectEditing is the slice of the diagram-js-direct-editing service we use.
type DirectEditing = {
  registerProvider: (p: unknown) => void
  activate: (el: Element) => void
  complete: () => void
  cancel: () => void
}

// A Canvas with the coordinate-transform helpers we need (not in the bundled
// types), to place the edit box in screen space.
type ViewboxCanvas = Canvas & {
  zoom: () => number
  getAbsoluteBBox: (b: { x: number; y: number; width: number; height: number }) => { x: number; y: number; width: number; height: number }
}

class DmnLabelEditing {
  static $inject = ['eventBus', 'directEditing', 'commandStack', 'canvas']

  private commandStack: CommandStack
  private canvas: ViewboxCanvas
  private directEditing: DirectEditing
  // The shape currently being inline-edited, so the box can follow it when the
  // canvas pans or zooms; null when no edit is active.
  private active: (Shape & { name?: string }) | null = null

  constructor(eventBus: EventBus, directEditing: DirectEditing, commandStack: CommandStack, canvas: Canvas) {
    this.commandStack = commandStack
    this.canvas = canvas as ViewboxCanvas
    this.directEditing = directEditing
    commandStack.registerHandler('element.updateName', UpdateNameHandler)
    directEditing.registerProvider(this)

    eventBus.on('element.dblclick', (event: { element: Element }) => {
      if (!isRenamable(event.element)) return
      void ensureFeel() // load the validator in the background
      directEditing.activate(event.element)
      this.wireLiveValidation()
    })

    // The context pad's rename action: rename any nameable shape on demand — the
    // deliberate gesture, so it works even for a decision with logic (whose
    // double-click is reserved for opening its editor).
    eventBus.on('dmn.renameElement', (event: { element?: Element }) => {
      if (!isNameable(event.element)) return
      void ensureFeel()
      directEditing.activate(event.element)
      this.wireLiveValidation()
    })

    // Keep the edit box glued to its element when the canvas viewbox changes
    // (scroll/trackpad pan or zoom) while editing — otherwise the absolutely
    // positioned box detaches and drifts far from the element.
    eventBus.on('canvas.viewbox.changed', () => this.reposition())
    eventBus.on(['directEditing.cancel', 'directEditing.complete', 'directEditing.deactivate'], () => {
      this.active = null
    })
  }

  // activate tells direct-editing what to edit and where. The box is positioned in
  // SCREEN space (the canvas may be zoomed/panned), so it sits exactly over the
  // element, with the font scaled to the zoom.
  activate(element: Element): { text: string; bounds: { x: number; y: number; width: number; height: number }; style: Record<string, string> } | undefined {
    // Gate on nameability, not the double-click rule: the context-pad rename
    // action drives this directly for decisions with logic, whose double-click
    // is otherwise reserved for opening the editor.
    if (!isNameable(element)) return undefined
    const shape = element as Shape & { name?: string }
    this.active = shape
    return {
      text: shape.name ?? '',
      bounds: this.screenBounds(shape),
      style: { textAlign: 'center', fontFamily: 'system-ui, sans-serif', fontSize: 13 * this.canvas.zoom() + 'px', fontWeight: '500' },
    }
  }

  // screenBounds maps a shape's model bounds to the canvas container's coordinate
  // space (accounting for zoom/scroll), where the edit box is absolutely placed.
  private screenBounds(shape: Shape): { x: number; y: number; width: number; height: number } {
    return this.canvas.getAbsoluteBBox({ x: shape.x ?? 0, y: shape.y ?? 0, width: shape.width ?? 0, height: shape.height ?? 0 })
  }

  // reposition re-places the open edit box over its element after a viewbox change.
  private reposition(): void {
    if (!this.active) return
    const container = this.canvas.getContainer()
    const box = container.querySelector<HTMLElement>('.djs-direct-editing-parent')
    if (!box) return
    const b = this.screenBounds(this.active)
    box.style.left = b.x + 'px'
    box.style.top = b.y + 'px'
    box.style.width = b.width + 'px'
    const content = container.querySelector<HTMLElement>('.djs-direct-editing-content')
    if (content) content.style.fontSize = 13 * this.canvas.zoom() + 'px'
  }

  // update is called on commit; apply only a valid, non-empty name.
  update(element: Element, newText: string): void {
    const name = (newText || '').trim()
    if (!name || !validateName(name).ok) return
    this.commandStack.execute('element.updateName', { element, newName: name })
  }

  // wireLiveValidation marks the editing box red while the typed name is not a
  // valid FEEL name, with the engine's reason as a tooltip.
  private wireLiveValidation(): void {
    const content = this.canvas.getContainer().querySelector<HTMLElement>('.djs-direct-editing-content')
    if (!content) return
    const check = (): void => {
      const res = validateName((content.textContent ?? '').trim())
      content.classList.toggle('name-invalid', !res.ok)
      content.title = res.ok ? '' : res.message ?? ''
    }
    content.addEventListener('input', check)
    // Clicking outside the box commits the edit (and closes it), so it does not
    // get stuck in edit mode. complete() removes the editing box from the DOM, so
    // defer it out of the blur handler — tearing the node down synchronously while
    // the browser is dispatching its own blur/focus teardown double-frees the node
    // ("removeChild: node no longer a child").
    content.addEventListener('blur', () => setTimeout(() => this.directEditing.complete(), 0))
    check()
  }
}

export const dmnLabelEditingModule = {
  __depends__: [DirectEditingModule],
  __init__: ['dmnLabelEditing'],
  dmnLabelEditing: ['type', DmnLabelEditing],
}
