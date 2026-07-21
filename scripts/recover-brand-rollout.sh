#!/usr/bin/env bash
# Recover / inspect a preserved brand rollout transaction.
# Usage: bash scripts/recover-brand-rollout.sh <brand-id> <tx-id> <status|rollback|finalize>
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID="${1:-${BRAND:-}}"
TX_ID="${2:-${TX_ID:-}}"
ACTION="${3:-${ACTION:-}}"

if [[ -z "${BRAND_ID}" || -z "${TX_ID}" || -z "${ACTION}" ]]; then
  brand_err "usage: bash $0 <brand-id> <tx-id> <status|rollback|finalize>"
  exit 1
fi

case "${ACTION}" in
  status|rollback|finalize) ;;
  *)
    brand_err "recover: ACTION must be status|rollback|finalize"
    exit 1
    ;;
esac

if ! brand_rollout_validate_tx_id "${TX_ID}"; then
  brand_err "recover: invalid TX_ID"
  exit 1
fi

brand_profile_load "${BRAND_ID}" || exit 1
brand_require_vars SERVER_HOST SERVICE_NAME REMOTE_DIR EXPECTED_BRAND_ID BRAND_LABEL \
  REMOTE_BINARY REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE || exit 1
brand_refresh_derived

export TX_ID
ROLLOUT_TX_DIR="${REMOTE_DIR}/.vpnbot-rollouts/${TX_ID}"
ROLLOUT_LOCK_DIR="${REMOTE_DIR}/.vpnbot-rollout.lock"

LOCAL_TMP="$(mktemp -d)"
chmod 0700 "${LOCAL_TMP}"
cleanup() {
  rm -rf "${LOCAL_TMP}"
}
trap cleanup EXIT

ENV_FILE="${LOCAL_TMP}/brand.env"
{
  printf 'export SERVICE_NAME=%q\n' "${SERVICE_NAME}"
  printf 'export REMOTE_DIR=%q\n' "${REMOTE_DIR}"
  printf 'export REMOTE_BINARY=%q\n' "${REMOTE_BINARY}"
  printf 'export REMOTE_LEGACY_CONFIG=%q\n' "${REMOTE_LEGACY_CONFIG}"
  printf 'export REMOTE_EXPLICIT_CONFIG=%q\n' "${REMOTE_EXPLICIT_CONFIG}"
  printf 'export DROPIN_FILE=%q\n' "${DROPIN_FILE}"
  printf 'export EXPECTED_BRAND_ID=%q\n' "${EXPECTED_BRAND_ID}"
  printf 'export BRAND_LABEL=%q\n' "${BRAND_LABEL}"
  printf 'export TX_ID=%q\n' "${TX_ID}"
  printf 'export ROLLOUT_TX_DIR=%q\n' "${ROLLOUT_TX_DIR}"
  printf 'export ROLLOUT_LOCK_DIR=%q\n' "${ROLLOUT_LOCK_DIR}"
} >"${ENV_FILE}"

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  brand_err "recover: invalid remote temp"
  exit 1
fi

# Best-effort cleanup of recovery upload temp only (never tx/lock).
cleanup_remote() {
  ssh "${SERVER_USER}@${SERVER_HOST}" "rm -rf $(printf %q "${REMOTE_TMP}")" >/dev/null 2>&1 || true
}
trap 'cleanup_remote; cleanup' EXIT

ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ENV_FILE}" \
  "${ROOT}/scripts/recover-brand-rollout-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
scp -q \
  "${ROOT}/scripts/lib/brand_ops.sh" \
  "${ROOT}/scripts/lib/brand_rollout.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

set +e
ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/recover-brand-rollout-remote.sh") $(printf %q "${ACTION}")"
rc=$?
set -e
exit "${rc}"
