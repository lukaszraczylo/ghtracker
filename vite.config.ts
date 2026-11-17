import { readFileSync } from 'node:fs'
import { fileURLToPath, URL } from 'node:url'
import { themeBootScript } from '@perfect-design/ui/stores/theme-boot'
import tailwindcss from '@tailwindcss/vite'
import vue from '@vitejs/plugin-vue'
import type { Plugin } from 'vite'
import { defineConfig } from 'vitest/config'

// Applies the saved light/dark mode before first paint so a dark-mode visitor never sees a light flash.
const themeBoot: Plugin = {
  name: 'pd-theme-boot',
  transformIndexHtml: (html) =>
    html.replace('</head>', `<script>${themeBootScript}</script></head>`),
}

const PD_DIST = '@perfect-design/ui/dist/'

// Tailwind only scans the PD modules listed in style.css, so fail the build when the bundle uses one it does not scan.
const pdSourceGuard: Plugin = {
  name: 'pd-source-guard',
  apply: 'build',
  generateBundle(_, bundle) {
    const css = readFileSync(fileURLToPath(new URL('./src/style.css', import.meta.url)), 'utf8')
    const listed = [...css.matchAll(/@source '[^']*@perfect-design\/ui\/dist\/([^']+)';/g)].map(
      (m) => m[1],
    )
    const missing = new Set<string>()
    for (const chunk of Object.values(bundle)) {
      if (chunk.type !== 'chunk') continue
      for (const id of Object.keys(chunk.modules)) {
        const at = id.lastIndexOf(PD_DIST)
        if (at < 0) continue
        const rel = id.slice(at + PD_DIST.length)
        if (!listed.some((p) => rel === p || rel.startsWith(`${p}/`))) missing.add(rel)
      }
    }
    if (missing.size > 0)
      this.error(`Add @source lines in src/style.css for PD modules:\n${[...missing].join('\n')}`)
  },
}

export default defineConfig({
  plugins: [vue(), tailwindcss(), themeBoot, pdSourceGuard],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  build: { outDir: 'dist', emptyOutDir: false },
  server: { proxy: { '/api': 'http://127.0.0.1:8080', '/refresh': 'http://127.0.0.1:8080' } },
  test: { environment: 'happy-dom', include: ['src/**/*.test.ts'] },
})
