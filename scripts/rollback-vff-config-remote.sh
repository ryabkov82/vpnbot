#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/vff_ops.sh
source "${SCRIPT_DIR}/lib/vff_ops.sh"
if ! vff_rollback_to_legacy; then
  vff_err "CRITICAL: VFF rollback to legacy failed"
  vff_safe_journal_tail
  exit 1
fi
