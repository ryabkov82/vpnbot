#!/usr/bin/env bash
# Finalize a successful coordinated brand rollout (retain binary backup).
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=lib/brand_ops.sh
source "${SCRIPT_DIR}/lib/brand_ops.sh"

brand_refresh_derived || exit 1
brand_rollout_finalize
