#!/usr/bin/env bash
# Remote activation of explicit brand config with automatic legacy rollback.
# Env profile must be exported by caller (SERVICE_NAME, REMOTE_*, EXPECTED_BRAND_ID, BRAND_LABEL, DROPIN_FILE).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived

CONFIGCHECK_BIN="${1:-}"
if [[ -z "${CONFIGCHECK_BIN}" ]]; then
  brand_err "usage: bash $0 /path/to/configcheck"
  exit 1
fi

brand_activate "${CONFIGCHECK_BIN}"
