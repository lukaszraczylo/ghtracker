// Empties dist/ but keeps .gitkeep, which lets `go build` embed dist/ before the first UI build.
import { mkdirSync, readdirSync, rmSync, writeFileSync } from 'node:fs'

mkdirSync('dist', { recursive: true })
for (const name of readdirSync('dist')) {
  if (name !== '.gitkeep') rmSync(`dist/${name}`, { recursive: true, force: true })
}
writeFileSync('dist/.gitkeep', '')
