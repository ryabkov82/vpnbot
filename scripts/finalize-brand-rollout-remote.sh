#!/usr/bin/env bash
# Finalize a successful coordinated brand rollout (publish marker, release lock).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived || exit 1
brand_require_vars TX_ID || exit 1

rc=0
brand_rollout_finalize || rc=$?
exit "${rc}"
