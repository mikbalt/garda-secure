import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import path from 'path'

/**
 * Same Content-Security-Policy as nginx-security-headers.conf (WEB-2), used
 * by `vite preview` so the e2e smoke tests catch inline scripts, eval or
 * foreign origins. (The MinIO img-src origin is not needed in demo mode.)
 */
const PREVIEW_CSP =
  "default-src 'self'; img-src 'self' data: blob:; connect-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; frame-src 'self'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'; object-src 'none'"

export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          react: ['react', 'react-dom', 'react-router', 'react-router-dom', '@tanstack/react-query'],
          charts: ['recharts'],
          vendor: ['axios', 'keycloak-js', 'lucide-react', 'qrcode'],
        },
      },
    },
  },
  preview: {
    headers: { 'Content-Security-Policy': PREVIEW_CSP },
  },
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost:8080',
        changeOrigin: true,
      },
      '/auth': {
        target: 'http://localhost:8081',
        changeOrigin: true,
      },
    },
  },
})
