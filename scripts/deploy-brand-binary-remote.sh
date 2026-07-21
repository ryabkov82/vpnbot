#!/usr/bin/env bash
# Replace REMOTE_BINARY with REMOTE_BINARY.new; no VPNBOT_CONFIG drop-in.
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived
brand_deploy_binary
