#!/usr/bin/env bash
# Manage SHM tg_payments_webapp template brand_id routing from vpnbot.
#
# Modes: check | diff | deploy | rollback
#
# Config (Basic Auth + api.base_url):
#   CONFIG=/path/to/config.json
#   else download root@fr-mrs-1:/opt/bot/config-vff.json
#
# Template backups on SHM host (default ru-msk-1):
#   /opt/shm/template-backups/tg_payments_webapp.<UTC>.html
#
# Never prints api_pass / credentials.
# Informational logs go to stderr; $(...) helpers print path-only on stdout.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=lib/shm_tg_payments_config.sh
source "${ROOT}/scripts/lib/shm_tg_payments_config.sh"

PATCHER="${ROOT}/deploy/shm/templates/tg_payments_webapp/patch_template.py"
TEMPLATE_ID="tg_payments_webapp"
MARKER="VPNBOT_TG_PAYMENT_ROUTING_VERSION=1"

CONFIG_HOST="${TG_TEMPLATE_CONFIG_HOST:-fr-mrs-1}"
CONFIG_USER="${TG_TEMPLATE_CONFIG_USER:-root}"
CONFIG_REMOTE_PATH="${TG_TEMPLATE_CONFIG_REMOTE:-/opt/bot/config-vff.json}"

SHM_BACKUP_HOST="${SHM_BACKUP_HOST:-ru-msk-1}"
SHM_BACKUP_USER="${SHM_BACKUP_USER:-root}"
SHM_BACKUP_DIR="${SHM_BACKUP_DIR:-/opt/shm/template-backups}"

MODE="${1:-}"
if [[ "${MODE}" == "--mode" ]]; then
  MODE="${2:-}"
  shift 2 || true
elif [[ $# -ge 1 ]]; then
  shift || true
fi

usage() {
  cat <<'EOF' >&2
usage:
  bash scripts/deploy-shm-tg-payments-template.sh check
  bash scripts/deploy-shm-tg-payments-template.sh diff
  bash scripts/deploy-shm-tg-payments-template.sh deploy
  bash scripts/deploy-shm-tg-payments-template.sh rollback  # BACKUP=/path/on/ru-msk-1

env:
  CONFIG                 local runtime/candidate config JSON
  TG_TEMPLATE_CONFIG_*   override remote VFF config fetch
  SHM_BACKUP_HOST/USER/DIR
EOF
}

if [[ -z "${MODE}" ]]; then
  usage
  exit 1
fi
case "${MODE}" in
  check|diff|deploy|rollback) ;;
  -h|--help|help) usage; exit 0 ;;
  *) brand_err "unknown mode: ${MODE}"; usage; exit 1 ;;
esac

LOCAL_TMP=""
API_BASE=""
API_LOGIN=""
API_PASS=""
ADMIN_URL=""
PUBLIC_URL=""
INSTALLED_BODY=""
REMOTE_BACKUP=""

cleanup() {
  local ec=$?
  if [[ -n "${INSTALLED_BODY}" && -n "${LOCAL_TMP}" && -f "${LOCAL_TMP}/backup.local.html" && "${ec}" -ne 0 ]]; then
    shm_tg_payments_log "failure after POST; restoring backup via POST"
    _post_template_file "${LOCAL_TMP}/backup.local.html" >/dev/null 2>&1 || true
  fi
  if [[ -n "${LOCAL_TMP}" && -d "${LOCAL_TMP}" ]]; then
    rm -rf "${LOCAL_TMP}"
  fi
  return "${ec}"
}
trap cleanup EXIT

ensure_tmp() {
  if [[ -n "${LOCAL_TMP}" && -d "${LOCAL_TMP}" ]]; then
    return 0
  fi
  LOCAL_TMP="$(mktemp -d)"
  chmod 0700 "${LOCAL_TMP}"
}

redact() {
  sed -E \
    -e '/api_pass/Id' \
    -e '/api_key/Id' \
    -e '/password/Id' \
    -e '/secret/Id' \
    -e '/Authorization/Id' \
    -e 's/(Basic [A-Za-z0-9+\/=]+)/Basic [REDACTED]/g'
}

# Resolve runtime config into LOCAL_TMP; validate file exists.
# Sets RUNTIME_CONFIG. Does not print the path to the caller via stdout.
prepare_runtime_config() {
  ensure_tmp
  local cfg
  # Command substitution keeps only the helper stdout (path). Logs stay on stderr.
  cfg="$(shm_tg_payments_resolve_config)" || return 1
  if [[ -z "${cfg}" || "${cfg}" == *$'\n'* || "${cfg}" == *$'\r'* ]]; then
    brand_err "deploy-tg-payments: resolve_config returned empty or multi-line path"
    return 1
  fi
  shm_tg_payments_require_config_file "${cfg}" || return 1
  RUNTIME_CONFIG="${cfg}"
}

load_api_from_config() {
  local cfg="$1"
  if ! command -v jq >/dev/null 2>&1; then
    brand_err "deploy-tg-payments: jq is required"
    return 1
  fi
  shm_tg_payments_require_config_file "${cfg}" || return 1

  API_BASE="$(jq -r '.api.base_url // empty' "${cfg}")"
  API_LOGIN="$(jq -r '.api.api_login // empty' "${cfg}")"
  API_PASS="$(jq -r '.api.api_pass // empty' "${cfg}")"
  API_BASE="$(printf '%s' "${API_BASE}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  API_BASE="${API_BASE%/}"
  if [[ -z "${API_BASE}" || ! "${API_BASE}" =~ ^https?:// ]]; then
    brand_err "deploy-tg-payments: config api.base_url missing/invalid"
    return 1
  fi
  if [[ -z "${API_LOGIN}" || -z "${API_PASS}" ]]; then
    brand_err "deploy-tg-payments: config api.api_login/api_pass missing"
    return 1
  fi
  ADMIN_URL="${API_BASE}/shm/v1/admin/template/${TEMPLATE_ID}"
  PUBLIC_URL="${API_BASE}/shm/v1/public/${TEMPLATE_ID}"
  shm_tg_payments_log "using api.base_url from config (credentials not printed)"
}

_curl_admin_get() {
  local out="$1"
  local code
  code="$(
    curl -sS --max-time 30 \
      -u "${API_LOGIN}:${API_PASS}" \
      -o "${out}" \
      -w '%{http_code}' \
      "${ADMIN_URL}"
  )"
  [[ "${code}" == "200" ]] || {
    brand_err "deploy-tg-payments: admin GET HTTP ${code}"
    return 1
  }
  [[ -s "${out}" ]] || {
    brand_err "deploy-tg-payments: admin GET empty body"
    return 1
  }
}

_post_template_file() {
  local file="$1"
  local code tmp
  tmp="${LOCAL_TMP}/post-response.body"
  code="$(
    curl -sS --max-time 60 \
      -u "${API_LOGIN}:${API_PASS}" \
      -H 'Content-Type: text/plain; charset=utf-8' \
      --data-binary @"${file}" \
      -o "${tmp}" \
      -w '%{http_code}' \
      -X POST \
      "${ADMIN_URL}"
  )"
  if [[ "${code}" != "200" && "${code}" != "201" && "${code}" != "204" ]]; then
    brand_err "deploy-tg-payments: admin POST HTTP ${code}"
    return 1
  fi
  return 0
}

_post_template_from_remote_backup() {
  local remote="$1"
  local local_bak="${LOCAL_TMP}/rollback.body"
  scp -q -o BatchMode=yes -o ConnectTimeout=20 \
    "${SHM_BACKUP_USER}@${SHM_BACKUP_HOST}:${remote}" "${local_bak}"
  _post_template_file "${local_bak}"
}

static_candidate_checks() {
  local file="$1"
  shm_tg_payments_log "static candidate checks"
  grep -Fq "${MARKER}" "${file}" || {
    brand_err "marker missing"; return 1
  }
  grep -Fq "urlParams.get('brand_id')" "${file}" || {
    brand_err "brand_id read missing"; return 1
  }
  grep -Fq "urlParams.get('yookassa_ps')" "${file}" || {
    brand_err "yookassa_ps read missing"; return 1
  }
  grep -Fq "searchParams.set('brand_id'" "${file}" || {
    brand_err "brand_id set missing"; return 1
  }
  grep -Fq "actualPs === ShmPayApp.yookassaPaySystem" "${file}" || {
    brand_err "ps match guard missing"; return 1
  }
  grep -Fq "var rawPaymentURL = shm_url + amount;" "${file}" || {
    brand_err "amount-first contract missing"; return 1
  }
  grep -Fq "searchParams.set('email'" "${file}" || {
    brand_err "email searchParams missing"; return 1
  }
  grep -Fq "{{ config.api.url }}/shm/v1/telegram/webapp/auth" "${file}" || {
    brand_err "auth URL changed"; return 1
  }
  grep -Fq "{{ config.api.url }}/shm/v1/user/pay/paysystems" "${file}" || {
    brand_err "paysystems URL changed"; return 1
  }
  if grep -qiE 'api_pass|api_key|sk_live_|sk_test_' "${file}"; then
    brand_err "candidate appears to contain credentials"
    return 1
  fi
  if grep -Fq "shm_url + amount + '&email='" "${file}"; then
    brand_err "legacy email concat remains"
    return 1
  fi
  return 0
}

public_probe() {
  local tmp code body
  tmp="${LOCAL_TMP}/public.body"
  code="$(
    curl -sS --max-time 20 \
      -o "${tmp}" \
      -w '%{http_code}' \
      "${PUBLIC_URL}"
  )"
  body="$(cat "${tmp}" 2>/dev/null || true)"
  shm_tg_payments_log "public GET ${PUBLIC_URL} -> ${code}"
  [[ "${code}" == "200" ]] || {
    brand_err "public endpoint HTTP ${code}"
    return 1
  }
  grep -Fq "${MARKER}" <<<"${body}" || {
    brand_err "public HTML missing ${MARKER}"
    return 1
  }
  shm_tg_payments_log "public probe OK (marker present; JS not executed)"
}

store_remote_backup() {
  local local_file="$1"
  local ts name
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  name="${TEMPLATE_ID}.${ts}.html"
  REMOTE_BACKUP="${SHM_BACKUP_DIR}/${name}"
  shm_tg_payments_log "storing backup ${SHM_BACKUP_USER}@${SHM_BACKUP_HOST}:${REMOTE_BACKUP}"
  ssh -o BatchMode=yes -o ConnectTimeout=20 \
    "${SHM_BACKUP_USER}@${SHM_BACKUP_HOST}" \
    "mkdir -p $(printf %q "${SHM_BACKUP_DIR}") && chmod 0750 $(printf %q "${SHM_BACKUP_DIR}")"
  scp -q -o BatchMode=yes -o ConnectTimeout=20 \
    "${local_file}" \
    "${SHM_BACKUP_USER}@${SHM_BACKUP_HOST}:${REMOTE_BACKUP}"
}

do_check_or_diff() {
  ensure_tmp
  prepare_runtime_config
  # Prove config still lives in shared TMP for the whole operation.
  shm_tg_payments_require_config_file "${RUNTIME_CONFIG}" || return 1
  load_api_from_config "${RUNTIME_CONFIG}"

  local current candidate
  current="${LOCAL_TMP}/current.html"
  candidate="${LOCAL_TMP}/candidate.html"
  shm_tg_payments_log "GET admin template"
  _curl_admin_get "${current}"
  shm_tg_payments_require_config_file "${RUNTIME_CONFIG}" || {
    brand_err "deploy-tg-payments: runtime config disappeared before patcher"
    return 1
  }
  shm_tg_payments_log "running patcher"
  python3 "${PATCHER}" --source "${current}" --output "${candidate}" >&2
  static_candidate_checks "${candidate}"
  if cmp -s "${current}" "${candidate}"; then
    shm_tg_payments_log "already at ${MARKER}"
  else
    shm_tg_payments_log "candidate differs from current"
    if [[ "${MODE}" == "diff" ]]; then
      shm_tg_payments_log "redacted diff:"
      diff -u "${current}" "${candidate}" | redact >&2 || true
    fi
  fi
  shm_tg_payments_log "${MODE} OK"
}

do_deploy() {
  ensure_tmp
  prepare_runtime_config
  load_api_from_config "${RUNTIME_CONFIG}"

  local current candidate verify
  current="${LOCAL_TMP}/current.html"
  candidate="${LOCAL_TMP}/candidate.html"
  verify="${LOCAL_TMP}/verify.html"

  _curl_admin_get "${current}"
  python3 "${PATCHER}" --source "${current}" --output "${candidate}" >&2
  static_candidate_checks "${candidate}"

  if cmp -s "${current}" "${candidate}"; then
    shm_tg_payments_log "nothing to POST; probing public"
    public_probe
    shm_tg_payments_log "deploy OK (already current)"
    return 0
  fi

  store_remote_backup "${current}"
  cp -a "${current}" "${LOCAL_TMP}/backup.local.html"

  shm_tg_payments_log "POST candidate (full body, UTF-8)"
  if ! _post_template_file "${candidate}"; then
    brand_err "deploy-tg-payments: POST failed; production unchanged"
    return 1
  fi
  INSTALLED_BODY=1

  shm_tg_payments_log "GET verify byte-for-byte"
  _curl_admin_get "${verify}"
  if ! cmp -s "${candidate}" "${verify}"; then
    brand_err "deploy-tg-payments: verification mismatch; rolling back"
    _post_template_file "${LOCAL_TMP}/backup.local.html"
    INSTALLED_BODY=""
    return 1
  fi

  if ! public_probe; then
    brand_err "deploy-tg-payments: public probe failed; rolling back"
    _post_template_file "${LOCAL_TMP}/backup.local.html"
    INSTALLED_BODY=""
    _curl_admin_get "${verify}"
    cmp -s "${LOCAL_TMP}/backup.local.html" "${verify}" || \
      brand_err "deploy-tg-payments: rollback verification failed"
    return 1
  fi

  INSTALLED_BODY=""
  shm_tg_payments_log "deploy OK (backup ${REMOTE_BACKUP})"
}

do_rollback() {
  local backup="${BACKUP:-}"
  if [[ -z "${backup}" ]]; then
    brand_err "deploy-tg-payments: rollback requires BACKUP=/absolute/path on ${SHM_BACKUP_HOST}"
    return 1
  fi
  if [[ "${backup}" != /* ]]; then
    brand_err "deploy-tg-payments: BACKUP must be absolute"
    return 1
  fi
  case "${backup}" in
    "${SHM_BACKUP_DIR}/${TEMPLATE_ID}".*.html) ;;
    *)
      brand_err "deploy-tg-payments: refusing BACKUP outside ${SHM_BACKUP_DIR}/${TEMPLATE_ID}.*.html"
      return 1
      ;;
  esac

  ensure_tmp
  prepare_runtime_config
  load_api_from_config "${RUNTIME_CONFIG}"
  local verify="${LOCAL_TMP}/verify.html"

  shm_tg_payments_log "restoring ${backup} via admin POST"
  _post_template_from_remote_backup "${backup}"
  _curl_admin_get "${verify}"
  local local_bak="${LOCAL_TMP}/rollback.body"
  cmp -s "${local_bak}" "${verify}" || {
    brand_err "deploy-tg-payments: rollback GET mismatch"
    return 1
  }
  shm_tg_payments_log "rollback OK"
}

case "${MODE}" in
  check|diff) do_check_or_diff ;;
  deploy) do_deploy ;;
  rollback) do_rollback ;;
esac
