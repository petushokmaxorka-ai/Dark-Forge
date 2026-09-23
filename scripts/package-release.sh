#!/usr/bin/env bash
# ⚒ DARK FORGE — package release artifacts into dist/.
# Produces:
#   dist/Dark.Forge-<ver>-linux-x64.tar.gz   (forge + forge-tui + vsix + launcher + config + README)
#   dist/Dark.Forge-<ver>-windows-x64.zip    (forge.exe + vsix + launcher + config + README)
# Optional: --with-ide  adds ide/VSCode-linux-x64 to the Linux tarball.
# Usage: scripts/package-release.sh [version] [--with-ide]
#        (version defaults to the latest git tag without the "v" prefix)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"
VERSION=""
WITH_IDE=0
for arg in "$@"; do
  case "${arg}" in
    --with-ide) WITH_IDE=1 ;;
    *) VERSION="${arg}" ;;
  esac
done
if [ -z "${VERSION}" ]; then
  VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
  VERSION="${VERSION#v}"
fi
VERSION="${VERSION:-1.0.0}"
DIST="${ROOT}/dist"
STAGE_LINUX="${DIST}/stage/Dark.Forge-${VERSION}-linux-x64"
STAGE_WIN="${DIST}/stage/Dark.Forge-${VERSION}-windows-x64"

echo "⚒ Packaging Dark Forge ${VERSION}…"

# 1. Backend binaries
echo "→ building forge (linux + windows)…"
( cd forge && go build -o forge ./cmd/forge && GOOS=windows GOARCH=amd64 go build -o forge.exe ./cmd/forge && go build -o forge-tui ./cmd/forge-tui )

# 2. Extensions (reuse vsix if present, else package)
echo "→ extensions…"
if ! ls extension/*.vsix >/dev/null 2>&1; then
  ( cd extension && npm ci --no-fund --no-audit >/dev/null 2>&1 && npx vsce package --allow-missing-repository >/dev/null )
fi
if ! ls extensions-ide/dark-forge-chat/*.vsix >/dev/null 2>&1; then
  ( cd extensions-ide/dark-forge-chat && npx --prefix "${ROOT}/extension" vsce package --allow-missing-repository >/dev/null )
fi

# 3. Stage Linux
rm -rf "${STAGE_LINUX}" && mkdir -p "${STAGE_LINUX}"
mkdir -p "${STAGE_LINUX}/forge" "${STAGE_LINUX}/extensions"
cp forge/forge forge/forge-tui "${STAGE_LINUX}/forge/"
cp extension/*.vsix extensions-ide/dark-forge-chat/*.vsix "${STAGE_LINUX}/extensions/"
cp launcher/launch-linux.sh "${STAGE_LINUX}/launch-linux.sh"
chmod +x "${STAGE_LINUX}/launch-linux.sh"
cp config/forge.example.yaml "${STAGE_LINUX}/forge.example.yaml"
cp README.md LICENSE THIRD_PARTY_NOTICES.md "${STAGE_LINUX}/"
if [ "${WITH_IDE}" = "1" ] && [ -x ide/VSCode-linux-x64/darkforge ]; then
  echo "→ bundling IDE tree…"
  cp -a ide/VSCode-linux-x64 "${STAGE_LINUX}/darkforge-ide"
fi
tar -czf "${DIST}/Dark.Forge-${VERSION}-linux-x64.tar.gz" -C "${DIST}/stage" "Dark.Forge-${VERSION}-linux-x64"

# 4. Stage Windows
rm -rf "${STAGE_WIN}" && mkdir -p "${STAGE_WIN}/forge" "${STAGE_WIN}/extensions"
cp forge/forge.exe "${STAGE_WIN}/forge/"
cp extension/*.vsix extensions-ide/dark-forge-chat/*.vsix "${STAGE_WIN}/extensions/"
cp launcher/launch-windows.ps1 "${STAGE_WIN}/"
cp config/forge.example.yaml "${STAGE_WIN}/forge.example.yaml"
cp README.md LICENSE THIRD_PARTY_NOTICES.md "${STAGE_WIN}/"
# zip via python3 (no zip CLI dependency)
python3 - "${DIST}" "${VERSION}" <<'PYZIP'
import zipfile, os, sys
dist, version = sys.argv[1], sys.argv[2]
stage = os.path.join(dist, "stage", f"Dark.Forge-{version}-windows-x64")
with zipfile.ZipFile(os.path.join(dist, f"Dark.Forge-{version}-windows-x64.zip"), "w", zipfile.ZIP_DEFLATED) as z:
    for root, dirs, files in os.walk(stage):
        for f in files:
            p = os.path.join(root, f)
            z.write(p, os.path.relpath(p, os.path.join(dist, "stage")))
print("zip created")
PYZIP

rm -rf "${DIST}/stage"
echo "✓ done:"
ls -lh "${DIST}"/Dark.Forge-"${VERSION}"-* || true
