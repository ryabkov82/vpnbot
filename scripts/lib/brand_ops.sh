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

brand_rollback_to_legacy() {
  brand_refresh_derived || return 1
  rm -f "${DROPIN_FILE}"
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

brand_require_active_brand_log() {
  local since="${1:?}"
  local needle="active brand: id=${EXPECTED_BRAND_ID}"
  if ! journalctl -u "${SERVICE_NAME}" --since "${since}" --no-pager 2>/dev/null |
    grep -Fq "${needle}"; then
    brand_err "activate-${BRAND_LABEL}-config: startup log missing '${needle}' since ${since}"
    return 1
  fi
  return 0
}

# Legacy binary deploy: any active brand line + telegram configured (id may still be synthesized vff).
brand_require_legacy_startup_log() {
  local since="${1:?}"
  local log
  log="$(journalctl -u "${SERVICE_NAME}" --since "${since}" --no-pager 2>/dev/null || true)"
  if ! grep -Fq 'active brand:' <<<"${log}"; then
    brand_err "deploy-${BRAND_LABEL}: startup log missing 'active brand:' since ${since}"
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

brand_rollback_binary() {
  brand_require_vars SERVICE_NAME REMOTE_BINARY BRAND_LABEL || return 1
  local bak="${1:-${BINARY_BACKUP}}"
  if [[ -z "${bak}" || ! -f "${bak}" ]]; then
    local marker="${REMOTE_DIR}/.vpnbot-last-binary-bak"
    if [[ -f "${marker}" ]]; then
      bak="$(cat "${marker}")"
    fi
  fi
  if [[ -z "${bak}" || ! -f "${bak}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback failed: backup not found"
    return 1
  fi
  local user group
  read -r user group <<<"$(brand_binary_owner_group)"
  cp -a "${bak}" "${REMOTE_BINARY}"
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: binary rollback unit not active"
    return 1
  fi
  brand_log "deploy-${BRAND_LABEL}: restored binary from ${bak}"
  return 0
}

# Install uploaded REMOTE_BINARY.new, restart, verify legacy startup logs. No drop-in.
brand_deploy_binary() {
  brand_refresh_derived || return 1
  brand_require_vars REMOTE_BINARY || return 1

  local new_path="${REMOTE_BINARY}.new"
  if [[ ! -f "${new_path}" ]]; then
    brand_err "deploy-${BRAND_LABEL}: missing ${new_path}"
    return 1
  fi

  local user group
  read -r user group <<<"$(brand_binary_owner_group)"

  if [[ -f "${REMOTE_BINARY}" ]]; then
    BINARY_BACKUP="${REMOTE_BINARY}.bak.$(date +%Y%m%d-%H%M%S)"
    cp -a "${REMOTE_BINARY}" "${BINARY_BACKUP}"
    printf '%s\n' "${BINARY_BACKUP}" >"${REMOTE_DIR}/.vpnbot-last-binary-bak"
    chmod 0600 "${REMOTE_DIR}/.vpnbot-last-binary-bak"
    brand_log "deploy-${BRAND_LABEL}: backup ${BINARY_BACKUP}"
  else
    BINARY_BACKUP=""
  fi

  chown "${user}:${group}" "${new_path}"
  chmod 0755 "${new_path}"
  mv "${new_path}" "${REMOTE_BINARY}"
  chown "${user}:${group}" "${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  BINARY_REPLACED=1

  local start_time
  start_time="$(date '+%Y-%m-%d %H:%M:%S')"

  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: restart failed; rolling back binary"
    brand_rollback_binary || true
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: unit not active; rolling back binary"
    brand_rollback_binary || true
    return 1
  fi
  sleep 2
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "deploy-${BRAND_LABEL}: unit inactive after stabilization; rolling back binary"
    brand_rollback_binary || true
    return 1
  fi
  if ! brand_require_legacy_startup_log "${start_time}"; then
    brand_err "deploy-${BRAND_LABEL}: startup log check failed; rolling back binary"
    brand_rollback_binary || true
    return 1
  fi

  brand_log "deploy-${BRAND_LABEL}: binary OK (legacy config ${REMOTE_LEGACY_CONFIG}; no drop-in)"
  return 0
}
