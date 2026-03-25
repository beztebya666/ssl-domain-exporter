import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'
import { readFileSync } from 'node:fs'

const pkg = JSON.parse(readFileSync(new URL('./package.json', import.meta.url), 'utf-8')) as { version: string }
const uiVersion = `v${pkg.version}`
const uiBuildID = process.env.UI_BUILD_ID || new Date().toISOString()

export default defineConfig({
  plugins: [react()],
  define: {
    __UI_VERSION__: JSON.stringify(uiVersion),
    __UI_BUILD_ID__: JSON.stringify(uiBuildID),
  },
  server: {
    host: '0.0.0.0',
    port: 5174,
    proxy: {
      '/api': 'http://localhost:8080',
      '/metrics': 'http://localhost:8080',
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: false,
    chunkSizeWarningLimit: 600,
    rollupOptions: {
      output: {
        manualChunks: {
          'react-vendor': ['react', 'react-dom', 'react-router-dom'],
          'ui': ['lucide-react', 'clsx', 'date-fns'],
          'query': ['@tanstack/react-query', 'axios'],
        },
      },
    },
  },
})
