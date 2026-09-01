import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// El build sale a dist/, que el binario de Go empotra con go:embed.
// En desarrollo, /api y /media se redirigen al servidor Go para que la
// recarga en caliente funcione contra datos reales.
export default defineConfig({
  plugins: [vue()],
  build: { outDir: 'dist', emptyOutDir: true },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://localhost:8787', changeOrigin: true },
      '/media': { target: 'http://localhost:8787', changeOrigin: true },
    },
  },
})
