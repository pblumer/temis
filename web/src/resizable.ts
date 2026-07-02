// A draggable divider that resizes an adjacent panel, remembering the chosen
// width in localStorage. Used for the left sidebar (model/flow catalog) and the
// flow-designer's right inspector, both of which otherwise sit at a fixed width.
//
// `edge` says which side of the handle the panel sits on: 'left' means the panel
// is to the LEFT of the handle, so dragging the handle right grows it (a left
// sidebar); 'right' means the panel is to the RIGHT, so dragging left grows it
// (a right inspector). apply() receives the clamped width and sets it on the
// panel; onResize (if given) runs after every applied change (e.g. to refit a
// diagram), onDone after the drag settles.

export type ResizableOpts = {
  // The thin divider the user grabs.
  handle: HTMLElement
  // Which side of the handle the resized panel is on.
  edge: 'left' | 'right'
  // The panel's default width, and the width restored on double-click.
  initial: number
  // Clamp bounds for the width (px).
  min?: number
  max?: number
  // localStorage key to persist the width under (omit to not persist).
  storageKey?: string
  // Sets the resolved width on the panel.
  apply: (width: number) => void
  // Called after every width change during a drag (throttle-free; keep it cheap).
  onResize?: (width: number) => void
  // Called once the drag ends (or a double-click reset lands).
  onDone?: (width: number) => void
}

export function makeResizable(opts: ResizableOpts): void {
  const { handle, edge, initial, min = 160, max = 720, storageKey, apply, onResize, onDone } = opts
  const clamp = (w: number): number => Math.max(min, Math.min(max, w))

  const read = (): number => {
    if (!storageKey) return initial
    const stored = Number(localStorage.getItem(storageKey))
    return Number.isFinite(stored) && stored > 0 ? stored : initial
  }
  const persist = (w: number): void => {
    if (!storageKey) return
    try {
      localStorage.setItem(storageKey, String(Math.round(w)))
    } catch {
      /* storage unavailable (private mode) — width just won't persist */
    }
  }

  let width = clamp(read())
  apply(width)

  let startX = 0
  let startW = 0
  const onMove = (e: PointerEvent): void => {
    const dx = e.clientX - startX
    width = clamp(edge === 'left' ? startW + dx : startW - dx)
    apply(width)
    onResize?.(width)
  }
  const onUp = (): void => {
    document.removeEventListener('pointermove', onMove)
    document.removeEventListener('pointerup', onUp)
    document.body.classList.remove('resizing')
    persist(width)
    onDone?.(width)
  }
  handle.addEventListener('pointerdown', (e) => {
    e.preventDefault()
    startX = e.clientX
    startW = width
    document.addEventListener('pointermove', onMove)
    document.addEventListener('pointerup', onUp)
    document.body.classList.add('resizing')
  })
  // Double-click the divider to restore the default width.
  handle.addEventListener('dblclick', () => {
    width = clamp(initial)
    apply(width)
    onResize?.(width)
    persist(width)
    onDone?.(width)
  })
}
