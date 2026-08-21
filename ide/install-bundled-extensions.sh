#!/usr/bin/env bash
# Install bundled Dark Forge extensions into a built IDE tree and register
# them in the built-in extensions manifest.
#
# Usage: ./install-bundled-extensions.sh [VSCode-linux-x64 dir]
# Called automatically at the end of build.sh.
set -euo pipefail

IDE_DIR="${1:-VSCode-linux-x64}"
EXT_ROOT="${IDE_DIR}/resources/app/extensions"
MANIFEST="${EXT_ROOT}/extensions.json"

if [ ! -d "${EXT_ROOT}" ]; then
  echo "install-bundled-extensions: ${EXT_ROOT} not found — build the IDE first" >&2
  exit 1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

for ext in dark-forge-chat heretic-arch; do
  SRC="${SCRIPT_DIR}/../extensions-ide/${ext}"
  [ "${ext}" = "heretic-arch" ] && SRC="${SCRIPT_DIR}/../extension"
  if [ ! -f "${SRC}/package.json" ]; then
    echo "install-bundled-extensions: skip ${ext} (no package.json at ${SRC})" >&2
    continue
  fi
  DEST="${EXT_ROOT}/${ext}"
  rm -rf "${DEST}"
  mkdir -p "${DEST}"
  cp -r "${SRC}/package.json" "${DEST}/"
  cp -r "${SRC}/out" "${DEST}/"
  echo "install-bundled-extensions: ${ext} -> ${DEST}"
done

# Register in the built-in manifest so the IDE loads them as built-ins.
if [ -f "${MANIFEST}" ]; then
  python3 - "${MANIFEST}" <<'PY'
import json, sys
path = sys.argv[1]
entries = json.load(open(path))
have = {e.get("identifier", {}).get("id") or e.get("id") for e in entries}
def add(name):
    ext_id = f"dark-forge.{name}"
    if ext_id in have:
        return
    entries.append({
        "identifier": {"id": ext_id},
        "version": "0.2.0",
        "path": name,
        "relativeLocation": name,
    })
    print(f"install-bundled-extensions: registered {ext_id}")
add("dark-forge-chat")
add("heretic-arch")
json.dump(entries, open(path, "w"))
PY
else
  echo "install-bundled-extensions: manifest ${MANIFEST} missing" >&2
fi

echo "install-bundled-extensions: done"
