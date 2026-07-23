#!/usr/bin/env bash
# Manage host-mounted SHM YooKassa CGI brand return_url routing from vpnbot.
#
# Modes:
#   check     download current CGI, patch locally, report status (no install)
#   diff      same as check, then show a redacted unified diff
#   deploy    backup + atomic install + perl -c + safe probes; auto-rollback on failure
#   rollback  restore a timestamped backup (BACKUP=/path/on/shm-host)
#
# Defaults (overridable):
#   SHM_HOST=ru-msk-1
#   SHM_USER=root
#   SHM_DIR=/opt/shm
#   SHM_CORE_SERVICE=core
#
# Brand mapping is read from deploy/brands/*.json (id + brand.public_base_url).
# Never prints YooKassa credentials or full pay-system config.
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT}"

# shellcheck source=lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"
# shellcheck source=lib/shm_yookassa_probes.sh
source "${ROOT}/scripts/lib/shm_yookassa_probes.sh"

SHM_HOST="${SHM_HOST:-ru-msk-1}"
SHM_USER="${SHM_USER:-root}"
SHM_DIR="${SHM_DIR:-/opt/shm}"
SHM_COMPOSE_DIR="${SHM_COMPOSE_DIR:-${SHM_DIR}}"
SHM_CORE_SERVICE="${SHM_CORE_SERVICE:-core}"

CGI_REL="pay_systems/yookassa.cgi"
CGI_HOST_PATH="${SHM_DIR}/${CGI_REL}"
CGI_CONTAINER_DIR="/app/data/pay_systems"
CGI_CONTAINER_PATH="${CGI_CONTAINER_DIR}/yookassa.cgi"

PATCHER="${ROOT}/deploy/shm/yookassa/patch_yookassa.py"
DEFAULT_PROFILES=(
  "${ROOT}/deploy/brands/vff.json"
  "${ROOT}/deploy/brands/fc.json"
)

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
  bash scripts/deploy-shm-yookassa.sh check
  bash scripts/deploy-shm-yookassa.sh diff
  bash scripts/deploy-shm-yookassa.sh deploy
  bash scripts/deploy-shm-yookassa.sh rollback   # requires BACKUP=/path/on/host

env overrides:
  SHM_HOST SHM_USER SHM_DIR SHM_COMPOSE_DIR SHM_CORE_SERVICE
  SHM_YK_BRAND_PROFILES   space-separated brand profile paths
  SHM_YK_PROBE_API_BASE   optional: skip remote config fetch for probes
  SHM_YK_PROBE_PAY_SYSTEM optional: default yookassa
EOF
}

if [[ -z "${MODE}" ]]; then
  usage
  exit 1
fi

case "${MODE}" in
  check|diff|deploy|rollback) ;;
  -h|--help|help)
    usage
    exit 0
    ;;
  *)
    brand_err "unknown mode: ${MODE}"
    usage
    exit 1
    ;;
esac

PROFILE_ARGS=()
if [[ -n "${SHM_YK_BRAND_PROFILES:-}" ]]; then
  # shellcheck disable=SC2206
  _profiles=( ${SHM_YK_BRAND_PROFILES} )
  for p in "${_profiles[@]}"; do
    PROFILE_ARGS+=(--brand-profile "${p}")
  done
else
  for p in "${DEFAULT_PROFILES[@]}"; do
    PROFILE_ARGS+=(--brand-profile "${p}")
  done
fi

LOCAL_TMP=""
REMOTE_CANDIDATE=""
REMOTE_BACKUP=""
INSTALLED=0

cleanup() {
  local ec=$?
  if [[ "${INSTALLED}" -eq 1 && -n "${REMOTE_BACKUP}" && "${ec}" -ne 0 ]]; then
    echo "deploy-shm-yookassa: failure after install; restoring backup ${REMOTE_BACKUP}" >&2
    ssh -o BatchMode=yes -o ConnectTimeout=20 \
      "${SHM_USER}@${SHM_HOST}" \
      "set -Eeuo pipefail; cp -a -- $(printf %q "${REMOTE_BACKUP}") $(printf %q "${CGI_HOST_PATH}")" \
      >/dev/null 2>&1 || true
  fi
  if [[ -n "${REMOTE_CANDIDATE}" ]]; then
    ssh -o BatchMode=yes -o ConnectTimeout=15 \
      "${SHM_USER}@${SHM_HOST}" \
      "rm -f -- $(printf %q "${REMOTE_CANDIDATE}")" \
      >/dev/null 2>&1 || true
  fi
  if [[ -n "${LOCAL_TMP}" && -d "${LOCAL_TMP}" ]]; then
    rm -rf "${LOCAL_TMP}"
  fi
  return "${ec}"
}
trap cleanup EXIT

redact_diff() {
  # Never print credential-looking lines or huge dumps.
  # CGI itself should not contain secrets; still filter defensively.
  sed -E \
    -e '/api_key/Id' \
    -e '/account_id/Id' \
    -e '/authorization/Id' \
    -e '/password/Id' \
    -e '/secret/Id' \
    -e '/Bearer /Id' \
    -e 's/(sk_live_[A-Za-z0-9]+)/[REDACTED]/g' \
    -e 's/(sk_test_[A-Za-z0-9]+)/[REDACTED]/g'
}

ssh_shm() {
  ssh -o BatchMode=yes -o ConnectTimeout=20 "${SHM_USER}@${SHM_HOST}" "$@"
}

download_cgi() {
  local dest="$1"
  echo "deploy-shm-yookassa: downloading ${SHM_USER}@${SHM_HOST}:${CGI_HOST_PATH}"
  if ! scp -q -o BatchMode=yes -o ConnectTimeout=20 \
    "${SHM_USER}@${SHM_HOST}:${CGI_HOST_PATH}" "${dest}"; then
    brand_err "deploy-shm-yookassa: failed to download current CGI"
    return 1
  fi
  if [[ ! -s "${dest}" ]]; then
    brand_err "deploy-shm-yookassa: downloaded CGI is empty"
    return 1
  fi
}

run_patcher() {
  local src="$1"
  local out="$2"
  python3 "${PATCHER}" \
    --source "${src}" \
    --output "${out}" \
    "${PROFILE_ARGS[@]}"
}

summarize_patch_delta() {
  local src="$1"
  local out="$2"
  if cmp -s "${src}" "${out}"; then
    echo "deploy-shm-yookassa: patch result identical to current CGI (routing already current)"
    return 0
  fi
  local added
  added="$(diff -u "${src}" "${out}" | grep -c '^+.*VPNBOT_BRAND_ROUTING' || true)"
  echo "deploy-shm-yookassa: patch would change CGI (routing markers touched≈${added})"
  if grep -q 'VPNBOT_BRAND_ROUTING_VERSION=1' "${src}"; then
    echo "deploy-shm-yookassa: note: source already has VERSION=1; regenerating from brand profiles"
  else
    echo "deploy-shm-yookassa: note: source has no brand routing marker; will insert VERSION=1"
  fi
}

show_redacted_diff() {
  local src="$1"
  local out="$2"
  echo "deploy-shm-yookassa: redacted diff (secrets filtered):"
  if cmp -s "${src}" "${out}"; then
    echo "(no differences)"
    return 0
  fi
  diff -u "${src}" "${out}" | redact_diff || true
}

resolve_probe_api_base() {
  if [[ -n "${SHM_YK_PROBE_API_BASE:-}" ]]; then
    printf '%s\n' "${SHM_YK_PROBE_API_BASE}"
    return 0
  fi

  # Prefer VFF runtime explicit config (shared SHM api.base_url).
  brand_profile_load "vff" || return 1
  local tmp
  tmp="$(smoke_yookassa_cgi_fetch_remote_config)" || return 1
  local api_base
  api_base="$(jq -r '.api.base_url // empty' "${tmp}")"
  rm -f "${tmp}"
  api_base="$(printf '%s' "${api_base}" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')"
  api_base="${api_base%/}"
  if ! smoke_yookassa_cgi_api_base_ok "${api_base}"; then
    brand_err "deploy-shm-yookassa: could not resolve api.base_url from VFF runtime config"
    return 1
  fi
  printf '%s\n' "${api_base}"
}

run_probes() {
  local api_base ps
  api_base="$(resolve_probe_api_base)" || return 1
  ps="${SHM_YK_PROBE_PAY_SYSTEM:-yookassa}"
  echo "deploy-shm-yookassa: running safe probes against api.base_url (not brand public URL)"
  SHM_YK_PROBE_LABEL="deploy-shm-yookassa" \
    shm_yookassa_run_brand_routing_probes "${api_base}" "${ps}"
}

remote_stat_meta() {
  # Prints: mode\tuid\tgid
  ssh_shm "stat -c '%a %u %g' -- $(printf %q "${CGI_HOST_PATH}")"
}

remote_perl_c() {
  local candidate_host="$1"
  local candidate_name
  candidate_name="$(basename "${candidate_host}")"
  local candidate_container="${CGI_CONTAINER_DIR}/${candidate_name}"
  echo "deploy-shm-yookassa: perl -c inside ${SHM_CORE_SERVICE} on ${candidate_container}"
  ssh_shm "set -Eeuo pipefail; cd $(printf %q "${SHM_COMPOSE_DIR}"); docker compose exec -T $(printf %q "${SHM_CORE_SERVICE}") perl -c $(printf %q "${candidate_container}")"
}

do_check_or_diff() {
  LOCAL_TMP="$(mktemp -d)"
  chmod 0700 "${LOCAL_TMP}"
  local src="${LOCAL_TMP}/yookassa.cgi.current"
  local out="${LOCAL_TMP}/yookassa.cgi.patched"
  download_cgi "${src}"
  echo "deploy-shm-yookassa: running patcher against brand profiles"
  run_patcher "${src}" "${out}"
  summarize_patch_delta "${src}" "${out}"
  if [[ "${MODE}" == "diff" ]]; then
    show_redacted_diff "${src}" "${out}"
  fi
  echo "deploy-shm-yookassa: ${MODE} OK"
}

do_deploy() {
  LOCAL_TMP="$(mktemp -d)"
  chmod 0700 "${LOCAL_TMP}"
  local src="${LOCAL_TMP}/yookassa.cgi.current"
  local out="${LOCAL_TMP}/yookassa.cgi.patched"
  download_cgi "${src}"
  echo "deploy-shm-yookassa: running patcher against brand profiles"
  run_patcher "${src}" "${out}"
  summarize_patch_delta "${src}" "${out}"
  show_redacted_diff "${src}" "${out}"

  if cmp -s "${src}" "${out}"; then
    echo "deploy-shm-yookassa: nothing to install; running probes only"
    run_probes
    echo "deploy-shm-yookassa: deploy OK (already current)"
    return 0
  fi

  local meta mode uid gid
  meta="$(remote_stat_meta)"
  mode="$(awk '{print $1}' <<<"${meta}")"
  uid="$(awk '{print $2}' <<<"${meta}")"
  gid="$(awk '{print $3}' <<<"${meta}")"
  if [[ -z "${mode}" || -z "${uid}" || -z "${gid}" ]]; then
    brand_err "deploy-shm-yookassa: failed to read CGI mode/owner/group"
    return 1
  fi
  echo "deploy-shm-yookassa: preserving mode=${mode} uid=${uid} gid=${gid}"

  local ts
  ts="$(date -u +%Y%m%dT%H%M%SZ)"
  REMOTE_BACKUP="${CGI_HOST_PATH}.bak.${ts}"
  REMOTE_CANDIDATE="${CGI_HOST_PATH}.vpnbot-candidate.${ts}"

  echo "deploy-shm-yookassa: creating backup ${REMOTE_BACKUP}"
  ssh_shm "set -Eeuo pipefail; cp -a -- $(printf %q "${CGI_HOST_PATH}") $(printf %q "${REMOTE_BACKUP}")"

  echo "deploy-shm-yookassa: uploading candidate"
  scp -q -o BatchMode=yes -o ConnectTimeout=20 \
    "${out}" "${SHM_USER}@${SHM_HOST}:${REMOTE_CANDIDATE}"

  ssh_shm "set -Eeuo pipefail; chmod $(printf %q "${mode}") -- $(printf %q "${REMOTE_CANDIDATE}"); chown $(printf %q "${uid}"):$(printf %q "${gid}") -- $(printf %q "${REMOTE_CANDIDATE}")"

  if ! remote_perl_c "${REMOTE_CANDIDATE}"; then
    brand_err "deploy-shm-yookassa: perl -c failed; candidate not installed"
    ssh_shm "rm -f -- $(printf %q "${REMOTE_CANDIDATE}")" >/dev/null 2>&1 || true
    REMOTE_CANDIDATE=""
    return 1
  fi

  echo "deploy-shm-yookassa: atomic install via rename"
  ssh_shm "set -Eeuo pipefail; mv -f -- $(printf %q "${REMOTE_CANDIDATE}") $(printf %q "${CGI_HOST_PATH}")"
  REMOTE_CANDIDATE=""
  INSTALLED=1

  if ! run_probes; then
    brand_err "deploy-shm-yookassa: probes failed; restoring backup"
    ssh_shm "set -Eeuo pipefail; cp -a -- $(printf %q "${REMOTE_BACKUP}") $(printf %q "${CGI_HOST_PATH}")"
    INSTALLED=0
    return 1
  fi

  INSTALLED=0
  echo "deploy-shm-yookassa: deploy OK (backup kept at ${REMOTE_BACKUP})"
}

do_rollback() {
  local backup="${BACKUP:-}"
  if [[ -z "${backup}" ]]; then
    brand_err "deploy-shm-yookassa: rollback requires BACKUP=/absolute/path/on/${SHM_HOST}"
    return 1
  fi
  if [[ "${backup}" != /* ]]; then
    brand_err "deploy-shm-yookassa: BACKUP must be an absolute path on the SHM host"
    return 1
  fi
  # Safety: only allow backups of the managed CGI.
  case "${backup}" in
    "${CGI_HOST_PATH}".bak.*) ;;
    *)
      brand_err "deploy-shm-yookassa: refusing BACKUP outside ${CGI_HOST_PATH}.bak.*"
      return 1
      ;;
  esac

  echo "deploy-shm-yookassa: restoring ${backup} -> ${CGI_HOST_PATH}"
  ssh_shm "set -Eeuo pipefail; test -f $(printf %q "${backup}"); cp -a -- $(printf %q "${backup}") $(printf %q "${CGI_HOST_PATH}")"
  echo "deploy-shm-yookassa: rollback OK"
}

case "${MODE}" in
  check|diff) do_check_or_diff ;;
  deploy) do_deploy ;;
  rollback) do_rollback ;;
esac
