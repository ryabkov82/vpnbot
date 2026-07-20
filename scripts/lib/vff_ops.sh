#!/usr/bin/env bash
# Shared VFF systemd helpers (no secrets). Sourced by remote scripts and tests.
# shellcheck shell=bash

SERVICE_NAME="${SERVICE_NAME:-bot.service}"
REMOTE_DIR="${REMOTE_DIR:-/opt/bot}"
REMOTE_CONFIG_VFF="${REMOTE_CONFIG_VFF:-${REMOTE_DIR}/config-vff.json}"
REMOTE_CONFIG_LEGACY="${REMOTE_CONFIG_LEGACY:-${REMOTE_DIR}/config.json}"
DROPIN_DIR="${DROPIN_DIR:-/etc/systemd/system/${SERVICE_NAME}.d}"
DROPIN_FILE="${DROPIN_FILE:-${DROPIN_DIR}/10-vpnbot-config.conf}"
EXPECTED_ENV="${EXPECTED_ENV:-VPNBOT_CONFIG=${REMOTE_CONFIG_VFF}}"
DROPIN_BODY="$(printf '%s\n' '[Service]' "Environment=${EXPECTED_ENV}")"

# Set to 1 after this activation run accepts/installs the managed drop-in.
VFF_DROPIN_ACTIVE=0

vff_log() {
  printf '%s\n' "$*"
}

vff_err() {
  printf '%s\n' "$*" >&2
}

vff_unit_user() {
  local user
  user="$(systemctl show "${SERVICE_NAME}" -p User --value 2>/dev/null || true)"
  if [[ -z "${user}" ]]; then
    user=root
  fi
  printf '%s\n' "${user}"
}

vff_unit_group() {
  local group user
  group="$(systemctl show "${SERVICE_NAME}" -p Group --value 2>/dev/null || true)"
  if [[ -z "${group}" ]]; then
    user="$(vff_unit_user)"
    group="$(id -gn "${user}")"
  fi
  printf '%s\n' "${group}"
}

vff_env_has_expected() {
  systemctl show "${SERVICE_NAME}" -p Environment --value 2>/dev/null |
    tr ' ' '\n' |
    grep -Fxq "${EXPECTED_ENV}"
}

vff_print_expected_env_only() {
  if vff_env_has_expected; then
    printf '%s\n' "${EXPECTED_ENV}"
    return 0
  fi
  return 1
}

vff_assert_expected_env_absent() {
  if vff_env_has_expected; then
    vff_err "rollback: ${EXPECTED_ENV} still present on ${SERVICE_NAME}"
    return 1
  fi
  return 0
}

vff_dropin_matches() {
  [[ -f "${DROPIN_FILE}" ]] || return 1
  local current
  current="$(cat "${DROPIN_FILE}")"
  [[ "${current}" == "${DROPIN_BODY}" ]]
}

vff_install_dropin_atomic() {
  mkdir -p "${DROPIN_DIR}"
  local tmp="${DROPIN_DIR}/10-vpnbot-config.conf.new"
  # Write via temp in same directory for atomic rename.
  printf '%s\n' "${DROPIN_BODY}" >"${tmp}"
  chmod 0644 "${tmp}"
  mv "${tmp}" "${DROPIN_FILE}"
  VFF_DROPIN_ACTIVE=1
}

vff_ensure_managed_dropin() {
  if [[ -f "${DROPIN_FILE}" ]]; then
    if vff_dropin_matches; then
      vff_log "activate-vff-config: managed drop-in already correct (idempotent)"
      VFF_DROPIN_ACTIVE=1
      return 0
    fi
    vff_err "activate-vff-config: refusing to overwrite existing drop-in with unexpected content: ${DROPIN_FILE}"
    return 1
  fi
  vff_install_dropin_atomic
  vff_log "activate-vff-config: installed managed drop-in ${DROPIN_FILE}"
}

vff_safe_journal_tail() {
  journalctl -u "${SERVICE_NAME}" -n 40 --no-pager 2>/dev/null |
    grep -E 'active brand:|telegram bot configured|configcheck|FATAL|panic|error|Error' || true
}

# Remove managed drop-in and return unit to legacy config search.
vff_rollback_to_legacy() {
  rm -f "${DROPIN_FILE}"
  if ! systemctl daemon-reload; then
    vff_err "rollback: daemon-reload failed"
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    vff_err "rollback: restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    vff_err "rollback: ${SERVICE_NAME} is not active"
    return 1
  fi
  sleep 1
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    vff_err "rollback: ${SERVICE_NAME} became inactive after stabilization wait"
    return 1
  fi
  if ! vff_assert_expected_env_absent; then
    return 1
  fi
  vff_log "rollback-vff-config: legacy active; managed drop-in removed; ${REMOTE_CONFIG_VFF} left in place"
}

vff_emergency_rollback() {
  local reason="${1:-unknown}"
  vff_err "activate-vff-config: activation failed (${reason}); rolling back to legacy"
  if ! vff_rollback_to_legacy; then
    vff_err "CRITICAL: VFF activation failed and automatic rollback failed"
    vff_safe_journal_tail
    return 1
  fi
  return 1
}

vff_require_active_brand_log() {
  local since="${1:?}"
  if ! journalctl -u "${SERVICE_NAME}" --since "${since}" --no-pager 2>/dev/null |
    grep -Fq 'active brand: id=vff'; then
    vff_err "activate-vff-config: startup log missing 'active brand: id=vff' since ${since}"
    return 1
  fi
  return 0
}

# Full remote activation (configcheck → drop-in → restart → checks).
# Does not run public smoke (caller/Make does that).
vff_activate() {
  local configcheck_bin="${1:?configcheck binary required}"

  if [[ ! -x "${configcheck_bin}" && ! -f "${configcheck_bin}" ]]; then
    vff_err "activate-vff-config: configcheck not found: ${configcheck_bin}"
    return 1
  fi
  chmod 0700 "${configcheck_bin}" 2>/dev/null || true

  if [[ ! -f "${REMOTE_CONFIG_VFF}" ]]; then
    vff_err "activate-vff-config: missing ${REMOTE_CONFIG_VFF}"
    return 1
  fi

  # Fail closed before touching systemd if config is invalid.
  if ! "${configcheck_bin}" -config "${REMOTE_CONFIG_VFF}"; then
    vff_err "activate-vff-config: configcheck failed (drop-in not installed)"
    return 1
  fi

  if ! vff_ensure_managed_dropin; then
    return 1
  fi

  local start_time
  start_time="$(date '+%Y-%m-%d %H:%M:%S')"

  if ! systemctl daemon-reload; then
    vff_emergency_rollback "daemon-reload failed"
    return 1
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    vff_emergency_rollback "restart failed"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    vff_emergency_rollback "unit not active after restart"
    return 1
  fi

  sleep 2

  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    vff_emergency_rollback "unit not active after stabilization"
    return 1
  fi

  if ! vff_print_expected_env_only; then
    vff_emergency_rollback "VPNBOT_CONFIG not set as expected"
    return 1
  fi

  if ! vff_require_active_brand_log "${start_time}"; then
    vff_emergency_rollback "startup brand log missing"
    return 1
  fi

  vff_log "activate-vff-config: remote activation OK (legacy ${REMOTE_CONFIG_LEGACY} unchanged)"
  return 0
}

# Arg: path to uploaded *.new file (atomic replace of REMOTE_CONFIG_VFF).
vff_deploy_config_file() {
  local new_path="${1:?uploaded .new path required}"
  local final_path="${REMOTE_CONFIG_VFF}"
  local user group

  if [[ ! -f "${new_path}" ]]; then
    vff_err "deploy-vff-config: missing upload ${new_path}"
    return 1
  fi
  if [[ ! -f "${REMOTE_CONFIG_LEGACY}" ]]; then
    vff_err "deploy-vff-config: legacy config missing: ${REMOTE_CONFIG_LEGACY}"
    return 1
  fi

  user="$(vff_unit_user)"
  group="$(vff_unit_group)"
  vff_log "deploy-vff-config: unit user=${user} group=${group}"

  chown "${user}:${group}" "${new_path}"
  chmod 0600 "${new_path}"
  mv "${new_path}" "${final_path}"
  chmod 0600 "${final_path}"
  chown "${user}:${group}" "${final_path}"

  if command -v runuser >/dev/null 2>&1; then
    if ! runuser -u "${user}" -- test -r "${final_path}"; then
      vff_err "deploy-vff-config: ${final_path} not readable by ${user}"
      return 1
    fi
  elif command -v su >/dev/null 2>&1; then
    if ! su -s /bin/sh "${user}" -c "test -r $(printf %q "${final_path}")"; then
      vff_err "deploy-vff-config: ${final_path} not readable by ${user}"
      return 1
    fi
  else
    vff_err "deploy-vff-config: neither runuser nor su available to verify readability"
    return 1
  fi

  vff_log "deploy-vff-config: installed ${final_path} (legacy ${REMOTE_CONFIG_LEGACY} unchanged; service not restarted)"
}
