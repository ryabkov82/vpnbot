#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"

CONFIG="${1:-}"
if [[ -z "${CONFIG}" ]]; then
  brand_err "usage: bash $0 /secure/path/config-explicit.json"
  exit 1
fi

SERVER_USER="${SERVER_USER:-root}"
brand_require_vars SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_EXPLICIT_CONFIG \
  EXPECTED_BRAND_ID BRAND_LABEL REMOTE_LEGACY_CONFIG DROPIN_FILE || exit 1
brand_refresh_derived

summary="$(go run ./cmd/configcheck -config "${CONFIG}")"
printf '%s' "${summary}"
if ! grep -Fxq "brand.id=${EXPECTED_BRAND_ID}" <<<"${summary}"; then
  brand_err "deploy-${BRAND_LABEL}-config: brand.id must be ${EXPECTED_BRAND_ID}"
  exit 1
fi

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
  brand_err "deploy-${BRAND_LABEL}-config: invalid remote temp dir"
  exit 1
fi

ENV_FILE="${REMOTE_TMP}/brand.env"
# Write env locally then scp — build in local tmp.
LOCAL_TMP="$(mktemp -d)"
chmod 0700 "${LOCAL_TMP}"
{
  printf 'export SERVICE_NAME=%q\n' "${SERVICE_NAME}"
  printf 'export REMOTE_DIR=%q\n' "${REMOTE_DIR}"
  printf 'export REMOTE_BINARY=%q\n' "${REMOTE_BINARY}"
  printf 'export REMOTE_LEGACY_CONFIG=%q\n' "${REMOTE_LEGACY_CONFIG}"
  printf 'export REMOTE_EXPLICIT_CONFIG=%q\n' "${REMOTE_EXPLICIT_CONFIG}"
  printf 'export DROPIN_FILE=%q\n' "${DROPIN_FILE}"
  printf 'export EXPECTED_BRAND_ID=%q\n' "${EXPECTED_BRAND_ID}"
  printf 'export BRAND_LABEL=%q\n' "${BRAND_LABEL}"
} >"${LOCAL_TMP}/brand.env"

scp -q "${CONFIG}" "${SERVER_USER}@${SERVER_HOST}:${REMOTE_EXPLICIT_CONFIG}.new"
scp -q \
  "${LOCAL_TMP}/brand.env" \
  "${ROOT}/scripts/deploy-brand-config-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/brand_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"
rm -rf "${LOCAL_TMP}"

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/deploy-brand-config-remote.sh") $(printf %q "${REMOTE_EXPLICIT_CONFIG}.new")"
