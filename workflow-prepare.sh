#!/usr/bin/env bash
# Run by the shared release workflow before the tests, the version calculation and
# GoReleaser. The binary embeds dist/, so the UI must be built first.
set -euo pipefail

PNPM_VERSION=11.21.0 # keep in sync with packageManager in package.json
NODE_VERSION=22.23.3
NODE_SHA256_LINUX_X64=df450af89261115ef9f9e3830c3eeb2cc9213b63c720b1af623cb5dcbe2e02de
NODE_MIN="22.13.0" # pnpm 11 needs Node 22.13 or newer

node_ok() {
  command -v node >/dev/null 2>&1 &&
    [ "$(printf '%s\n%s\n' "$NODE_MIN" "$(node -p 'process.versions.node')" | sort -V | head -n1)" = "$NODE_MIN" ]
}

install_node() {
  if [ "$(uname -s)-$(uname -m)" != "Linux-x86_64" ]; then
    echo "Node >= $NODE_MIN is required; install it and run again." >&2
    exit 1
  fi
  local dir="${RUNNER_TEMP:-/tmp}/node-v${NODE_VERSION}"
  local archive="node-v${NODE_VERSION}-linux-x64.tar.xz"
  mkdir -p "$dir"
  curl -fsSL "https://nodejs.org/dist/v${NODE_VERSION}/${archive}" -o "$dir/$archive"
  echo "${NODE_SHA256_LINUX_X64}  $dir/$archive" | sha256sum -c -
  tar -xJf "$dir/$archive" -C "$dir" --strip-components=1
  export PATH="$dir/bin:$PATH"
  if [ -n "${GITHUB_PATH:-}" ]; then echo "$dir/bin" >> "$GITHUB_PATH"; fi
}

node_ok || install_node

npx --yes "pnpm@${PNPM_VERSION}" install --frozen-lockfile
npx --yes "pnpm@${PNPM_VERSION}" build
