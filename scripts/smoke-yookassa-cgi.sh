#!/usr/bin/env bash
# Brand-agnostic smoke: SHM YooKassa CGI controlled rejection probe.
#
# Usage:
#   bash scripts/smoke-yookassa-cgi.sh <brand-id> [--config <explicit-config.json>]
#   CONFIG=/path/config.json bash scripts/smoke-yookassa-cgi.sh <brand-id>
#
# Reads .api.base_url and .brand.yookassa_pay_system from candidate/runtime config.
# Never uses brand.public_base_url / SMOKE_BASE_URL for the CGI request.
# If --config/CONFIG/SMOKE_CONFIG is omitted, fetches REMOTE_EXPLICIT_CONFIG via SSH.
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

SMOKE_YOOKASSA_LABEL="${BRAND_LABEL}-yookassa"
export SMOKE_YOOKASSA_LABEL
if [[ -n "${SMOKE_CONFIG_ARG}" ]]; then
  export SMOKE_CONFIG="${SMOKE_CONFIG_ARG}"
fi

smoke_yookassa_cgi_resolve_and_check "${SMOKE_CONFIG_ARG}"
