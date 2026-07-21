#!/usr/bin/env bash
# Backward-compatible VFF defaults + aliases over brand_ops.sh.
# shellcheck shell=bash

SCRIPT_DIR_BRAND="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=brand_ops.sh
source "${SCRIPT_DIR_BRAND}/brand_ops.sh"

# VFF profile defaults (overridable by environment).
SERVICE_NAME="${SERVICE_NAME:-bot.service}"
REMOTE_DIR="${REMOTE_DIR:-/opt/bot}"
REMOTE_LEGACY_CONFIG="${REMOTE_LEGACY_CONFIG:-${REMOTE_DIR}/config.json}"
REMOTE_EXPLICIT_CONFIG="${REMOTE_EXPLICIT_CONFIG:-${REMOTE_CONFIG_VFF:-${REMOTE_DIR}/config-vff.json}}"
REMOTE_CONFIG_VFF="${REMOTE_EXPLICIT_CONFIG}"
REMOTE_CONFIG_LEGACY="${REMOTE_LEGACY_CONFIG}"
DROPIN_FILE="${DROPIN_FILE:-/etc/systemd/system/${SERVICE_NAME}.d/10-vpnbot-config.conf}"
EXPECTED_BRAND_ID="${EXPECTED_BRAND_ID:-vff}"
BRAND_LABEL="${BRAND_LABEL:-VFF}"
REMOTE_BINARY="${REMOTE_BINARY:-${REMOTE_DIR}/bot}"
brand_refresh_derived || true

VFF_DROPIN_ACTIVE=0

# Aliases used by existing VFF tests/scripts.
vff_log() { brand_log "$@"; }
vff_err() { brand_err "$@"; }
vff_unit_user() { brand_unit_user; }
vff_unit_group() { brand_unit_group; }
vff_env_has_expected() { brand_env_has_expected; }
vff_print_expected_env_only() { brand_print_expected_env_only; }
vff_assert_expected_env_absent() { brand_assert_expected_env_absent; }
vff_dropin_matches() { brand_dropin_matches; }
vff_install_dropin_atomic() {
  brand_install_dropin_atomic
  VFF_DROPIN_ACTIVE="${BRAND_DROPIN_ACTIVE}"
}
vff_ensure_managed_dropin() {
  brand_ensure_managed_dropin
  VFF_DROPIN_ACTIVE="${BRAND_DROPIN_ACTIVE}"
}
vff_safe_journal_tail() { brand_safe_journal_tail; }
vff_rollback_to_legacy() { brand_rollback_to_legacy; }
vff_emergency_rollback() { brand_emergency_rollback "$@"; }
vff_require_active_brand_log() { brand_require_active_brand_log "$@"; }
vff_sync_paths() {
  # Older tests mutate REMOTE_CONFIG_VFF / REMOTE_CONFIG_LEGACY.
  if [[ -n "${REMOTE_CONFIG_VFF:-}" ]]; then
    REMOTE_EXPLICIT_CONFIG="${REMOTE_CONFIG_VFF}"
  fi
  if [[ -n "${REMOTE_CONFIG_LEGACY:-}" ]]; then
    REMOTE_LEGACY_CONFIG="${REMOTE_CONFIG_LEGACY}"
  fi
  EXPECTED_BRAND_ID="${EXPECTED_BRAND_ID:-vff}"
  BRAND_LABEL="${BRAND_LABEL:-VFF}"
  brand_refresh_derived
}

vff_activate() {
  vff_sync_paths
  brand_activate "$@"
}
vff_deploy_config_file() {
  vff_sync_paths
  brand_deploy_config_file "$@"
}
vff_rollback_to_legacy() {
  vff_sync_paths
  brand_rollback_to_legacy
}
vff_emergency_rollback() {
  vff_sync_paths
  brand_emergency_rollback "$@"
}
vff_ensure_managed_dropin() {
  vff_sync_paths
  brand_ensure_managed_dropin
  VFF_DROPIN_ACTIVE="${BRAND_DROPIN_ACTIVE}"
}
