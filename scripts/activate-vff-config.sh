#!/usr/bin/env bash
# Local orchestrator: build configcheck, upload to unique remote temp dir, activate, smoke, cleanup.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

SERVER_USER="${SERVER_USER:-root}"
SERVER_HOST="${SERVER_HOST:-fr-mrs-1}"
REMOTE_CONFIG_VFF="${REMOTE_CONFIG_VFF:-/opt/bot/config-vff.json}"

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

echo "activate-vff-config: building configcheck..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/configcheck" ./cmd/configcheck

echo "activate-vff-config: preparing remote temp dir..."
REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  echo "activate-vff-config: invalid remote temp dir" >&2
  exit 1
fi

ssh "${SERVER_USER}@${SERVER_HOST}" "test -f $(printf %q "${REMOTE_CONFIG_VFF}")"

scp -q \
  "${LOCAL_TMP}/configcheck" \
  "${ROOT}/scripts/activate-vff-config-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"

# lib/ must be next to the remote script
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/vff_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

echo "activate-vff-config: running remote activation..."
ssh "${SERVER_USER}@${SERVER_HOST}" \
  "bash $(printf %q "${REMOTE_TMP}/activate-vff-config-remote.sh") $(printf %q "${REMOTE_TMP}/configcheck")"

echo "activate-vff-config: public smoke..."
if ! bash "${ROOT}/scripts/smoke-vff.sh"; then
  echo "activate-vff-config: smoke failed; rolling back to legacy" >&2
  bash "${ROOT}/scripts/rollback-vff-config.sh"
  exit 1
fi

echo "activate-vff-config: OK"
