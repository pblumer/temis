import DirectEditingModule from 'diagram-js-direct-editing'
import type EventBus from 'diagram-js/lib/core/EventBus'
import type Canvas from 'diagram-js/lib/core/Canvas'
import type CommandStack from 'diagram-js/lib/command/CommandStack'
import type { Element, Shape } from 'diagram-js/lib/model/Types'
import { ensureFeel, validateName, feelRefFor } from './feel'

// A named DMN shape (decision, input data, BKM) — anything but the requirement
// edges, which carry no name. This gates whether a name can be edited at all.
// Renaming is a deliberate gesture: the context pad's pencil icon, the Enter or
// F2 key, or — for a freshly placed element — automatically, so it can be named
// in the same gesture that creates it. Double-click is reserved for switching to
// an element's content (see canvas.ts), so it never inline-renames.
const isNameable = (el: Element | undefined): el is Shape =>
  !!el &&
  typeof el.type === 'string' &&
  el.type.indexOf('dmn:') === 0 &&
  el.type !== 'dmn:informationRequirement' &&
  el.type !== 'dmn:knowledgeRequirement'

// A shape whose FEEL identifier (variable name) is separable from its display
// name: a decision or an input data. A BKM is referenced by its name, so its name
// must itself stay a valid FEEL identifier and carries no separate variable.
const hasSeparableVarName = (el: Element | undefined): el is Shape =>
  !!el && (el.type === 'dmn:decision' || el.type === 'dmn:inputData')

// A mutable DMN shape carrying the fields the label editor touches: the free-form
// display name, the FEEL identifier and whether that identifier was set explicitly
// (so a display rename does not overwrite it).
type NamedShape = Shape & { name?: string; varName?: string; varNameLocked?: boolean; type?: string }

// Undoable display rename: change the element's name and report it changed so the
// renderer redraws. diagram-js core has no label command, so we add one. For a
// decision or input data the FEEL identifier follows the (new) display name unless
// it was explicitly set — so the common case needs no second edit, while a
// free-form label that FEEL would reject still yields a usable identifier.
class UpdateNameHandler {
  execute(ctx: { element: NamedShape; newName: string; old?: string; oldVar?: string }): Element[] {
    ctx.old = ctx.element.name
    ctx.oldVar = ctx.element.varName
    ctx.element.name = ctx.newName
    if (hasSeparableVarName(ctx.element) && !ctx.element.varNameLocked) {
      ctx.element.varName = feelRefFor(ctx.newName)
    }
    return [ctx.element]
  }
  revert(ctx: { element: NamedShape; old?: string; oldVar?: string }): Element[] {
    ctx.element.name = ctx.old
    ctx.element.varName = ctx.oldVar
    return [ctx.element]
  }
}

// Undoable FEEL-identifier rename: set the element's variable name explicitly and
// mark it locked, so it stops following the display name. Reachable only for a
// decision or input data (see hasSeparableVarName).
class UpdateVarNameHandler {
  execute(ctx: { element: NamedShape; newVarName: string; oldVar?: string; oldLocked?: boolean }): Element[] {
    ctx.oldVar = ctx.element.varName
    ctx.oldLocked = ctx.element.varNameLocked
    ctx.element.varName = ctx.newVarName
    ctx.element.varNameLocked = true
    return [ctx.element]
  }
  revert(ctx: { element: NamedShape; oldVar?: string; oldLocked?: boolean }): Element[] {
    ctx.element.varName = ctx.oldVar
    ctx.element.varNameLocked = ctx.oldLocked
    return [ctx.element]
  }
}

// Rename a node inline via the pencil icon, the Enter/F2 keys or an auto-rename
// on create. The display name is free-form for a decision or input data (its FEEL
// identifier is edited separately); a BKM's name and any FEEL identifier are
// validated live against the FEEL engine (spaces ok, operator characters not) and
// applied only when valid — through the command stack, so edits undo/redo
// (ADR-0016, WP-65). DirectEditing is the slice of diagram-js-direct-editing we use.
type DirectEditing = {
  registerProvider: (p: unknown) => void
  activate: (el: Element) => void
  complete: () => void
  cancel: () => void
}

// The slice of the diagram-js selection service we use — the elements currently
// selected on the canvas, so Enter/F2 know which one to rename.
type SelectionService = { get: () => Element[] }

// A Canvas with the coordinate-transform helpers we need (not in the bundled
// types), to place the edit box in screen space.
type ViewboxCanvas = Canvas & {
  zoom: () => number
  getAbsoluteBBox: (b: { x: number; y: number; width: number; height: number }) => { x: number; y: number; width: number; height: number }
}

class DmnLabelEditing {
  static $inject = ['eventBus', 'directEditing', 'commandStack', 'canvas', 'selection']

  private commandStack: CommandStack
  private canvas: ViewboxCanvas
  private directEditing: DirectEditing
  // The shape currently being inline-edited, so the box can follow it when the
  // canvas pans or zooms; null when no edit is active.
  private active: NamedShape | null = null
  // Which field the open editor edits: the free-form display 'name', or the FEEL
  // 'varName' identifier. Set before each activate() and read back inside it.
  private mode: 'name' | 'varName' = 'name'

  constructor(eventBus: EventBus, directEditing: DirectEditing, commandStack: CommandStack, canvas: Canvas, selection: SelectionService) {
    this.commandStack = commandStack
    this.canvas = canvas as ViewboxCanvas
    this.directEditing = directEditing
    commandStack.registerHandler('element.updateName', UpdateNameHandler)
    commandStack.registerHandler('element.updateVarName', UpdateVarNameHandler)
    directEditing.registerProvider(this)

    // Renaming is a deliberate gesture only — never double-click, which is
    // reserved for switching to an element's content (see canvas.ts). The ways in:
    // the context pad's pencil icon, and the Enter/F2 key on the selection.
    eventBus.on('dmn.renameElement', (event: { element?: Element }) => {
      if (!isNameable(event.element)) return
      this.startRename(event.element)
    })

    // Edit the FEEL identifier (variable name) directly — the context pad's
    // FEEL-name action, offered only for a decision or input data. It opens the
    // same inline box bound to the variable name, validated against the engine.
    eventBus.on('dmn.renameVariable', (event: { element?: Element }) => {
      if (!hasSeparableVarName(event.element)) return
      this.startRenameVar(event.element)
    })

    // Naming a new element is part of creating it: a shape placed from the palette
    // or appended via the context pad opens its inline-rename box immediately, so
    // the modeler types the name in the same gesture instead of hunting for the
    // pencil icon afterwards (the default "Neue Decision" name stays if they just
    // press Escape). Both creation paths run through the `shape.create` command —
    // the palette's create-drop nests one createShape per element — so this single
    // hook covers both. Loading a diagram uses canvas.addShape (no command) and
    // undo/redo replays fire only `executed`, not `postExecuted`, so neither ever
    // triggers an auto-rename. Deferred a tick so the create session's own cleanup
    // and any sibling command (the append's requirement edge) finish and the shape
    // is selected before the box opens.
    eventBus.on('commandStack.shape.create.postExecuted', (event: { context?: { shape?: Element } }) => {
      const shape = event.context?.shape
      if (!isNameable(shape)) return
      setTimeout(() => {
        if (this.active) return
        this.startRename(shape)
      }, 0)
    })

    // Enter or F2 renames the single selected nameable shape — the keyboard
    // equivalent of the pencil icon. Enter is the Finder-style rename, offered
    // because most other keys are already claimed by the browser or OS while
    // Enter over a selected node is otherwise unused here; F2 is the classic
    // desktop rename. Ignored when a modifier is held, while already editing, or
    // while a text field / button / link has focus (the FEEL editor, the model
    // search, the inline-rename box itself, a focused toolbar button), so it never
    // hijacks a keystroke meant for something else — an Enter meant to submit a
    // field or click a button still does that.
    document.addEventListener('keydown', (e) => {
      if ((e.key !== 'F2' && e.key !== 'Enter') || e.defaultPrevented || this.active) return
      if (e.ctrlKey || e.metaKey || e.altKey || e.shiftKey) return
      const target = e.target as HTMLElement | null
      if (target && (target.isContentEditable || /^(INPUT|TEXTAREA|SELECT|BUTTON|A)$/.test(target.tagName))) return
      const sel = selection.get()
      if (sel.length !== 1 || !isNameable(sel[0])) return
      e.preventDefault()
      this.startRename(sel[0])
    })

    // Keep the edit box glued to its element when the canvas viewbox changes
    // (scroll/trackpad pan or zoom) while editing — otherwise the absolutely
    // positioned box detaches and drifts far from the element.
    eventBus.on('canvas.viewbox.changed', () => this.reposition())
    eventBus.on(['directEditing.cancel', 'directEditing.complete', 'directEditing.deactivate'], () => {
      this.active = null
    })
  }

  // startRename opens the inline editor over a nameable shape to edit its display
  // name. Shared by the pencil icon, Enter/F2 and the auto-rename on create.
  private startRename(element: Element): void {
    this.mode = 'name'
    void ensureFeel() // load the validator in the background
    this.directEditing.activate(element)
    this.wireLiveValidation()
  }

  // startRenameVar opens the inline editor over a decision/input-data shape to edit
  // its FEEL identifier (variable name) directly, always validated.
  private startRenameVar(element: Element): void {
    this.mode = 'varName'
    void ensureFeel()
    this.directEditing.activate(element)
    this.wireLiveValidation()
  }

  // feelRequiredNow reports whether the open editor's text must be a valid FEEL
  // name: always when editing the variable name, and when editing a BKM's display
  // name (a BKM is referenced by its name). A decision/input-data display name is
  // free-form.
  private feelRequiredNow(): boolean {
    if (this.mode === 'varName') return true
    return !!this.active && this.active.type === 'dmn:businessKnowledgeModel'
  }

  // activate tells direct-editing what to edit and where. The box is positioned in
  // SCREEN space (the canvas may be zoomed/panned), so it sits exactly over the
  // element, with the font scaled to the zoom.
  activate(element: Element): { text: string; bounds: { x: number; y: number; width: number; height: number }; style: Record<string, string>; options: { centerVertically: boolean } } | undefined {
    // Gate on editability: any DMN shape's display name is editable; the FEEL-name
    // mode is offered only for a decision or input data. Either way a decision with
    // logic stays renamable (its double-click is reserved for opening its editor).
    if (this.mode === 'varName' ? !hasSeparableVarName(element) : !isNameable(element)) return undefined
    const shape = element as NamedShape
    this.active = shape
    // The variable-name editor seeds from the current FEEL identifier (falling back
    // to the display name); the display editor seeds from the display name.
    const text = this.mode === 'varName' ? shape.varName || shape.name || '' : shape.name ?? ''
    return {
      text,
      bounds: this.screenBounds(shape),
      // The box is the node's full width, so a longer name wraps within it (like
      // the rendered label) instead of spilling past the borders; centerVertically
      // keeps that wrapped text centred in the node rather than riding the top edge.
      // The outer parent is made invisible (transparent, no border) so only the
      // inner content pill — the actual editor, sized to its wrapped text — shows,
      // instead of an empty node-sized frame sitting behind it.
      style: { textAlign: 'center', fontFamily: 'system-ui, sans-serif', fontSize: 13 * this.canvas.zoom() + 'px', fontWeight: '500', lineHeight: '1.25', backgroundColor: 'transparent', border: 'none' },
      options: { centerVertically: true },
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

  // update is called on commit. The variable-name edit applies only a valid,
  // non-empty FEEL name; a BKM's display name likewise (it is the BKM's reference);
  // a decision/input-data display name is free-form (only non-empty), its FEEL
  // identifier following along or edited separately.
  update(element: Element, newText: string): void {
    const text = (newText || '').trim()
    if (!text) return
    if (this.feelRequiredNow() && !validateName(text).ok) return
    if (this.mode === 'varName') {
      this.commandStack.execute('element.updateVarName', { element, newVarName: text })
    } else {
      this.commandStack.execute('element.updateName', { element, newName: text })
    }
  }

  // wireLiveValidation marks the editing box red while the typed text is not a
  // valid FEEL name — but only when validity is required (the variable name, or a
  // BKM's name). A free-form display label is never flagged.
  private wireLiveValidation(): void {
    const content = this.canvas.getContainer().querySelector<HTMLElement>('.djs-direct-editing-content')
    if (!content) return
    const check = (): void => {
      if (!this.feelRequiredNow()) {
        content.classList.remove('name-invalid')
        content.title = ''
        return
      }
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
