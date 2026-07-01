// Small promise-based modal dialogs (ADR-0016): a text prompt and a confirm, so
// the modeler can ask for a name or a yes/no without the browser's native
// window.prompt/confirm — styled to match the app and dismissible with Esc. Each
// dialog builds on the shared .dt-overlay backdrop used by the editor overlays.

function el(tag: string, attrs: Record<string, string> = {}, ...children: (string | Node)[]): HTMLElement {
  const n = document.createElement(tag)
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v)
  n.append(...children)
  return n
}

// promptDialog asks for a single line of text and resolves to the trimmed value,
// or null when cancelled (Cancel button, Esc or backdrop click). The OK button
// stays disabled while the field is empty. An optional hint(value) shows live
// feedback below the field (e.g. a duplicate-name warning) without blocking.
export function promptDialog(opts: {
  title: string
  label?: string
  value?: string
  placeholder?: string
  okLabel?: string
  hint?: (value: string) => string | null
}): Promise<string | null> {
  return new Promise((resolve) => {
    const input = el('input', { class: 'dlg-input', type: 'text', value: opts.value ?? '', placeholder: opts.placeholder ?? '' }) as HTMLInputElement
    const hint = el('div', { class: 'dlg-hint' })
    const okBtn = el('button', { class: 'dlg-btn dlg-btn-primary', type: 'button' }, opts.okLabel ?? 'OK') as HTMLButtonElement
    const cancelBtn = el('button', { class: 'dlg-btn', type: 'button' }, 'Abbrechen') as HTMLButtonElement

    let done = false
    const finish = (val: string | null): void => {
      if (done) return
      done = true
      overlay.remove()
      document.removeEventListener('keydown', onKey)
      resolve(val)
    }
    const submit = (): void => {
      const v = input.value.trim()
      if (v) finish(v)
    }
    const sync = (): void => {
      const v = input.value.trim()
      okBtn.disabled = v === ''
      const h = opts.hint?.(v) ?? null
      hint.textContent = h ?? ''
      hint.style.display = h ? '' : 'none'
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') finish(null)
      else if (e.key === 'Enter') submit()
    }

    okBtn.addEventListener('click', submit)
    cancelBtn.addEventListener('click', () => finish(null))
    input.addEventListener('input', sync)
    document.addEventListener('keydown', onKey)

    const modal = el(
      'div',
      { class: 'dlg-modal' },
      el('div', { class: 'dlg-head' }, el('span', { class: 'dlg-title' }, opts.title)),
      el('div', { class: 'dlg-body' }, ...(opts.label ? [el('label', { class: 'dlg-label' }, opts.label)] : []), input, hint),
      el('div', { class: 'dlg-actions' }, cancelBtn, okBtn),
    )
    const overlay = el('div', { class: 'dt-overlay' }, modal)
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) finish(null)
    })
    document.body.append(overlay)
    sync()
    input.focus()
    input.select()
  })
}

// confirmDialog asks a yes/no question and resolves true only when confirmed.
// Cancel, Esc and backdrop click resolve false. With danger, the confirm button
// is styled as a destructive action (used for delete).
export function confirmDialog(opts: { title: string; message: string; okLabel?: string; danger?: boolean }): Promise<boolean> {
  return new Promise((resolve) => {
    const okBtn = el('button', { class: 'dlg-btn ' + (opts.danger ? 'dlg-btn-danger' : 'dlg-btn-primary'), type: 'button' }, opts.okLabel ?? 'OK') as HTMLButtonElement
    const cancelBtn = el('button', { class: 'dlg-btn', type: 'button' }, 'Abbrechen') as HTMLButtonElement

    let done = false
    const finish = (val: boolean): void => {
      if (done) return
      done = true
      overlay.remove()
      document.removeEventListener('keydown', onKey)
      resolve(val)
    }
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') finish(false)
      else if (e.key === 'Enter') finish(true)
    }
    okBtn.addEventListener('click', () => finish(true))
    cancelBtn.addEventListener('click', () => finish(false))
    document.addEventListener('keydown', onKey)

    const modal = el(
      'div',
      { class: 'dlg-modal' },
      el('div', { class: 'dlg-head' }, el('span', { class: 'dlg-title' }, opts.title)),
      el('div', { class: 'dlg-body' }, el('div', { class: 'dlg-message' }, opts.message)),
      el('div', { class: 'dlg-actions' }, cancelBtn, okBtn),
    )
    const overlay = el('div', { class: 'dt-overlay' }, modal)
    overlay.addEventListener('click', (e) => {
      if (e.target === overlay) finish(false)
    })
    document.body.append(overlay)
    okBtn.focus()
  })
}
