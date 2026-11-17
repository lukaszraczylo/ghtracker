---
name: developing-ui-components
description: Use when adding, changing or removing a ghtracker Vue component in src/components, when choosing which Perfect Design (@perfect-design/ui) component fits a UI need, or when a component throws a PD ContractError, fails vue-tsc on a variant or size value, or renders without styles.
---

# UI component development

Every ghtracker component is a composition of Perfect Design (PD) components with tracker logic. The work is mostly a lookup: find the PD part, read its installed API, then wire the data. The rules in `.claude/rules/ui-components.md` apply to all of it.

## Current components

| File                | Job                                                                   |
| ------------------- | --------------------------------------------------------------------- |
| `App.vue`           | Page shell, error and notice messages, screen-reader announcements    |
| `AppHeader.vue`     | PD `AppHeader` with brand, scan status, rescan button, `ThemeToggle`  |
| `LoadingScreen.vue` | First-load progress and skeleton panels                               |
| `FleetOverview.vue` | Headline, legend and the five fact tiles (`StatGroup` + `KpiTile`)    |
| `FleetStrip.vue`    | One segment per repo, coloured by health, with a tooltip              |
| `FilterBar.vue`     | Search (`SearchInput`, `/` shortcut), severity and kind toggles       |
| `AttentionList.vue` | Attention section heading, repo groups, empty state                   |
| `RepoGroup.vue`     | One repo `Card` with release badge, refresh button and rows           |
| `AttentionRow.vue`  | One alert row: severity icon, subject link, PR age meter (`Progress`) |
| `RowActions.vue`    | Configured actions per row with an `AlertDialog` confirmation         |
| `ProjectList.vue`   | All repos with a `StatusDot`, links and a refresh button              |

Change the component that owns the job. Add a component only for a new job.

## Find the PD component

1. Search the index by the job, not by a component name that you guess. Each line of https://perfect-design.raczylo.com/llms.txt names one component and its job:
   ```sh
   curl -s https://perfect-design.raczylo.com/llms.txt | grep -iE 'collapse|show more|expand'
   ```
   https://perfect-design.raczylo.com/llms-full.txt has props, slots and examples for all components.
2. Search the installed type comments for the same words. This also finds components that the website does not have yet:
   ```sh
   grep -rliE 'collapse|show more' node_modules/@perfect-design/ui/dist/components --include='*.d.ts'
   ```
3. Read the installed API in the component directory, for example `dist/components/primitives/ShowMore/`:
   - `<Name>.vue.d.ts`: props, slots and emits. The JSDoc comments give limits that the types do not, for example a numeric range.
   - `variants.d.ts`: the allowed `variant`, `size` and `tone` values.
   - `index.d.ts`: the exported parts, for example `CardHeader` and `CardTitle`.
4. If the website and the installed files disagree, the installed files are correct. The website shows the latest PD release, but the page runs the version pinned in `package.json`.

If no PD component fits, stop and ask the user. Do not build a local primitive. The user can add the component to PD.

## Write the component

- Use `<script setup lang="ts">` with typed `defineProps` and `defineEmits`.
- Read tracker data from the `tracker` store (`src/stores/tracker.ts`). Put pure logic in `src/lib/` with its own test.
- Use the PD prop for a state before you add a class: `tone`, `variant`, `size`, `loading`, `disabled`.
- Use `as` or `as-child` to change the rendered element, for example `CardHeader as="header"` or `Badge as-child` around a link.
- Add a `data-test` attribute to each element that a test or the user needs to find.

## Add the @source lines

1. Run `pnpm build`.
2. If the guard prints `Add @source lines in src/style.css for PD modules:`, add one line per directory that it lists:
   ```css
   @source '../node_modules/@perfect-design/ui/dist/components/data-display/KpiTile';
   ```
   A helper file outside a component directory gets its own line, for example `components/feedback/tones.js`.
3. Run `pnpm build` again until it passes.

When you remove the last use of a PD component, delete its line and run `pnpm build`. If the guard fails, the bundle still uses the module, so put the line back.

## Test the component

Add tests to `src/components/components.test.ts`, or to a `<Name>.test.ts` next to the component when the component has many cases.

```ts
import { mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

describe('AppHeader', () => {
  // ThemeToggle and the tracker store need an active Pinia.
  beforeEach(() => setActivePinia(createPinia()))

  it('shows the scan status', () => {
    const w = mount(AppHeader, { attachTo: document.body })
    expect(w.find('[data-test=updated]').text()).toContain('Scanned')
  })
})
```

- Build data with `repo()`, `alert()` and `state()` from `src/test/fixtures.ts`.
- Use `attachTo: document.body` for overlays (`AlertDialog`, `Tooltip`). These components render outside the component root.
- Assert on what the user gets: text, `role`, `aria-label`, `aria-valuenow`, `data-slot`. A PD `Progress` exposes its value on `[role=progressbar]`. A PD loading state renders `[data-slot=spinner]`.
- The tests run in happy-dom, which has no layout. `scrollHeight`, `clientWidth` and overflow are always 0. Test behaviour that depends on layout in the browser check of `validating-ui-components`. In the unit test, assert the props and the markup only.

## Fix common errors

| Error or symptom                                                          | Cause                                                                | Fix                                                                                                             |
| ------------------------------------------------------------------------- | -------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `[perfect-design] Avatar: requires a \`name\` or an \`alt\``              | `Avatar` is for a person or an entity with an image.                 | Use `Icon` for a repo or a status mark. Pass `name` if you need an `Avatar`.                                    |
| `ContractError` that names `AlertDialogTitle` or `AlertDialogDescription` | The dialog is missing a required part.                               | Add both parts.                                                                                                 |
| `getActivePinia()` was called but there was no active Pinia               | A test mounts `ThemeToggle` or uses the tracker store without Pinia. | Add `beforeEach(() => setActivePinia(createPinia()))`.                                                          |
| vue-tsc: a type is not assignable on `size` or `variant`                  | The value is not in `variants.d.ts`.                                 | Use an allowed value. For example, `KpiTile` sizes are `lg`, `default` and `compact`.                           |
| A component renders without its styles in `pnpm build` output             | Its PD module has no `@source` line.                                 | Run `pnpm build` and add the lines that the guard lists.                                                        |
| The page scrolls sideways at 390px                                        | PD `AppHeader` actions do not wrap.                                  | Move secondary text out of `#actions` on small screens. `AppHeader.vue` shows the status line below the header. |
| A test cannot find text that the page shows                               | The text moved into an `aria-label` or an overlay.                   | Assert on the `aria-label`, or mount with `attachTo: document.body`.                                            |

## Finish

Run the `validating-ui-components` skill before you commit. A UI change is not complete until every gate passes.
