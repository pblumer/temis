// The modeling assistant panel (ADR-0024): a chat docked in the modeler that
// lets an LLM drive temis's tools to help build decisions. The browser only
// holds the visible conversation; the server runs the tool-calling loop and,
// when the assistant creates or changes a model, returns its id so we reload it.
import { chat, type ChatMessage, type ChatStep } from './api'

// AssistHandle controls the panel from the toolbar.
export type AssistHandle = { toggle: () => void; isOpen: () => boolean }

// Persisted settings (kept in the browser only). The API key never leaves the
// browser except as the per-request X-LLM-Token header to our own server.
const KEY_TOKEN = 'temis.assist.token'
const KEY_PROVIDER = 'temis.assist.provider'
const KEY_MODEL = 'temis.assist.model'
const KEY_REMEMBER = 'temis.assist.remember'

function load(key: string): string {
  try {
    return localStorage.getItem(key) ?? ''
  } catch {
    return ''
  }
}
function save(key: string, value: string): void {
  try {
    if (value) localStorage.setItem(key, value)
    else localStorage.removeItem(key)
  } catch {
    /* ignore storage failures (private mode) */
  }
}

// The BYOK API key is a secret, so by default it lives in sessionStorage and is
// dropped when the tab closes — it is not persisted across sessions unless the
// user explicitly opts in via the "remember" checkbox (audit finding M8). A
// persisted key can be read by any script on the page, so the opt-in is a
// deliberate, informed trade-off.
function loadToken(): string {
  try {
    return sessionStorage.getItem(KEY_TOKEN) ?? load(KEY_TOKEN)
  } catch {
    return load(KEY_TOKEN)
  }
}
function saveToken(value: string, remember: boolean): void {
  try {
    if (value) sessionStorage.setItem(KEY_TOKEN, value)
    else sessionStorage.removeItem(KEY_TOKEN)
  } catch {
    /* ignore storage failures (private mode) */
  }
  // Only mirror into persistent storage when the user asked us to remember it;
  // otherwise make sure no stale persisted copy survives.
  save(KEY_TOKEN, remember ? value : '')
}

// mountAssist renders the panel into host and returns a handle to toggle it.
// currentModelId supplies the open model's id as context for each turn;
// onModelChanged is called when the assistant produced a new model revision.
export function mountAssist(
  host: HTMLElement,
  opts: { currentModelId: () => string; onModelChanged: (id: string) => void },
): AssistHandle {
  host.innerHTML = `
    <div class="assist-head">
      <span class="assist-title">Modellierungs-Assistent</span>
      <span class="assist-head-actions">
        <button id="assistSettings" class="icon-btn" type="button" title="Einstellungen">⚙</button>
        <button id="assistClose" class="icon-btn" type="button" title="Schließen">×</button>
      </span>
    </div>
    <div id="assistConfig" class="assist-config" hidden>
      <label>Anbieter
        <select id="assistProvider">
          <option value="">Server-Standard</option>
          <option value="anthropic">Anthropic</option>
          <option value="openai">OpenAI</option>
        </select>
      </label>
      <label>Modell <input id="assistModel" type="text" placeholder="Standard" /></label>
      <label>API-Key (optional)
        <input id="assistToken" type="password" placeholder="eigener Schlüssel" autocomplete="off" />
      </label>
      <label class="assist-remember"><input id="assistRemember" type="checkbox" /> Auf diesem Gerät merken</label>
      <p class="assist-hint">Der Schlüssel bleibt im Browser und wird nur für deine Anfragen mitgeschickt.
      Standardmäßig nur für diese Sitzung; „merken“ speichert ihn dauerhaft (dann für Skripte auf dieser Seite lesbar).
      Ohne Schlüssel nutzt der Server seinen eigenen.</p>
    </div>
    <div id="assistLog" class="assist-log">
      <p class="assist-empty">Frag mich, wie du eine Decision baust — z. B. „Erstelle eine Tabelle, die ab 18 Jahren ‚erwachsen‘ ausgibt.“</p>
    </div>
    <form id="assistForm" class="assist-form">
      <textarea id="assistInput" rows="2" placeholder="Nachricht … (Strg/Cmd+Enter zum Senden)"></textarea>
      <button id="assistSend" type="submit" class="tbtn">Senden</button>
    </form>`

  const config = host.querySelector<HTMLElement>('#assistConfig')!
  const providerSel = host.querySelector<HTMLSelectElement>('#assistProvider')!
  const modelInput = host.querySelector<HTMLInputElement>('#assistModel')!
  const tokenInput = host.querySelector<HTMLInputElement>('#assistToken')!
  const rememberInput = host.querySelector<HTMLInputElement>('#assistRemember')!
  const log = host.querySelector<HTMLElement>('#assistLog')!
  const form = host.querySelector<HTMLFormElement>('#assistForm')!
  const input = host.querySelector<HTMLTextAreaElement>('#assistInput')!
  const sendBtn = host.querySelector<HTMLButtonElement>('#assistSend')!

  providerSel.value = load(KEY_PROVIDER)
  modelInput.value = load(KEY_MODEL)
  tokenInput.value = loadToken()
  rememberInput.checked = load(KEY_REMEMBER) === '1'
  providerSel.addEventListener('change', () => save(KEY_PROVIDER, providerSel.value))
  modelInput.addEventListener('change', () => save(KEY_MODEL, modelInput.value.trim()))
  tokenInput.addEventListener('change', () => saveToken(tokenInput.value.trim(), rememberInput.checked))
  rememberInput.addEventListener('change', () => {
    save(KEY_REMEMBER, rememberInput.checked ? '1' : '')
    saveToken(tokenInput.value.trim(), rememberInput.checked)
  })

  host.querySelector('#assistSettings')?.addEventListener('click', () => {
    config.hidden = !config.hidden
  })
  host.querySelector('#assistClose')?.addEventListener('click', () => setOpen(false))

  // The conversation we show and replay to the server (user + assistant text).
  const history: ChatMessage[] = []
  let busy = false

  const addBubble = (role: 'user' | 'assistant', text: string): HTMLElement => {
    host.querySelector('.assist-empty')?.remove()
    const el = document.createElement('div')
    el.className = 'assist-msg assist-' + role
    el.textContent = text
    log.appendChild(el)
    log.scrollTop = log.scrollHeight
    return el
  }

  const addSteps = (steps: ChatStep[]): void => {
    if (!steps.length) return
    const wrap = document.createElement('div')
    wrap.className = 'assist-steps'
    for (const s of steps) {
      const chip = document.createElement('span')
      chip.className = 'assist-step' + (s.error ? ' assist-step-error' : '')
      chip.textContent = (s.error ? '✕ ' : '✓ ') + s.tool
      if (s.result) chip.title = s.result
      wrap.appendChild(chip)
    }
    log.appendChild(wrap)
    log.scrollTop = log.scrollHeight
  }

  const send = async (): Promise<void> => {
    const text = input.value.trim()
    if (!text || busy) return
    busy = true
    sendBtn.disabled = true
    input.value = ''
    addBubble('user', text)
    history.push({ role: 'user', text })

    // Give the assistant the open model's id as a transient context turn (not
    // stored in the visible history).
    const modelId = opts.currentModelId()
    const messages: ChatMessage[] = modelId
      ? [{ role: 'user', text: `Kontext: Das aktuell geöffnete Modell hat die id "${modelId}". Nutze sie für Werkzeuge, wenn passend.` }, ...history]
      : [...history]

    const pending = addBubble('assistant', '…')
    try {
      const provider = providerSel.value
      const model = modelInput.value.trim()
      const token = tokenInput.value.trim()
      const reply = await chat(messages, {
        token: token || undefined,
        provider: provider || undefined,
        model: model || undefined,
      })
      pending.textContent = reply.reply || '(keine Antwort)'
      history.push({ role: 'assistant', text: reply.reply })
      addSteps(reply.steps ?? [])
      // The assistant built or changed a model: reload that revision.
      if (reply.modelId && reply.modelId !== modelId) {
        opts.onModelChanged(reply.modelId)
      }
    } catch (e) {
      pending.textContent = '⚠ ' + (e as Error).message
      pending.classList.add('assist-error')
    } finally {
      busy = false
      sendBtn.disabled = false
      input.focus()
    }
  }

  form.addEventListener('submit', (e) => {
    e.preventDefault()
    void send()
  })
  input.addEventListener('keydown', (e) => {
    if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
      e.preventDefault()
      void send()
    }
  })

  const setOpen = (open: boolean): void => {
    host.classList.toggle('is-open', open)
    if (open) input.focus()
  }

  return {
    toggle: () => setOpen(!host.classList.contains('is-open')),
    isOpen: () => host.classList.contains('is-open'),
  }
}
