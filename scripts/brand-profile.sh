#!/usr/bin/env bash
# Print a safe, non-secret summary of a brand profile.
# Usage: bash scripts/brand-profile.sh <brand-id>   (or BRAND=<id>)
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID="${1:-${BRAND:-}}"
if [[ "${BRAND_ID}" == "--brand" ]]; then
  BRAND_ID="${2:-}"
fi
if [[ -z "${BRAND_ID}" ]]; then
  echo "usage: bash $0 <brand-id>" >&2
  exit 1
fi

brand_profile_summary "${BRAND_ID}"
