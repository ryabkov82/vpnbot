#!/usr/bin/env bash
# Public smoke checks for a brand base URL (no secrets, no mutations).
# Usage: bash scripts/smoke-brand.sh <brand-id>   (or BRAND=<id>)
# SMOKE_BASE_URL comes exclusively from the brand profile.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID="${1:-${BRAND:-}}"
if [[ "${BRAND_ID}" == "--brand" ]]; then BRAND_ID="${2:-}"; fi
if [[ -z "${BRAND_ID}" ]]; then
  echo "usage: bash $0 <brand-id>" >&2
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

echo "smoke-${BRAND_LABEL}: OK"
