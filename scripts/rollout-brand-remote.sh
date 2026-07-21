#!/usr/bin/env bash
# Remote coordinated brand rollout (binary + config + drop-in).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived || exit 1
brand_require_vars REMOTE_TMP TX_ID || exit 1

rc=0
brand_rollout_run \
  "${REMOTE_TMP}/bot" \
  "${REMOTE_TMP}/config.json" \
  "${REMOTE_TMP}/configcheck" || rc=$?
exit "${rc}"
