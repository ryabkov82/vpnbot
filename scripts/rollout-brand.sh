#!/usr/bin/env bash
# Coordinated binary + explicit config + drop-in rollout for a brand unit.
# Usage: bash scripts/rollout-brand.sh <brand-id> <explicit-config.json>
#        (or BRAND=<id> CONFIG=/path/to/config.json)
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

cleanup() {
  local ec=$?
  if [[ -n "${REMOTE_TMP}" ]]; then
    ssh "${SERVER_USER}@${SERVER_HOST}" "rm -rf $(printf %q "${REMOTE_TMP}")" >/dev/null 2>&1 || true
  fi
  if [[ -n "${LOCAL_TMP}" ]]; then
    rm -rf "${LOCAL_TMP}"
  fi
  return "${ec}"
}
trap cleanup EXIT

LOCAL_TMP="$(mktemp -d)"
chmod 0700 "${LOCAL_TMP}"

echo "rollout-${BRAND_LABEL}: building linux/amd64 binaries..."
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/bot" ./cmd/bot
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/configcheck" ./cmd/configcheck

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  brand_err "rollout-${BRAND_LABEL}: invalid remote temp dir"
  exit 1
fi

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
  printf 'export ROLLOUT_TX_DIR=%q\n' "${REMOTE_TMP}/tx"
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
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "mv $(printf %q "${REMOTE_TMP}/$(basename "${CONFIG}")") $(printf %q "${REMOTE_TMP}/config.json")"

echo "rollout-${BRAND_LABEL}: running coordinated rollout on ${SERVER_HOST}..."
if ! ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollout-brand-remote.sh")"; then
  brand_err "rollout-${BRAND_LABEL}: remote rollout failed"
  exit 1
fi

echo "rollout-${BRAND_LABEL}: public smoke..."
if ! bash "${ROOT}/scripts/smoke-brand.sh" "${EXPECTED_BRAND_ID}"; then
  brand_err "rollout-${BRAND_LABEL}: smoke failed"
  if ssh "${SERVER_USER}@${SERVER_HOST}" \
    "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollback-brand-rollout-remote.sh")"; then
    brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
  else
    brand_err "CRITICAL: ${BRAND_LABEL} rollout smoke failed and automatic rollback failed"
  fi
  exit 1
fi

if ! ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/finalize-brand-rollout-remote.sh")"; then
  brand_err "rollout-${BRAND_LABEL}: finalize failed"
  exit 1
fi

printf 'rollout-%s: OK\nbinary=%s\nconfig=%s\nbrand.id=%s\n' \
  "${BRAND_LABEL}" "${REMOTE_BINARY}" "${REMOTE_EXPLICIT_CONFIG}" "${EXPECTED_BRAND_ID}"
