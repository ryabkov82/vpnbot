#!/usr/bin/env bash
# Runtime config helpers for SHM tg_payments_webapp deploy.
# stdout of resolve helpers is path-only; logs go to stderr.
#
# shellcheck shell=bash

shm_tg_payments_log() {
  echo "deploy-tg-payments: $*" >&2
}

# shm_tg_payments_resolve_config
# Requires:
#   LOCAL_TMP  — existing writable temp directory (lifecycle owned by caller)
# Optional env:
#   CONFIG                 local config path (skip scp)
#   CONFIG_USER/HOST/REMOTE_PATH  remote VFF runtime config
#
# Prints exactly one absolute path on stdout. Never prints secrets.
shm_tg_payments_resolve_config() {
  if [[ -z "${LOCAL_TMP:-}" || ! -d "${LOCAL_TMP}" ]]; then
    echo "deploy-tg-payments: LOCAL_TMP must be an existing directory before resolve_config" >&2
    return 1
  fi

  if [[ -n "${CONFIG:-}" ]]; then
    if [[ ! -f "${CONFIG}" ]]; then
      echo "deploy-tg-payments: CONFIG not found: ${CONFIG}" >&2
      return 1
    fi
    # Normalize to absolute path for callers.
    local abs
    abs="$(cd "$(dirname "${CONFIG}")" && pwd)/$(basename "${CONFIG}")"
    if [[ ! -f "${abs}" ]]; then
      echo "deploy-tg-payments: CONFIG path unresolved: ${CONFIG}" >&2
      return 1
    fi
    printf '%s\n' "${abs}"
    return 0
  fi

  local dest="${LOCAL_TMP}/runtime-config.json"
  local user="${CONFIG_USER:-${TG_TEMPLATE_CONFIG_USER:-root}}"
  local host="${CONFIG_HOST:-${TG_TEMPLATE_CONFIG_HOST:-fr-mrs-1}}"
  local remote="${CONFIG_REMOTE_PATH:-${TG_TEMPLATE_CONFIG_REMOTE:-/opt/bot/config-vff.json}}"

  shm_tg_payments_log "fetching runtime config from ${user}@${host}:${remote}"
  if ! scp -q -o BatchMode=yes -o ConnectTimeout=20 \
    "${user}@${host}:${remote}" "${dest}"; then
    echo "deploy-tg-payments: failed to download runtime config" >&2
    return 1
  fi
  if [[ ! -f "${dest}" ]]; then
    echo "deploy-tg-payments: downloaded runtime config missing: ${dest}" >&2
    return 1
  fi
  printf '%s\n' "${dest}"
}

# shm_tg_payments_require_config_file <path>
shm_tg_payments_require_config_file() {
  local path="${1:-}"
  if [[ -z "${path}" || ! -f "${path}" ]]; then
    echo "deploy-tg-payments: runtime config file missing or not a file: ${path:-<empty>}" >&2
    return 1
  fi
  return 0
}
