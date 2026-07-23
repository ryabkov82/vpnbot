#!/usr/bin/env bash
# Safe, non-mutating smoke probe for SHM YooKassa CGI.
# Expects a controlled failure for a non-existent user (HTTP 400 + "unknown user").
# Does not create a payment.
#
# SHM/API base URL must come from runtime/candidate config `.api.base_url`.
# Never use brand.public_base_url / SMOKE_BASE_URL for this probe — brand web nginx
# is not required to proxy /shm/.
#
# Usage (after sourcing):
#   smoke_yookassa_cgi_check <shm-api-base-url> <yookassa_pay_system>
#   smoke_yookassa_cgi_check_from_config <config.json>
#   smoke_yookassa_cgi_read_config <config.json>  # prints: <api_base>\t<pay_system>
#
# Success: HTTP 400 and body contains "unknown user" (text or JSON).
# Failure: 2xx, 5xx, empty body, transport/timeout, or unexpected 400 payload.

SMOKE_YOOKASSA_CGI_PATH="/shm/pay_systems/yookassa.cgi"
SMOKE_YOOKASSA_CGI_MAX_TIME="${SMOKE_YOOKASSA_CGI_MAX_TIME:-10}"

smoke_yookassa_cgi_pay_system_ok() {
  local ps="${1:-}"
  [[ -n "${ps}" ]] || return 1
  [[ "${ps}" =~ ^[a-z0-9][a-z0-9_-]*$ ]] || return 1
  return 0
}

smoke_yookassa_cgi_api_base_ok() {
  local base="${1:-}"
  [[ -n "${base}" ]] || return 1
  [[ "${base}" =~ ^https?://[^[:space:]]+$ ]] || return 1
  return 0
}

# Returns 0 if body indicates the expected controlled rejection.
smoke_yookassa_cgi_body_ok() {
  local body="${1:-}"
  [[ -n "${body}" ]] || return 1
  local lc
  lc="$(printf '%s' "${body}" | tr '[:upper:]' '[:lower:]')"
  [[ "${lc}" == *"unknown user"* ]]
}

# smoke_yookassa_cgi_read_config <config.json>
# Prints: <api.base_url>\t<brand.yookassa_pay_system>
# Does not print secrets. Fail-closed on missing/invalid fields.
smoke_yookassa_cgi_read_config() {
  local cfg="${1:-}"
  local label="${SMOKE_YOOKASSA_LABEL:-yookassa-cgi}"
  if [[ -z "${cfg}" || ! -f "${cfg}" ]]; then
    echo "smoke-${label}: config file required for SHM/API base URL" >&2
    return 1
  fi
  if ! command -v jq >/dev/null 2>&1; then
    echo "smoke-${label}: jq is required to read api.base_url from config" >&2
    return 1
  fi

  local api_base ps public_base
  api_base="$(jq -r '.api.base_url // empty' "${cfg}" 2>/dev/null || true)"
  ps="$(jq -r '.brand.yookassa_pay_system // empty' "${cfg}" 2>/dev/null || true)"
  public_base="$(jq -r '.brand.public_base_url // empty' "${cfg}" 2>/dev/null || true)"

  api_base="$(printf '%s' "${api_base}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  api_base="${api_base%/}"
  ps="$(printf '%s' "${ps}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  public_base="$(printf '%s' "${public_base}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  public_base="${public_base%/}"

  if ! smoke_yookassa_cgi_api_base_ok "${api_base}"; then
    echo "smoke-${label}: config api.base_url is missing or invalid" >&2
    return 1
  fi
  if ! smoke_yookassa_cgi_pay_system_ok "${ps}"; then
    echo "smoke-${label}: config brand.yookassa_pay_system is missing or invalid" >&2
    return 1
  fi
  # Guard against accidental use of brand public URL as SHM base.
  if [[ -n "${public_base}" && "${api_base}" == "${public_base}" ]]; then
    echo "smoke-${label}: api.base_url must not equal brand.public_base_url (SHM is not brand web)" >&2
    return 1
  fi

  printf '%s\t%s\n' "${api_base}" "${ps}"
}

# smoke_yookassa_cgi_check_from_config <config.json>
smoke_yookassa_cgi_check_from_config() {
  local cfg="${1:-}"
  local label="${SMOKE_YOOKASSA_LABEL:-yookassa-cgi}"
  local pair api_base ps
  pair="$(smoke_yookassa_cgi_read_config "${cfg}")" || return 1
  api_base="${pair%%$'\t'*}"
  ps="${pair#*$'\t'}"
  echo "smoke-${label}: using api.base_url from config (not brand.public_base_url)"
  smoke_yookassa_cgi_check "${api_base}" "${ps}"
}

# Fetch runtime explicit config from the brand host (no secrets printed).
# Requires brand_profile_load exports: SERVER_USER SERVER_HOST REMOTE_EXPLICIT_CONFIG.
smoke_yookassa_cgi_fetch_remote_config() {
  local label="${SMOKE_YOOKASSA_LABEL:-yookassa-cgi}"
  local out
  if [[ -z "${SERVER_USER:-}" || -z "${SERVER_HOST:-}" || -z "${REMOTE_EXPLICIT_CONFIG:-}" ]]; then
    echo "smoke-${label}: remote config fetch requires SERVER_USER/SERVER_HOST/REMOTE_EXPLICIT_CONFIG" >&2
    return 1
  fi
  out="$(mktemp)"
  if ! ssh -o BatchMode=yes -o ConnectTimeout=15 \
    "${SERVER_USER}@${SERVER_HOST}" \
    "test -f $(printf %q "${REMOTE_EXPLICIT_CONFIG}") && cat $(printf %q "${REMOTE_EXPLICIT_CONFIG}")" \
    >"${out}"; then
    rm -f "${out}"
    echo "smoke-${label}: failed to fetch runtime config ${REMOTE_EXPLICIT_CONFIG} from ${SERVER_HOST}" >&2
    return 1
  fi
  printf '%s\n' "${out}"
}

# Resolve candidate/runtime config path:
#   1) explicit argument
#   2) SMOKE_CONFIG
#   3) CONFIG
# Returns path on stdout. Caller deletes temp files created for remote fetch
# when path is under ${TMPDIR:-/tmp} and named smoke-yk-*.json — handled by
# smoke_yookassa_cgi_resolve_and_check.
smoke_yookassa_cgi_resolve_config_path() {
  local explicit="${1:-}"
  local label="${SMOKE_YOOKASSA_LABEL:-yookassa-cgi}"
  local cfg="${explicit:-${SMOKE_CONFIG:-${CONFIG:-}}}"
  if [[ -n "${cfg}" ]]; then
    if [[ ! -f "${cfg}" ]]; then
      echo "smoke-${label}: config not found: ${cfg}" >&2
      return 1
    fi
    printf '%s\n' "${cfg}"
    return 0
  fi

  local tmp
  tmp="$(smoke_yookassa_cgi_fetch_remote_config)" || return 1
  printf '%s\n' "${tmp}"
}

# smoke_yookassa_cgi_resolve_and_check [optional-config-path]
smoke_yookassa_cgi_resolve_and_check() {
  local explicit="${1:-}"
  local provided="${explicit:-${SMOKE_CONFIG:-${CONFIG:-}}}"
  local cfg tmp_owned=0
  cfg="$(smoke_yookassa_cgi_resolve_config_path "${explicit}")" || return 1
  # Remote fetch returns a temp file; local CONFIG/SMOKE_CONFIG must not be deleted.
  if [[ -z "${provided}" ]]; then
    tmp_owned=1
  fi
  local rc=0
  smoke_yookassa_cgi_check_from_config "${cfg}" || rc=$?
  if [[ "${tmp_owned}" -eq 1 ]]; then
    rm -f "${cfg}"
  fi
  return "${rc}"
}

# smoke_yookassa_cgi_check <shm_api_base_url> <pay_system>
smoke_yookassa_cgi_check() {
  local base="${1:-}"
  local ps="${2:-}"
  local label="${SMOKE_YOOKASSA_LABEL:-yookassa-cgi}"

  base="${base%/}"
  base="$(printf '%s' "${base}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  ps="$(printf '%s' "${ps}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"

  if ! smoke_yookassa_cgi_api_base_ok "${base}"; then
    echo "smoke-${label}: SHM/API base URL (api.base_url) is missing or invalid" >&2
    return 1
  fi
  if ! smoke_yookassa_cgi_pay_system_ok "${ps}"; then
    echo "smoke-${label}: invalid or empty yookassa_pay_system" >&2
    return 1
  fi

  local tmp code curl_rc=0 body
  tmp="$(mktemp)"

  set +e
  code="$(
    curl -sS \
      --max-time "${SMOKE_YOOKASSA_CGI_MAX_TIME}" \
      -o "${tmp}" \
      -w '%{http_code}' \
      --get \
      --data-urlencode "action=create" \
      --data-urlencode "user_id=-1" \
      --data-urlencode "amount=1" \
      --data-urlencode "ps=${ps}" \
      "${base}${SMOKE_YOOKASSA_CGI_PATH}"
  )"
  curl_rc=$?
  set -e

  body="$(cat "${tmp}" 2>/dev/null || true)"
  rm -f "${tmp}"

  if [[ "${curl_rc}" -ne 0 ]]; then
    echo "smoke-${label}: transport/timeout error (curl rc=${curl_rc}) for ${SMOKE_YOOKASSA_CGI_PATH}" >&2
    return 1
  fi

  echo "smoke-${label}: ${base}${SMOKE_YOOKASSA_CGI_PATH} (ps from brand.yookassa_pay_system) -> ${code}"

  case "${code}" in
    '' | 000)
      echo "smoke-${label}: empty/zero HTTP status" >&2
      return 1
      ;;
    2*)
      echo "smoke-${label}: unexpected success HTTP ${code} (probe must not create a payment)" >&2
      return 1
      ;;
    5*)
      echo "smoke-${label}: unexpected HTTP ${code} from YooKassa CGI" >&2
      if [[ -n "${body}" ]]; then
        echo "smoke-${label}: body: ${body}" >&2
      else
        echo "smoke-${label}: empty response body" >&2
      fi
      return 1
      ;;
    400)
      if [[ -z "${body}" ]]; then
        echo "smoke-${label}: HTTP 400 with empty body" >&2
        return 1
      fi
      if ! smoke_yookassa_cgi_body_ok "${body}"; then
        echo "smoke-${label}: HTTP 400 without expected 'unknown user' rejection" >&2
        echo "smoke-${label}: body: ${body}" >&2
        return 1
      fi
      echo "smoke-${label}: OK (controlled unknown-user rejection)"
      return 0
      ;;
    *)
      echo "smoke-${label}: unexpected HTTP ${code}" >&2
      if [[ -n "${body}" ]]; then
        echo "smoke-${label}: body: ${body}" >&2
      fi
      return 1
      ;;
  esac
}
