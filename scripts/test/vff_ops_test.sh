#!/usr/bin/env bash
# Unit tests for scripts/lib/vff_ops.sh using PATH mocks.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/vff_ops.sh
source "${ROOT}/scripts/lib/vff_ops.sh"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

write_systemctl_mock() {
  cat >"${MOCK}/systemctl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
cmd="\${1:-}"
case "\$cmd" in
  show)
    # systemctl show <unit> -p KEY [--value]
    prop=""
    value_only=0
    shift || true
    while [[ \$# -gt 0 ]]; do
      case "\$1" in
        -p) prop="\${2:-}"; shift 2 || true ;;
        --value) value_only=1; shift ;;
        *) shift ;;
      esac
    done
    case "\$prop" in
      User) echo "root"; exit 0 ;;
      Group) echo ""; exit 0 ;;
      Environment) cat "${WORK}/state_env"; exit 0 ;;
      *) echo "unexpected systemctl show prop=\$prop" >&2; exit 99 ;;
    esac
    ;;
  daemon-reload)
    if [[ -f "${WORK}/daemon_reload_fail" ]]; then exit 1; fi
    exit 0
    ;;
  restart)
    if [[ -f "${WORK}/restart_fail_once" ]]; then
      rm -f "${WORK}/restart_fail_once"
      exit 1
    fi
    if [[ -f "${WORK}/restart_always_fail" ]]; then
      exit 1
    fi
    if [[ -f "${DROPIN_FILE}" ]]; then
      printf '%s\n' "FOO=1 ${EXPECTED_ENV}" >"${WORK}/state_env"
      if [[ ! -f "${WORK}/suppress_brand_log" ]]; then
        printf 'active brand: id=vff name="VPN for Friends" public_base_url=https://x service_category=c allowed_hosts=h\n' >>"${WORK}/journal/log"
        printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      else
        printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      fi
      if [[ -f "${WORK}/force_inactive_with_dropin" ]]; then
        printf 'inactive\n' >"${WORK}/state_active"
      else
        printf 'active\n' >"${WORK}/state_active"
      fi
    else
      printf '%s\n' "FOO=1 BAR=2" >"${WORK}/state_env"
      printf 'active\n' >"${WORK}/state_active"
    fi
    exit 0
    ;;
  is-active)
    # systemctl is-active [--quiet] <unit>
    if [[ "\$(cat "${WORK}/state_active")" == "active" ]]; then
      exit 0
    fi
    exit 3
    ;;
  *)
    echo "unexpected systemctl: \$*" >&2
    exit 99
    ;;
esac
EOF
  chmod 0700 "${MOCK}/systemctl"
}

setup_workspace() {
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
  MOCK="${WORK}/mockbin"
  mkdir -p "${MOCK}" "${WORK}/dropin" "${WORK}/opt" "${WORK}/journal"
  DROPIN_DIR="${WORK}/dropin"
  DROPIN_FILE="${DROPIN_DIR}/10-vpnbot-config.conf"
  REMOTE_DIR="${WORK}/opt"
  REMOTE_CONFIG_VFF="${REMOTE_DIR}/config-vff.json"
  REMOTE_CONFIG_LEGACY="${REMOTE_DIR}/config.json"
  REMOTE_EXPLICIT_CONFIG="${REMOTE_CONFIG_VFF}"
  REMOTE_LEGACY_CONFIG="${REMOTE_CONFIG_LEGACY}"
  REMOTE_BINARY="${REMOTE_DIR}/bot"
  EXPECTED_BRAND_ID="vff"
  BRAND_LABEL="VFF"
  EXPECTED_ENV="VPNBOT_CONFIG=${REMOTE_EXPLICIT_CONFIG}"
  DROPIN_BODY="$(printf '%s\n' '[Service]' "Environment=${EXPECTED_ENV}")"
  SERVICE_NAME="bot.service"
  VFF_DROPIN_ACTIVE=0
  BRAND_DROPIN_ACTIVE=0

  printf '%s\n' '{}' >"${REMOTE_CONFIG_VFF}"
  printf '%s\n' '{}' >"${REMOTE_CONFIG_LEGACY}"
  printf '#!/bin/true\n' >"${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  : >"${WORK}/journal/log"
  printf 'active\n' >"${WORK}/state_active"
  printf '%s\n' "FOO=1 ${EXPECTED_ENV} BAR=2" >"${WORK}/state_env"

  write_systemctl_mock

  cat >"${MOCK}/journalctl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
cat "${WORK}/journal/log"
EOF
  chmod 0700 "${MOCK}/journalctl"

  cat >"${MOCK}/id" <<'EOF'
#!/usr/bin/env bash
if [[ "${1:-}" == "-gn" ]]; then echo "root"; exit 0; fi
exit 1
EOF
  chmod 0700 "${MOCK}/id"

  cat >"${MOCK}/sleep" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod 0700 "${MOCK}/sleep"

  cat >"${MOCK}/runuser" <<'EOF'
#!/usr/bin/env bash
while [[ $# -gt 0 ]]; do
  case "$1" in
    -u) shift 2 || true ;;
    --) shift; break ;;
    *) shift ;;
  esac
done
exec "$@"
EOF
  chmod 0700 "${MOCK}/runuser"
  cat >"${MOCK}/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod 0700 "${MOCK}/chown"

  cat >"${WORK}/configcheck_ok" <<'EOF'
#!/usr/bin/env bash
echo "config valid"
echo "brand.id=vff"
exit 0
EOF
  chmod 0700 "${WORK}/configcheck_ok"

  cat >"${WORK}/configcheck_bad" <<'EOF'
#!/usr/bin/env bash
echo "configcheck: invalid" >&2
exit 1
EOF
  chmod 0700 "${WORK}/configcheck_bad"

  export PATH="${MOCK}:${PATH}"
}

teardown() {
  rm -rf "${WORK:-}"
}

# 1. successful activation
test_success() {
  setup_workspace
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail success "activate failed: ${out}"; teardown; return; fi
  if [[ ! -f "${DROPIN_FILE}" ]]; then fail success "drop-in missing"; teardown; return; fi
  if ! grep -Fq 'remote activation OK' <<<"${out}"; then fail success "missing OK: ${out}"; teardown; return; fi
  if grep -Fq 'FOO=1' <<<"${out}"; then fail success "full Environment leaked: ${out}"; teardown; return; fi
  if ! grep -Fxq "${EXPECTED_ENV}" <<<"${out}"; then fail success "expected env not printed: ${out}"; teardown; return; fi
  pass success
  teardown
}

# 2. configcheck failure before drop-in
test_configcheck_fail() {
  setup_workspace
  local rc=0
  vff_activate "${WORK}/configcheck_bad" >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail configcheck_fail "should fail"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail configcheck_fail "drop-in should not exist"; teardown; return; fi
  pass configcheck_fail
  teardown
}

# 3. restart failure after drop-in → successful auto-rollback
test_restart_fail_rollback() {
  setup_workspace
  : >"${WORK}/restart_fail_once"
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail restart_fail "should fail"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail restart_fail "drop-in should be removed"; teardown; return; fi
  if ! grep -Fq 'rolling back to legacy' <<<"${out}"; then fail restart_fail "no rollback msg: ${out}"; teardown; return; fi
  if grep -Fq 'CRITICAL' <<<"${out}"; then fail restart_fail "unexpected CRITICAL on successful rollback: ${out}"; teardown; return; fi
  pass restart_fail_rollback
  teardown
}

# 4. unit not active after restart → rollback
test_not_active() {
  setup_workspace
  : >"${WORK}/force_inactive_with_dropin"
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail not_active "should fail"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail not_active "drop-in not rolled back"; teardown; return; fi
  pass not_active
  teardown
}

# 5. startup log missing id=vff
test_missing_brand_log() {
  setup_workspace
  : >"${WORK}/suppress_brand_log"
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail missing_brand_log "should fail"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail missing_brand_log "drop-in not rolled back"; teardown; return; fi
  pass missing_brand_log
  teardown
}

# 6. smoke transport error handled
test_smoke_transport() {
  local smoke_mock
  smoke_mock="$(mktemp -d)"
  cat >"${smoke_mock}/curl" <<'EOF'
#!/usr/bin/env bash
exit 7
EOF
  chmod 0700 "${smoke_mock}/curl"
  local out rc=0
  out="$(PATH="${smoke_mock}:${PATH}" bash "${ROOT}/scripts/smoke-vff.sh" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail smoke_transport "should fail"; rm -rf "${smoke_mock}"; return; fi
  if ! grep -Fq 'transport error' <<<"${out}"; then fail smoke_transport "msg: ${out}"; rm -rf "${smoke_mock}"; return; fi
  pass smoke_transport
  rm -rf "${smoke_mock}"
}

# 7. rollback returns legacy
test_rollback_ok() {
  setup_workspace
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  printf '%s\n' "X ${EXPECTED_ENV}" >"${WORK}/state_env"
  local out rc=0
  out="$(vff_rollback_to_legacy 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail rollback_ok "${out}"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail rollback_ok "drop-in remains"; teardown; return; fi
  if ! grep -Fq 'legacy active' <<<"${out}"; then fail rollback_ok "${out}"; teardown; return; fi
  pass rollback_ok
  teardown
}

# 8. rollback failure → CRITICAL
test_rollback_critical() {
  setup_workspace
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  : >"${WORK}/restart_always_fail"
  # Ensure rollback restart path cannot clear the fail flag.
  local out rc=0
  out="$(vff_emergency_rollback "forced" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail rollback_critical "should fail"; teardown; return; fi
  if ! grep -Fq 'CRITICAL: VFF activation failed and automatic rollback failed' <<<"${out}"; then
    fail rollback_critical "missing CRITICAL: ${out}"; teardown; return
  fi
  pass rollback_critical
  teardown
}

# 9. foreign drop-in not overwritten
test_foreign_dropin() {
  setup_workspace
  printf '%s\n' '[Service]' 'Environment=VPNBOT_CONFIG=/other.json' >"${DROPIN_FILE}"
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail foreign_dropin "should refuse"; teardown; return; fi
  if ! grep -Fq 'refusing to overwrite' <<<"${out}"; then fail foreign_dropin "${out}"; teardown; return; fi
  if ! grep -Fq '/other.json' "${DROPIN_FILE}"; then fail foreign_dropin "foreign file changed"; teardown; return; fi
  pass foreign_dropin
  teardown
}

# 10. idempotent same drop-in
test_idempotent() {
  setup_workspace
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail idempotent "${out}"; teardown; return; fi
  if ! grep -Fq 'already correct (idempotent)' <<<"${out}"; then fail idempotent "${out}"; teardown; return; fi
  if ! grep -Fq 'remote activation OK' <<<"${out}"; then fail idempotent "no OK: ${out}"; teardown; return; fi
  pass idempotent
  teardown
}

# 11. temp cleanup pattern
test_temp_cleanup_pattern() {
  local d
  d="$(mktemp -d)"
  chmod 0700 "${d}"
  printf 'x' >"${d}/configcheck"
  rm -rf "${d}"
  if [[ -d "${d}" ]]; then fail temp_cleanup "dir remains"; return; fi
  pass temp_cleanup_pattern
}

# 12. full Environment not printed
test_no_full_environment() {
  setup_workspace
  local out rc=0
  out="$(vff_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail no_full_env "${out}"; teardown; return; fi
  if grep -Fq 'FOO=1' <<<"${out}"; then fail no_full_env "FOO leaked: ${out}"; teardown; return; fi
  if grep -Fq 'BAR=2' <<<"${out}"; then fail no_full_env "BAR leaked: ${out}"; teardown; return; fi
  if ! grep -Fxq "${EXPECTED_ENV}" <<<"${out}"; then fail no_full_env "expected line missing"; teardown; return; fi
  pass no_full_environment
  teardown
}

test_success
test_configcheck_fail
test_restart_fail_rollback
test_not_active
test_missing_brand_log
test_smoke_transport
test_rollback_ok
test_rollback_critical
test_foreign_dropin
test_idempotent
test_temp_cleanup_pattern
test_no_full_environment

if [[ "${FAILS}" -ne 0 ]]; then
  echo "vff_ops_test: ${FAILS} failed" >&2
  exit 1
fi
echo "vff_ops_test: all passed"
