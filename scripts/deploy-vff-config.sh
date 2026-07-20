#!/usr/bin/env bash
# Local orchestrator: config-check, upload .new, remote ownership finalize.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

CONFIG="${1:-}"
if [[ -z "${CONFIG}" ]]; then
  echo "usage: bash $0 /secure/path/config-vff.json" >&2
  exit 1
fi

SERVER_USER="${SERVER_USER:-root}"
SERVER_HOST="${SERVER_HOST:-fr-mrs-1}"
REMOTE_CONFIG_VFF="${REMOTE_CONFIG_VFF:-/opt/bot/config-vff.json}"

go run ./cmd/configcheck -config "${CONFIG}"

REMOTE_TMP=""
cleanup() {
  local ec=$?
  if [[ -n "${REMOTE_TMP}" ]]; then
    ssh "${SERVER_USER}@${SERVER_HOST}" "rm -rf $(printf %q "${REMOTE_TMP}")" >/dev/null 2>&1 || true
  fi
  return "${ec}"
}
trap cleanup EXIT

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  echo "deploy-vff-config: invalid remote temp dir" >&2
  exit 1
fi

scp -q "${CONFIG}" "${SERVER_USER}@${SERVER_HOST}:${REMOTE_CONFIG_VFF}.new"
scp -q \
  "${ROOT}/scripts/deploy-vff-config-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/vff_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "bash $(printf %q "${REMOTE_TMP}/deploy-vff-config-remote.sh") $(printf %q "${REMOTE_CONFIG_VFF}.new")"
