#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived

if ! brand_rollback_to_legacy; then
  brand_err "CRITICAL: ${BRAND_LABEL} rollback to legacy failed"
  brand_safe_journal_tail
  exit 1
fi
