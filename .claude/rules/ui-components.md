---
paths:
  - 'src/**/*.vue'
  - 'src/components/**/*.ts'
  - 'src/main.ts'
  - 'src/style.css'
  - 'vite.config.ts'
  - 'package.json'
  - 'pnpm-workspace.yaml'
---

# UI components

The ghtracker page uses Perfect Design (PD), the `@perfect-design/ui` package. PD is the user's own library. These rules keep the page on PD.

## Build from PD

- Build every UI element from a PD component. Look up PD before you write markup. Use the `developing-ui-components` skill for the lookup.
- Do not add a local primitive (button, badge, card, dialog, progress bar, tooltip, toggle, input) when PD has one. Do not add `src/components/ui/`.
- Do not write a wrapper component. A ghtracker component must own domain logic: tracker data, state or the layout of several PD parts. A component that only passes props to one PD component is a wrapper.
- Import components from `@perfect-design/ui`. Do not import from `@perfect-design/ui/dist/...`. The one exception is `@perfect-design/ui/stores/theme-boot` in `vite.config.ts`.
- Do not add a UI helper package (reka-ui, class-variance-authority, clsx, tailwind-merge, @vueuse/core, an icon package). PD bundles what it needs.

## Style with tokens

- Style a PD component through its props first, then through `class`.
- Use the semantic token classes only: `bg-destructive`, `text-warning`, `bg-success/30`, `text-muted-foreground`, `border-border`. Do not write a hex value, `rgb()` or a Tailwind palette colour (`red-500`) in a `.vue` file.
- Colour values live only in `src/style.css`, in `:root` (light) and `.dark` (dark). A new colour token needs a value in both blocks.
- The tokens change with the `.dark` class, so a token class is already correct in both modes. Use a `dark:` class only when one mode needs a different token, for example `dark:text-muted-foreground`. Never put a raw colour in a `dark:` class.
- PD sets `text-base` to 14px. Pick text sizes from the PD scale. Do not compensate with arbitrary sizes.
- Icons are `Icon` or `IconButton` with data from `@fortawesome/free-solid-svg-icons`.

## Keep the Tailwind scan complete

- `src/style.css` lists each PD module that the bundle uses as an `@source` line. Tailwind scans only those modules.
- When you use a PD component for the first time, add its `@source` line. `pnpm build` fails with `Add @source lines in src/style.css for PD modules:` and the missing paths if you forget. The `pd-source-guard` plugin in `vite.config.ts` does this check.
- The pull request CI runs `pnpm build`. Run it yourself before you push, so that a missing line does not cost a CI run.
- When you stop using a PD component, remove its `@source` line. Each extra line adds CSS.

## Respect the PD contract

- `src/main.ts` sets `configureStrictMode` to `throw` in development and `warn` in production. Tests do not load `main.ts`, so they run with the PD default, `throw`.
- A `ContractError` names the missing part or attribute. Add it. Do not set strict mode to `off` or `warn` to hide the error, and do not catch the error.
- An icon-only button needs a label. An `AlertDialog` needs `AlertDialogTitle` and `AlertDialogDescription`. A state that colour shows also needs text or an `aria-label`.

## Test hooks

- Select elements in tests by `data-test` (ghtracker hooks), `data-slot` (PD parts), `role` or `aria-*`. Do not select by a Tailwind class or by PD markup structure.
- Before you commit a UI change, run the `validating-ui-components` skill.
