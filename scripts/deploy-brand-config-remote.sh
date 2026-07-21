#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived

NEW_PATH="${1:-}"
if [[ -z "${NEW_PATH}" ]]; then
  brand_err "usage: bash $0 /path/to/config-explicit.json.new"
  exit 1
fi

brand_deploy_config_file "${NEW_PATH}"
