#!/usr/bin/env bash
set -Eeuo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/vff_ops.sh
source "${SCRIPT_DIR}/lib/vff_ops.sh"
NEW_PATH="${1:-}"
if [[ -z "${NEW_PATH}" ]]; then
  vff_err "usage: bash $0 /opt/bot/config-vff.json.new"
  exit 1
fi
vff_deploy_config_file "${NEW_PATH}"
