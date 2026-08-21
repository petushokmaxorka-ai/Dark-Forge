#!/usr/bin/env bash
# ⚒ DARK FORGE — full Linux IDE build (VSCodium pipeline, ~1h).
# Produces ide/VSCode-linux-x64/darkforge with both extensions bundled.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}/ide"
echo "⚒ Building Dark Forge IDE (fetches VSCode pinned by upstream/stable.json)…"
export APP_NAME="Dark Forge" BINARY_NAME="darkforge" ORG_NAME="dark-forge"
./build.sh
echo "✓ IDE ready: ide/VSCode-linux-x64/darkforge"
