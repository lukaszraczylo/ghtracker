---
name: upgrading-perfect-design
description: Use when bumping @perfect-design/ui in ghtracker, when a Renovate or autoupdate pull request changes its version, when a needed Perfect Design component exists only in a newer release, or when pnpm refuses a fresh PD release because of minimumReleaseAge.
---

# Perfect Design upgrade

`package.json` pins `@perfect-design/ui` to an exact version. A new PD version can move module paths, which breaks the Tailwind `@source` list, and it can add colour tokens that ghtracker does not set. Check both before you merge the upgrade.

## Steps

1. Save the current theme for a later comparison:
   ```sh
   cp node_modules/@perfect-design/ui/src/styles/theme.css "$TMPDIR/pd-theme-old.css"
   ```
2. Read the release notes for every version between the current one and the target:
   ```sh
   gh release list -R lukaszraczylo/perfect-design --limit 10
   gh release view v<version> -R lukaszraczylo/perfect-design
   ```
   Write down each removed or renamed component, prop or variant that ghtracker uses.
3. Install the exact version:
   ```sh
   pnpm add @perfect-design/ui@<version>
   ```
   Keep the version exact, without `^`.
4. Check `pnpm-workspace.yaml`. PD is the user's own package, so its `minimumReleaseAgeExclude` entry is intended and stays. Keep one entry for `@perfect-design/ui`, with the new version. Remove an entry for an old version.
5. Find `@source` lines whose path no longer exists:
   ```sh
   grep -o "@perfect-design/ui/dist/[^']*" src/style.css | while read -r p; do [ -e "node_modules/$p" ] || echo "gone: $p"; done
   ```
   Delete or correct each line that this prints.
6. Run `pnpm build`. If the guard prints `Add @source lines in src/style.css for PD modules:`, add the listed paths. A new PD version can split a component into new helper files.
7. Compare the theme tokens:
   ```sh
   diff <(grep -oE '^\s*--[a-z0-9-]+' "$TMPDIR/pd-theme-old.css" | sort -u) \
        <(grep -oE '^\s*--[a-z0-9-]+' node_modules/@perfect-design/ui/src/styles/theme.css | sort -u)
   ```
   If PD adds a colour token, decide with the user whether ghtracker sets its own value. If yes, add it to both `:root` and `.dark` in `src/style.css`.
8. Fix each break from step 2. Use the `developing-ui-components` skill.
9. Run all gates of the `validating-ui-components` skill, including the browser check.

## Commit

- A version bump alone is `chore(deps): update @perfect-design/ui to v<version>`. This gives a patch release.
- A bump that adds a new component to the page is `feat(ui): ...`. This gives a minor release.
- `semver.yaml` uses regex matching. Do not write a `Semver-Major:` line unless the user asks for a major release.
