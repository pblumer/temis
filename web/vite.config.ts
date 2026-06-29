import { defineConfig } from 'vite'

// The app is served by temisd under /app/ (go:embed, WP-60), so assets must be
// referenced relative to that base. No CDN — everything is bundled and embedded.
export default defineConfig({
  base: '/app/',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    // Deterministic-ish, small output; the built dist/ is committed and embedded.
    assetsDir: 'assets',
  },
})
