#!/usr/bin/env bash
# Read-only systemd status for a brand unit (no mutations, no config output).
# Usage: bash scripts/status-brand.sh <brand-id>   (or BRAND=<id>)
set -Eeuo pipefail

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

ssh "${SERVER_USER}@${SERVER_HOST}" \
  "systemctl --no-pager --full status $(printf %q "${SERVICE_NAME}")"
