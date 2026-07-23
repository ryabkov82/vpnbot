#!/usr/bin/env bash
# Public smoke checks for a brand base URL (no secrets, no mutations),
# plus a separate SHM YooKassa CGI probe using config api.base_url.
#
# Usage:
#   bash scripts/smoke-brand.sh <brand-id> [--config <explicit-config.json>]
#   SMOKE_CONFIG=/path/config.json bash scripts/smoke-brand.sh <brand-id>
#   CONFIG=/path/config.json bash scripts/smoke-brand.sh <brand-id>
#
# SMOKE_BASE_URL (brand.public_base_url) is used only for public web checks.
# YooKassa CGI uses .api.base_url from candidate/runtime config — never public_base_url.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"
# shellcheck source=lib/smoke_yookassa_cgi.sh
source "${ROOT}/scripts/lib/smoke_yookassa_cgi.sh"

BRAND_ID="${1:-${BRAND:-}}"
if [[ "${BRAND_ID}" == "--brand" ]]; then
  BRAND_ID="${2:-}"
  shift 2 || true
else
  shift $(( $# > 0 ? 1 : 0 )) || true
fi

SMOKE_CONFIG_ARG=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --config)
      SMOKE_CONFIG_ARG="${2:-}"
      if [[ -z "${SMOKE_CONFIG_ARG}" ]]; then
        echo "usage: bash $0 <brand-id> [--config <explicit-config.json>]" >&2
        exit 1
      fi
      shift 2 || true
      ;;
    *)
      echo "usage: bash $0 <brand-id> [--config <explicit-config.json>]" >&2
      exit 1
      ;;
  esac
done

if [[ -z "${BRAND_ID}" ]]; then
  echo "usage: bash $0 <brand-id> [--config <explicit-config.json>]" >&2
  exit 1
fi
brand_profile_load "${BRAND_ID}" || exit 1

if [[ -z "${SMOKE_BASE_URL:-}" ]]; then
  echo "smoke-${BRAND_LABEL}: SMOKE_BASE_URL missing from profile" >&2
  exit 1
fi

SMOKE_BASE_URL="${SMOKE_BASE_URL%/}"

URLS=(
  "${SMOKE_BASE_URL}/api/public/services"
  "${SMOKE_BASE_URL}/account"
  "${SMOKE_BASE_URL}/buy"
  "${SMOKE_BASE_URL}/premium-connect"
)

for u in "${URLS[@]}"; do
  code=""
  if ! code="$(curl -sS -o /dev/null -w '%{http_code}' "$u")"; then
    echo "smoke-${BRAND_LABEL}: transport error for $u" >&2
    exit 1
  fi
  echo "$u -> $code"
  case "$code" in
    5*)
      echo "smoke-${BRAND_LABEL}: unexpected 5xx from $u" >&2
      exit 1
      ;;
    '' | 000)
      echo "smoke-${BRAND_LABEL}: empty/zero status for $u" >&2
      exit 1
      ;;
  esac
done

# Controlled YooKassa CGI probe against SHM/API base (.api.base_url), not brand web.
# Invoked from coordinated rollout before finalize so failures trigger rollback.
SMOKE_YOOKASSA_LABEL="${BRAND_LABEL}-yookassa"
export SMOKE_YOOKASSA_LABEL
if [[ -n "${SMOKE_CONFIG_ARG}" ]]; then
  export SMOKE_CONFIG="${SMOKE_CONFIG_ARG}"
fi
if ! smoke_yookassa_cgi_resolve_and_check "${SMOKE_CONFIG_ARG}"; then
  echo "smoke-${BRAND_LABEL}: YooKassa CGI probe failed" >&2
  exit 1
fi

echo "smoke-${BRAND_LABEL}: OK"
