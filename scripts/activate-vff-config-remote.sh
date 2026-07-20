#!/usr/bin/env bash
# Remote activation of explicit VFF config with automatic legacy rollback.
# Intended to run as: bash /path/to/activate-vff-config-remote.sh /path/to/configcheck
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
