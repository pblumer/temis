import { defineConfig } from 'vite'

// The app is served by temisd at the site root (go:embed; ADR-0016 WP-67 cutover
// replaced the legacy dmn-js /ui). A relative base makes the bundled assets
// resolve from any mount point (root, or the /app/ redirect). No CDN — everything
// is embedded and offline.
export default defineConfig({
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Deterministic-ish, small output; the built dist/ is committed and embedded.
    assetsDir: 'assets',
  },
})
