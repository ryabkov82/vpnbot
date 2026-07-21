#!/usr/bin/env bash
# Remote recovery actions for a preserved rollout transaction.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

ACTION="${1:?action required}"
brand_refresh_derived || exit 1
brand_require_vars TX_ID || exit 1
brand_rollout_paths_init || exit 1

case "${ACTION}" in
  status)
    brand_rollout_status_print
    exit 0
    ;;
  rollback)
    rc=0
    brand_rollout_rollback || rc=$?
    exit "${rc}"
    ;;
  finalize)
    rc=0
    brand_rollout_finalize || rc=$?
    exit "${rc}"
    ;;
  *)
    brand_err "recover: unknown action"
    exit 1
    ;;
esac
