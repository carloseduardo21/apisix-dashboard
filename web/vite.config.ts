import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

export default defineConfig({
  plugins: [react()],
  build: {
    outDir: '../internal/web/dist',
    emptyOutDir: true,
    sourcemap: false,
    chunkSizeWarningLimit: 1000,
  },
  server: {
    // `npm run dev` proxies the API to a locally running backend.
    proxy: { '/api': 'http://localhost:8080' },
  },
})
