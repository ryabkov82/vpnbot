#!/usr/bin/env bash
# Safe post-deploy probes for brand-aware SHM YooKassa CGI.
# Create probes never create a payment (user_id=-1).
# Route-check probes only read the public brand return_url mapping.
#
# shellcheck shell=bash

# shellcheck source=smoke_yookassa_cgi.sh
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=smoke_yookassa_cgi.sh
source "${SOURCE_DIR}/smoke_yookassa_cgi.sh"

SHM_YK_CGI_PATH="${SMOKE_YOOKASSA_CGI_PATH:-/shm/pay_systems/yookassa.cgi}"
SHM_YK_CGI_MAX_TIME="${SMOKE_YOOKASSA_CGI_MAX_TIME:-10}"

# Default brand profiles for expected route-check URLs (override via env).
SHM_YK_PROBES_ROOT="$(cd "${SOURCE_DIR}/../.." && pwd)"
SHM_YK_VFF_PROFILE="${SHM_YK_VFF_PROFILE:-${SHM_YK_PROBES_ROOT}/deploy/brands/vff.json}"
SHM_YK_FC_PROFILE="${SHM_YK_FC_PROFILE:-${SHM_YK_PROBES_ROOT}/deploy/brands/fc.json}"

shm_yookassa_probe_body_has() {
  local body="${1:-}"
  local needle="${2:-}"
  [[ -n "${body}" && -n "${needle}" ]] || return 1
  local lc needle_lc
  lc="$(printf '%s' "${body}" | tr '[:upper:]' '[:lower:]')"
  needle_lc="$(printf '%s' "${needle}" | tr '[:upper:]' '[:lower:]')"
  [[ "${lc}" == *"${needle_lc}"* ]]
}

# shm_yookassa_return_url_from_profile <brand-profile.json>
# Prints: <public_base_url>/payment/return (trailing slash normalized).
shm_yookassa_return_url_from_profile() {
  local profile="${1:-}"
  if [[ -z "${profile}" || ! -f "${profile}" ]]; then
    echo "shm-yookassa-probe: brand profile required: ${profile}" >&2
    return 1
  fi
  python3 - <<'PY' "${profile}"
import json, sys
path = sys.argv[1]
data = json.load(open(path, encoding="utf-8"))
base = data["brand"]["public_base_url"].strip().rstrip("/")
brand_id = data["id"]
if not brand_id or not base:
    raise SystemExit("invalid brand profile")
print(f"{base}/payment/return")
PY
}

# shm_yookassa_probe_cgi <api_base> <pay_system> <expected_needle> [brand_id]
# brand_id may be omitted (pass nothing / use sentinel via 4th empty with omit).
# Use brand_id=__omit__ or omit 4th arg via calling with only 3 args — for
# absent brand_id pass no 4th argument by using empty and special handling:
# callers pass "" with a 5th flag, or use shm_yookassa_probe_cgi_create helpers.
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
  echo "${label}: create ${base}${SHM_YK_CGI_PATH} (${brand_note}) -> ${code}"

  if [[ "${code}" != "400" ]]; then
    echo "${label}: expected HTTP 400, got ${code}" >&2
    if [[ -n "${body}" ]]; then
      echo "${label}: body: ${body}" >&2
    fi
    return 1
  fi
  if ! shm_yookassa_probe_body_has "${body}" "${expected}"; then
    echo "${label}: HTTP 400 without expected '${expected}'" >&2
    echo "${label}: body: ${body}" >&2
    return 1
  fi
  echo "${label}: OK create (${expected})"
  return 0
}

# shm_yookassa_probe_route_check <api_base> <pay_system> <brand_id|__omit__> <mode> [expected_return_url]
# mode=ok     → HTTP 200 + JSON status/brand_id/return_url
# mode=reject → HTTP 400 + unknown brand_id
shm_yookassa_probe_route_check() {
  local base="${1:-}"
  local ps="${2:-}"
  local brand_id="${3-__omit__}"
  local mode="${4:-}"
  local expected_url="${5:-}"
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
  if [[ "${mode}" != "ok" && "${mode}" != "reject" ]]; then
    echo "${label}: route_check mode must be ok|reject" >&2
    return 1
  fi
  if [[ "${mode}" == "ok" && -z "${expected_url}" ]]; then
    echo "${label}: expected return_url required for ok mode" >&2
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
    --data-urlencode "action=vpnbot_route_check"
    --data-urlencode "ps=${ps}"
  )
  # Intentionally omit user_id/amount/email/description.
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
    echo "${label}: route_check transport/timeout error (curl rc=${curl_rc})" >&2
    return 1
  fi

  local brand_note
  if [[ "${brand_id}" == "__omit__" ]]; then
    brand_note="brand_id=<absent>"
  else
    brand_note="brand_id=${brand_id}"
  fi
  echo "${label}: route_check ${base}${SHM_YK_CGI_PATH} (${brand_note}) -> ${code}"

  if [[ "${mode}" == "reject" ]]; then
    if [[ "${code}" != "400" ]]; then
      echo "${label}: route_check expected HTTP 400, got ${code}" >&2
      echo "${label}: body: ${body}" >&2
      return 1
    fi
    if ! shm_yookassa_probe_body_has "${body}" "unknown brand_id"; then
      echo "${label}: route_check HTTP 400 without unknown brand_id" >&2
      echo "${label}: body: ${body}" >&2
      return 1
    fi
    echo "${label}: OK route_check (unknown brand_id)"
    return 0
  fi

  if [[ "${code}" != "200" ]]; then
    echo "${label}: route_check expected HTTP 200, got ${code}" >&2
    echo "${label}: body: ${body}" >&2
    return 1
  fi

  if ! python3 - <<'PY' "${body}" "${brand_id}" "${expected_url}"
import json, sys
body, brand_id, expected_url = sys.argv[1:4]
try:
    data = json.loads(body)
except Exception as exc:
    print(f"invalid JSON: {exc}", file=sys.stderr)
    raise SystemExit(1)
if not isinstance(data, dict):
    print("JSON root must be object", file=sys.stderr)
    raise SystemExit(1)
for key in ("status", "brand_id", "return_url"):
    if key not in data:
        print(f"missing field {key}", file=sys.stderr)
        raise SystemExit(1)
# Reject unexpected credential-like fields.
forbidden = ("api_key", "account_id", "password", "secret", "callback", "shop_id")
for key in data:
    lk = str(key).lower()
    if any(f in lk for f in forbidden):
        print(f"forbidden field present: {key}", file=sys.stderr)
        raise SystemExit(1)
if data.get("status") != 200:
    print(f"status field want 200 got {data.get('status')!r}", file=sys.stderr)
    raise SystemExit(1)
if data.get("brand_id") != brand_id:
    print(f"brand_id want {brand_id!r} got {data.get('brand_id')!r}", file=sys.stderr)
    raise SystemExit(1)
if data.get("return_url") != expected_url:
    print(
        f"return_url want {expected_url!r} got {data.get('return_url')!r}",
        file=sys.stderr,
    )
    raise SystemExit(1)
raise SystemExit(0)
PY
  then
    echo "${label}: route_check JSON validation failed" >&2
    echo "${label}: body: ${body}" >&2
    return 1
  fi

  echo "${label}: OK route_check (${brand_id} → ${expected_url})"
  return 0
}

# shm_yookassa_run_route_check_probes <api_base> <pay_system>
shm_yookassa_run_route_check_probes() {
  local api_base="${1:-}"
  local ps="${2:-}"
  local label="${SHM_YK_PROBE_LABEL:-shm-yookassa-probe}"
  local vff_url fc_url

  vff_url="$(shm_yookassa_return_url_from_profile "${SHM_YK_VFF_PROFILE}")" || return 1
  fc_url="$(shm_yookassa_return_url_from_profile "${SHM_YK_FC_PROFILE}")" || return 1

  echo "${label}: route_check vff (expect ${vff_url})"
  shm_yookassa_probe_route_check "${api_base}" "${ps}" "vff" "ok" "${vff_url}" || return 1

  echo "${label}: route_check fc (expect ${fc_url})"
  shm_yookassa_probe_route_check "${api_base}" "${ps}" "fc" "ok" "${fc_url}" || return 1

  echo "${label}: route_check invalid brand_id (expect unknown brand_id)"
  shm_yookassa_probe_route_check "${api_base}" "${ps}" "not-a-brand" "reject" || return 1

  echo "${label}: route_check without brand_id (expect unknown brand_id)"
  shm_yookassa_probe_route_check "${api_base}" "${ps}" "__omit__" "reject" || return 1

  return 0
}

# shm_yookassa_run_create_probes <api_base> <pay_system>
shm_yookassa_run_create_probes() {
  local api_base="${1:-}"
  local ps="${2:-}"
  local label="${SHM_YK_PROBE_LABEL:-shm-yookassa-probe}"

  echo "${label}: create vff + user_id=-1 (expect unknown user)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" "vff" || return 1

  echo "${label}: create fc + user_id=-1 (expect unknown user)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" "fc" || return 1

  echo "${label}: create invalid brand_id (expect unknown brand_id)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown brand_id" "not-a-brand" || return 1

  echo "${label}: create without brand_id (expect unknown user / legacy)"
  shm_yookassa_probe_cgi "${api_base}" "${ps}" "unknown user" || return 1

  return 0
}

# shm_yookassa_run_brand_routing_probes <api_base> <pay_system>
# Full post-deploy suite: route-check + create probes.
shm_yookassa_run_brand_routing_probes() {
  local api_base="${1:-}"
  local ps="${2:-}"
  shm_yookassa_run_route_check_probes "${api_base}" "${ps}" || return 1
  shm_yookassa_run_create_probes "${api_base}" "${ps}" || return 1
  return 0
}
