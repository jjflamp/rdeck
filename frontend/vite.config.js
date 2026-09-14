import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// Wails injects window.go / window.runtime bindings at runtime.
export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    strictPort: true,
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
})
