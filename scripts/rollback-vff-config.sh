#!/usr/bin/env bash
# Local orchestrator for manual (or smoke-triggered) VFF → legacy rollback.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

SERVER_USER="${SERVER_USER:-root}"
SERVER_HOST="${SERVER_HOST:-fr-mrs-1}"

LOCAL_TMP="$(mktemp -d)"
chmod 0700 "${LOCAL_TMP}"
REMOTE_TMP=""

cleanup() {
  local ec=$?
  if [[ -n "${REMOTE_TMP}" ]]; then
    ssh "${SERVER_USER}@${SERVER_HOST}" "rm -rf $(printf %q "${REMOTE_TMP}")" >/dev/null 2>&1 || true
  fi
  rm -rf "${LOCAL_TMP}"
  return "${ec}"
}
trap cleanup EXIT

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"

scp -q \
  "${ROOT}/scripts/rollback-vff-config-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/vff_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "bash $(printf %q "${REMOTE_TMP}/rollback-vff-config-remote.sh")"
