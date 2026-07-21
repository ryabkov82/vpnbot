#!/usr/bin/env bash
# Hardened coordinated brand rollout (binary + explicit config + drop-in).
# Sourced by brand_ops.sh. No secrets. No eval of untrusted input.
# shellcheck shell=bash

# Remote exit-code contract (also used by local orchestrator):
#   0  = remote rollout OK, pending smoke, lock held
#  10  = safe abort before mutation, lock released
#  20  = failure with successful rollback, lock released
#  30  = CRITICAL, recovery required, lock held
#  40  = lock busy, no mutation
ROLLOUT_RC_OK=0
ROLLOUT_RC_SAFE_ABORT=10
ROLLOUT_RC_ROLLED_BACK=20
ROLLOUT_RC_CRITICAL=30
ROLLOUT_RC_LOCK_BUSY=40

brand_rollout_validate_tx_id() {
  local id="${1:-}"
  [[ -n "${id}" ]] || return 1
  [[ "${id}" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] || return 1
  return 0
}

brand_checked_mkdir() {
  local path="${1:?}" mode="${2:-}"
  mkdir -p "${path}" || {
    brand_err "rollout-${BRAND_LABEL}: mkdir failed: ${path}"
    return 1
  }
  if [[ -n "${mode}" ]]; then
    chmod "${mode}" "${path}" || {
      brand_err "rollout-${BRAND_LABEL}: chmod ${mode} failed: ${path}"
      return 1
    }
  fi
  return 0
}

brand_checked_cp() {
  local src="${1:?}" dst="${2:?}"
  cp -a "${src}" "${dst}" || {
    brand_err "rollout-${BRAND_LABEL}: cp failed: ${src} -> ${dst}"
    return 1
  }
  return 0
}

brand_checked_mv() {
  local src="${1:?}" dst="${2:?}"
  mv -f "${src}" "${dst}" || {
    brand_err "rollout-${BRAND_LABEL}: mv failed: ${src} -> ${dst}"
    return 1
  }
  return 0
}

brand_checked_chmod() {
  local mode="${1:?}" path="${2:?}"
  chmod "${mode}" "${path}" || {
    brand_err "rollout-${BRAND_LABEL}: chmod ${mode} failed: ${path}"
    return 1
  }
  return 0
}

brand_checked_chown() {
  local owner="${1:?}" path="${2:?}"
  chown "${owner}" "${path}" || {
    brand_err "rollout-${BRAND_LABEL}: chown ${owner} failed: ${path}"
    return 1
  }
  return 0
}

brand_checked_rm() {
  local path="${1:?}"
  rm -f "${path}" || {
    brand_err "rollout-${BRAND_LABEL}: rm failed: ${path}"
    return 1
  }
  return 0
}

brand_file_sha256() {
  local path="${1:?}" out
  if [[ ! -f "${path}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: sha256 target missing: ${path}"
    return 1
  fi
  out="$(sha256sum "${path}" 2>/dev/null | awk '{print $1}')" || {
    brand_err "rollout-${BRAND_LABEL}: sha256sum failed: ${path}"
    return 1
  }
  if [[ -z "${out}" || ! "${out}" =~ ^[a-f0-9]{64}$ ]]; then
    brand_err "rollout-${BRAND_LABEL}: invalid sha256 for ${path}"
    return 1
  fi
  printf '%s\n' "${out}"
}

brand_rollout_paths_init() {
  brand_refresh_derived || return 1
  brand_require_vars REMOTE_DIR TX_ID SERVICE_NAME || return 1
  if ! brand_rollout_validate_tx_id "${TX_ID}"; then
    brand_err "rollout-${BRAND_LABEL}: invalid TX_ID"
    return 1
  fi
  ROLLOUT_ROOT="${REMOTE_DIR}/.vpnbot-rollouts"
  ROLLOUT_TX_DIR="${ROLLOUT_ROOT}/${TX_ID}"
  ROLLOUT_LOCK_DIR="${REMOTE_DIR}/.vpnbot-rollout.lock"
  ROLLOUT_MARKER="${REMOTE_DIR}/.vpnbot-last-binary-bak"
  return 0
}

brand_rollout_tx_write() {
  brand_require_vars ROLLOUT_TX_DIR TX_ID SERVICE_NAME EXPECTED_BRAND_ID BRAND_LABEL || return 1
  local manifest="${ROLLOUT_TX_DIR}/tx.env"
  local tmp="${ROLLOUT_TX_DIR}/tx.env.new"
  {
    printf 'TX_ID=%q\n' "${TX_ID}"
    printf 'SERVICE_NAME=%q\n' "${SERVICE_NAME}"
    printf 'EXPECTED_BRAND_ID=%q\n' "${EXPECTED_BRAND_ID}"
    printf 'BRAND_LABEL=%q\n' "${BRAND_LABEL}"
    printf 'TX_STATUS=%q\n' "${TX_STATUS:-initialized}"
    printf 'MUTATION_STARTED=%q\n' "${MUTATION_STARTED:-0}"
    printf 'CONFIG_STATE=%q\n' "${CONFIG_STATE:-not_started}"
    printf 'BINARY_STATE=%q\n' "${BINARY_STATE:-not_started}"
    printf 'DROPIN_STATE=%q\n' "${DROPIN_STATE:-not_started}"
    printf 'RESTART_STATE=%q\n' "${RESTART_STATE:-not_started}"
    printf 'PREV_ENV_PRESENT=%q\n' "${PREV_ENV_PRESENT:-0}"
    printf 'PREV_CONFIG_EXISTED=%q\n' "${PREV_CONFIG_EXISTED:-0}"
    printf 'PREV_DROPIN_EXISTED=%q\n' "${PREV_DROPIN_EXISTED:-0}"
    printf 'PREV_MARKER_EXISTED=%q\n' "${PREV_MARKER_EXISTED:-0}"
    printf 'BINARY_BACKUP=%q\n' "${BINARY_BACKUP:-}"
    printf 'PREV_BINARY_SHA256=%q\n' "${PREV_BINARY_SHA256:-}"
    printf 'PREV_CONFIG_SHA256=%q\n' "${PREV_CONFIG_SHA256:-}"
    printf 'PREV_DROPIN_SHA256=%q\n' "${PREV_DROPIN_SHA256:-}"
    printf 'NEW_BINARY_SHA256=%q\n' "${NEW_BINARY_SHA256:-}"
    printf 'NEW_CONFIG_SHA256=%q\n' "${NEW_CONFIG_SHA256:-}"
    printf 'NEW_DROPIN_SHA256=%q\n' "${NEW_DROPIN_SHA256:-}"
    printf 'ROLLOUT_START_TIME=%q\n' "${ROLLOUT_START_TIME:-}"
    printf 'MARKER_PUBLISHED=%q\n' "${MARKER_PUBLISHED:-0}"
    printf 'ROLLOUT_COMPLETED=%q\n' "${ROLLOUT_COMPLETED:-0}"
    printf 'REMOTE_TMP=%q\n' "${REMOTE_TMP:-}"
  } >"${tmp}" || {
    brand_err "rollout-${BRAND_LABEL}: manifest write failed"
    rm -f "${tmp}" 2>/dev/null || true
    return 1
  }
  brand_checked_chmod 0600 "${tmp}" || {
    rm -f "${tmp}" 2>/dev/null || true
    return 1
  }
  brand_checked_mv "${tmp}" "${manifest}" || {
    rm -f "${tmp}" 2>/dev/null || true
    return 1
  }
  return 0
}

brand_rollout_tx_load() {
  brand_require_vars ROLLOUT_TX_DIR || return 1
  local f="${ROLLOUT_TX_DIR}/tx.env"
  if [[ ! -f "${f}" || -L "${f}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: missing transaction state: ${f}"
    return 1
  fi
  # shellcheck disable=SC1090
  source "${f}" || {
    brand_err "rollout-${BRAND_LABEL}: failed to load transaction state"
    return 1
  }
  if ! brand_rollout_validate_tx_id "${TX_ID:-}"; then
    brand_err "rollout-${BRAND_LABEL}: loaded TX_ID invalid"
    return 1
  fi
  case "${TX_STATUS:-}" in
    initialized|preflight|backed_up|mutating|pending_smoke|rolling_back|rolled_back|finalizing|completed|aborted_before_mutation|critical) ;;
    *)
      brand_err "rollout-${BRAND_LABEL}: invalid TX_STATUS"
      return 1
      ;;
  esac
  return 0
}

brand_rollout_tx_set() {
  local key="${1:?}" val="${2:?}"
  case "${key}" in
    TX_STATUS|MUTATION_STARTED|CONFIG_STATE|BINARY_STATE|DROPIN_STATE|RESTART_STATE|PREV_ENV_PRESENT|PREV_CONFIG_EXISTED|PREV_DROPIN_EXISTED|PREV_MARKER_EXISTED|BINARY_BACKUP|PREV_BINARY_SHA256|PREV_CONFIG_SHA256|PREV_DROPIN_SHA256|NEW_BINARY_SHA256|NEW_CONFIG_SHA256|NEW_DROPIN_SHA256|ROLLOUT_START_TIME|MARKER_PUBLISHED|ROLLOUT_COMPLETED|REMOTE_TMP) ;;
    *)
      brand_err "rollout-${BRAND_LABEL}: invalid transaction key: ${key}"
      return 1
      ;;
  esac
  brand_rollout_tx_load || return 1
  printf -v "${key}" '%s' "${val}"
  brand_rollout_tx_write || return 1
}

brand_rollout_artifact_touched() {
  local state="${1:-not_started}"
  case "${state}" in
    installing|installed|restoring|restored) return 0 ;;
    *) return 1 ;;
  esac
}

brand_rollout_lock_write_meta() {
  brand_require_vars ROLLOUT_LOCK_DIR TX_ID ROLLOUT_TX_DIR SERVICE_NAME || return 1
  local created
  created="$(date -u +%Y-%m-%dT%H:%M:%SZ)" || created="unknown"
  {
    printf '%s\n' "${TX_ID}" >"${ROLLOUT_LOCK_DIR}/tx_id" || return 1
    printf '%s\n' "${ROLLOUT_TX_DIR}" >"${ROLLOUT_LOCK_DIR}/tx_dir" || return 1
    printf '%s\n' "${REMOTE_TMP:-}" >"${ROLLOUT_LOCK_DIR}/remote_tmp" || return 1
    printf '%s\n' "${SERVICE_NAME}" >"${ROLLOUT_LOCK_DIR}/service" || return 1
    printf '%s\n' "${created}" >"${ROLLOUT_LOCK_DIR}/created_at" || return 1
  } || {
    brand_err "rollout-${BRAND_LABEL}: lock metadata write failed"
    return 1
  }
  brand_checked_chmod 0600 "${ROLLOUT_LOCK_DIR}/tx_id" || return 1
  brand_checked_chmod 0600 "${ROLLOUT_LOCK_DIR}/tx_dir" || return 1
  brand_checked_chmod 0600 "${ROLLOUT_LOCK_DIR}/remote_tmp" || return 1
  brand_checked_chmod 0600 "${ROLLOUT_LOCK_DIR}/service" || return 1
  brand_checked_chmod 0600 "${ROLLOUT_LOCK_DIR}/created_at" || return 1
  return 0
}

brand_rollout_lock_acquire() {
  brand_rollout_paths_init || return 1
  if ! mkdir "${ROLLOUT_LOCK_DIR}" 2>/dev/null; then
    brand_err "rollout-${BRAND_LABEL}: another rollout is already in progress"
    return "${ROLLOUT_RC_LOCK_BUSY}"
  fi
  brand_checked_chmod 0700 "${ROLLOUT_LOCK_DIR}" || {
    rmdir "${ROLLOUT_LOCK_DIR}" 2>/dev/null || true
    return 1
  }
  if ! brand_rollout_lock_write_meta; then
    rm -f "${ROLLOUT_LOCK_DIR}/tx_id" "${ROLLOUT_LOCK_DIR}/tx_dir" \
      "${ROLLOUT_LOCK_DIR}/remote_tmp" "${ROLLOUT_LOCK_DIR}/service" \
      "${ROLLOUT_LOCK_DIR}/created_at" 2>/dev/null || true
    rmdir "${ROLLOUT_LOCK_DIR}" 2>/dev/null || true
    return 1
  fi
  return 0
}

brand_rollout_lock_owns() {
  brand_require_vars ROLLOUT_LOCK_DIR TX_ID ROLLOUT_TX_DIR SERVICE_NAME || return 1
  local lid ldir lsvc
  if [[ ! -d "${ROLLOUT_LOCK_DIR}" ]]; then
    brand_err "CRITICAL: transaction does not own rollout lock"
    return 1
  fi
  lid="$(tr -d '\n' <"${ROLLOUT_LOCK_DIR}/tx_id" 2>/dev/null || true)"
  ldir="$(tr -d '\n' <"${ROLLOUT_LOCK_DIR}/tx_dir" 2>/dev/null || true)"
  lsvc="$(tr -d '\n' <"${ROLLOUT_LOCK_DIR}/service" 2>/dev/null || true)"
  if [[ "${lid}" != "${TX_ID}" || "${ldir}" != "${ROLLOUT_TX_DIR}" || "${lsvc}" != "${SERVICE_NAME}" ]]; then
    brand_err "CRITICAL: transaction does not own rollout lock"
    return 1
  fi
  return 0
}

brand_rollout_lock_release() {
  brand_rollout_paths_init || return 1
  if [[ ! -d "${ROLLOUT_LOCK_DIR}" ]]; then
    return 0
  fi
  brand_rollout_lock_owns || return 1
  brand_checked_rm "${ROLLOUT_LOCK_DIR}/tx_id" || return 1
  brand_checked_rm "${ROLLOUT_LOCK_DIR}/tx_dir" || return 1
  brand_checked_rm "${ROLLOUT_LOCK_DIR}/remote_tmp" || return 1
  brand_checked_rm "${ROLLOUT_LOCK_DIR}/service" || return 1
  brand_checked_rm "${ROLLOUT_LOCK_DIR}/created_at" || return 1
  rmdir "${ROLLOUT_LOCK_DIR}" || {
    brand_err "rollout-${BRAND_LABEL}: rmdir lock failed: ${ROLLOUT_LOCK_DIR}"
    return 1
  }
  return 0
}

brand_rollout_critical() {
  local msg="${1:-rollout failed and automatic rollback failed}"
  TX_STATUS=critical
  brand_rollout_tx_write || true
  brand_err "CRITICAL: ${BRAND_LABEL} ${msg}"
  brand_err "transaction.id=${TX_ID}"
  brand_err "transaction.dir=${ROLLOUT_TX_DIR}"
  brand_err "lock=${ROLLOUT_LOCK_DIR}"
  brand_err "make brand-rollout-recover BRAND=${EXPECTED_BRAND_ID} TX_ID=${TX_ID} ACTION=status"
  brand_safe_journal_tail
  return "${ROLLOUT_RC_CRITICAL}"
}

brand_rollout_tx_init() {
  brand_rollout_paths_init || return 1
  brand_checked_mkdir "${ROLLOUT_ROOT}" 0700 || return 1
  brand_checked_mkdir "${ROLLOUT_TX_DIR}" 0700 || return 1
  brand_checked_mkdir "${ROLLOUT_TX_DIR}/backups" 0700 || return 1
  brand_checked_mkdir "${ROLLOUT_TX_DIR}/recovery" 0700 || return 1
  TX_STATUS=initialized
  MUTATION_STARTED=0
  CONFIG_STATE=not_started
  BINARY_STATE=not_started
  DROPIN_STATE=not_started
  RESTART_STATE=not_started
  PREV_ENV_PRESENT=0
  PREV_CONFIG_EXISTED=0
  PREV_DROPIN_EXISTED=0
  PREV_MARKER_EXISTED=0
  BINARY_BACKUP=""
  PREV_BINARY_SHA256=""
  PREV_CONFIG_SHA256=""
  PREV_DROPIN_SHA256=""
  NEW_BINARY_SHA256=""
  NEW_CONFIG_SHA256=""
  NEW_DROPIN_SHA256=""
  ROLLOUT_START_TIME=""
  MARKER_PUBLISHED=0
  ROLLOUT_COMPLETED=0
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: transaction ${TX_ID} initialized"
  return 0
}

brand_rollout_cleanup_tx_backups() {
  # Best-effort; never remove BINARY_BACKUP.
  rm -f "${ROLLOUT_TX_DIR}/backups/config.bak" \
    "${ROLLOUT_TX_DIR}/backups/dropin.bak" \
    "${ROLLOUT_TX_DIR}/backups/marker.bak" \
    "${ROLLOUT_TX_DIR}/recovery/dropin.intended" 2>/dev/null || true
  return 0
}

brand_rollout_safe_abort_cleanup() {
  # Only when mutation never started.
  brand_rollout_tx_load || true
  if [[ "${MUTATION_STARTED:-0}" == "1" ]]; then
    return 1
  fi
  TX_STATUS=aborted_before_mutation
  brand_rollout_tx_write || {
    brand_rollout_critical "safe abort manifest write failed"
    return "${ROLLOUT_RC_CRITICAL}"
  }
  if [[ -n "${BINARY_BACKUP:-}" && -f "${BINARY_BACKUP}" ]]; then
    brand_checked_rm "${BINARY_BACKUP}" || {
      brand_rollout_critical "safe abort binary backup remove failed"
      return "${ROLLOUT_RC_CRITICAL}"
    }
  fi
  # Best-effort remove any leftover staging files for this TX.
  rm -f "${REMOTE_EXPLICIT_CONFIG}.new.${TX_ID}" \
    "${REMOTE_BINARY}.new.${TX_ID}" \
    "${DROPIN_FILE}.new.${TX_ID}" 2>/dev/null || true

  # Release lock before deleting the transaction directory so a failed release
  # leaves recoverable manifest/state on disk.
  if ! brand_rollout_lock_release; then
    brand_rollout_critical "safe abort lock release failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  if [[ -d "${ROLLOUT_TX_DIR}" ]]; then
    find "${ROLLOUT_TX_DIR}" -mindepth 1 -delete 2>/dev/null || true
    if ! rmdir "${ROLLOUT_TX_DIR}" 2>/dev/null; then
      brand_err "rollout-${BRAND_LABEL}: safe abort lock released but transaction dir cleanup failed: ${ROLLOUT_TX_DIR}"
      return 1
    fi
  fi
  return 0
}

brand_rollout_preflight() {
  local uploaded_binary="${1:?}"
  local uploaded_config="${2:?}"
  local uploaded_configcheck="${3:?}"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1
  TX_STATUS=preflight
  brand_rollout_tx_write || return 1

  if ! brand_systemctl_unit_exists; then
    brand_err "rollout-${BRAND_LABEL}: systemd unit not found: ${SERVICE_NAME}"
    return 1
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} is not active"
    return 1
  fi
  sleep 1 || true
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

  NEW_BINARY_SHA256="$(brand_file_sha256 "${uploaded_binary}")" || return 1
  NEW_CONFIG_SHA256="$(brand_file_sha256 "${uploaded_config}")" || return 1
  # Drop-in body fingerprint from intended managed content.
  local dropin_tmp
  dropin_tmp="${ROLLOUT_TX_DIR}/recovery/dropin.intended"
  printf '%s\n' "${DROPIN_BODY}" >"${dropin_tmp}" || return 1
  brand_checked_chmod 0600 "${dropin_tmp}" || return 1
  NEW_DROPIN_SHA256="$(brand_file_sha256 "${dropin_tmp}")" || return 1

  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: preflight OK"
  return 0
}

brand_rollout_backup() {
  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  BINARY_BACKUP="${REMOTE_BINARY}.bak.${TX_ID}"
  brand_checked_cp "${REMOTE_BINARY}" "${BINARY_BACKUP}" || return 1
  if [[ ! -f "${BINARY_BACKUP}" || -L "${BINARY_BACKUP}" || ! -x "${BINARY_BACKUP}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: binary backup invalid: ${BINARY_BACKUP}"
    return 1
  fi
  PREV_BINARY_SHA256="$(brand_file_sha256 "${REMOTE_BINARY}")" || return 1
  local bak_hash
  bak_hash="$(brand_file_sha256 "${BINARY_BACKUP}")" || return 1
  if [[ "${bak_hash}" != "${PREV_BINARY_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: binary backup hash mismatch"
    return 1
  fi

  if [[ -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    PREV_CONFIG_EXISTED=1
    brand_checked_cp "${REMOTE_EXPLICIT_CONFIG}" "${ROLLOUT_TX_DIR}/backups/config.bak" || return 1
    PREV_CONFIG_SHA256="$(brand_file_sha256 "${ROLLOUT_TX_DIR}/backups/config.bak")" || return 1
  else
    PREV_CONFIG_EXISTED=0
    PREV_CONFIG_SHA256=""
  fi

  if [[ -f "${DROPIN_FILE}" ]] && brand_dropin_matches; then
    PREV_DROPIN_EXISTED=1
    brand_checked_cp "${DROPIN_FILE}" "${ROLLOUT_TX_DIR}/backups/dropin.bak" || return 1
    PREV_DROPIN_SHA256="$(brand_file_sha256 "${ROLLOUT_TX_DIR}/backups/dropin.bak")" || return 1
  else
    PREV_DROPIN_EXISTED=0
    PREV_DROPIN_SHA256=""
  fi

  if [[ -f "${ROLLOUT_MARKER}" ]]; then
    PREV_MARKER_EXISTED=1
    brand_checked_cp "${ROLLOUT_MARKER}" "${ROLLOUT_TX_DIR}/backups/marker.bak" || return 1
  else
    PREV_MARKER_EXISTED=0
  fi

  TX_STATUS=backed_up
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: backup complete"
  return 0
}

brand_rollout_install_config() {
  local uploaded_config="${1:?}"
  local user group
  local final_path="${REMOTE_EXPLICIT_CONFIG}"
  local new_path="${REMOTE_EXPLICIT_CONFIG}.new.${TX_ID}"

  user="$(brand_unit_user)"
  group="$(brand_unit_group)"

  CONFIG_STATE=installing
  TX_STATUS=mutating
  brand_rollout_tx_write || return 1

  brand_checked_cp "${uploaded_config}" "${new_path}" || return 1
  brand_checked_chown "${user}:${group}" "${new_path}" || return 1
  brand_checked_chmod 0600 "${new_path}" || return 1

  local staged_hash
  staged_hash="$(brand_file_sha256 "${new_path}")" || return 1
  if [[ "${staged_hash}" != "${NEW_CONFIG_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: staged config hash mismatch"
    brand_checked_rm "${new_path}" || true
    return 1
  fi

  if command -v runuser >/dev/null 2>&1; then
    if ! runuser -u "${user}" -- test -r "${new_path}"; then
      brand_checked_rm "${new_path}" || true
      brand_err "rollout-${BRAND_LABEL}: staged config not readable by ${user}"
      return 1
    fi
  elif command -v su >/dev/null 2>&1; then
    if ! su -s /bin/sh "${user}" -c "test -r $(printf %q "${new_path}")"; then
      brand_checked_rm "${new_path}" || true
      brand_err "rollout-${BRAND_LABEL}: staged config not readable by ${user}"
      return 1
    fi
  else
    brand_checked_rm "${new_path}" || true
    brand_err "rollout-${BRAND_LABEL}: neither runuser nor su available to verify readability"
    return 1
  fi

  MUTATION_STARTED=1
  brand_rollout_tx_write || return 1
  brand_checked_mv "${new_path}" "${final_path}" || return 1
  brand_checked_chmod 0600 "${final_path}" || return 1
  brand_checked_chown "${user}:${group}" "${final_path}" || return 1

  local prod_hash
  prod_hash="$(brand_file_sha256 "${final_path}")" || return 1
  if [[ "${prod_hash}" != "${NEW_CONFIG_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: installed config hash mismatch"
    return 1
  fi

  CONFIG_STATE=installed
  brand_rollout_tx_write || return 1
  return 0
}

brand_rollout_install_binary() {
  local uploaded_binary="${1:?}"
  local user group
  local new_path="${REMOTE_BINARY}.new.${TX_ID}"

  read -r user group <<<"$(brand_binary_owner_group)"
  BINARY_STATE=installing
  brand_rollout_tx_write || return 1

  brand_checked_cp "${uploaded_binary}" "${new_path}" || return 1
  brand_checked_chown "${user}:${group}" "${new_path}" || return 1
  brand_checked_chmod 0755 "${new_path}" || return 1

  local staged_hash
  staged_hash="$(brand_file_sha256 "${new_path}")" || return 1
  if [[ "${staged_hash}" != "${NEW_BINARY_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: staged binary hash mismatch"
    brand_checked_rm "${new_path}" || true
    return 1
  fi
  if [[ ! -x "${new_path}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: staged binary not executable"
    brand_checked_rm "${new_path}" || true
    return 1
  fi

  MUTATION_STARTED=1
  brand_rollout_tx_write || return 1
  brand_checked_mv "${new_path}" "${REMOTE_BINARY}" || return 1
  brand_checked_chown "${user}:${group}" "${REMOTE_BINARY}" || return 1
  brand_checked_chmod 0755 "${REMOTE_BINARY}" || return 1

  local prod_hash
  prod_hash="$(brand_file_sha256 "${REMOTE_BINARY}")" || return 1
  if [[ "${prod_hash}" != "${NEW_BINARY_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: installed binary hash mismatch"
    return 1
  fi

  BINARY_STATE=installed
  brand_rollout_tx_write || return 1
  return 0
}

brand_rollout_install_dropin() {
  local new_path="${DROPIN_FILE}.new.${TX_ID}"
  DROPIN_STATE=installing
  brand_rollout_tx_write || return 1

  brand_checked_mkdir "${DROPIN_DIR}" 0755 || return 1
  printf '%s\n' "${DROPIN_BODY}" >"${new_path}" || {
    brand_err "rollout-${BRAND_LABEL}: drop-in staging write failed"
    return 1
  }
  brand_checked_chown "root:root" "${new_path}" || return 1
  brand_checked_chmod 0644 "${new_path}" || return 1

  local staged_hash
  staged_hash="$(brand_file_sha256 "${new_path}")" || return 1
  if [[ "${staged_hash}" != "${NEW_DROPIN_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: staged drop-in hash mismatch"
    brand_checked_rm "${new_path}" || true
    return 1
  fi

  MUTATION_STARTED=1
  brand_rollout_tx_write || return 1
  brand_checked_mv "${new_path}" "${DROPIN_FILE}" || return 1
  brand_checked_chown "root:root" "${DROPIN_FILE}" || return 1
  brand_checked_chmod 0644 "${DROPIN_FILE}" || return 1

  local prod_hash
  prod_hash="$(brand_file_sha256 "${DROPIN_FILE}")" || return 1
  if [[ "${prod_hash}" != "${NEW_DROPIN_SHA256}" ]]; then
    brand_err "rollout-${BRAND_LABEL}: installed drop-in hash mismatch"
    return 1
  fi

  DROPIN_STATE=installed
  BRAND_DROPIN_ACTIVE=1
  brand_rollout_tx_write || return 1
  return 0
}

brand_rollout_install() {
  local uploaded_binary="${1:?}"
  local uploaded_config="${2:?}"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1
  brand_rollout_lock_owns || return 1

  brand_rollout_install_config "${uploaded_config}" || return 1
  brand_rollout_install_binary "${uploaded_binary}" || return 1
  brand_rollout_install_dropin || return 1

  ROLLOUT_START_TIME="$(date '+%Y-%m-%d %H:%M:%S')" || return 1
  brand_rollout_tx_write || return 1

  if ! systemctl daemon-reload; then
    brand_err "rollout-${BRAND_LABEL}: daemon-reload failed"
    return 1
  fi

  RESTART_STATE=starting
  brand_rollout_tx_write || return 1
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: restart failed"
    return 1
  fi
  RESTART_STATE=completed
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: install complete (single restart)"
  return 0
}

brand_rollout_verify() {
  local uploaded_configcheck="${1:?}"

  brand_refresh_derived || return 1
  brand_rollout_tx_load || return 1

  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_err "rollout-${BRAND_LABEL}: ${SERVICE_NAME} is not active after restart"
    return 1
  fi
  sleep 2 || true
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

  TX_STATUS=pending_smoke
  brand_rollout_tx_write || return 1
  brand_log "rollout-${BRAND_LABEL}: verify OK"
  return 0
}

brand_rollout_restore_via_staging() {
  local backup="${1:?}" dest="${2:?}" label="${3:?}" expected_hash="${4:-}"
  local staging="${dest}.rollback.${TX_ID}"

  if [[ ! -f "${backup}" ]]; then
    brand_err "CRITICAL: rollout-${BRAND_LABEL}: ${label} backup missing"
    return 1
  fi
  brand_checked_cp "${backup}" "${staging}" || return 1
  if [[ -n "${expected_hash}" ]]; then
    local h
    h="$(brand_file_sha256 "${staging}")" || return 1
    if [[ "${h}" != "${expected_hash}" ]]; then
      brand_err "CRITICAL: rollout-${BRAND_LABEL}: ${label} staging hash mismatch"
      brand_checked_rm "${staging}" || true
      return 1
    fi
  fi
  brand_checked_mv "${staging}" "${dest}" || return 1
  return 0
}

brand_rollout_remove_if_ours() {
  local path="${1:?}" expected_hash="${2:?}" label="${3:?}"
  if [[ ! -e "${path}" ]]; then
    return 0
  fi
  if [[ ! -f "${path}" || -L "${path}" ]]; then
    brand_err "CRITICAL: refusing to remove ${label} changed outside transaction"
    return 1
  fi
  local h
  h="$(brand_file_sha256 "${path}")" || return 1
  if [[ "${h}" != "${expected_hash}" ]]; then
    brand_err "CRITICAL: refusing to remove ${label} changed outside transaction"
    return 1
  fi
  brand_checked_rm "${path}" || return 1
  return 0
}

brand_rollout_rollback() {
  brand_rollout_paths_init || return "${ROLLOUT_RC_CRITICAL}"
  brand_rollout_tx_load || return "${ROLLOUT_RC_CRITICAL}"

  if [[ "${TX_STATUS:-}" == "rolled_back" ]]; then
    brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
    return "${ROLLOUT_RC_ROLLED_BACK}"
  fi
  if [[ "${TX_STATUS:-}" == "completed" ]]; then
    brand_err "rollout-${BRAND_LABEL}: rollback not allowed for completed transaction"
    return 1
  fi

  brand_rollout_lock_owns || return "${ROLLOUT_RC_CRITICAL}"

  TX_STATUS=rolling_back
  brand_rollout_tx_write || return "${ROLLOUT_RC_CRITICAL}"

  local restore_failed=0
  local binary_needed=0
  if brand_rollout_artifact_touched "${BINARY_STATE:-not_started}"; then
    binary_needed=1
  fi

  if [[ "${binary_needed}" -eq 1 ]]; then
    BINARY_STATE=restoring
    brand_rollout_tx_write || true
    if [[ -z "${BINARY_BACKUP:-}" || ! -f "${BINARY_BACKUP}" ]]; then
      brand_err "CRITICAL: rollout-${BRAND_LABEL}: binary backup missing; cannot restore"
      restore_failed=1
    else
      # Refuse overwrite if current binary is neither new nor previous known hash.
      if [[ -f "${REMOTE_BINARY}" ]]; then
        local cur
        cur="$(brand_file_sha256 "${REMOTE_BINARY}")" || restore_failed=1
        if [[ "${restore_failed}" -eq 0 && \
              "${cur}" != "${NEW_BINARY_SHA256:-}" && \
              "${cur}" != "${PREV_BINARY_SHA256:-}" ]]; then
          brand_err "CRITICAL: refusing to overwrite binary changed outside transaction"
          restore_failed=1
        fi
      fi
      if [[ "${restore_failed}" -eq 0 ]]; then
        if brand_rollout_restore_via_staging "${BINARY_BACKUP}" "${REMOTE_BINARY}" binary "${PREV_BINARY_SHA256}"; then
          BINARY_STATE=restored
          brand_rollout_tx_write || true
        else
          restore_failed=1
        fi
      fi
    fi
  fi

  if brand_rollout_artifact_touched "${CONFIG_STATE:-not_started}"; then
    CONFIG_STATE=restoring
    brand_rollout_tx_write || true
    if [[ "${PREV_CONFIG_EXISTED:-0}" == "1" ]]; then
      if [[ -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
        local curc
        curc="$(brand_file_sha256 "${REMOTE_EXPLICIT_CONFIG}")" || restore_failed=1
        if [[ "${restore_failed}" -eq 0 && \
              "${curc}" != "${NEW_CONFIG_SHA256:-}" && \
              "${curc}" != "${PREV_CONFIG_SHA256:-}" ]]; then
          brand_err "CRITICAL: refusing to overwrite config changed outside transaction"
          restore_failed=1
        fi
      fi
      if [[ "${restore_failed}" -eq 0 ]]; then
        if brand_rollout_restore_via_staging \
          "${ROLLOUT_TX_DIR}/backups/config.bak" \
          "${REMOTE_EXPLICIT_CONFIG}" config "${PREV_CONFIG_SHA256}"; then
          CONFIG_STATE=restored
          brand_rollout_tx_write || true
        else
          restore_failed=1
        fi
      fi
    else
      if ! brand_rollout_remove_if_ours "${REMOTE_EXPLICIT_CONFIG}" "${NEW_CONFIG_SHA256}" config; then
        restore_failed=1
      else
        CONFIG_STATE=restored
        brand_rollout_tx_write || true
      fi
    fi
  fi

  if brand_rollout_artifact_touched "${DROPIN_STATE:-not_started}"; then
    DROPIN_STATE=restoring
    brand_rollout_tx_write || true
    if [[ "${PREV_DROPIN_EXISTED:-0}" == "1" ]]; then
      if [[ -f "${DROPIN_FILE}" ]]; then
        local curd
        curd="$(brand_file_sha256 "${DROPIN_FILE}")" || restore_failed=1
        if [[ "${restore_failed}" -eq 0 && \
              "${curd}" != "${NEW_DROPIN_SHA256:-}" && \
              "${curd}" != "${PREV_DROPIN_SHA256:-}" ]]; then
          brand_err "CRITICAL: refusing to overwrite drop-in changed outside transaction"
          restore_failed=1
        fi
      fi
      if [[ "${restore_failed}" -eq 0 ]]; then
        if brand_rollout_restore_via_staging \
          "${ROLLOUT_TX_DIR}/backups/dropin.bak" \
          "${DROPIN_FILE}" dropin "${PREV_DROPIN_SHA256}"; then
          DROPIN_STATE=restored
          brand_rollout_tx_write || true
        else
          restore_failed=1
        fi
      fi
    else
      if ! brand_rollout_remove_if_ours "${DROPIN_FILE}" "${NEW_DROPIN_SHA256}" dropin; then
        restore_failed=1
      else
        DROPIN_STATE=restored
        brand_rollout_tx_write || true
      fi
    fi
  fi

  # Marker restore if published during partial finalize.
  if [[ "${MARKER_PUBLISHED:-0}" == "1" ]]; then
    if [[ "${PREV_MARKER_EXISTED:-0}" == "1" ]]; then
      if ! brand_rollout_restore_via_staging \
        "${ROLLOUT_TX_DIR}/backups/marker.bak" \
        "${ROLLOUT_MARKER}" marker; then
        restore_failed=1
      fi
    else
      if [[ -f "${ROLLOUT_MARKER}" ]]; then
        local mcontent
        mcontent="$(tr -d '\n' <"${ROLLOUT_MARKER}" 2>/dev/null || true)"
        if [[ "${mcontent}" == "${BINARY_BACKUP}" ]]; then
          brand_checked_rm "${ROLLOUT_MARKER}" || restore_failed=1
        else
          brand_err "CRITICAL: refusing to remove marker changed outside transaction"
          restore_failed=1
        fi
      fi
    fi
    MARKER_PUBLISHED=0
    brand_rollout_tx_write || true
  fi

  if [[ "${restore_failed}" -eq 1 ]]; then
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  if ! systemctl daemon-reload; then
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi
  if ! systemctl restart "${SERVICE_NAME}"; then
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi
  sleep 2 || true
  if ! systemctl is-active --quiet "${SERVICE_NAME}"; then
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  if [[ "${PREV_ENV_PRESENT:-0}" == "1" ]]; then
    if ! brand_env_has_expected; then
      brand_rollout_critical "rollout failed and automatic rollback failed"
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    if [[ "${PREV_CONFIG_EXISTED:-0}" == "1" && -n "${REMOTE_TMP:-}" && -x "${REMOTE_TMP}/configcheck" ]]; then
      if ! brand_run_configcheck_as_unit "${REMOTE_TMP}/configcheck" "${REMOTE_EXPLICIT_CONFIG}" >/dev/null; then
        brand_rollout_critical "rollout failed and automatic rollback failed"
        return "${ROLLOUT_RC_CRITICAL}"
      fi
    fi
  else
    if ! brand_assert_expected_env_absent; then
      brand_rollout_critical "rollout failed and automatic rollback failed"
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    if [[ "${PREV_CONFIG_EXISTED:-0}" == "0" && -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then
      brand_rollout_critical "rollout failed and automatic rollback failed"
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    if [[ "${PREV_DROPIN_EXISTED:-0}" == "0" && -f "${DROPIN_FILE}" ]]; then
      brand_rollout_critical "rollout failed and automatic rollback failed"
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    if [[ -n "${PREV_BINARY_SHA256:-}" ]]; then
      local bh
      bh="$(brand_file_sha256 "${REMOTE_BINARY}")" || {
        brand_rollout_critical "rollout failed and automatic rollback failed"
        return "${ROLLOUT_RC_CRITICAL}"
      }
      if [[ "${bh}" != "${PREV_BINARY_SHA256}" ]]; then
        brand_rollout_critical "rollout failed and automatic rollback failed"
        return "${ROLLOUT_RC_CRITICAL}"
      fi
    fi
  fi

  TX_STATUS=rolled_back
  brand_rollout_tx_write || {
    brand_rollout_critical "rollout failed and automatic rollback failed"
    return "${ROLLOUT_RC_CRITICAL}"
  }

  if ! brand_rollout_lock_release; then
    brand_rollout_critical "rollback succeeded but lock release failed"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  brand_err "rollout-${BRAND_LABEL}: rollout failed; previous state restored"
  return "${ROLLOUT_RC_ROLLED_BACK}"
}

brand_rollout_finalize() {
  brand_rollout_paths_init || return 1
  brand_rollout_tx_load || return "${ROLLOUT_RC_CRITICAL}"

  case "${TX_STATUS:-}" in
    completed)
      if [[ "${ROLLOUT_COMPLETED:-0}" != "1" ]]; then
        brand_err "rollout-${BRAND_LABEL}: finalize not allowed from status ${TX_STATUS}"
        return 1
      fi
      brand_log "rollout-${BRAND_LABEL}: finalize idempotent OK"
      # Partial prior failure may have left the lock held after completed was recorded.
      if [[ -d "${ROLLOUT_LOCK_DIR}" ]]; then
        brand_rollout_lock_owns || return "${ROLLOUT_RC_CRITICAL}"
        if ! brand_rollout_lock_release; then
          brand_err "CRITICAL: ${BRAND_LABEL} finalize lock release failed"
          brand_err "transaction.id=${TX_ID}"
          brand_err "transaction.dir=${ROLLOUT_TX_DIR}"
          brand_err "lock=${ROLLOUT_LOCK_DIR}"
          brand_err "make brand-rollout-recover BRAND=${EXPECTED_BRAND_ID} TX_ID=${TX_ID} ACTION=status"
          return "${ROLLOUT_RC_CRITICAL}"
        fi
      fi
      brand_rollout_cleanup_tx_backups
      return 0
      ;;
    pending_smoke|finalizing)
      ;;
    *)
      brand_err "rollout-${BRAND_LABEL}: finalize not allowed from status ${TX_STATUS:-}"
      return 1
      ;;
  esac

  brand_rollout_lock_owns || return "${ROLLOUT_RC_CRITICAL}"

  local marker_tmp="${ROLLOUT_MARKER}.new.${TX_ID}"
  local mcontent=""
  local need_marker_mv=1

  # Resume after crash: marker intent already recorded and content already correct.
  if [[ "${MARKER_PUBLISHED:-0}" == "1" && -f "${ROLLOUT_MARKER}" ]]; then
    mcontent="$(tr -d '\n' <"${ROLLOUT_MARKER}" 2>/dev/null || true)"
    if [[ "${mcontent}" == "${BINARY_BACKUP}" ]]; then
      need_marker_mv=0
    fi
  fi

  if [[ "${need_marker_mv}" -eq 1 ]]; then
    printf '%s\n' "${BINARY_BACKUP}" >"${marker_tmp}" || {
      brand_rollout_critical "finalize marker write failed"
      return "${ROLLOUT_RC_CRITICAL}"
    }
    brand_checked_chmod 0600 "${marker_tmp}" || {
      brand_checked_rm "${marker_tmp}" || true
      brand_rollout_critical "finalize marker chmod failed"
      return "${ROLLOUT_RC_CRITICAL}"
    }

    # Intent before atomic replace: marker may be changed after this point.
    MARKER_PUBLISHED=1
    TX_STATUS=finalizing
    brand_rollout_tx_write || {
      brand_checked_rm "${marker_tmp}" || true
      brand_rollout_critical "finalize marker intent write failed"
      return "${ROLLOUT_RC_CRITICAL}"
    }

    brand_checked_mv "${marker_tmp}" "${ROLLOUT_MARKER}" || {
      brand_checked_rm "${marker_tmp}" || true
      brand_rollout_critical "finalize marker mv failed"
      return "${ROLLOUT_RC_CRITICAL}"
    }
  else
    TX_STATUS=finalizing
    brand_rollout_tx_write || return "${ROLLOUT_RC_CRITICAL}"
  fi

  mcontent="$(tr -d '\n' <"${ROLLOUT_MARKER}" 2>/dev/null || true)"
  if [[ "${mcontent}" != "${BINARY_BACKUP}" ]]; then
    brand_rollout_critical "finalize marker content mismatch"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  ROLLOUT_COMPLETED=1
  TX_STATUS=completed
  brand_rollout_tx_write || return "${ROLLOUT_RC_CRITICAL}"

  # Keep recovery backups until lock release succeeds.
  if ! brand_rollout_lock_release; then
    # Status stays completed so recovery finalize can retry release + cleanup.
    brand_err "CRITICAL: ${BRAND_LABEL} finalize lock release failed"
    brand_err "transaction.id=${TX_ID}"
    brand_err "transaction.dir=${ROLLOUT_TX_DIR}"
    brand_err "lock=${ROLLOUT_LOCK_DIR}"
    brand_err "make brand-rollout-recover BRAND=${EXPECTED_BRAND_ID} TX_ID=${TX_ID} ACTION=status"
    return "${ROLLOUT_RC_CRITICAL}"
  fi

  brand_rollout_cleanup_tx_backups
  brand_log "rollout-${BRAND_LABEL}: finalized (binary backup retained)"
  return 0
}

brand_rollout_status_print() {
  brand_rollout_paths_init || return 1
  brand_rollout_tx_load || return 1
  local lock_owned=0
  if brand_rollout_lock_owns 2>/dev/null; then
    lock_owned=1
  fi
  local previous_mode=legacy
  if [[ "${PREV_ENV_PRESENT:-0}" == "1" ]]; then
    previous_mode=explicit
  fi
  printf 'tx.id=%s\n' "${TX_ID}"
  printf 'tx.status=%s\n' "${TX_STATUS}"
  printf 'service=%s\n' "${SERVICE_NAME}"
  printf 'brand.id=%s\n' "${EXPECTED_BRAND_ID}"
  printf 'mutation_started=%s\n' "${MUTATION_STARTED:-0}"
  printf 'config_state=%s\n' "${CONFIG_STATE:-not_started}"
  printf 'binary_state=%s\n' "${BINARY_STATE:-not_started}"
  printf 'dropin_state=%s\n' "${DROPIN_STATE:-not_started}"
  printf 'restart_state=%s\n' "${RESTART_STATE:-not_started}"
  printf 'previous_mode=%s\n' "${previous_mode}"
  printf 'lock_owned=%s\n' "${lock_owned}"
  return 0
}

brand_rollout_run() {
  local uploaded_binary="${1:?}"
  local uploaded_config="${2:?}"
  local uploaded_configcheck="${3:?}"
  local rc=0

  brand_refresh_derived || return 1
  brand_require_vars TX_ID || return 1
  brand_rollout_paths_init || return 1

  rc=0
  brand_rollout_lock_acquire || rc=$?
  if [[ "${rc}" -ne 0 ]]; then
    return "${rc}"
  fi

  if ! brand_rollout_tx_init; then
    brand_rollout_lock_release || true
    return 1
  fi

  if ! brand_rollout_preflight "${uploaded_binary}" "${uploaded_config}" "${uploaded_configcheck}"; then
    rc=0
    brand_rollout_safe_abort_cleanup || rc=$?
    if [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]]; then
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    return "${ROLLOUT_RC_SAFE_ABORT}"
  fi

  if ! brand_rollout_backup; then
    rc=0
    brand_rollout_safe_abort_cleanup || rc=$?
    if [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]]; then
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    return "${ROLLOUT_RC_SAFE_ABORT}"
  fi

  if ! brand_rollout_install "${uploaded_binary}" "${uploaded_config}"; then
    brand_rollout_tx_load || true
    # Clean leftover staging files from a failed pre-mv attempt.
    rm -f "${REMOTE_EXPLICIT_CONFIG}.new.${TX_ID}" \
      "${REMOTE_BINARY}.new.${TX_ID}" \
      "${DROPIN_FILE}.new.${TX_ID}" 2>/dev/null || true
    if [[ "${MUTATION_STARTED:-0}" == "1" ]]; then
      rc=0
      brand_rollout_rollback || rc=$?
      return "${rc}"
    fi
    rc=0
    brand_rollout_safe_abort_cleanup || rc=$?
    if [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]]; then
      return "${ROLLOUT_RC_CRITICAL}"
    fi
    brand_err "rollout-${BRAND_LABEL}: install failed before mutation; previous state unchanged"
    return "${ROLLOUT_RC_SAFE_ABORT}"
  fi

  if ! brand_rollout_verify "${uploaded_configcheck}"; then
    rc=0
    brand_rollout_rollback || rc=$?
    return "${rc}"
  fi

  brand_log "rollout-${BRAND_LABEL}: coordinated rollout OK"
  return "${ROLLOUT_RC_OK}"
}
