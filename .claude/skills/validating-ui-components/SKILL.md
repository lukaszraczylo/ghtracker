---
name: validating-ui-components
description: Use when a change touches src/components, src/App.vue, src/main.ts, src/style.css, vite.config.ts or the @perfect-design/ui version, before you commit, push or say that a UI change is done. Also use to check the page in a browser for light and dark mode, phone width, console errors or sideways scroll.
---

# UI component validation

A UI change is complete only when all six gates pass. The pull request CI (`.github/workflows/ci.yaml`, job `ui`) runs gates 1 to 3 only. It does not run the build with the PD source guard, the Prettier check or the browser check, so you must run them.

## Gates

Run the gates in this order. Stop at the first failure, fix it, then start again at gate 1.

| #   | Command                          | Pass condition                                                                      |
| --- | -------------------------------- | ----------------------------------------------------------------------------------- |
| 1   | `pnpm lint`                      | No ESLint errors                                                                    |
| 2   | `pnpm typecheck`                 | No vue-tsc errors                                                                   |
| 3   | `pnpm test`                      | All vitest tests pass                                                               |
| 4   | `pnpm build`                     | Ends with `built in`. No `Add @source lines in src/style.css for PD modules:` error |
| 5   | `pnpm exec prettier --check src` | `All matched files use Prettier code style!`                                        |
| 6   | Browser check (below)            | Every item in the checklist passes                                                  |

A PD `ContractError` in gate 3 means a required part is missing. Fix it with the `developing-ui-components` skill. Do not change the strict mode.

## Compare the bundle size

Gate 4 prints the CSS and JS sizes. Before you change code, run the same command once to get the baseline:

```sh
pnpm build 2>&1 | grep -E 'index-.*\.(css|js)'
```

If you did not record a baseline, build `main` in a throwaway worktree outside the repo. The command removes the worktree when it ends:

```sh
B="$TMPDIR/ghtracker-size-base"; git worktree add -q --detach "$B" main && (cd "$B" && pnpm install --frozen-lockfile >/dev/null && pnpm build 2>&1 | grep -E 'index-.*\.(css|js)'); git worktree remove --force "$B"
```

Report the difference. If the CSS grows by more than 10 kB for one new component, check `src/style.css` for an `@source` line that covers a whole group directory instead of one component.

## Check the page in a browser

The mock server serves `dist/` with fixed tracker data: 28 repos, every health state, and a configured row action. It needs no GitHub token.

1. Start the server in the background:
   ```sh
   node .claude/skills/validating-ui-components/mock-server.mjs dist 5288
   ```
   If port 5288 is in use, pick another port. Do not stop a process that you did not start.
2. Open `http://127.0.0.1:5288` with `mcp__plugin_playwright_playwright__browser_navigate`.
3. Check each of these four views:
   - 1440 × 900, light mode.
   - 1440 × 900, dark mode.
   - 390 × 844, light mode.
   - 390 × 844, dark mode.

   Use `mcp__plugin_playwright_playwright__browser_resize` to set the size. To change the mode, click the header button whose label starts with `Theme:`. It cycles through dark, system and light. Check `document.documentElement.classList.contains('dark')`, because the browser can start in either mode.

4. In each view, run this check with `mcp__plugin_playwright_playwright__browser_evaluate`:

   <!-- prettier-ignore -->
   ```js
   () => {
     const lum = (c) => {
       const f = (v) => ((v /= 255) <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4)
       const [r, g, b] = c.match(/[\d.]+/g).map(Number)
       return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b)
     }
     const bgOf = (el) => {
       for (let e = el; e; e = e.parentElement) {
         const a = getComputedStyle(e).backgroundColor.match(/[\d.]+/g)
         if (a && (a.length < 4 || +a[3] > 0.5)) return getComputedStyle(e).backgroundColor
       }
       return 'rgb(255, 255, 255)'
     }
     const lowContrast = []
     for (const el of document.querySelectorAll('body *')) {
       const text = [...el.childNodes].some((n) => n.nodeType === 3 && n.textContent.trim())
       const s = getComputedStyle(el)
       if (!text || !el.getClientRects().length || !s.color.startsWith('rgb')) continue
       const bg = bgOf(el)
       if (!bg.startsWith('rgb')) continue
       const [hi, lo] = [lum(s.color), lum(bg)].sort((x, y) => y - x)
       const ratio = (hi + 0.05) / (lo + 0.05)
       if (ratio < 3)
         lowContrast.push({ text: el.textContent.trim().slice(0, 30), ratio: +ratio.toFixed(2) })
     }
     return { sideways: document.documentElement.scrollWidth - window.innerWidth, lowContrast }
   }
   ```

   - `sideways` must be `0` or less. A vertical scrollbar makes it negative.
   - `lowContrast` must be empty. It lists text with a contrast ratio below 3:1 against the nearest opaque background. A PD tone class with a ghtracker `-foreground` token that does not fit can cause this. For example, `text-warning-foreground` on the page background.

5. Read `mcp__plugin_playwright_playwright__browser_console_messages`. The production build sets PD strict mode to `warn`, so a broken PD contract shows as a `[perfect-design]` console error. There must be no errors and no warnings.
6. Take a screenshot of each view with `mcp__plugin_playwright_playwright__browser_take_screenshot` and look at it. Check that colours follow the health state: red for critical, amber for warning, green for OK.
7. If the change touches a row action, click the action button on a critical row. The confirmation dialog must open with a title and a description. Click Cancel to close it.
8. Clean up:
   - Close the browser with `mcp__plugin_playwright_playwright__browser_close`.
   - Stop the mock server.
   - Delete the `.playwright-mcp/` directory. Playwright saves screenshots there because it does not write outside the repo.

## When the mock data is out of date

`mock-server.mjs` builds a `TrackerState` object. If `src/lib/types.ts` changes that type, update the mock in the same change. Otherwise the browser check tests a page that production never shows.

## Report

End with one line per gate, the bundle size difference and the four browser views:

```
lint ok · typecheck ok · test 126/126 · build ok · prettier ok
CSS 129.48 kB -> 131.02 kB (+1.54) · JS 391.53 kB -> 391.60 kB (+0.07)
browser: 1440 light ok · 1440 dark ok · 390 light ok · 390 dark ok · console clean · no sideways scroll
```

If you skipped a gate, say which gate and why. Do not report a skipped gate as passed.
