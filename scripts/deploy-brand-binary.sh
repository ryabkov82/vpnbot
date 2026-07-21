#!/usr/bin/env bash
# Build + deploy binary for a brand unit without activating explicit config.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"

SERVER_USER="${SERVER_USER:-root}"
brand_require_vars SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_BINARY \
  REMOTE_LEGACY_CONFIG BRAND_LABEL SMOKE_BASE_URL EXPECTED_BRAND_ID \
  REMOTE_EXPLICIT_CONFIG DROPIN_FILE || exit 1

echo "deploy-${BRAND_LABEL}: running tests..."
go test ./...

echo "deploy-${BRAND_LABEL}: building linux/amd64 binary..."
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

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o "${LOCAL_TMP}/bot" ./cmd/bot

REMOTE_TMP="$(ssh "${SERVER_USER}@${SERVER_HOST}" 'd=$(mktemp -d); chmod 0700 "$d"; printf %s "$d"')"
if [[ -z "${REMOTE_TMP}" || "${REMOTE_TMP}" != /* ]]; then
  brand_err "deploy-${BRAND_LABEL}: invalid remote temp dir"
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

scp -q "${LOCAL_TMP}/bot" "${SERVER_USER}@${SERVER_HOST}:${REMOTE_BINARY}.new"
scp -q \
  "${ENV_FILE}" \
  "${ROOT}/scripts/deploy-brand-binary-remote.sh" \
  "${ROOT}/scripts/rollback-brand-binary-remote.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/"
ssh "${SERVER_USER}@${SERVER_HOST}" "mkdir -p $(printf %q "${REMOTE_TMP}/lib")"
scp -q \
  "${ROOT}/scripts/lib/brand_ops.sh" \
  "${SERVER_USER}@${SERVER_HOST}:${REMOTE_TMP}/lib/"

echo "deploy-${BRAND_LABEL}: installing binary on ${SERVER_HOST}..."
if ! ssh "${SERVER_USER}@${SERVER_HOST}" \
  "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/deploy-brand-binary-remote.sh")"; then
  brand_err "deploy-${BRAND_LABEL}: remote binary install failed"
  exit 1
fi

echo "deploy-${BRAND_LABEL}: public smoke..."
if ! SMOKE_BASE_URL="${SMOKE_BASE_URL}" BRAND_LABEL="${BRAND_LABEL}" bash "${ROOT}/scripts/smoke-brand.sh"; then
  brand_err "deploy-${BRAND_LABEL}: smoke failed; rolling back binary"
  ssh "${SERVER_USER}@${SERVER_HOST}" \
    "set -Eeuo pipefail; source $(printf %q "${REMOTE_TMP}/brand.env"); bash $(printf %q "${REMOTE_TMP}/rollback-brand-binary-remote.sh")" || true
  exit 1
fi

echo "deploy-${BRAND_LABEL}: OK (explicit config not activated; legacy ${REMOTE_LEGACY_CONFIG})"
