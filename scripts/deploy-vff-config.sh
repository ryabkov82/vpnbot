#!/usr/bin/env bash
set -Eeuo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CONFIG="${1:-}"
if [[ -z "${CONFIG}" ]]; then
  echo "usage: bash $0 /secure/path/config-vff.json" >&2
  exit 1
fi
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"
brand_profile_export vff
bash "${ROOT}/scripts/deploy-brand-config.sh" "${CONFIG}"
