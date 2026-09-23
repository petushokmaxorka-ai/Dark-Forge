#!/usr/bin/env bash
# ⚒ DARK FORGE — full Linux IDE build (VSCodium pipeline, ~1h).
# Produces ide/VSCode-linux-<arch>/darkforge with both extensions bundled.
# Needs Node 24 (ide/.nvmrc), Rust, jq and the usual VSCode build deps.
# Extra args are passed to ide/dev/build.sh (e.g. -s: reuse fetched sources).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# build.sh bundles swarm-chat from extension/out — compile it first.
echo "⚒ Compiling swarm-chat extension…"
( cd "${ROOT}/extension" && npm ci --no-fund --no-audit && npm run compile )

cd "${ROOT}/ide"
echo "⚒ Building Dark Forge IDE (fetches VSCode pinned by upstream/stable.json)…"
# dev/build.sh sets the build environment (SHOULD_BUILD, OS_NAME, arch,
# branding), fetches the pinned VSCode via get_repo.sh and runs build.sh.
./dev/build.sh "$@"
IDE_BIN=""
for bin in VSCode-linux-*/darkforge; do
  if [ -x "${bin}" ]; then IDE_BIN="${bin}"; fi
done
if [ -z "${IDE_BIN}" ]; then
  echo "✗ build finished but no ide/VSCode-linux-*/darkforge was produced" >&2
  exit 1
fi
echo "✓ IDE ready: ide/${IDE_BIN}"
