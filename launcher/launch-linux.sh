#!/usr/bin/env bash
# ⚒ DARK FORGE — canonical Linux launcher.
# 1. Starts the forge backend on :9091 (if not already running).
# 2. Opens the Dark Forge IDE (bundled build), falling back to system codium.
#
# Usage: ./launch-linux.sh [workspace] [--system-codium]
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
FORGE_BIN="${ROOT}/forge/forge"
IDE_BIN="${ROOT}/ide/VSCode-linux-x64/darkforge"
CONFIG="${DARKFORGE_CONFIG:-${ROOT}/config/forge.example.yaml}"
LOG="${TMPDIR:-/tmp}/dark-forge.log"

WORKSPACE="${1:-$PWD}"
SYSTEM_CODIUM=0
for arg in "$@"; do
  [ "$arg" = "--system-codium" ] && SYSTEM_CODIUM=1
done

forge_up() {
  curl -s --max-time 2 -o /dev/null -w "%{http_code}" http://127.0.0.1:9091/api/status 2>/dev/null | grep -q "200"
}

if ! forge_up; then
  if [ -x "${FORGE_BIN}" ]; then
    echo "⚒ Dark Forge: starting backend on :9091…" >&2
    setsid nohup "${FORGE_BIN}" --config "${CONFIG}" --repo "${WORKSPACE}" \
      </dev/null >>"${LOG}" 2>&1 &
    disown
    for _ in $(seq 1 15); do
      sleep 1
      forge_up && break
    done
    forge_up || echo "⚠ backend did not answer yet — see ${LOG}" >&2
  else
    echo "⚠ forge backend not found at ${FORGE_BIN} — build it: cd forge && go build ./cmd/forge" >&2
  fi
else
  echo "⚒ Dark Forge: backend already running on :9091" >&2
fi

if [ "${SYSTEM_CODIUM}" = "1" ] || [ ! -x "${IDE_BIN}" ]; then
  if command -v codium >/dev/null 2>&1; then
    echo "⚒ Opening system VSCodium (install swarm-chat-*.vsix for the chat panel)" >&2
    exec codium "${WORKSPACE}"
  fi
  echo "✗ No bundled IDE and no system codium found." >&2
  echo "  Build the IDE: scripts/build-ide-linux.sh  (or install VSCodium + the vsix)" >&2
  exit 1
fi

exec "${IDE_BIN}" --no-sandbox "${WORKSPACE}"
