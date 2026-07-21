// access.ts — the "Zugriff" sidebar section (WP-107, ADR-0028/0035). Visible to
// everyone as a login affordance; the management panels (API keys, public
// decisions) render only for an admin credential. It drives the whole modeler's
// auth: on a secured server the user pastes a key here, it is stored (session.ts)
// and attached to every request, and the page reloads so all data re-fetches
// authenticated.

import { confirmDialog } from './dialog'
import { escapeHtml } from './dom'
import {
  clearBearer,
  createKey,
  getPublicConfig,
  KeyMgmtDisabled,
  listKeys,
  revokeKey,
  rotateKey,
  SCOPES,
  setBearer,
  whoami,
  type AccessIdentity,
  type CreatedKey,
  type KeyView,
} from './session'

// mountAccess renders the section into host and toggles the surrounding group's
// visibility. It fetches the current identity and branches: prompt for a
// credential when auth is on but the caller is anonymous; show the admin panels
// only to an admin; otherwise show a minimal identity + logout.
export async function mountAccess(group: HTMLElement, host: HTMLElement): Promise<void> {
  let id: AccessIdentity
  try {
    id = await whoami()
  } catch {
    // whoami unreachable (old server / offline): keep the section hidden rather
    // than showing a broken panel.
    group.hidden = true
    return
  }
  group.hidden = false
  host.innerHTML = ''

  if (id.authEnabled && !id.authenticated) {
    host.append(renderLogin('Dieser Server ist abgesichert. Melde dich mit einem Zugangs-Key an.'))
    return
  }
  host.append(renderIdentity(id))
  if (id.isAdmin) {
    host.append(renderKeysPanel(!id.authEnabled), renderPublicPanel())
  }
}

// renderLogin is the credential prompt: a masked field + "Anmelden". On submit it
// stores the bearer and reloads, so every subsequent fetch is authenticated.
function renderLogin(message: string): HTMLElement {
  const wrap = div('access-block')
  wrap.append(p('access-note', message))
  const input = document.createElement('input')
  input.type = 'password'
  input.className = 'access-input'
  input.placeholder = 'kid.secret'
  input.autocomplete = 'off'
  const btn = button('access-btn access-btn-primary', 'Anmelden')
  const err = p('access-err', '')
  err.hidden = true
  const submit = (): void => {
    const v = input.value.trim()
    if (!v) return
    setBearer(v)
    location.reload()
  }
  btn.addEventListener('click', submit)
  input.addEventListener('keydown', (e) => {
    if (e.key === 'Enter') submit()
  })
  wrap.append(input, btn, err)
  return wrap
}

// renderIdentity shows who is logged in (subject + scopes) and a logout button.
// In an open (unauthenticated) server it notes that no auth is configured.
function renderIdentity(id: AccessIdentity): HTMLElement {
  const wrap = div('access-block')
  if (!id.authEnabled) {
    wrap.append(p('access-note', 'Offene API — keine Authentifizierung aktiv. Lege unten einen Admin-Key an, um den Server abzusichern (persistente Keys via -keys-dir).'))
    return wrap
  }
  const who = div('access-identity')
  who.innerHTML = `<span class="access-who">${escapeHtml(id.subject ?? '—')}</span>` + (id.isAdmin ? '<span class="access-badge">admin</span>' : '')
  wrap.append(who)
  if (id.scopes?.length) wrap.append(p('access-scopes', id.scopes.join(' · ')))
  const logout = button('access-btn', 'Abmelden')
  logout.addEventListener('click', () => {
    clearBearer()
    location.reload()
  })
  wrap.append(logout)
  return wrap
}

// renderKeysPanel lists the managed API keys and offers create/rotate/revoke. The
// list loads lazily; a dormant lifecycle API (no -keys-dir) shows a hint.
function renderKeysPanel(openMode: boolean): HTMLElement {
  const panel = div('access-block')
  panel.append(heading('API-Keys'))

  // Trust-on-first-use bootstrap: on an OPEN server the lifecycle API is reachable
  // (requireScope is transparent without configured auth), so the very first admin
  // key can be minted right here. Creating any key flips the server to "secured" at
  // runtime, so we force admin scope — otherwise the operator would lock the door
  // with no admin key inside — and adopt the returned bearer as the session
  // credential immediately, so the same person is now the admin of the secured
  // server rather than shut out.
  if (openMode) {
    const boot = div('access-block')
    boot.append(p('access-note', '🔓 Dieser Server ist offen. Lege einen Admin-Key an, um ihn abzusichern — du wirst dann automatisch als dieser Admin angemeldet.'))
    const secureBtn = button('access-btn access-btn-primary', '🔒 Admin-Key anlegen & absichern')
    const err = p('access-err', '')
    err.hidden = true
    secureBtn.addEventListener('click', async () => {
      secureBtn.disabled = true
      err.hidden = true
      try {
        const created = await createKey({ scopes: ['admin'], owner: 'bootstrap admin' })
        setBearer(created.bearer) // adopt the new admin as the session so we aren't locked out
        boot.replaceChildren()
        showSecret(boot, created, 'Server abgesichert — du bist jetzt als Admin angemeldet. Secret einmalig sichtbar, jetzt kopieren:', true)
      } catch (e) {
        secureBtn.disabled = false
        err.textContent =
          e instanceof KeyMgmtDisabled
            ? 'Key-Verwaltung ist deaktiviert. Starte den Server mit -keys-dir, damit angelegte Keys persistiert werden und den Neustart überstehen.'
            : (e as Error).message
        err.hidden = false
      }
    })
    boot.append(secureBtn, err)
    panel.append(boot)
    return panel
  }

  const list = div('access-list')
  const status = p('access-err', '')
  status.hidden = true
  const newBtn = button('access-btn access-btn-primary', '+ Neuer Key')

  const refresh = async (): Promise<void> => {
    list.innerHTML = ''
    status.hidden = true
    newBtn.hidden = false
    try {
      const keys = await listKeys()
      if (!keys.length) {
        list.append(p('access-note', 'Noch keine Keys angelegt.'))
        return
      }
      for (const k of keys) list.append(renderKeyRow(k, refresh))
    } catch (e) {
      if (e instanceof KeyMgmtDisabled) {
        list.append(p('access-note', 'Key-Verwaltung ist deaktiviert (Server ohne -keys-dir). Keys sind dann nur statisch über -keys-file konfigurierbar.'))
        newBtn.hidden = true
        return
      }
      status.textContent = (e as Error).message
      status.hidden = false
    }
  }

  newBtn.addEventListener('click', () => {
    if (list.querySelector('.access-create')) return
    list.prepend(renderCreateForm(refresh))
  })
  panel.append(newBtn, status, list)
  void refresh()
  return panel
}

// renderKeyRow is one key: kid, scopes, owner/expiry/revoked, plus rotate/revoke
// for a live managed key.
function renderKeyRow(k: KeyView, refresh: () => Promise<void>): HTMLElement {
  const row = div('access-key' + (k.revoked ? ' is-revoked' : ''))
  const meta: string[] = []
  if (k.owner) meta.push(escapeHtml(k.owner))
  if (k.expiresAt) meta.push('bis ' + escapeHtml(k.expiresAt.slice(0, 10)))
  if (k.revoked) meta.push('widerrufen')
  else if (!k.managed) meta.push('statisch')
  row.innerHTML =
    `<div class="access-key-head"><code class="access-kid">${escapeHtml(k.kid)}</code>` +
    (meta.length ? `<span class="access-key-meta">${meta.join(' · ')}</span>` : '') +
    `</div><div class="access-key-scopes">${escapeHtml((k.scopes ?? []).join(' · '))}</div>`
  if (k.managed && !k.revoked) {
    const actions = div('access-key-actions')
    const rot = button('access-btn access-btn-sm', 'Rotieren')
    rot.addEventListener('click', async () => {
      try {
        showSecret(row, await rotateKey(k.kid), 'Neues Secret — einmalig sichtbar:')
        await refresh()
      } catch (e) {
        alertRow(row, (e as Error).message)
      }
    })
    const rev = button('access-btn access-btn-sm access-btn-danger', 'Widerrufen')
    rev.addEventListener('click', async () => {
      if (!(await confirmDialog({ title: 'Key widerrufen', message: `Key ${k.kid} dauerhaft entwerten? Der Aufrufer verliert sofort den Zugriff.`, okLabel: 'Widerrufen', danger: true }))) return
      try {
        await revokeKey(k.kid)
        await refresh()
      } catch (e) {
        alertRow(row, (e as Error).message)
      }
    })
    actions.append(rot, rev)
    row.append(actions)
  }
  return row
}

// renderCreateForm collects scopes + owner + expiry and mints a key, showing its
// one-time secret inline.
function renderCreateForm(refresh: () => Promise<void>): HTMLElement {
  const form = div('access-create')
  const owner = document.createElement('input')
  owner.className = 'access-input'
  owner.placeholder = 'Besitzer (optional, z. B. "CI" oder "Agent")'
  const scopeBox = div('access-scope-grid')
  const checks: HTMLInputElement[] = []
  for (const s of SCOPES) {
    const label = document.createElement('label')
    label.className = 'access-scope'
    const cb = document.createElement('input')
    cb.type = 'checkbox'
    cb.value = s.scope
    checks.push(cb)
    label.append(cb, document.createTextNode(' ' + s.label))
    scopeBox.append(label)
  }
  const expiry = document.createElement('input')
  expiry.type = 'date'
  expiry.className = 'access-input'
  expiry.title = 'Ablaufdatum (optional)'
  const err = p('access-err', '')
  err.hidden = true
  const create = button('access-btn access-btn-primary', 'Erstellen')
  const cancel = button('access-btn', 'Abbrechen')
  cancel.addEventListener('click', () => form.remove())
  create.addEventListener('click', async () => {
    const scopes = checks.filter((c) => c.checked).map((c) => c.value)
    if (!scopes.length) {
      err.textContent = 'Mindestens einen Scope wählen.'
      err.hidden = false
      return
    }
    const req: { scopes: string[]; owner?: string; expiresAt?: string } = { scopes }
    if (owner.value.trim()) req.owner = owner.value.trim()
    if (expiry.value) req.expiresAt = new Date(expiry.value + 'T00:00:00Z').toISOString()
    try {
      const created = await createKey(req)
      form.replaceChildren()
      showSecret(form, created, 'Key erstellt — Secret einmalig sichtbar, jetzt kopieren:')
      await refresh()
    } catch (e) {
      err.textContent = (e as Error).message
      err.hidden = false
    }
  })
  form.append(p('access-note', 'Scopes für den neuen Key:'), scopeBox, owner, expiry, err, rowOf(create, cancel))
  return form
}

// showSecret renders the one-time bearer with a copy button. The secret is never
// retrievable again, so this is the only chance to grab it.
function showSecret(container: HTMLElement, key: CreatedKey, title: string, reload = false): void {
  const box = div('access-secret')
  box.append(p('access-note', title))
  const code = document.createElement('code')
  code.className = 'access-secret-value'
  code.textContent = key.bearer
  const copy = button('access-btn access-btn-sm', 'Kopieren')
  copy.addEventListener('click', () => {
    void navigator.clipboard?.writeText(key.bearer)
    copy.textContent = 'Kopiert ✓'
  })
  box.append(code, copy)
  // After a bootstrap (open → secured) the bearer is already stored; a reload
  // re-reads whoami so the section switches to the authenticated admin view.
  if (reload) {
    const rl = button('access-btn access-btn-primary access-btn-sm', 'Fertig — neu laden')
    rl.addEventListener('click', () => location.reload())
    box.append(rl)
  }
  container.prepend(box)
}

// renderPublicPanel shows the effective public-decision configuration (ADR-0035),
// read-only: which evaluations are open to anonymous callers.
function renderPublicPanel(): HTMLElement {
  const panel = div('access-block')
  panel.append(heading('Public Decisions'))
  const body = div('access-list')
  panel.append(body)
  void (async () => {
    try {
      const cfg = await getPublicConfig()
      if (cfg.evaluate) {
        body.append(p('access-note', '🌐 Globaler Schalter aktiv: jede Auswertung ist anonym offen (write/admin bleiben geschützt).'))
      }
      if (cfg.models?.length) {
        body.append(p('access-note', 'Öffentlich auswertbare Modelle (per modelId oder Name):'))
        const ul = document.createElement('ul')
        ul.className = 'access-public-list'
        for (const m of cfg.models) {
          const li = document.createElement('li')
          li.innerHTML = `<code>${escapeHtml(m)}</code>`
          ul.append(li)
        }
        body.append(ul)
      }
      if (!cfg.evaluate && !cfg.models?.length) {
        body.append(p('access-note', 'Keine öffentlichen Decisions — jede Auswertung braucht einen Key.'))
      }
      body.append(p('access-hint', 'Konfiguration beim Serverstart (-public-evaluate / -public-models); hier read-only.'))
    } catch (e) {
      body.append(p('access-err', (e as Error).message))
    }
  })()
  return panel
}

// --- tiny DOM helpers (kept local; the section is self-contained) ---

function div(cls: string): HTMLElement {
  const d = document.createElement('div')
  d.className = cls
  return d
}
function p(cls: string, text: string): HTMLElement {
  const n = document.createElement('p')
  n.className = cls
  n.textContent = text
  return n
}
function heading(text: string): HTMLElement {
  const h = document.createElement('div')
  h.className = 'access-heading'
  h.textContent = text
  return h
}
function button(cls: string, text: string): HTMLButtonElement {
  const b = document.createElement('button')
  b.type = 'button'
  b.className = cls
  b.textContent = text
  return b
}
function rowOf(...nodes: Node[]): HTMLElement {
  const r = div('access-actions')
  r.append(...nodes)
  return r
}
function alertRow(row: HTMLElement, msg: string): void {
  const e = p('access-err', msg)
  row.append(e)
}
