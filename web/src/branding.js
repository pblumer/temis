// Theme-/Branding-System (CI-Anpassung)
//
// Erlaubt es, die Oberfläche an die Corporate Identity (CI) einer Firma
// anzupassen — Farben, Logo, Produktname — ohne dmn-js oder dieses Frontend zu
// forken (vgl. ADR-0006, ADR-0012). Es gibt zwei Ebenen:
//
//   1. Eingebaute Themes (dunkel/hell), zwischen denen Nutzer umschalten können.
//      Die Wahl wird in localStorage gemerkt.
//   2. Optionales Deployment-Branding über `window.TEMIS_BRANDING`. Eine
//      ausgelieferte `public/branding.js` (oder ein Reverse-Proxy) kann ein
//      Firmen-Theme registrieren und Logo/Produktname setzen — ganz ohne Neubau
//      des Bundles.
//
// Themes sind reine Sammlungen von CSS-Custom-Properties; sie werden auf
// <html> gesetzt und überschreiben so die Defaults aus style.css. Die dmn-js-
// Zeichenfläche behält bewusst ihr eigenes (helles) Theme.

const STORAGE_KEY = 'temis.theme';

// Alle von Themes verwalteten Variablen. Beim Umschalten werden zuerst alle
// gelöscht und dann die des aktiven Themes gesetzt — so bleiben keine Reste
// eines vorher aktiven Themes hängen.
const MANAGED_VARS = [
  '--bg',
  '--panel',
  '--border',
  '--fg',
  '--muted',
  '--accent',
  '--accent-fg',
  '--ok',
  '--err',
  '--warn',
];

// Kanonisches Basis-Theme. Spiegelt die Defaults aus style.css :root wider, ist
// hier aber die maßgebliche Quelle: Themes mit `base` erben hiervon (bzw. von
// einem anderen Theme) und überschreiben nur, was abweicht.
const DARK = {
  '--bg': '#0f1115',
  '--panel': '#1a1d24',
  '--border': '#2b303b',
  '--fg': '#e6e9ef',
  '--muted': '#98a2b3',
  '--accent': '#5b8def',
  '--accent-fg': '#ffffff',
  '--ok': '#3fb950',
  '--err': '#f85149',
  '--warn': '#d29922',
};

// Eingebaute Themes. `vars` darf unvollständig sein und wird über das jeweilige
// `base` (Default: temis-dark) gelegt.
const BUILT_IN = {
  'temis-dark': { label: 'Temis Dunkel', vars: DARK },
  'temis-light': {
    label: 'Temis Hell',
    base: null, // eigenständiges, vollständiges Theme
    vars: {
      '--bg': '#ffffff',
      '--panel': '#f5f6f8',
      '--border': '#d8dde5',
      '--fg': '#1a1d24',
      '--muted': '#5b6470',
      '--accent': '#2f6fe0',
      '--accent-fg': '#ffffff',
      '--ok': '#1a7f37',
      '--err': '#cf222e',
      '--warn': '#9a6700',
    },
  },
};

// Laufzeit-Registry: id -> { label, vars (vollständig aufgelöst) }.
const registry = new Map();

function resolveVars(def, seen = new Set()) {
  // `base: undefined` -> auf temis-dark aufbauen; `base: null` -> eigenständig.
  const baseId = def.base === undefined ? 'temis-dark' : def.base;
  let base = {};
  if (baseId && !seen.has(baseId)) {
    seen.add(baseId);
    const parent = BUILT_IN[baseId];
    if (parent) base = resolveVars(parent, seen);
  }
  return { ...base, ...(def.vars || {}) };
}

function register(id, def) {
  registry.set(id, { label: def.label || id, vars: resolveVars(def) });
}

function readConfig() {
  return (typeof window !== 'undefined' && window.TEMIS_BRANDING) || {};
}

function readUrlTheme() {
  try {
    return new URLSearchParams(window.location.search).get('theme');
  } catch {
    return null;
  }
}

function applyTheme(id) {
  const theme = registry.get(id);
  if (!theme) return false;
  const root = document.documentElement;
  MANAGED_VARS.forEach((v) => root.style.removeProperty(v));
  Object.entries(theme.vars).forEach(([k, val]) => root.style.setProperty(k, val));
  root.dataset.theme = id;
  return true;
}

function storeChoice(id) {
  try {
    localStorage.setItem(STORAGE_KEY, id);
  } catch {
    /* localStorage kann blockiert sein (Privatmodus) — dann eben nicht merken. */
  }
}

function storedChoice() {
  try {
    return localStorage.getItem(STORAGE_KEY);
  } catch {
    return null;
  }
}

function applyBrand(cfg) {
  if (cfg.brand) {
    const title = document.getElementById('brandTitle');
    if (title) title.textContent = cfg.brand;
    if (cfg.brand) document.title = cfg.brand;
  }
  if (cfg.subtitle) {
    const sub = document.querySelector('header .sub');
    if (sub) sub.textContent = cfg.subtitle;
  }
  const logo = document.getElementById('brandLogo');
  if (logo && cfg.logo) {
    logo.src = cfg.logo;
    logo.alt = cfg.brand ? cfg.brand + ' Logo' : 'Logo';
    logo.hidden = false;
  }
}

function buildSwitcher(activeId, onChange) {
  const sel = document.getElementById('themeSelect');
  const wrap = document.getElementById('themeSwitch');
  if (!sel) return;
  sel.innerHTML = '';
  registry.forEach((theme, id) => {
    const o = document.createElement('option');
    o.value = id;
    o.textContent = theme.label;
    if (id === activeId) o.selected = true;
    sel.appendChild(o);
  });
  sel.addEventListener('change', () => onChange(sel.value));
  if (wrap) wrap.hidden = false;
}

// Initialisiert Themes + Branding. Reihenfolge der Theme-Auswahl:
//   ?theme= (URL) > gespeicherte Nutzerwahl > cfg.defaultTheme > Firmen-Theme >
//   temis-dark.
export function initBranding() {
  // Eingebaute Themes registrieren.
  Object.entries(BUILT_IN).forEach(([id, def]) => register(id, def));

  const cfg = readConfig();

  // Optionales Firmen-Theme aus dem Deployment-Branding registrieren.
  let companyId = null;
  if (cfg.theme && typeof cfg.theme === 'object') {
    companyId = cfg.theme.id || 'company';
    register(companyId, cfg.theme);
  }

  applyBrand(cfg);

  const candidates = [
    readUrlTheme(),
    storedChoice(),
    cfg.defaultTheme,
    companyId,
    'temis-dark',
  ];
  const active = candidates.find((id) => id && registry.has(id)) || 'temis-dark';
  applyTheme(active);

  // Nutzer-Umschalter, sofern das Deployment ihn nicht abschaltet.
  if (cfg.allowUserSwitch !== false) {
    buildSwitcher(active, (id) => {
      if (applyTheme(id)) storeChoice(id);
    });
  }

  return active;
}
