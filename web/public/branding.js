// Deployment-Branding (CI-Anpassung) — OPTIONAL.
//
// Diese Datei wird von index.html geladen, bevor das Frontend startet, und von
// Vite unverändert nach `dist/branding.js` kopiert. Standardmäßig tut sie
// nichts: dann gelten die eingebauten Temis-Themes.
//
// Zum Anpassen an die Corporate Identity einer Firma diese Datei beim Ausliefern
// überschreiben (kein Neubau des Bundles nötig) und `window.TEMIS_BRANDING`
// setzen. Alle Felder sind optional:
//
// window.TEMIS_BRANDING = {
//   brand: 'ACME AG',                 // Produktname in Kopfzeile + Tab-Titel
//   subtitle: 'Entscheidungs-Editor', // Untertitel in der Kopfzeile
//   logo: '/branding/acme-logo.svg',  // Logo links in der Kopfzeile
//
//   // Eigenes Firmen-Theme. `base` erbt von einem eingebauten Theme
//   // ('temis-dark' oder 'temis-light'); `vars` überschreibt nur Abweichendes.
//   theme: {
//     id: 'acme',
//     label: 'ACME',
//     base: 'temis-light',
//     vars: {
//       '--accent': '#e4002b',
//       '--accent-fg': '#ffffff',
//     },
//   },
//
//   defaultTheme: 'acme',     // initial aktives Theme (id)
//   allowUserSwitch: true,    // false blendet den Theme-Umschalter aus
// };
