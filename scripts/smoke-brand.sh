#!/usr/bin/env bash
# Public smoke checks for a brand base URL (no secrets, no mutations).
set -euo pipefail

SMOKE_BASE_URL="${SMOKE_BASE_URL:-}"
BRAND_LABEL="${BRAND_LABEL:-brand}"

if [[ -z "${SMOKE_BASE_URL}" ]]; then
  echo "smoke-${BRAND_LABEL}: SMOKE_BASE_URL is required" >&2
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
