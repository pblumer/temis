import { defineConfig } from 'vite';

// The editor frontend (dev server on :5173) talks to a locally running temisd
// (default :8080). Proxying /v1, /healthz and /readyz keeps requests same-origin,
// so no CORS handling is needed on the Go side. Override the target with
// TEMIS_API when temisd listens elsewhere.
const apiTarget = process.env.TEMIS_API || 'http://localhost:8080';

export default defineConfig({
  server: {
    proxy: {
      '/v1': { target: apiTarget, changeOrigin: true },
      '/healthz': { target: apiTarget, changeOrigin: true },
      '/readyz': { target: apiTarget, changeOrigin: true },
    },
  },
});
