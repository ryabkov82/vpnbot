#!/usr/bin/env bash
# Compatibility remote entry: prefer activate-brand-config-remote.sh with profile env.
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/vff_ops.sh
source "${SCRIPT_DIR}/lib/vff_ops.sh"
CONFIGCHECK_BIN="${1:-}"
if [[ -z "${CONFIGCHECK_BIN}" ]]; then
  vff_err "usage: bash $0 /path/to/configcheck"
  exit 1
fi
vff_activate "${CONFIGCHECK_BIN}"
