#!/usr/bin/env bash
# Coordinated binary + explicit config + drop-in rollout for a brand unit.
# Usage: bash scripts/rollout-brand.sh <brand-id> <explicit-config.json>
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID="${1:-${BRAND:-}}"
if [[ "${BRAND_ID}" == "--brand" ]]; then
  BRAND_ID="${2:-}"
  shift 2 || true
else
  shift $(( $# > 0 ? 1 : 0 )) || true
fi
CONFIG="${1:-${CONFIG:-}}"
if [[ -z "${BRAND_ID}" || -z "${CONFIG}" ]]; then
  brand_err "usage: bash $0 <brand-id> <explicit-config.json>"
  exit 1
fi
if [[ ! -f "${CONFIG}" ]]; then
  brand_err "rollout: config file not found: ${CONFIG}"
  exit 1
fi

brand_profile_load "${BRAND_ID}" || exit 1
brand_require_vars SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_BINARY \
  REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE EXPECTED_BRAND_ID \
  BRAND_LABEL SMOKE_BASE_URL || exit 1
brand_refresh_derived

summary="$(go run ./cmd/configcheck -config "${CONFIG}")"
printf '%s' "${summary}"
if ! grep -Fxq "brand.id=${EXPECTED_BRAND_ID}" <<<"${summary}"; then
  brand_err "rollout-${BRAND_LABEL}: brand.id must be ${EXPECTED_BRAND_ID}"
  exit 1
fi

echo "rollout-${BRAND_LABEL}: running tests..."
go test ./...

LOCAL_TMP=""
REMOTE_TMP=""
REMOTE_CLEANUP_SAFE=0
TRANSACTION_STARTED=0
TX_ID=""
INTERRUPTED=0

print_recovery() {
  if [[ -n "${TX_ID}" ]]; then
    brand_err "recovery: make brand-rollout-recover BRAND=${EXPECTED_BRAND_ID} TX_ID=${TX_ID} ACTION=status"
    brand_err "transaction.id=${TX_ID}"
    brand_err "remote_tmp=${REMOTE_TMP:-}"
  fi
}

cleanup() {
  local ec=$?
  if [[ "${INTERRUPTED}" -eq 1 ]]; then
    REMOTE_CLEANUP_SAFE=0
    print_recovery
  fi
  if [[ "${REMOTE_CLEANUP_SAFE}" -eq 1 && -n "${REMOTE_TMP}" ]]; then
    ssh "${SERVER_USER}@${SERVER_HOST}" "rm -rf $(printf %q "${REMOTE_TMP}")" >/dev/null 2>&1 || true
  elif [[ -n "${REMOTE_TMP}" && "${REMOTE_CLEANUP_SAFE}" -eq 0 ]]; then
    brand_err "rollout-${BRAND_LABEL}: preserving remote temp ${REMOTE_TMP}"
    print_recovery
  fi
  if [[ -n "${LOCAL_TMP}" ]]; then
    rm -rf "${LOCAL_TMP}"
  fi
  return "${ec}"
}

on_signal() {
  INTERRUPTED=1
  REMOTE_CLEANUP_SAFE=0
  brand_err "rollout-${BRAND_LABEL}: interrupted; remote state preserved"
  print_recovery
  exit 130
}

trap cleanup EXIT
trap on_signal INT TERM HUP

LOCAL_TMP="$(mktemp -d)"
chmod 0700 "${LOCAL_TMP}"

TX_ID="$(date -u +%Y%m%dT%H%M%S%N)-$$-${RANDOM}${RANDOM}"
if ! brand_rollout_validate_tx_id "${TX_ID}"; then
  brand_err "rollout-${BRAND_LABEL}: generated TX_ID invalid"
  REMOTE_CLEANUP_SAFE=1
  exit 1
fi

echo "rollout-${BRAND_LABEL}: building linux/amd64 binaries..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/bot" ./cmd/bot
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/configcheck" ./cmd/configcheck

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  brand_err "rollout-${BRAND_LABEL}: invalid remote temp dir"
  REMOTE_CLEANUP_SAFE=1
  exit 1
fi

ROLLOUT_TX_DIR="${REMOTE_DIR}/.vpnbot-rollouts/${TX_ID}"
ROLLOUT_LOCK_DIR="${REMOTE_DIR}/.vpnbot-rollout.lock"

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
  printf 'export REMOTE_TMP=%q\n' "${REMOTE_TMP}"
  printf 'export TX_ID=%q\n' "${TX_ID}"
  printf 'export ROLLOUT_TX_DIR=%q\n' "${ROLLOUT_TX_DIR}"
  printf 'export ROLLOUT_LOCK_DIR=%q\n' "${ROLLOUT_LOCK_DIR}"
} >"${ENV_FILE}"

ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${LOCAL_TMP}/bot" \
  "${CONFIG}" \
  "${LOCAL_TMP}/configcheck" \
  "${ENV_FILE}" \
  "${ROOT}/scripts/rollout-brand-remote.sh" \
  "${ROOT}/scripts/rollback-brand-rollout-remote.sh" \
  "${ROOT}/scripts/finalize-brand-rollout-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
scp -q \
  "${ROOT}/scripts/lib/brand_ops.sh" \
  "${ROOT}/scripts/lib/brand_rollout.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "mv $(printf %q "${REMOTE_TMP}/$(basename "${CONFIG}")") $(printf %q "${REMOTE_TMP}/config.json")"

TRANSACTION_STARTED=1
echo "rollout-${BRAND_LABEL}: running coordinated rollout on ${SERVER_HOST} (tx=${TX_ID})..."
set +e
ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Euo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollout-brand-remote.sh")"
remote_rc=$?
set -e

case "${remote_rc}" in
  0)
    ;;
  10)
    brand_err "rollout-${BRAND_LABEL}: remote aborted before mutation"
    REMOTE_CLEANUP_SAFE=1
    exit 1
    ;;
  20)
    brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
    REMOTE_CLEANUP_SAFE=1
    exit 1
    ;;
  30)
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    print_recovery
    REMOTE_CLEANUP_SAFE=0
    exit 1
    ;;
  40)
    brand_err "rollout-${BRAND_LABEL}: another rollout is already in progress"
    REMOTE_CLEANUP_SAFE=1
    exit 1
    ;;
  255)
    brand_err "rollout-${BRAND_LABEL}: SSH/transport failure (remote state preserved)"
    print_recovery
    REMOTE_CLEANUP_SAFE=0
    exit 1
    ;;
  *)
    brand_err "rollout-${BRAND_LABEL}: unknown remote exit ${remote_rc} (remote state preserved)"
    print_recovery
    REMOTE_CLEANUP_SAFE=0
    exit 1
    ;;
esac

# Public smoke includes YooKassa CGI controlled-rejection probe (via smoke-brand.sh).
# CGI probe uses candidate config .api.base_url (not brand.public_base_url).
# Failure here rolls back before finalize.
echo "rollout-${BRAND_LABEL}: public smoke (includes YooKassa CGI)..."
set +e
bash "${ROOT}/scripts/smoke-brand.sh" "${EXPECTED_BRAND_ID}" --config "${CONFIG}"
smoke_rc=$?
set -e
if [[ "${smoke_rc}" -ne 0 ]]; then
  brand_err "rollout-${BRAND_LABEL}: smoke failed"
  set +e
  ssh "${SERVER_USER}@${SERVER_HOST}" \
    "set -Euo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollback-brand-rollout-remote.sh")"
  rb_rc=$?
  set -e
  case "${rb_rc}" in
    20)
      brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
      REMOTE_CLEANUP_SAFE=1
      ;;
    30)
      brand_err "CRITICAL: ${BRAND_LABEL} rollout smoke failed and automatic rollback failed"
      print_recovery
      REMOTE_CLEANUP_SAFE=0
      ;;
    *)
      brand_err "CRITICAL: ${BRAND_LABEL} rollout smoke failed and automatic rollback failed"
      print_recovery
      REMOTE_CLEANUP_SAFE=0
      ;;
  esac
  exit 1
fi

set +e
ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Euo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/finalize-brand-rollout-remote.sh")"
fin_rc=$?
set -e
if [[ "${fin_rc}" -ne 0 ]]; then
  brand_err "rollout-${BRAND_LABEL}: finalize failed"
  print_recovery
  REMOTE_CLEANUP_SAFE=0
  exit 1
fi

REMOTE_CLEANUP_SAFE=1
printf 'rollout-%s: OK\nbinary=%s\nconfig=%s\nbrand.id=%s\n' \
  "${BRAND_LABEL}" "${REMOTE_BINARY}" "${REMOTE_EXPLICIT_CONFIG}" "${EXPECTED_BRAND_ID}"
