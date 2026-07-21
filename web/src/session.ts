// session.ts — bearer-based auth for the modeler (WP-107, ADR-0028/0035).
//
// The credential is treated as an OPAQUE bearer string on purpose: today it is a
// kid.secret API key the admin pastes, but the exact same handling carries an
// OIDC access token unchanged once a Keycloak authenticator lands (ADR-0036) —
// only this module changes, no caller. A single fetch wrapper is the one seam
// that attaches the credential, and whoami() (issuer-agnostic: subject + scopes)
// is the one place the UI learns who the caller is.

const BEARER_KEY = 'temis.bearer'

// getBearer returns the stored session credential, or null when none is set.
// sessionStorage (not localStorage) so the credential dies with the tab.
export function getBearer(): string | null {
  try {
    return sessionStorage.getItem(BEARER_KEY)
  } catch {
    return null
  }
}

export function setBearer(v: string): void {
  try {
    sessionStorage.setItem(BEARER_KEY, v.trim())
  } catch {
    /* storage unavailable — the credential simply won't persist */
  }
}

export function clearBearer(): void {
  try {
    sessionStorage.removeItem(BEARER_KEY)
  } catch {
    /* ignore */
  }
}

// installFetchAuth wraps window.fetch once so every same-origin API request
// carries the stored bearer. It is the single place the credential is attached —
// the one seam a future OIDC flow needs to touch. A request that already sets an
// Authorization header is left as-is (explicit override wins).
let installed = false
export function installFetchAuth(): void {
  if (installed) return
  installed = true
  const orig = window.fetch.bind(window)
  window.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const bearer = getBearer()
    if (!bearer) return orig(input, init)
    const url = typeof input === 'string' ? input : input instanceof URL ? input.href : input.url
    if (!isApiPath(url)) return orig(input, init)
    const headers = new Headers(init?.headers ?? (input instanceof Request ? input.headers : undefined))
    if (!headers.has('Authorization')) headers.set('Authorization', 'Bearer ' + bearer)
    return orig(input, { ...init, headers })
  }
}

// isApiPath reports whether url targets a temis API surface the bearer guards
// (/v1, /mcp). It matches same-origin absolute paths and full URLs on this origin;
// cross-origin URLs are never touched, so a BYOK call to another host is safe.
function isApiPath(url: string): boolean {
  let path = url
  if (/^https?:\/\//i.test(url)) {
    try {
      const u = new URL(url)
      if (u.origin !== window.location.origin) return false
      path = u.pathname
    } catch {
      return false
    }
  }
  return path.startsWith('/v1/') || path === '/v1' || path.startsWith('/mcp')
}

// AccessIdentity mirrors service.AccessIdentity (issuer-agnostic): subject is a
// kid today, an OIDC subject later; scopes/isAdmin drive what the UI reveals.
export type AccessIdentity = {
  authEnabled: boolean
  authenticated: boolean
  subject?: string
  scopes?: string[]
  isAdmin: boolean
}

// whoami asks the server who the current credential is. It never 401s (the
// endpoint is public and self-inspecting), so the caller can branch on
// authenticated/isAdmin to prompt for a key or reveal the admin section.
export async function whoami(): Promise<AccessIdentity> {
  const r = await fetch('/v1/whoami')
  if (!r.ok) throw new Error('whoami: ' + r.status)
  return (await r.json()) as AccessIdentity
}

// KeyView mirrors service.KeyView: a secret-free snapshot for the listing.
export type KeyView = {
  kid: string
  scopes: string[]
  owner?: string
  expiresAt?: string
  revoked: boolean
  managed: boolean
}

// CreatedKey mirrors service.createdKeyResponse: the one-time secret, shown once.
export type CreatedKey = {
  kid: string
  secret: string
  bearer: string
  scopes?: string[]
  owner?: string
  expiresAt?: string
}

// PublicConfig mirrors service.AccessPublicConfig (ADR-0035): the global evaluate
// switch (startup config), the immutable static allowlist (-public-models) and the
// runtime-toggleable managed set. persistent = managed toggles survive a restart.
export type PublicConfig = { evaluate: boolean; static: string[]; managed: string[]; persistent: boolean }

// keyMgmtDisabled is thrown when the lifecycle API is dormant (no -keys-dir): the
// server answers 404 and the UI shows a hint rather than an error.
export class KeyMgmtDisabled extends Error {
  constructor() {
    super('key management disabled')
    this.name = 'KeyMgmtDisabled'
  }
}

async function keysJSON<T>(r: Response): Promise<T> {
  if (r.status === 404) throw new KeyMgmtDisabled()
  if (!r.ok) throw new Error((await r.text()) || 'request failed: ' + r.status)
  return (await r.json()) as T
}

export async function listKeys(): Promise<KeyView[]> {
  const data = await keysJSON<{ keys: KeyView[] }>(await fetch('/v1/keys'))
  return data.keys ?? []
}

export async function createKey(req: { scopes: string[]; owner?: string; expiresAt?: string }): Promise<CreatedKey> {
  return keysJSON<CreatedKey>(
    await fetch('/v1/keys', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(req),
    }),
  )
}

export async function rotateKey(kid: string): Promise<CreatedKey> {
  return keysJSON<CreatedKey>(await fetch('/v1/keys/' + encodeURIComponent(kid) + '/rotate', { method: 'POST' }))
}

export async function revokeKey(kid: string): Promise<void> {
  await keysJSON<unknown>(await fetch('/v1/keys/' + encodeURIComponent(kid) + '/revoke', { method: 'POST' }))
}

export async function getPublicConfig(): Promise<PublicConfig> {
  const r = await fetch('/v1/access/public')
  if (!r.ok) throw new Error('public config: ' + r.status)
  return (await r.json()) as PublicConfig
}

// setPublicModel opens (public=true) or closes (false) one model's anonymous
// evaluation at runtime and returns the updated configuration. The model is a
// modelId or a display name. Removing a static entry is a 409 (deployment config).
export async function setPublicModel(model: string, isPublic: boolean): Promise<PublicConfig> {
  const r = await fetch('/v1/access/public/models', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ model, public: isPublic }),
  })
  if (!r.ok) throw new Error((await r.text()) || 'toggle failed: ' + r.status)
  const cfg = (await r.json()) as PublicConfig
  // Broadcast so every public-decision view (the toolbar toggle and the Zugriff
  // panel) stays in sync without polling — whichever widget made the change, the
  // others repaint from the fresh config.
  document.dispatchEvent(new CustomEvent<PublicConfig>('temis:public-changed', { detail: cfg }))
  return cfg
}

// PUBLIC_CHANGED is the event name carrying a fresh PublicConfig after any toggle.
export const PUBLIC_CHANGED = 'temis:public-changed'

// SCOPES is the closed scope vocabulary (service.knownScopes, ADR-0028 §2), for
// the create-key form's checkboxes.
export const SCOPES: { scope: string; label: string }[] = [
  { scope: 'evaluate', label: 'Auswerten (evaluate)' },
  { scope: 'models:read', label: 'Modelle lesen (models:read)' },
  { scope: 'models:write', label: 'Modelle schreiben (models:write)' },
  { scope: 'flow', label: 'Flows (flow)' },
  { scope: 'git', label: 'Git (git)' },
  { scope: 'assist', label: 'Assistent (assist)' },
  { scope: 'audit', label: 'Audit-Log lesen (audit)' },
  { scope: 'admin', label: 'Admin — Key-Verwaltung, Löschen (admin)' },
]
