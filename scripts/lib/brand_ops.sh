#!/usr/bin/env bash
# Shared brand systemd/deploy helpers (VFF/FC). No secrets. No eval.
# shellcheck shell=bash

# Required for config activation / deploy / rollback (set by caller profile).
# SERVICE_NAME, REMOTE_DIR, REMOTE_LEGACY_CONFIG, REMOTE_EXPLICIT_CONFIG,
# DROPIN_FILE, EXPECTED_BRAND_ID, BRAND_LABEL
#
# Optional / derived:
# REMOTE_BINARY, EXPECTED_ENV, DROPIN_DIR, DROPIN_BODY, SMOKE_BASE_URL

BRAND_DROPIN_ACTIVE=0
BINARY_BACKUP=""
BINARY_REPLACED=0

brand_log() { printf '%s\n' "$*"; }
brand_err() { printf '%s\n' "$*" >&2; }

brand_require_vars() {
  local missing=() v
  for v in "$@"; do
    if [[ -z "${!v:-}" ]]; then
      missing+=("${v}")
    fi
  done
  if ((${#missing[@]} > 0)); then
    brand_err "brand_ops: missing required parameters: ${missing[*]}"
    return 1
  fi
  return 0
}

brand_refresh_derived() {
  brand_require_vars SERVICE_NAME REMOTE_DIR REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG \
    DROPIN_FILE EXPECTED_BRAND_ID BRAND_LABEL || return 1
  EXPECTED_ENV="VPNBOT_CONFIG=${REMOTE_EXPLICIT_CONFIG}"
  DROPIN_DIR="$(dirname "${DROPIN_FILE}")"
  DROPIN_BODY="$(printf '%s\n' '[Service]' "Environment=${EXPECTED_ENV}")"
  REMOTE_BINARY="${REMOTE_BINARY:-${REMOTE_DIR}/bot}"
}

brand_unit_user() {
  local user
  user="$(systemctl show "${SERVICE_NAME}" -p User --value 2>/dev/null || true)"
  if [[ -z "${user}" ]]; then
    user=root
  fi
  printf '%s\n' "${user}"
}

brand_unit_group() {
  local group user
  group="$(systemctl show "${SERVICE_NAME}" -p Group --value 2>/dev/null || true)"
  if [[ -z "${group}" ]]; then
    user="$(brand_unit_user)"
    group="$(id -gn "${user}")"
  fi
  printf '%s\n' "${group}"
}

brand_env_has_expected() {
  systemctl show "${SERVICE_NAME}" -p Environment --value 2>/dev/null |
    tr ' ' '\n' |
    grep -Fxq "${EXPECTED_ENV}"
}

brand_print_expected_env_only() {
  if brand_env_has_expected; then
    printf '%s\n' "${EXPECTED_ENV}"
    return 0
  fi
  return 1
}

brand_assert_expected_env_absent() {
  if brand_env_has_expected; then
    brand_err "rollback: ${EXPECTED_ENV} still present on ${SERVICE_NAME}"
    return 1
  fi
  return 0
}

brand_dropin_matches() {
  [[ -f "${DROPIN_FILE}" ]] || return 1
  local current
  current="$(cat "${DROPIN_FILE}")"
  [[ "${current}" == "${DROPIN_BODY}" ]]
}

brand_install_dropin_atomic() {
  mkdir -p "${DROPIN_DIR}"
  local tmp="${DROPIN_DIR}/10-vpnbot-config.conf.new"
  printf '%s\n' "${DROPIN_BODY}" >"${tmp}"
  chmod 0644 "${tmp}"
  mv "${tmp}" "${DROPIN_FILE}"
  BRAND_DROPIN_ACTIVE=1
}

brand_ensure_managed_dropin() {
  if [[ -f "${DROPIN_FILE}" ]]; then
    if brand_dropin_matches; then
      brand_log "activate-${BRAND_LABEL}-config: managed drop-in already correct (idempotent)"
      BRAND_DROPIN_ACTIVE=1
      return 0
    fi
    brand_err "activate-${BRAND_LABEL}-config: refusing to overwrite existing drop-in with unexpected content: ${DROPIN_FILE}"
    return 1
  fi
  brand_install_dropin_atomic
  brand_log "activate-${BRAND_LABEL}-config: installed managed drop-in ${DROPIN_FILE}"
}

brand_safe_journal_tail() {
  journalctl -u "${SERVICE_NAME}" -n 40 --no-pager 2>/dev/null |
    grep -E 'active brand:|telegram bot configured|configcheck|FATAL|panic|error|Error' || true
}

# Remove managed drop-in only when absent (idempotent) or content matches DROPIN_BODY.
brand_remove_managed_dropin() {
  brand_refresh_derived || return 1
  if [[ ! -f "${DROPIN_FILE}" ]]; then
    brand_log "rollback-${BRAND_LABEL}-config: drop-in absent (idempotent)"
    return 0
  fi
  if brand_dropin_matches; then
    rm -f "${DROPIN_FILE}"
    return 0
  fi
  brand_err "rollback-${BRAND_LABEL}-config: refusing to remove drop-in with unexpected content: ${DROPIN_FILE}"
  return 1
}

brand_rollback_to_legacy() {
  brand_refresh_derived || return 1
  if ! brand_remove_managed_dropin; then
    return 1
  fi
  if ! systemctl daemon-reload; then
    brand_err "rollback: daemon-reload failed"
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "rollback: restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollback: ${SERVICE_NAME} is not active"
    return 1
  fi
  sleep 1
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollback: ${SERVICE_NAME} became inactive after stabilization wait"
    return 1
  fi
  if ! brand_assert_expected_env_absent; then
    return 1
  fi
  brand_log "rollback-${BRAND_LABEL}-config: legacy active; managed drop-in removed; ${REMOTE_EXPLICIT_CONFIG} left in place"
}

brand_emergency_rollback() {
  local reason="${1:-unknown}"
  brand_err "activate-${BRAND_LABEL}-config: activation failed (${reason}); rolling back to legacy"
  if ! brand_rollback_to_legacy; then
    brand_err "CRITICAL: ${BRAND_LABEL} activation failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi
  return 1
}

# Exact brand marker: the startup log is "active brand: id=<id> name=...", so require the
# trailing ' name=' boundary. This rejects prefix collisions (fc vs fc2, vff vs vff-test).
# Fixed-string match avoids assembling a regex from brand.id.
brand_require_active_brand_log() {
  local since="${1:?}"
  local needle="active brand: id=${EXPECTED_BRAND_ID} name="
  if ! journalctl -u "${SERVICE_NAME}" --since "${since}" --no-pager 2>/dev/null |
    grep -Fq "${needle}"; then
    brand_err "activate-${BRAND_LABEL}-config: startup log missing '${needle}' since ${since}"
    return 1
  fi
  return 0
}

# Binary deploy startup check: runtime requires an explicit brand, so the new binary
# must log the exact active brand id (no implicit VFF) plus the telegram marker.
# A legacy config without brand fails config validation and never emits these lines.
# The ' name=' boundary rejects prefix collisions (e.g. fc vs fc2, vff vs vff-test).
brand_require_startup_log() {
  local since="${1:?}"
  local log needle="active brand: id=${EXPECTED_BRAND_ID} name="
  log="$(journalctl -u "${SERVICE_NAME}" --since "${since}" --no-pager 2>/dev/null || true)"
  if ! grep -Fq "${needle}" <<<"${log}"; then
    brand_err "deploy-${BRAND_LABEL}: startup log missing '${needle}' since ${since}"
    return 1
  fi
  if ! grep -Fq 'telegram bot configured' <<<"${log}"; then
    brand_err "deploy-${BRAND_LABEL}: startup log missing 'telegram bot configured' since ${since}"
    return 1
  fi
  return 0
}

brand_activate() {
  local configcheck_bin="${1:?configcheck binary required}"
  brand_refresh_derived || return 1

  if [[ ! -f "${configcheck_bin}" ]]; then
    brand_err "activate-${BRAND_LABEL}-config: configcheck not found: ${configcheck_bin}"
    return 1
  fi
  chmod 0700 "${configcheck_bin}" 2>/dev/null || true

  if [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    brand_err "activate-${BRAND_LABEL}-config: missing ${REMOTE_EXPLICIT_CONFIG}"
    return 1
  fi

  local summary
  if ! summary="$("${configcheck_bin}" -config "${REMOTE_EXPLICIT_CONFIG}")"; then
    brand_err "activate-${BRAND_LABEL}-config: configcheck failed (drop-in not installed)"
    return 1
  fi
  printf '%s' "${summary}"
  if ! grep -Fxq "brand.id=${EXPECTED_BRAND_ID}" <<<"${summary}"; then
    brand_err "activate-${BRAND_LABEL}-config: configcheck brand.id != ${EXPECTED_BRAND_ID}"
    return 1
  fi

  if ! brand_ensure_managed_dropin; then
    return 1
  fi

  local start_time
  start_time="$(date '+%Y-%m-%d %H:%M:%S')"

  if ! systemctl daemon-reload; then
    brand_emergency_rollback "daemon-reload failed"
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_emergency_rollback "restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_emergency_rollback "unit not active after restart"
    return 1
  fi

  sleep 2

  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_emergency_rollback "unit not active after stabilization"
    return 1
  fi

  if ! brand_print_expected_env_only; then
    brand_emergency_rollback "VPNBOT_CONFIG not set as expected"
    return 1
  fi

  if ! brand_require_active_brand_log "${start_time}"; then
    brand_emergency_rollback "startup brand log missing"
    return 1
  fi

  brand_log "activate-${BRAND_LABEL}-config: remote activation OK (legacy ${REMOTE_LEGACY_CONFIG} unchanged)"
  return 0
}

brand_deploy_config_file() {
  local new_path="${1:?uploaded .new path required}"
  brand_refresh_derived || return 1
  local final_path="${REMOTE_EXPLICIT_CONFIG}"
  local user group

  if [[ ! -f "${new_path}" ]]; then
    brand_err "deploy-${BRAND_LABEL}-config: missing upload ${new_path}"
    return 1
  fi
  if [[ ! -f "${REMOTE_LEGACY_CONFIG}" ]]; then
    brand_err "deploy-${BRAND_LABEL}-config: legacy config missing: ${REMOTE_LEGACY_CONFIG}"
    return 1
  fi

  user="$(brand_unit_user)"
  group="$(brand_unit_group)"
  brand_log "deploy-${BRAND_LABEL}-config: unit user=${user} group=${group}"

  chown "${user}:${group}" "${new_path}"
  chmod 0600 "${new_path}"
  mv "${new_path}" "${final_path}"
  chmod 0600 "${final_path}"
  chown "${user}:${group}" "${final_path}"

  if command -v runuser >/dev/null 2>&1; then
    if ! runuser -u "${user}" -- test -r "${final_path}"; then
      brand_err "deploy-${BRAND_LABEL}-config: ${final_path} not readable by ${user}"
      return 1
    fi
  elif command -v su >/dev/null 2>&1; then
    if ! su -s /bin/sh "${user}" -c "test -r $(printf %q "${final_path}")"; then
      brand_err "deploy-${BRAND_LABEL}-config: ${final_path} not readable by ${user}"
      return 1
    fi
  else
    brand_err "deploy-${BRAND_LABEL}-config: neither runuser nor su available to verify readability"
    return 1
  fi

  brand_log "deploy-${BRAND_LABEL}-config: installed ${final_path} (legacy ${REMOTE_LEGACY_CONFIG} unchanged; service not restarted)"
}

brand_binary_owner_group() {
  local user group
  if [[ -f "${REMOTE_BINARY}" ]]; then
    user="$(stat -c '%U' "${REMOTE_BINARY}" 2>/dev/null || true)"
    group="$(stat -c '%G' "${REMOTE_BINARY}" 2>/dev/null || true)"
  fi
  if [[ -z "${user}" ]]; then
    user="$(brand_unit_user)"
  fi
  if [[ -z "${group}" ]]; then
    group="$(brand_unit_group)"
  fi
  printf '%s %s\n' "${user}" "${group}"
}

brand_cleanup_binary_temps() {
  if [[ -n "${REMOTE_BINARY:-}" ]]; then
    rm -f "${REMOTE_BINARY}.new" "${REMOTE_BINARY}.rollback.new"
  fi
}

# Restore from an exact backup path (automatic rollback). No marker fallback.
brand_rollback_binary() {
  brand_require_vars SERVICE_NAME REMOTE_BINARY BRAND_LABEL REMOTE_DIR || return 1
  local bak="${1:-}"
  if [[ -z "${bak}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback requires exact backup path"
    return 1
  fi
  if [[ ! -f "${bak}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback failed: backup not found: ${bak}"
    return 1
  fi

  local user group tmp=""
  read -r user group <<<"$(brand_binary_owner_group)"
  tmp="${REMOTE_BINARY}.rollback.new"
  rm -f "${tmp}"

  cleanup_rollback_tmp() {
    rm -f "${tmp}"
  }
  trap cleanup_rollback_tmp EXIT

  cp -a "${bak}" "${tmp}"
  chown "${user}:${group}" "${tmp}"
  chmod 0755 "${tmp}"
  mv "${tmp}" "${REMOTE_BINARY}"
  tmp=""
  trap - EXIT
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  brand_cleanup_binary_temps

  # Restored previous binary may predate new startup log markers.
  # Only require systemd stability (active → sleep → active), not journal text.
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback unit not active after restart"
    return 1
  fi
  sleep 2
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback unit not active after stabilization"
    return 1
  fi
  brand_log "deploy-${BRAND_LABEL}: previous binary restored and unit is stable"
  return 0
}

# Manual / post-deploy rollback using ${REMOTE_DIR}/.vpnbot-last-binary-bak only.
brand_rollback_binary_from_marker() {
  brand_require_vars REMOTE_DIR BRAND_LABEL || return 1
  local marker="${REMOTE_DIR}/.vpnbot-last-binary-bak"
  if [[ ! -f "${marker}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback marker missing: ${marker}"
    return 1
  fi
  local bak
  bak="$(tr -d '\n' <"${marker}")"
  if [[ -z "${bak}" || ! -f "${bak}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback marker points to missing backup"
    return 1
  fi
  brand_rollback_binary "${bak}"
}

# On deploy failure: attempt exact-backup rollback; never suppress rollback status.
brand_fail_binary_deploy() {
  local reason="${1:-unknown failure}"
  brand_err "deploy-${BRAND_LABEL}: ${reason}"
  brand_cleanup_binary_temps

  if [[ -z "${BINARY_BACKUP}" || ! -f "${BINARY_BACKUP}" ]]; then
    brand_err "CRITICAL: ${BRAND_LABEL} binary deployment failed and automatic binary rollback failed"
    brand_safe_journal_tail
    return 1
  fi

  if brand_rollback_binary "${BINARY_BACKUP}"; then
    brand_err "deploy-${BRAND_LABEL}: deployment failed; previous binary restored"
    return 1
  fi

  brand_err "CRITICAL: ${BRAND_LABEL} binary deployment failed and automatic binary rollback failed"
  brand_safe_journal_tail
  return 1
}

# Require VPNBOT_CONFIG=<explicit>, explicit config file, and matching managed drop-in.
brand_require_explicit_mode_active() {
  brand_refresh_derived || return 1
  if [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: explicit config is not active; use brand-rollout"
    return 1
  fi
  if ! brand_dropin_matches; then
    brand_err "deploy-${BRAND_LABEL}: explicit config is not active; use brand-rollout"
    return 1
  fi
  if ! brand_env_has_expected; then
    brand_err "deploy-${BRAND_LABEL}: explicit config is not active; use brand-rollout"
    return 1
  fi
  return 0
}

brand_systemctl_unit_exists() {
  local state
  state="$(systemctl show "${SERVICE_NAME}" -p LoadState --value 2>/dev/null || true)"
  if [[ "${state}" == "loaded" ]]; then
    return 0
  fi
  systemctl cat "${SERVICE_NAME}" >/dev/null 2>&1
}

brand_run_configcheck_as_unit() {
  local configcheck_bin="${1:?configcheck binary required}"
  local config_path="${2:?config path required}"
  local user summary

  if [[ ! -f "${configcheck_bin}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: configcheck not found: ${configcheck_bin}"
    return 1
  fi
  if [[ ! -f "${config_path}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: config not found: ${config_path}"
    return 1
  fi
  chmod 0700 "${configcheck_bin}" 2>/dev/null || true

  user="$(brand_unit_user)"
  if command -v runuser >/dev/null 2>&1; then
    if ! summary="$(runuser -u "${user}" -- "${configcheck_bin}" -config "${config_path}")"; then
      brand_err "rollout-${BRAND_LABEL}: configcheck failed for ${config_path}"
      return 1
    fi
  elif command -v su >/dev/null 2>&1; then
    if ! summary="$(su -s /bin/sh "${user}" -c \
      "$(printf '%s -config %s' "$(printf %q "${configcheck_bin}")" "$(printf %q "${config_path}")")")"; then
      brand_err "rollout-${BRAND_LABEL}: configcheck failed for ${config_path}"
      return 1
    fi
  else
    brand_err "rollout-${BRAND_LABEL}: neither runuser nor su available for configcheck"
    return 1
  fi
  printf '%s' "${summary}"
  if ! grep -Fxq "brand.id=${EXPECTED_BRAND_ID}" <<<"${summary}"; then
    brand_err "rollout-${BRAND_LABEL}: configcheck brand.id != ${EXPECTED_BRAND_ID}"
    return 1
  fi
  return 0
}

brand_rollout_tx_write() {
  brand_require_vars ROLLOUT_TX_DIR TX_ID SERVICE_NAME EXPECTED_BRAND_ID BRAND_LABEL || return 1
  local f="${ROLLOUT_TX_DIR}/tx.env"
  {
    printf 'TX_ID=%q\n' "${TX_ID}"
    printf 'SERVICE_NAME=%q\n' "${SERVICE_NAME}"
    printf 'EXPECTED_BRAND_ID=%q\n' "${EXPECTED_BRAND_ID}"
    printf 'BRAND_LABEL=%q\n' "${BRAND_LABEL}"
    printf 'STAGE_CONFIG_INSTALLED=%q\n' "${STAGE_CONFIG_INSTALLED:-0}"
    printf 'STAGE_BINARY_INSTALLED=%q\n' "${STAGE_BINARY_INSTALLED:-0}"
    printf 'STAGE_DROPIN_INSTALLED=%q\n' "${STAGE_DROPIN_INSTALLED:-0}"
    printf 'STAGE_RESTARTED=%q\n' "${STAGE_RESTARTED:-0}"
    printf 'PREV_ENV_PRESENT=%q\n' "${PREV_ENV_PRESENT:-0}"
    printf 'PREV_CONFIG_EXISTED=%q\n' "${PREV_CONFIG_EXISTED:-0}"
    printf 'PREV_DROPIN_EXISTED=%q\n' "${PREV_DROPIN_EXISTED:-0}"
    printf 'BINARY_BACKUP=%q\n' "${BINARY_BACKUP:-}"
    printf 'ROLLOUT_START_TIME=%q\n' "${ROLLOUT_START_TIME:-}"
    printf 'ROLLOUT_COMPLETED=%q\n' "${ROLLOUT_COMPLETED:-0}"
  } >"${f}"
}

brand_rollout_tx_init() {
  brand_refresh_derived || return 1
  brand_require_vars ROLLOUT_TX_DIR SERVICE_NAME EXPECTED_BRAND_ID BRAND_LABEL || return 1
  mkdir -p "${ROLLOUT_TX_DIR}/backups" "${ROLLOUT_TX_DIR}/stages"
  TX_ID="$(date +%Y%m%d-%H%M%S)"
  STAGE_CONFIG_INSTALLED=0
  STAGE_BINARY_INSTALLED=0
  STAGE_DROPIN_INSTALLED=0
  STAGE_RESTARTED=0
  PREV_ENV_PRESENT=0
  PREV_CONFIG_EXISTED=0
  PREV_DROPIN_EXISTED=0
  BINARY_BACKUP=""
  ROLLOUT_START_TIME=""
  ROLLOUT_COMPLETED=0
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: transaction ${TX_ID} initialized"
}

brand_rollout_tx_load() {
  brand_require_vars ROLLOUT_TX_DIR || return 1
  local f="${ROLLOUT_TX_DIR}/tx.env"
  if [[ ! -f "${f}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing transaction state: ${f}"
    return 1
  fi
  # shellcheck disable=SC1090
  source "${f}"
}

brand_rollout_tx_set() {
  local key="${1:?tx key required}" val="${2:?tx value required}"
  case "${key}" in
    STAGE_CONFIG_INSTALLED|STAGE_BINARY_INSTALLED|STAGE_DROPIN_INSTALLED|STAGE_RESTARTED|PREV_ENV_PRESENT|PREV_CONFIG_EXISTED|PREV_DROPIN_EXISTED|BINARY_BACKUP|ROLLOUT_START_TIME|ROLLOUT_COMPLETED) ;;
    *)
      brand_err "rollout-${BRAND_LABEL}: invalid transaction key: ${key}"
      return 1
      ;;
  esac
  brand_rollout_tx_load || return 1
  printf -v "${key}" '%s' "${val}"
  brand_rollout_tx_write || return 1
}

brand_rollout_preflight() {
  local uploaded_binary="${1:?uploaded binary required}"
  local uploaded_config="${2:?uploaded config required}"
  local uploaded_configcheck="${3:?uploaded configcheck required}"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  if ! brand_systemctl_unit_exists; then
    brand_err "rollout-${BRAND_LABEL}: systemd unit not found: ${SERVICE_NAME}"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} is not active"
    return 1
  fi
  sleep 1
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} became inactive during preflight"
    return 1
  fi

  if [[ ! -f "${REMOTE_BINARY}" || ! -x "${REMOTE_BINARY}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing or non-executable binary: ${REMOTE_BINARY}"
    return 1
  fi
  if [[ ! -f "${REMOTE_LEGACY_CONFIG}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: legacy config missing: ${REMOTE_LEGACY_CONFIG}"
    return 1
  fi
  if [[ ! -f "${uploaded_binary}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing uploaded binary: ${uploaded_binary}"
    return 1
  fi
  if [[ ! -f "${uploaded_config}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing uploaded config: ${uploaded_config}"
    return 1
  fi
  if [[ ! -f "${uploaded_configcheck}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing uploaded configcheck: ${uploaded_configcheck}"
    return 1
  fi

  if [[ -f "${DROPIN_FILE}" ]] && ! brand_dropin_matches; then
    brand_err "rollout-${BRAND_LABEL}: refusing rollout with unexpected drop-in content: ${DROPIN_FILE}"
    return 1
  fi

  if ! brand_run_configcheck_as_unit "${uploaded_configcheck}" "${uploaded_config}" >/dev/null; then
    return 1
  fi

  if brand_env_has_expected; then
    PREV_ENV_PRESENT=1
  else
    PREV_ENV_PRESENT=0
  fi
  brand_rollout_tx_set PREV_ENV_PRESENT "${PREV_ENV_PRESENT}" || return 1
  brand_log "rollout-${BRAND_LABEL}: preflight OK"
}

brand_rollout_backup() {
  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  if [[ -f "${REMOTE_BINARY}" ]]; then
    BINARY_BACKUP="${REMOTE_BINARY}.bak.${TX_ID}"
    cp -a "${REMOTE_BINARY}" "${BINARY_BACKUP}"
    printf '%s\n' "${BINARY_BACKUP}" >"${REMOTE_DIR}/.vpnbot-last-binary-bak"
    chmod 0600 "${REMOTE_DIR}/.vpnbot-last-binary-bak"
    brand_log "rollout-${BRAND_LABEL}: binary backup ${BINARY_BACKUP}"
  else
    BINARY_BACKUP=""
  fi

  if [[ -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    PREV_CONFIG_EXISTED=1
    cp -a "${REMOTE_EXPLICIT_CONFIG}" "${ROLLOUT_TX_DIR}/backups/config.bak"
  else
    PREV_CONFIG_EXISTED=0
  fi

  if [[ -f "${DROPIN_FILE}" ]] && brand_dropin_matches; then
    PREV_DROPIN_EXISTED=1
    cp -a "${DROPIN_FILE}" "${ROLLOUT_TX_DIR}/backups/dropin.bak"
  else
    PREV_DROPIN_EXISTED=0
  fi

  # Persist all backup fields in one write — brand_rollout_tx_set reloads tx.env and
  # would clobber sibling shell variables set in this function.
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: backup complete"
}

brand_rollout_install_config() {
  local uploaded_config="${1:?uploaded config required}"
  local user group final_path="${REMOTE_EXPLICIT_CONFIG}" new_path="${REMOTE_EXPLICIT_CONFIG}.new"

  if [[ ! -f "${uploaded_config}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing uploaded config: ${uploaded_config}"
    return 1
  fi

  user="$(brand_unit_user)"
  group="$(brand_unit_group)"
  cp -a "${uploaded_config}" "${new_path}"
  chown "${user}:${group}" "${new_path}"
  chmod 0600 "${new_path}"

  # Verify readability on the staging file before the atomic replace so a
  # failed check cannot leave a half-applied production config.
  if command -v runuser >/dev/null 2>&1; then
    if ! runuser -u "${user}" -- test -r "${new_path}"; then
      rm -f "${new_path}"
      brand_err "rollout-${BRAND_LABEL}: ${new_path} not readable by ${user}"
      return 1
    fi
  elif command -v su >/dev/null 2>&1; then
    if ! su -s /bin/sh "${user}" -c "test -r $(printf %q "${new_path}")"; then
      rm -f "${new_path}"
      brand_err "rollout-${BRAND_LABEL}: ${new_path} not readable by ${user}"
      return 1
    fi
  else
    rm -f "${new_path}"
    brand_err "rollout-${BRAND_LABEL}: neither runuser nor su available to verify readability"
    return 1
  fi

  mv "${new_path}" "${final_path}"
  chmod 0600 "${final_path}"
  chown "${user}:${group}" "${final_path}"
  return 0
}

brand_restore_binary_atomic() {
  local bak="${1:?backup path required}"
  local user group tmp=""

  if [[ ! -f "${bak}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: binary restore backup not found: ${bak}"
    return 1
  fi

  read -r user group <<<"$(brand_binary_owner_group)"
  tmp="${REMOTE_BINARY}.rollback.new"
  rm -f "${tmp}"

  cleanup_restore_tmp() {
    rm -f "${tmp}"
  }
  trap cleanup_restore_tmp EXIT

  cp -a "${bak}" "${tmp}"
  chown "${user}:${group}" "${tmp}"
  chmod 0755 "${tmp}"
  mv "${tmp}" "${REMOTE_BINARY}"
  tmp=""
  trap - EXIT
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  brand_cleanup_binary_temps
  return 0
}

brand_rollout_install() {
  local uploaded_binary="${1:?uploaded binary required}"
  local uploaded_config="${2:?uploaded config required}"
  local user group new_path="${REMOTE_BINARY}.new"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  if ! brand_rollout_install_config "${uploaded_config}"; then
    return 1
  fi
  brand_rollout_tx_set STAGE_CONFIG_INSTALLED 1 || return 1

  if [[ ! -f "${uploaded_binary}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing uploaded binary: ${uploaded_binary}"
    return 1
  fi
  read -r user group <<<"$(brand_binary_owner_group)"
  cp -a "${uploaded_binary}" "${new_path}"
  chown "${user}:${group}" "${new_path}"
  chmod 0755 "${new_path}"
  mv "${new_path}" "${REMOTE_BINARY}"
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  brand_cleanup_binary_temps
  brand_rollout_tx_set STAGE_BINARY_INSTALLED 1 || return 1

  if ! brand_install_dropin_atomic; then
    return 1
  fi
  brand_rollout_tx_set STAGE_DROPIN_INSTALLED 1 || return 1

  ROLLOUT_START_TIME="$(date '+%Y-%m-%d %H:%M:%S')"
  brand_rollout_tx_set ROLLOUT_START_TIME "${ROLLOUT_START_TIME}" || return 1

  if ! systemctl daemon-reload; then
    brand_err "rollout-${BRAND_LABEL}: daemon-reload failed"
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: restart failed"
    return 1
  fi
  brand_rollout_tx_set STAGE_RESTARTED 1 || return 1
  brand_log "rollout-${BRAND_LABEL}: install complete (single restart)"
}

brand_rollout_verify() {
  local uploaded_configcheck="${1:?uploaded configcheck required}"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} is not active after restart"
    return 1
  fi
  sleep 2
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} became inactive after stabilization"
    return 1
  fi

  if ! brand_env_has_expected; then
    brand_err "rollout-${BRAND_LABEL}: ${EXPECTED_ENV} not active on ${SERVICE_NAME}"
    return 1
  fi

  if [[ -z "${ROLLOUT_START_TIME:-}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing rollout start time for log verification"
    return 1
  fi
  if ! brand_require_startup_log "${ROLLOUT_START_TIME}"; then
    return 1
  fi

  if ! brand_run_configcheck_as_unit "${uploaded_configcheck}" "${REMOTE_EXPLICIT_CONFIG}" >/dev/null; then
    return 1
  fi

  brand_log "rollout-${BRAND_LABEL}: verify OK"
  return 0
}

brand_rollout_rollback() {
  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  local restore_failed=0

  if [[ -n "${BINARY_BACKUP:-}" && -f "${BINARY_BACKUP}" ]]; then
    if ! brand_restore_binary_atomic "${BINARY_BACKUP}"; then
      restore_failed=1
    fi
  fi

  if [[ "${PREV_CONFIG_EXISTED:-0}" == "1" ]]; then
    if [[ -f "${ROLLOUT_TX_DIR}/backups/config.bak" ]]; then
      cp -a "${ROLLOUT_TX_DIR}/backups/config.bak" "${REMOTE_EXPLICIT_CONFIG}"
      chmod 0600 "${REMOTE_EXPLICIT_CONFIG}"
      chown "$(brand_unit_user):$(brand_unit_group)" "${REMOTE_EXPLICIT_CONFIG}"
    else
      brand_err "CRITICAL: rollout-${BRAND_LABEL}: config backup missing during rollback"
      restore_failed=1
    fi
  elif [[ "${STAGE_CONFIG_INSTALLED:-0}" == "1" && -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    rm -f "${REMOTE_EXPLICIT_CONFIG}"
  fi

  if [[ "${PREV_DROPIN_EXISTED:-0}" == "1" ]]; then
    if [[ -f "${ROLLOUT_TX_DIR}/backups/dropin.bak" ]]; then
      cp -a "${ROLLOUT_TX_DIR}/backups/dropin.bak" "${DROPIN_FILE}"
      chmod 0644 "${DROPIN_FILE}"
    else
      brand_err "CRITICAL: rollout-${BRAND_LABEL}: drop-in backup missing during rollback"
      restore_failed=1
    fi
  elif [[ "${STAGE_DROPIN_INSTALLED:-0}" == "1" ]]; then
    if [[ -f "${DROPIN_FILE}" ]]; then
      if brand_dropin_matches; then
        rm -f "${DROPIN_FILE}"
      else
        brand_err "CRITICAL: rollout-${BRAND_LABEL}: refusing blind drop-in delete (unexpected content)"
        restore_failed=1
      fi
    fi
  fi

  if [[ "${restore_failed}" -eq 1 ]]; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi

  if ! systemctl daemon-reload; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi
  sleep 2
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi

  if [[ "${PREV_ENV_PRESENT:-0}" == "1" ]]; then
    if ! brand_env_has_expected; then
      brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
      brand_safe_journal_tail
      return 1
    fi
  elif ! brand_assert_expected_env_absent; then
    brand_err "CRITICAL: ${BRAND_LABEL} rollout failed and automatic rollback failed"
    brand_safe_journal_tail
    return 1
  fi

  brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
  return 1
}

brand_rollout_finalize() {
  brand_rollout_tx_load || return 1
  brand_rollout_tx_set ROLLOUT_COMPLETED 1 || return 1
  rm -f "${ROLLOUT_TX_DIR}/backups/config.bak" "${ROLLOUT_TX_DIR}/backups/dropin.bak"
  if [[ -n "${REMOTE_TMP:-}" ]]; then
    rm -f "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" "${REMOTE_TMP}/bot"
  fi
  brand_log "rollout-${BRAND_LABEL}: finalized (binary backup retained)"
  return 0
}

brand_rollout_run() {
  local uploaded_binary="${1:?uploaded binary required}"
  local uploaded_config="${2:?uploaded config required}"
  local uploaded_configcheck="${3:?uploaded configcheck required}"

  brand_refresh_derived || return 1
  brand_require_vars ROLLOUT_TX_DIR || return 1

  if ! brand_rollout_tx_init; then
    return 1
  fi
  if ! brand_rollout_preflight "${uploaded_binary}" "${uploaded_config}" "${uploaded_configcheck}"; then
    return 1
  fi
  if ! brand_rollout_backup; then
    return 1
  fi
  if ! brand_rollout_install "${uploaded_binary}" "${uploaded_config}"; then
    brand_rollout_tx_load || true
    if [[ "${STAGE_CONFIG_INSTALLED:-0}" == "1" ||
          "${STAGE_BINARY_INSTALLED:-0}" == "1" ||
          "${STAGE_DROPIN_INSTALLED:-0}" == "1" ||
          "${STAGE_RESTARTED:-0}" == "1" ]]; then
      brand_rollout_rollback || true
    else
      brand_err "rollout-${BRAND_LABEL}: install failed before mutation; previous state unchanged"
    fi
    return 1
  fi
  if ! brand_rollout_verify "${uploaded_configcheck}"; then
    brand_rollout_rollback || true
    return 1
  fi

  brand_log "rollout-${BRAND_LABEL}: coordinated rollout OK"
  return 0
}

# Install uploaded REMOTE_BINARY.new, restart, verify explicit-brand startup logs.
# The new binary requires an explicit brand in the active config; if the active config
# is still legacy (no brand), startup fails validation and the binary is rolled back.
# Coordinated binary+config+drop-in rollout is handled by brand_rollout_run.
brand_deploy_binary() {
  brand_refresh_derived || return 1
  brand_require_vars REMOTE_BINARY || return 1
  if ! brand_require_explicit_mode_active; then
    return 1
  fi

  local new_path="${REMOTE_BINARY}.new"
  if [[ ! -f "${new_path}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: missing ${new_path}"
    return 1
  fi

  local user group
  read -r user group <<<"$(brand_binary_owner_group)"

  BINARY_BACKUP=""
  BINARY_REPLACED=0
  if [[ -f "${REMOTE_BINARY}" ]]; then
    BINARY_BACKUP="${REMOTE_BINARY}.bak.$(date +%Y%m%d-%H%M%S)"
    cp -a "${REMOTE_BINARY}" "${BINARY_BACKUP}"
    printf '%s\n' "${BINARY_BACKUP}" >"${REMOTE_DIR}/.vpnbot-last-binary-bak"
    chmod 0600 "${REMOTE_DIR}/.vpnbot-last-binary-bak"
    brand_log "deploy-${BRAND_LABEL}: backup ${BINARY_BACKUP}"
  fi

  chown "${user}:${group}" "${new_path}"
  chmod 0755 "${new_path}"
  mv "${new_path}" "${REMOTE_BINARY}"
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  BINARY_REPLACED=1
  brand_cleanup_binary_temps

  local start_time
  start_time="$(date '+%Y-%m-%d %H:%M:%S')"

  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_fail_binary_deploy "restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_fail_binary_deploy "unit not active after restart"
    return 1
  fi
  sleep 2
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_fail_binary_deploy "unit inactive after stabilization"
    return 1
  fi
  if ! brand_require_startup_log "${start_time}"; then
    brand_fail_binary_deploy "startup log check failed"
    return 1
  fi

  brand_log "deploy-${BRAND_LABEL}: binary OK (explicit brand startup verified)"
  return 0
}
