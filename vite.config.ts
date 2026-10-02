import { fileURLToPath, URL } from 'node:url'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import { defineConfig } from 'vitest/config'

export default defineConfig({
  plugins: [vue(), tailwindcss()],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: 'dist', emptyOutDir: false },
  server: { proxy: { '/api': 'http://127.0.0.1:8080', '/refresh': 'http://127.0.0.1:8080' } },
  test: { environment: 'happy-dom', include: ['src/**/*.test.ts'] },
})
