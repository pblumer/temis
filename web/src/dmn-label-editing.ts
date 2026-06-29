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
const isRenamable = (el: (Element & { hasTable?: boolean; hasLiteral?: boolean }) | undefined): el is Shape =>
  !!el &&
  typeof el.type === 'string' &&
  el.type.indexOf('dmn:') === 0 &&
  el.type !== 'dmn:informationRequirement' &&
  el.type !== 'dmn:knowledgeRequirement' &&
  !(el.type === 'dmn:decision' && (el.hasTable || el.hasLiteral))

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
class DmnLabelEditing {
  static $inject = ['eventBus', 'directEditing', 'commandStack', 'canvas']

  private commandStack: CommandStack
  private canvas: Canvas

  constructor(
    eventBus: EventBus,
    directEditing: { registerProvider: (p: unknown) => void; activate: (el: Element) => void },
    commandStack: CommandStack,
    canvas: Canvas,
  ) {
    this.commandStack = commandStack
    this.canvas = canvas
    commandStack.registerHandler('element.updateName', UpdateNameHandler)
    directEditing.registerProvider(this)

    eventBus.on('element.dblclick', (event: { element: Element }) => {
      if (!isRenamable(event.element)) return
      void ensureFeel() // load the validator in the background
      directEditing.activate(event.element)
      this.wireLiveValidation()
    })
  }

  // activate tells direct-editing what to edit and where.
  activate(element: Element): { text: string; bounds: { x: number; y: number; width: number; height: number }; style: Record<string, string> } | undefined {
    if (!isRenamable(element)) return undefined
    const shape = element as Shape & { name?: string }
    return {
      text: shape.name ?? '',
      bounds: { x: shape.x ?? 0, y: shape.y ?? 0, width: shape.width ?? 0, height: shape.height ?? 0 },
      style: { textAlign: 'center', fontFamily: 'system-ui, sans-serif', fontSize: '13px', fontWeight: '500' },
    }
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
    check()
  }
}

export const dmnLabelEditingModule = {
  __depends__: [DirectEditingModule],
  __init__: ['dmnLabelEditing'],
  dmnLabelEditing: ['type', DmnLabelEditing],
}
