#!/usr/bin/env bash
# Safe post-deploy probes for brand-aware SHM YooKassa CGI.
# Never creates a payment; always uses user_id=-1.
#
# shellcheck shell=bash

# shellcheck source=smoke_yookassa_cgi.sh
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=smoke_yookassa_cgi.sh
source "${SOURCE_DIR}/smoke_yookassa_cgi.sh"

SHM_YK_CGI_PATH="${SMOKE_YOOKASSA_CGI_PATH:-/shm/pay_systems/yookassa.cgi}"
SHM_YK_CGI_MAX_TIME="${SMOKE_YOOKASSA_CGI_MAX_TIME:-10}"

shm_yookassa_probe_body_has() {
  local body="${1:-}"
  local needle="${2:-}"
  [[ -n "${body}" && -n "${needle}" ]] || return 1
  local lc needle_lc
  lc="$(printf '%s' "${body}" | tr '[:upper:]' '[:lower:]')"
  needle_lc="$(printf '%s' "${needle}" | tr '[:upper:]' '[:lower:]')"
  [[ "${lc}" == *"${needle_lc}"* ]]
}

# shm_yookassa_probe_cgi <api_base> <pay_system> <expected_needle> [brand_id]
# brand_id may be empty to omit the query parameter entirely.
shm_yookassa_probe_cgi() {
  local base="${1:-}"
  local ps="${2:-}"
  local expected="${3:-}"
  local brand_id="${4-__omit__}"
  local label="${SHM_YK_PROBE_LABEL:-shm-yookassa-probe}"

  base="${base%/}"
  base="$(printf '%s' "${base}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  ps="$(printf '%s' "${ps}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"

  if ! smoke_yookassa_cgi_api_base_ok "${base}"; then
    echo "${label}: invalid api.base_url" >&2
    return 1
  fi
  if ! smoke_yookassa_cgi_pay_system_ok "${ps}"; then
    echo "${label}: invalid yookassa pay system" >&2
    return 1
  fi
  if [[ -z "${expected}" ]]; then
    echo "${label}: expected body needle required" >&2
    return 1
  fi

  local tmp code curl_rc=0 body
  tmp="$(mktemp)"
  local -a curl_args=(
    curl -sS
    --max-time "${SHM_YK_CGI_MAX_TIME}"
    -o "${tmp}"
    -w '%{http_code}'
    --get
    --data-urlencode "action=create"
    --data-urlencode "user_id=-1"
    --data-urlencode "amount=1"
    --data-urlencode "ps=${ps}"
  )
  if [[ "${brand_id}" != "__omit__" ]]; then
    curl_args+=(--data-urlencode "brand_id=${brand_id}")
  fi
  curl_args+=("${base}${SHM_YK_CGI_PATH}")

  set +e
  code="$("${curl_args[@]}")"
  curl_rc=$?
  set -e

  body="$(cat "${tmp}" 2>/dev/null || true)"
  rm -f "${tmp}"

  if [[ "${curl_rc}" -ne 0 ]]; then
    echo "${label}: transport/timeout error (curl rc=${curl_rc})" >&2
    return 1
  fi

  local brand_note
  if [[ "${brand_id}" == "__omit__" ]]; then
    brand_note="brand_id=<absent>"
  else
    brand_note="brand_id=${brand_id}"
  fi
  echo "${label}: ${base}${SHM_YK_CGI_PATH} (${brand_note}) -> ${code}"

  if [[ "${code}" != "400" ]]; then
    echo "${label}: expected HTTP 400, got ${code}" >&2
    if [[ -n "${body}" ]]; then
      # Avoid dumping secrets; CGI error bodies are short plain text/JSON.
      echo "${label}: body: ${body}" >&2
    fi
    return 1
  fi
  if ! shm_yookassa_probe_body_has "${body}" "${expected}"; then
    echo "${label}: HTTP 400 without expected '${expected}'" >&2
    echo "${label}: body: ${body}" >&2
    return 1
  fi
  echo "${label}: OK (${expected})"
  return 0
}

# shm_yookassa_run_brand_routing_probes <api_base> <pay_system>
# Runs the four safe probes required after CGI brand-routing deploy.
shm_yookassa_run_brand_routing_probes() {
  local api_base="${1:-}"
  local ps="${2:-}"
  local label="${SHM_YK_PROBE_LABEL:-shm-yookassa-probe}"

  echo "${label}: probe vff + user_id=-1 (expect unknown user)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" "vff" || return 1

  echo "${label}: probe fc + user_id=-1 (expect unknown user)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" "fc" || return 1

  echo "${label}: probe invalid brand_id (expect unknown brand_id)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown brand_id" "not-a-brand" || return 1

  echo "${label}: probe without brand_id (expect unknown user / legacy)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" || return 1

  return 0
}
