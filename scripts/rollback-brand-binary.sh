#!/usr/bin/env bash
# Manual binary rollback for a brand unit using the last-recorded backup marker.
# Usage: bash scripts/rollback-brand-binary.sh <brand-id>   (or BRAND=<id>)
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID="${1:-${BRAND:-}}"
if [[ "${BRAND_ID}" == "--brand" ]]; then BRAND_ID="${2:-}"; fi
if [[ -z "${BRAND_ID}" ]]; then
  brand_err "usage: bash $0 <brand-id>"
  exit 1
fi
brand_profile_load "${BRAND_ID}" || exit 1

brand_require_vars SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_BINARY \
  REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE EXPECTED_BRAND_ID \
  BRAND_LABEL || exit 1

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
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  brand_err "rollback-${BRAND_LABEL}: invalid remote temp dir"
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
} >"${ENV_FILE}"

scp -q \
  "${ENV_FILE}" \
  "${ROOT}/scripts/rollback-brand-binary-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/brand_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollback-brand-binary-remote.sh")"
