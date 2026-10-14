import { defineConfig } from 'vite-plus'
import vue from '@vitejs/plugin-vue'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  base: './',
  build: {
    outDir: 'dist',
    emptyOutDir: true,
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/api': 'http://127.0.0.1:8181',
      '/videos': 'http://127.0.0.1:8181',
      '/live': 'http://127.0.0.1:8181',
    },
  },
})
