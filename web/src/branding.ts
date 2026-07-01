// Theming/Branding (CI-Anpassung), siehe ADR-0018.
//
// Erlaubt es, die Oberfläche ohne Neubau an die Corporate Identity einer Firma
// anzupassen: Produktname, Logo und Akzentfarbe. Ein Deployment setzt dazu vor
// dem Laden des Bundles ein globales `window.TEMIS_BRANDING` (z. B. per
// Reverse-Proxy injiziert). Ist es nicht gesetzt, gilt das Temis-Standarddesign.
//
// Die Akzentfarbe ist bereits eine CSS-Variable (--accent); Branding überschreibt
// sie (plus --accent-fg für Text auf der Akzentfläche) auf <html>. Ein vollständiger
// Hell/Dunkel-Umschalter folgt separat (er braucht einen CSS-Refactor der Flächen).

export interface Branding {
  /** Produktname in Seitentitel und Seitenleiste (Default: „Temis"). */
  brand?: string
  /** Logo als URL oder Data-URI; ersetzt das Standard-Rautenlogo. */
  logo?: string
  /** CI-Akzentfarbe, z. B. '#e4002b'. */
  accent?: string
  /** Textfarbe auf der Akzentfläche (Default: #ffffff). */
  accentFg?: string
  /** Fortgeschritten: beliebige CSS-Custom-Properties überschreiben. */
  vars?: Record<string, string>
}

// Standard-Rautenlogo (Entscheidungssymbol in DMN/Flowcharts + Häkchen). Erbt
// über currentColor die Akzentfarbe des aktiven Themes.
const DEFAULT_LOGO_SVG =
  '<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor"' +
  ' stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">' +
  '<path d="M12 3 21 12 12 21 3 12Z"/><path d="M8.4 12.2 11 14.8 15.6 9.6"/></svg>'

function config(): Branding {
  return (typeof window !== 'undefined' && (window as unknown as { TEMIS_BRANDING?: Branding }).TEMIS_BRANDING) || {}
}

/** The configured product name, or „Temis" by default. */
export function brandName(): string {
  return config().brand || 'Temis'
}

/**
 * Inline markup for the header logo: a company logo `<img>` when configured,
 * otherwise the theme-colored default diamond mark.
 */
export function brandLogoHTML(): string {
  const cfg = config()
  if (cfg.logo) {
    const alt = (cfg.brand || 'Logo').replace(/"/g, '&quot;')
    return `<img class="brand-logo-img" src="${cfg.logo}" alt="${alt}">`
  }
  return DEFAULT_LOGO_SVG
}

/**
 * Apply the deployment branding: accent color, product name and any custom CSS
 * variables. Safe to call once at startup before the app renders.
 */
export function initBranding(): void {
  const cfg = config()
  const root = document.documentElement
  if (cfg.accent) root.style.setProperty('--accent', cfg.accent)
  if (cfg.accentFg) root.style.setProperty('--accent-fg', cfg.accentFg)
  if (cfg.vars) {
    for (const [k, v] of Object.entries(cfg.vars)) root.style.setProperty(k, v)
  }
  if (cfg.brand) document.title = cfg.brand + ' — DMN Modeler'
}
