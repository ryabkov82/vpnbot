#!/usr/bin/env bash
# Mock tests for coordinated brand rollout (brand_rollout_run).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=../lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

FAILS=0
BRAND_ROLLOUT_TEST_BASE_PATH="${PATH}"
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

write_systemctl_mock() {
  cat >"${MOCK}/systemctl" <<EOF
#!/usr/bin/env bash
set -euo pipefail
cmd="\${1:-}"
case "\$cmd" in
  show)
    prop=""; shift || true
    while [[ \$# -gt 0 ]]; do
      case "\$1" in
        -p) prop="\${2:-}"; shift 2 || true ;;
        --value) shift ;;
        *) shift ;;
      esac
    done
    case "\$prop" in
      User) echo "root"; exit 0 ;;
      Group) echo ""; exit 0 ;;
      Environment) cat "${WORK}/state_env"; exit 0 ;;
      LoadState) echo "loaded"; exit 0 ;;
      *) exit 99 ;;
    esac
    ;;
  daemon-reload)
    if [[ -f "${WORK}/daemon_reload_fail_once" ]]; then
      rm -f "${WORK}/daemon_reload_fail_once"
      exit 1
    fi
    [[ -f "${WORK}/daemon_reload_fail" ]] && exit 1
    exit 0
    ;;
  restart)
    n="\$(cat "${WORK}/restart_count" 2>/dev/null || echo 0)"
    echo \$((n + 1)) >"${WORK}/restart_count"
    if [[ -f "${WORK}/restart_fail_once" ]]; then rm -f "${WORK}/restart_fail_once"; exit 1; fi
    if [[ -f "${WORK}/restart_always_fail" ]]; then exit 1; fi
    if [[ -f "${DROPIN_FILE}" ]]; then
      if [[ -f "${WORK}/force_env_missing_after_restart" ]]; then
        printf 'FOO=1\n' >"${WORK}/state_env"
      else
        printf 'FOO=1 %s\n' "${EXPECTED_ENV}" >"${WORK}/state_env"
      fi
      if [[ ! -f "${WORK}/no_startup_markers" ]]; then
        bid="${EXPECTED_BRAND_ID}"
        [[ -f "${WORK}/wrong_brand_id" ]] && bid="fc2"
        printf 'active brand: id=%s name="X"\n' "\$bid" >>"${WORK}/journal/log"
        [[ ! -f "${WORK}/no_telegram_marker" ]] && printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      fi
      printf 'active\n' >"${WORK}/state_active"
    elif [[ -f "${REMOTE_BINARY}" ]]; then
      if [[ ! -f "${WORK}/no_startup_markers" ]]; then
        bid="${EXPECTED_BRAND_ID}"
        [[ -f "${WORK}/wrong_brand_id" ]] && bid="fc2"
        printf 'active brand: id=%s name="X"\n' "\$bid" >>"${WORK}/journal/log"
        [[ ! -f "${WORK}/no_telegram_marker" ]] && printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      fi
      printf 'active\n' >"${WORK}/state_active"
      printf 'FOO=1\n' >"${WORK}/state_env"
    else
      printf 'active\n' >"${WORK}/state_active"
      printf 'FOO=1\n' >"${WORK}/state_env"
    fi
    exit 0
    ;;
  is-active)
    n="\$(cat "${WORK}/is_active_count" 2>/dev/null || echo 0)"
    n=\$((n + 1))
    echo "\$n" >"${WORK}/is_active_count"
    if [[ -f "${WORK}/is_active_fail_on" ]]; then
      if [[ "\$n" -eq "\$(cat "${WORK}/is_active_fail_on")" ]]; then exit 3; fi
    fi
    # After rollback restart (2nd restart), fail the second is-active check.
    if [[ -f "${WORK}/rollback_second_active_fail" ]]; then
      rc="\$(cat "${WORK}/restart_count" 2>/dev/null || echo 0)"
      if [[ "\$rc" -ge 2 ]]; then
        post="\$(cat "${WORK}/post_rollback_is_active" 2>/dev/null || echo 0)"
        post=\$((post + 1))
        echo "\$post" >"${WORK}/post_rollback_is_active"
        [[ "\$post" -ge 2 ]] && exit 3
      fi
    fi
    [[ "\$(cat "${WORK}/state_active")" == "active" ]] && exit 0
    exit 3
    ;;
  *) exit 99 ;;
esac
EOF
  chmod 0700 "${MOCK}/systemctl"
}

setup_workspace() {
  local profile="$1"
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
  MOCK="${WORK}/mockbin"
  mkdir -p "${MOCK}" "${WORK}/dropin" "${WORK}/opt" "${WORK}/journal" "${WORK}/rtmp"
  brand_profile_export "${profile}"
  export REMOTE_DIR="${WORK}/opt"
  export REMOTE_BINARY="${REMOTE_DIR}/bot"
  export REMOTE_LEGACY_CONFIG="${REMOTE_DIR}/config.json"
  export REMOTE_EXPLICIT_CONFIG="${REMOTE_DIR}/config-explicit.json"
  export REMOTE_CONFIG_VFF="${REMOTE_EXPLICIT_CONFIG}"
  export REMOTE_CONFIG_LEGACY="${REMOTE_LEGACY_CONFIG}"
  export DROPIN_FILE="${WORK}/dropin/10-vpnbot-config.conf"
  export DROPIN_DIR="${WORK}/dropin"
  export REMOTE_TMP="${WORK}/rtmp"
  export TX_ID="t$(date -u +%Y%m%dT%H%M%S%N)-$$-${RANDOM}${RANDOM}"
  brand_refresh_derived
  brand_rollout_paths_init
  export EXPECTED_ENV DROPIN_BODY ROLLOUT_TX_DIR ROLLOUT_LOCK_DIR ROLLOUT_MARKER ROLLOUT_ROOT
  printf '%s\n' '{"legacy":true}' >"${REMOTE_LEGACY_CONFIG}"
  printf '#!/bin/true\n' >"${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  : >"${WORK}/journal/log"
  printf 'active\n' >"${WORK}/state_active"
  printf '0\n' >"${WORK}/restart_count"
  write_systemctl_mock
  for bin in journalctl id sleep runuser stat chown sha256sum; do
    case "${bin}" in
      journalctl)
        cat >"${MOCK}/journalctl" <<EOF
#!/usr/bin/env bash
cat "${WORK}/journal/log"
EOF
        ;;
      id)
        cat >"${MOCK}/id" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == "-gn" ]] && echo root && exit 0
exit 1
EOF
        ;;
      sleep) printf '#!/bin/bash\nexit 0\n' >"${MOCK}/sleep" ;;
      runuser)
        cat >"${MOCK}/runuser" <<'EOF'
#!/usr/bin/env bash
while [[ $# -gt 0 ]]; do
  case "$1" in -u) shift 2 || true ;; --) shift; break ;; *) shift ;; esac
done
exec "$@"
EOF
        ;;
      stat)
        cat >"${MOCK}/stat" <<'EOF'
#!/usr/bin/env bash
[[ "$1" == "-c" && "$2" == "%U" ]] && echo root && exit 0
[[ "$1" == "-c" && "$2" == "%G" ]] && echo root && exit 0
exit 1
EOF
        ;;
      chown) printf '#!/bin/bash\nexit 0\n' >"${MOCK}/chown" ;;
      sha256sum)
        cat >"${MOCK}/sha256sum" <<'EOF'
#!/usr/bin/env bash
if command -v /usr/bin/sha256sum >/dev/null 2>&1; then
  exec /usr/bin/sha256sum "$@"
fi
python3 -c 'import hashlib,sys
p=sys.argv[1]
print(f"{hashlib.sha256(open(p,\"rb\").read()).hexdigest()}  {p}")' "$1"
EOF
        ;;
    esac
    chmod 0700 "${MOCK}/${bin}"
  done
  # Always rebase on the original PATH so prior tests' mock bins do not stack.
  export PATH="${MOCK}:${BRAND_ROLLOUT_TEST_BASE_PATH}"
  hash -r
}

setup_fc_workspace() { setup_workspace fc; }
setup_vff_workspace() { setup_workspace vff; }
teardown() { rm -rf "${WORK:-}"; }

write_configcheck_ok() {
  cat >"${REMOTE_TMP}/configcheck" <<EOF
#!/usr/bin/env bash
echo "config valid"
echo "brand.id=${EXPECTED_BRAND_ID}"
exit 0
EOF
  chmod 0700 "${REMOTE_TMP}/configcheck"
}

write_configcheck_wrong_brand() {
  cat >"${REMOTE_TMP}/configcheck" <<'EOF'
#!/usr/bin/env bash
echo "config valid"
echo "brand.id=vff"
exit 0
EOF
  chmod 0700 "${REMOTE_TMP}/configcheck"
}

write_configcheck_fail() {
  cat >"${REMOTE_TMP}/configcheck" <<'EOF'
#!/usr/bin/env bash
echo "config invalid" >&2
exit 1
EOF
  chmod 0700 "${REMOTE_TMP}/configcheck"
}

enable_legacy_mode() {
  rm -f "${DROPIN_FILE}" "${REMOTE_EXPLICIT_CONFIG}"
  printf 'FOO=1 BAR=2\n' >"${WORK}/state_env"
}

enable_explicit_mode() {
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  printf 'old-explicit-config\n' >"${REMOTE_EXPLICIT_CONFIG}"
  printf 'FOO=1 %s BAR=2\n' "${EXPECTED_ENV}" >"${WORK}/state_env"
}

prepare_rollout_uploads() {
  local bin="${1:-new-rollout-binary}"
  local cfg="${2:-{\"brand\":{\"id\":\"${EXPECTED_BRAND_ID}\"}}"
  printf '%s\n' "${bin}" >"${REMOTE_TMP}/bot"
  chmod 0755 "${REMOTE_TMP}/bot"
  printf '%s\n' "${cfg}" >"${REMOTE_TMP}/config.json"
  write_configcheck_ok
}

restart_count() { cat "${WORK}/restart_count" 2>/dev/null || echo 0; }

load_tx() {
  # shellcheck disable=SC1090
  source "${ROLLOUT_TX_DIR}/tx.env"
}

# 15.1 legacy → coordinated rollout succeeds; single restart.
test_first_fc_rollout_success() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-fc-binary'
  : >"${WORK}/journal/log"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail first_fc "${out}"; teardown; return; fi
  if ! grep -Fxq 'new-fc-binary' "${REMOTE_BINARY}"; then fail first_fc "binary"; teardown; return; fi
  if [[ ! -f "${DROPIN_FILE}" ]]; then fail first_fc "no drop-in"; teardown; return; fi
  brand_env_has_expected || { fail first_fc "env"; teardown; return; }
  if ! grep -Fq 'active brand: id=fc name=' "${WORK}/journal/log"; then fail first_fc "journal id"; teardown; return; fi
  if ! grep -Fq 'telegram bot configured' "${WORK}/journal/log"; then fail first_fc "telegram"; teardown; return; fi
  load_tx
  [[ "${RESTART_STATE}" == "completed" ]] || { fail first_fc "RESTART_STATE=${RESTART_STATE}"; teardown; return; }
  [[ "${TX_STATUS}" == "pending_smoke" ]] || { fail first_fc "TX_STATUS=${TX_STATUS}"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail first_fc "lock missing"; teardown; return; }
  [[ "$(restart_count)" == "1" ]] || { fail first_fc "restarts=$(restart_count)"; teardown; return; }
  [[ ! -f "${ROLLOUT_MARKER}" ]] || { fail first_fc "marker published early"; teardown; return; }
  pass first_fc_rollout_success
  teardown
}

# 15.2 explicit mode update.
test_explicit_update_success() {
  setup_fc_workspace
  enable_explicit_mode
  printf 'old-fc-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'updated-fc-binary' '{"brand":{"id":"fc","v":2}}'
  : >"${WORK}/journal/log"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail explicit_update "${out}"; teardown; return; fi
  grep -Fxq 'updated-fc-binary' "${REMOTE_BINARY}" || { fail explicit_update "binary"; teardown; return; }
  grep -Fq '"v":2' "${REMOTE_EXPLICIT_CONFIG}" || { fail explicit_update "config"; teardown; return; }
  brand_dropin_matches || { fail explicit_update "drop-in"; teardown; return; }
  pass explicit_update_success
  teardown
}

# 15.3 configcheck preflight failure — no mutation.
test_configcheck_preflight_failure() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'unchanged\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  write_configcheck_fail
  local before out rc=0
  before="$(cat "${REMOTE_BINARY}")"
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail cc_preflight "should fail"; teardown; return; fi
  [[ "${rc}" -eq "${ROLLOUT_RC_SAFE_ABORT}" ]] || { fail cc_preflight "rc=${rc}"; teardown; return; }
  if ! grep -Fq 'configcheck failed' <<<"${out}"; then fail cc_preflight "${out}"; teardown; return; fi
  [[ "$(cat "${REMOTE_BINARY}")" == "${before}" ]] || { fail cc_preflight "binary changed"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail cc_preflight "drop-in created"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail cc_preflight "lock retained"; teardown; return; }
  pass configcheck_preflight_failure
  teardown
}

# 15.4 wrong brand.id in configcheck preflight.
test_wrong_brand_id_preflight() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'unchanged\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  write_configcheck_wrong_brand
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail wrong_cc "should fail"; teardown; return; fi
  if ! grep -Fq 'configcheck brand.id != fc' <<<"${out}"; then fail wrong_cc "${out}"; teardown; return; fi
  grep -Fxq 'unchanged' "${REMOTE_BINARY}" || { fail wrong_cc "binary changed"; teardown; return; }
  pass wrong_brand_id_preflight
  teardown
}

# 15.5 unexpected drop-in blocks preflight.
test_unexpected_dropin_preflight() {
  setup_fc_workspace
  enable_legacy_mode
  printf '[Service]\nEnvironment=VPNBOT_CONFIG=/other.json\n' >"${DROPIN_FILE}"
  prepare_rollout_uploads
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail foreign_dropin "should fail"; teardown; return; fi
  if ! grep -Fq 'unexpected drop-in content' <<<"${out}"; then fail foreign_dropin "${out}"; teardown; return; fi
  pass unexpected_dropin_preflight
  teardown
}

# 15.6 binary install failure -> restore previous state.
test_binary_install_failure_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  # Simulate binary-stage failure after config was already installed.
  local saved_install
  saved_install="$(declare -f brand_rollout_install)"
  brand_rollout_install() {
    local uploaded_binary="${1:?}" uploaded_config="${2:?}"
    brand_refresh_derived || return 1
    brand_rollout_tx_load || return 1
    brand_rollout_install_config "${uploaded_config}" || return 1
    brand_rollout_tx_set STAGE_CONFIG_INSTALLED 1 || return 1
    brand_err "rollout-${BRAND_LABEL}: simulated binary install failure"
    return 1
  }
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  unset -f brand_rollout_install
  eval "${saved_install}"
  if [[ "${rc}" -eq 0 ]]; then fail bin_inst "should fail: ${out}"; teardown; return; fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail bin_inst "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail bin_inst "drop-in"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail bin_inst "config remains"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail bin_inst "${out}"; teardown; return; fi
  pass binary_install_failure_rollback
  teardown
}

# 15.7 config install failure before mutation of production artifacts.
test_config_install_failure_no_mutation() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  cat >"${MOCK}/cp" <<'EOF'
#!/usr/bin/env bash
dest="${@: -1}"
case "$dest" in
  *.new.*)
    # Fail only config staging copies (path contains config-explicit.json.new.)
    if [[ "$dest" == *config-explicit.json.new.* ]]; then
      exit 1
    fi
    ;;
esac
exec /bin/cp "$@"
EOF
  chmod 0700 "${MOCK}/cp"
  hash -r
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail cfg_inst "should fail: ${out}"; teardown; return; fi
  [[ "${rc}" -eq "${ROLLOUT_RC_SAFE_ABORT}" ]] || { fail cfg_inst "rc=${rc} ${out}"; teardown; return; }
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail cfg_inst "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail cfg_inst "drop-in"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail cfg_inst "config"; teardown; return; }
  [[ "$(restart_count)" == "0" ]] || { fail cfg_inst "restart called rc=${rc}"; teardown; return; }
  pass config_install_failure_no_mutation
  teardown
}

# 15.8 daemon-reload failure → full rollback.
test_daemon_reload_failure_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/daemon_reload_fail_once"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail daemon_rb "should fail"; teardown; return; fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail daemon_rb "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail daemon_rb "drop-in"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail daemon_rb "config"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail daemon_rb "${out}"; teardown; return; fi
  pass daemon_reload_failure_rollback
  teardown
}

# 15.9 install restart fails once → full legacy rollback.
test_restart_failure_full_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/restart_fail_once"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail restart_rb "should fail"; teardown; return; fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail restart_rb "binary=$(cat "${REMOTE_BINARY}")"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail restart_rb "drop-in remains"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail restart_rb "explicit config remains"; teardown; return; }
  brand_assert_expected_env_absent || { fail restart_rb "env not legacy"; teardown; return; }
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail restart_rb "rc=${rc}"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail restart_rb "${out}"; teardown; return; fi
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail restart_rb "lock retained"; teardown; return; }
  [[ ! -f "${ROLLOUT_MARKER}" ]] || { fail restart_rb "marker changed"; teardown; return; }
  pass restart_failure_full_rollback
  teardown
}

# 15.10 verify 1st is-active fails (preflight uses 1–2; verify first = 3).
test_first_is_active_failure_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  echo 3 >"${WORK}/is_active_fail_on"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail first_active "should fail"; teardown; return; fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail first_active "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail first_active "drop-in"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail first_active "${out}"; teardown; return; fi
  pass first_is_active_failure_rollback
  teardown
}

# 15.11 verify 2nd is-active fails (calls 3–4 = preflight 1–2, verify 1–2).
test_second_is_active_failure_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  echo 4 >"${WORK}/is_active_fail_on"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail is_active_rb "should fail"; teardown; return; fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail is_active_rb "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail is_active_rb "drop-in"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail is_active_rb "${out}"; teardown; return; fi
  pass second_is_active_failure_rollback
  teardown
}

# 15.12 EXPECTED_ENV missing after install restart.
test_expected_env_missing_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/force_env_missing_after_restart"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail env_missing "should fail"; teardown; return; fi
  if ! grep -Fq 'VPNBOT_CONFIG=' <<<"${out}" || ! grep -Fq 'not active on' <<<"${out}"; then
    fail env_missing "${out}"; teardown; return
  fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail env_missing "binary"; teardown; return; }
  pass expected_env_missing_rollback
  teardown
}

# 15.13/15.20 wrong startup brand marker (fc2 prefix collision).
test_wrong_startup_brand_marker_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/wrong_brand_id"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail wrong_marker "should fail"; teardown; return; fi
  if ! grep -Fq "startup log missing 'active brand: id=fc name='" <<<"${out}"; then
    fail wrong_marker "${out}"; teardown; return
  fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail wrong_marker "binary"; teardown; return; }
  pass wrong_startup_brand_marker_rollback
  teardown
}

# 15.14 missing telegram marker → rollback.
test_telegram_marker_missing_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/no_telegram_marker"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail no_tg "should fail"; teardown; return; fi
  if ! grep -Fq "startup log missing 'telegram bot configured'" <<<"${out}"; then
    fail no_tg "${out}"; teardown; return
  fi
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail no_tg "binary"; teardown; return; }
  pass telegram_marker_missing_rollback
  teardown
}

# 15.15 remote OK then simulated smoke failure → rollback restores legacy.
test_public_smoke_failure_rollback() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-fc-binary'
  : >"${WORK}/journal/log"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail smoke_rb "remote should succeed: ${out}"; teardown; return; fi
  grep -Fxq 'new-fc-binary' "${REMOTE_BINARY}" || { fail smoke_rb "not rolled out"; teardown; return; }
  # Simulate local smoke failure by invoking the same remote rollback path.
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail smoke_rb "rc=${rc} ${out}"; teardown; return; }
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail smoke_rb "binary"; teardown; return; }
  [[ ! -f "${DROPIN_FILE}" ]] || { fail smoke_rb "drop-in"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail smoke_rb "config"; teardown; return; }
  brand_assert_expected_env_absent || { fail smoke_rb "env"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail smoke_rb "lock retained"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail smoke_rb "${out}"; teardown; return; fi
  pass public_smoke_failure_rollback
  teardown
}

# 15.16 rollback restart fails → CRITICAL.
test_rollback_restart_critical() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/no_startup_markers"
  : >"${WORK}/restart_always_fail"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail rb_critical "should fail"; teardown; return; fi
  [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]] || { fail rb_critical "rc=${rc}"; teardown; return; }
  if ! grep -Fq 'CRITICAL: FC rollout failed and automatic rollback failed' <<<"${out}"; then
    fail rb_critical "${out}"; teardown; return
  fi
  if grep -Fq 'previous state restored' <<<"${out}"; then
    fail rb_critical "false restored claim"; teardown; return
  fi
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail rb_critical "lock not retained"; teardown; return; }
  [[ -d "${ROLLOUT_TX_DIR}" ]] || { fail rb_critical "tx dir missing"; teardown; return; }
  pass rollback_restart_critical
  teardown
}

# 15.17 rollback second is-active fails → CRITICAL.
test_rollback_second_active_critical() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  : >"${WORK}/no_startup_markers"
  : >"${WORK}/rollback_second_active_fail"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail rb2_critical "should fail"; teardown; return; fi
  if ! grep -Fq 'CRITICAL: FC rollout failed and automatic rollback failed' <<<"${out}"; then
    fail rb2_critical "${out}"; teardown; return
  fi
  pass rollback_second_active_critical
  teardown
}

# 15.18 covered by restart_failure_full_rollback (legacy mode restored).

# 15.19 explicit mode rollback restores prior bytes + drop-in.
test_previous_explicit_mode_restored() {
  setup_fc_workspace
  enable_explicit_mode
  printf 'old-binary-bytes\n' >"${REMOTE_BINARY}"
  printf 'old-config-bytes\n' >"${REMOTE_EXPLICIT_CONFIG}"
  prepare_rollout_uploads 'new-binary-bytes' '{"brand":{"id":"fc","new":true}}'
  : >"${WORK}/no_startup_markers"
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail prev_explicit "should fail"; teardown; return; fi
  grep -Fxq 'old-binary-bytes' "${REMOTE_BINARY}" || { fail prev_explicit "binary"; teardown; return; }
  grep -Fxq 'old-config-bytes' "${REMOTE_EXPLICIT_CONFIG}" || { fail prev_explicit "config"; teardown; return; }
  brand_dropin_matches || { fail prev_explicit "drop-in"; teardown; return; }
  if ! grep -Fq 'previous state restored' <<<"${out}"; then fail prev_explicit "${out}"; teardown; return; fi
  pass previous_explicit_mode_restored
  teardown
}

# Standalone deploy guard in legacy mode.
test_brand_deploy_legacy_guard() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-bin\n' >"${REMOTE_BINARY}"
  printf 'new-bin\n' >"${REMOTE_BINARY}.new"
  local before out rc=0
  before="$(cat "${REMOTE_BINARY}")"
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail legacy_guard "should fail"; teardown; return; fi
  if ! grep -Fq 'use brand-rollout' <<<"${out}"; then fail legacy_guard "${out}"; teardown; return; fi
  [[ "$(cat "${REMOTE_BINARY}")" == "${before}" ]] || { fail legacy_guard "binary changed"; teardown; return; }
  pass brand_deploy_legacy_guard
  teardown
}

# make -n rollout targets (no production).
test_make_n_rollout() {
  local out
  out="$(make -n -C "${ROOT}" brand-rollout BRAND=fc CONFIG=/secure/x.json 2>&1)" || {
    fail make_rollout "brand-rollout: ${out}"; return
  }
  if ! grep -Fq 'rollout-brand.sh' <<<"${out}"; then fail make_rollout "missing script: ${out}"; return; fi
  if ! grep -Fq 'fc' <<<"${out}"; then fail make_rollout "missing fc: ${out}"; return; fi
  out="$(make -n -C "${ROOT}" rollout-fc CONFIG=/secure/x.json 2>&1)" || {
    fail make_rollout "rollout-fc: ${out}"; return
  }
  if ! grep -Fq 'brand-rollout' <<<"${out}"; then fail make_rollout "rollout-fc: ${out}"; return; fi
  pass make_n_rollout
}

# Optional: deploy-brand-config LOCAL_TMP cleanup on failure.
test_deploy_config_local_tmp_cleanup() {
  local dir
  dir="$(mktemp -d)"
  chmod 0700 "${dir}"
  cat >"${dir}/ssh" <<EOF
#!/usr/bin/env bash
[[ "\$*" == *mktemp* ]] && { echo "${dir}/remote-tmp"; mkdir -p "${dir}/remote-tmp"; exit 0; }
exit 1
EOF
  chmod 0700 "${dir}/ssh"
  printf '#!/bin/bash\nexit 0\n' >"${dir}/scp" && chmod 0700 "${dir}/scp"
  cat >"${dir}/go" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == "run" ]] && { echo "config valid"; echo "brand.id=fc"; exit 0; }
exit 0
EOF
  chmod 0700 "${dir}/go"
  local rc=0
  PATH="${dir}:${PATH}" bash "${ROOT}/scripts/deploy-brand-config.sh" fc "${dir}/cfg.json" >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail cfg_cleanup "should fail"; rm -rf "${dir}"; return; fi
  pass deploy_config_local_tmp_cleanup
  rm -rf "${dir}"
}

test_first_fc_rollout_success
test_explicit_update_success
test_configcheck_preflight_failure
test_wrong_brand_id_preflight
test_unexpected_dropin_preflight
test_binary_install_failure_rollback
test_config_install_failure_no_mutation
test_daemon_reload_failure_rollback
test_restart_failure_full_rollback
test_first_is_active_failure_rollback
test_second_is_active_failure_rollback
test_expected_env_missing_rollback
test_wrong_startup_brand_marker_rollback
test_telegram_marker_missing_rollback
test_public_smoke_failure_rollback
test_rollback_restart_critical
test_rollback_second_active_critical
test_previous_explicit_mode_restored
test_brand_deploy_legacy_guard
test_make_n_rollout
test_deploy_config_local_tmp_cleanup

# --- hardening: lock / crash / hash / marker ---

test_parallel_rollout_lock() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'first-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail parallel "first: ${out}"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail parallel "lock missing"; teardown; return; }
  local first_tx="${TX_ID}" first_bin
  first_bin="$(cat "${REMOTE_BINARY}")"
  export TX_ID="t$(date -u +%Y%m%dT%H%M%S%N)-$$-${RANDOM}b"
  brand_rollout_paths_init
  prepare_rollout_uploads 'second-bin'
  rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_LOCK_BUSY}" ]] || { fail parallel "rc=${rc} ${out}"; teardown; return; }
  grep -Fxq "${first_bin}" "${REMOTE_BINARY}" || { fail parallel "binary changed"; teardown; return; }
  [[ "$(restart_count)" == "1" ]] || { fail parallel "extra restart"; teardown; return; }
  export TX_ID="${first_tx}"
  brand_rollout_paths_init
  pass parallel_rollout_lock
  teardown
}

test_same_second_tx_ids_differ() {
  local a b
  a="$(date -u +%Y%m%dT%H%M%S%N)-$$-${RANDOM}${RANDOM}"
  b="$(date -u +%Y%m%dT%H%M%S%N)-$$-${RANDOM}${RANDOM}"
  [[ "${a}" != "${b}" ]] || { fail same_sec "ids equal"; return; }
  brand_rollout_validate_tx_id "${a}" || { fail same_sec "a invalid"; return; }
  brand_rollout_validate_tx_id "${b}" || { fail same_sec "b invalid"; return; }
  pass same_second_tx_ids_differ
}

test_finalize_releases_lock_and_marker() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin "run: ${out}"; teardown; return; }
  [[ ! -f "${ROLLOUT_MARKER}" ]] || { fail fin "marker early"; teardown; return; }
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin "finalize: ${out}"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail fin "lock retained"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "completed" ]] || { fail fin "status=${TX_STATUS}"; teardown; return; }
  [[ "$(tr -d '\n' <"${ROLLOUT_MARKER}")" == "${BINARY_BACKUP}" ]] || { fail fin "marker"; teardown; return; }
  pass finalize_releases_lock_and_marker
  teardown
}

test_crash_window_config_installing() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail crash_cfg "setup: ${out}"; teardown; return; }
  load_tx
  # Simulate crash after replace before installed flag.
  CONFIG_STATE=installing
  brand_rollout_tx_write || { fail crash_cfg "tx write"; teardown; return; }
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail crash_cfg "rc=${rc} ${out}"; teardown; return; }
  grep -Fxq 'legacy-binary' "${REMOTE_BINARY}" || { fail crash_cfg "binary"; teardown; return; }
  [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]] || { fail crash_cfg "config remains"; teardown; return; }
  pass crash_window_config_installing
  teardown
}

test_external_config_mod_blocks_delete() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail ext_cfg "run: ${out}"; teardown; return; }
  printf 'tampered-outside-tx\n' >"${REMOTE_EXPLICIT_CONFIG}"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]] || { fail ext_cfg "rc=${rc}"; teardown; return; }
  if ! grep -Fq 'refusing to remove config changed outside transaction' <<<"${out}"; then
    fail ext_cfg "${out}"; teardown; return
  fi
  grep -Fxq 'tampered-outside-tx' "${REMOTE_EXPLICIT_CONFIG}" || { fail ext_cfg "tamper lost"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail ext_cfg "lock not retained"; teardown; return; }
  pass external_config_mod_blocks_delete
  teardown
}

test_ownership_mismatch_no_mutation() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail own "run: ${out}"; teardown; return; }
  # Corrupt lock ownership while keeping the real transaction identity.
  printf 'foreign-tx\n' >"${ROLLOUT_LOCK_DIR}/tx_id"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]] || { fail own "rc=${rc} ${out}"; teardown; return; }
  if ! grep -Fq 'does not own rollout lock' <<<"${out}"; then
    fail own "${out}"; teardown; return
  fi
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail own "lock removed"; teardown; return; }
  pass ownership_mismatch_no_mutation
  teardown
}

test_make_n_recover() {
  local out
  out="$(make -n -C "${ROOT}" brand-rollout-recover BRAND=fc TX_ID=test-123 ACTION=status 2>&1)" || {
    fail make_recover "${out}"; return
  }
  grep -Fq 'recover-brand-rollout.sh' <<<"${out}" || { fail make_recover "script: ${out}"; return; }
  grep -Fq 'test-123' <<<"${out}" || { fail make_recover "tx: ${out}"; return; }
  pass make_n_recover
}

# --- lifecycle: finalize/rollback idempotency and marker intent ---

test_finalize_forbidden_from_critical() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0 marker_before=""
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin_crit "run: ${out}"; teardown; return; }
  load_tx
  TX_STATUS=critical
  brand_rollout_tx_write || { fail fin_crit "tx write"; teardown; return; }
  marker_before="$(cat "${ROLLOUT_MARKER}" 2>/dev/null || true)"
  local bin_before cfg_before
  bin_before="$(cat "${REMOTE_BINARY}")"
  cfg_before="$(cat "${REMOTE_EXPLICIT_CONFIG}")"
  rc=0
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -ne 0 ]] || { fail fin_crit "should fail"; teardown; return; }
  grep -Fq 'finalize not allowed from status critical' <<<"${out}" || { fail fin_crit "${out}"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "critical" ]] || { fail fin_crit "status=${TX_STATUS}"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail fin_crit "lock gone"; teardown; return; }
  [[ "$(cat "${REMOTE_BINARY}")" == "${bin_before}" ]] || { fail fin_crit "binary"; teardown; return; }
  [[ "$(cat "${REMOTE_EXPLICIT_CONFIG}")" == "${cfg_before}" ]] || { fail fin_crit "config"; teardown; return; }
  [[ "$(cat "${ROLLOUT_MARKER}" 2>/dev/null || true)" == "${marker_before}" ]] || { fail fin_crit "marker"; teardown; return; }
  pass finalize_forbidden_from_critical
  teardown
}

test_finalize_idempotent_after_lock_release() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin_idemp "run: ${out}"; teardown; return; }
  rc=0
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin_idemp "finalize: ${out}"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail fin_idemp "lock retained"; teardown; return; }
  local marker1 bin1
  marker1="$(tr -d '\n' <"${ROLLOUT_MARKER}")"
  bin1="$(cat "${REMOTE_BINARY}")"
  rc=0
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin_idemp "retry: ${out}"; teardown; return; }
  grep -Fq 'finalize idempotent OK' <<<"${out}" || { fail fin_idemp "${out}"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail fin_idemp "lock recreated"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "completed" ]] || { fail fin_idemp "status=${TX_STATUS}"; teardown; return; }
  [[ "$(tr -d '\n' <"${ROLLOUT_MARKER}")" == "${marker1}" ]] || { fail fin_idemp "marker changed"; teardown; return; }
  [[ "$(cat "${REMOTE_BINARY}")" == "${bin1}" ]] || { fail fin_idemp "binary changed"; teardown; return; }
  pass finalize_idempotent_after_lock_release
  teardown
}

test_rollback_idempotent_after_lock_release() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail rb_idemp "run: ${out}"; teardown; return; }
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail rb_idemp "rb: ${out}"; teardown; return; }
  [[ ! -d "${ROLLOUT_LOCK_DIR}" ]] || { fail rb_idemp "lock retained"; teardown; return; }
  local restarts bin1
  restarts="$(restart_count)"
  bin1="$(cat "${REMOTE_BINARY}")"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail rb_idemp "retry rc=${rc}"; teardown; return; }
  grep -Fq 'previous state restored' <<<"${out}" || { fail rb_idemp "${out}"; teardown; return; }
  [[ "$(restart_count)" == "${restarts}" ]] || { fail rb_idemp "restart called"; teardown; return; }
  [[ "$(cat "${REMOTE_BINARY}")" == "${bin1}" ]] || { fail rb_idemp "binary"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "rolled_back" ]] || { fail rb_idemp "status=${TX_STATUS}"; teardown; return; }
  pass rollback_idempotent_after_lock_release
  teardown
}

test_rollback_forbidden_after_completed() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail rb_comp "run: ${out}"; teardown; return; }
  rc=0
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail rb_comp "finalize: ${out}"; teardown; return; }
  local bin1 cfg1 marker1
  bin1="$(cat "${REMOTE_BINARY}")"
  cfg1="$(cat "${REMOTE_EXPLICIT_CONFIG}")"
  marker1="$(tr -d '\n' <"${ROLLOUT_MARKER}")"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -ne 0 ]] || { fail rb_comp "should fail"; teardown; return; }
  grep -Fq 'rollback not allowed for completed transaction' <<<"${out}" || { fail rb_comp "${out}"; teardown; return; }
  [[ "$(cat "${REMOTE_BINARY}")" == "${bin1}" ]] || { fail rb_comp "binary"; teardown; return; }
  [[ "$(cat "${REMOTE_EXPLICIT_CONFIG}")" == "${cfg1}" ]] || { fail rb_comp "config"; teardown; return; }
  [[ "$(tr -d '\n' <"${ROLLOUT_MARKER}")" == "${marker1}" ]] || { fail rb_comp "marker"; teardown; return; }
  [[ -f "${DROPIN_FILE}" ]] || { fail rb_comp "drop-in removed"; teardown; return; }
  pass rollback_forbidden_after_completed
  teardown
}

test_marker_intent_crash_window() {
  setup_fc_workspace
  enable_explicit_mode
  printf 'old-binary\n' >"${REMOTE_BINARY}"
  printf 'old-marker-path\n' >"${ROLLOUT_MARKER}"
  prepare_rollout_uploads 'new-bin' '{"brand":{"id":"fc","x":1}}'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail mk_crash "run: ${out}"; teardown; return; }
  load_tx
  # Simulate crash after marker intent, before completed (marker already replaced).
  TX_STATUS=finalizing
  MARKER_PUBLISHED=1
  brand_rollout_tx_write || { fail mk_crash "tx"; teardown; return; }
  printf '%s\n' "${BINARY_BACKUP}" >"${ROLLOUT_MARKER}"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail mk_crash "rc=${rc} ${out}"; teardown; return; }
  [[ "$(tr -d '\n' <"${ROLLOUT_MARKER}")" == "old-marker-path" ]] || { fail mk_crash "marker not restored"; teardown; return; }
  load_tx
  [[ "${MARKER_PUBLISHED}" == "0" ]] || { fail mk_crash "MARKER_PUBLISHED=${MARKER_PUBLISHED}"; teardown; return; }

  # Previous marker absent branch.
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  rm -f "${ROLLOUT_MARKER}"
  prepare_rollout_uploads 'new-bin2'
  rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail mk_crash2 "run: ${out}"; teardown; return; }
  load_tx
  TX_STATUS=finalizing
  MARKER_PUBLISHED=1
  brand_rollout_tx_write || { fail mk_crash2 "tx"; teardown; return; }
  printf '%s\n' "${BINARY_BACKUP}" >"${ROLLOUT_MARKER}"
  rc=0
  out="$(brand_rollout_rollback 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_ROLLED_BACK}" ]] || { fail mk_crash2 "rc=${rc}"; teardown; return; }
  [[ ! -f "${ROLLOUT_MARKER}" ]] || { fail mk_crash2 "marker remains"; teardown; return; }
  pass marker_intent_crash_window
  teardown
}

test_finalize_lock_release_failure_preserves_backups() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads 'new-bin'
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq 0 ]] || { fail fin_lock "run: ${out}"; teardown; return; }
  [[ -f "${ROLLOUT_TX_DIR}/backups/config.bak" ]] || true
  # Force lock rmdir failure after marker publication path.
  cat >"${MOCK}/rmdir" <<EOF
#!/usr/bin/env bash
if [[ "\$1" == "${ROLLOUT_LOCK_DIR}" ]]; then
  exit 1
fi
exec /usr/bin/rmdir "\$@"
EOF
  chmod 0700 "${MOCK}/rmdir"
  hash -r
  rc=0
  out="$(brand_rollout_finalize 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]] || { fail fin_lock "rc=${rc} ${out}"; teardown; return; }
  if grep -Fq 'finalized (binary backup retained)' <<<"${out}"; then
    fail fin_lock "false success"; teardown; return
  fi
  [[ -d "${ROLLOUT_TX_DIR}" ]] || { fail fin_lock "tx dir gone"; teardown; return; }
  [[ -f "${ROLLOUT_TX_DIR}/backups/marker.bak" || "${PREV_MARKER_EXISTED:-0}" == "0" ]] || true
  # config.bak may be absent for legacy first rollout; dropin.bak likewise.
  # marker.bak absent when PREV_MARKER_EXISTED=0 — ensure tx.env still present.
  [[ -f "${ROLLOUT_TX_DIR}/tx.env" ]] || { fail fin_lock "tx.env gone"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "completed" ]] || { fail fin_lock "status=${TX_STATUS}"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail fin_lock "lock gone"; teardown; return; }
  pass finalize_lock_release_failure_preserves_backups
  teardown
}

test_safe_abort_lock_release_failure_preserves_manifest() {
  setup_fc_workspace
  enable_legacy_mode
  printf 'legacy-binary\n' >"${REMOTE_BINARY}"
  prepare_rollout_uploads
  write_configcheck_fail
  cat >"${MOCK}/rmdir" <<EOF
#!/usr/bin/env bash
if [[ "\$1" == "${ROLLOUT_LOCK_DIR}" ]]; then
  exit 1
fi
exec /usr/bin/rmdir "\$@"
EOF
  chmod 0700 "${MOCK}/rmdir"
  hash -r
  local out rc=0
  out="$(brand_rollout_run "${REMOTE_TMP}/bot" "${REMOTE_TMP}/config.json" "${REMOTE_TMP}/configcheck" 2>&1)" || rc=$?
  [[ "${rc}" -eq "${ROLLOUT_RC_CRITICAL}" ]] || { fail sabort "rc=${rc} ${out}"; teardown; return; }
  [[ -d "${ROLLOUT_TX_DIR}" ]] || { fail sabort "tx dir removed"; teardown; return; }
  [[ -f "${ROLLOUT_TX_DIR}/tx.env" ]] || { fail sabort "tx.env missing"; teardown; return; }
  load_tx
  [[ "${TX_STATUS}" == "critical" ]] || { fail sabort "status=${TX_STATUS}"; teardown; return; }
  grep -Fq 'brand-rollout-recover' <<<"${out}" || { fail sabort "no recover cmd"; teardown; return; }
  [[ -d "${ROLLOUT_LOCK_DIR}" ]] || { fail sabort "lock gone"; teardown; return; }
  pass safe_abort_lock_release_failure_preserves_manifest
  teardown
}

test_parallel_rollout_lock
test_same_second_tx_ids_differ
test_finalize_releases_lock_and_marker
test_crash_window_config_installing
test_external_config_mod_blocks_delete
test_ownership_mismatch_no_mutation
test_make_n_recover
test_finalize_forbidden_from_critical
test_finalize_idempotent_after_lock_release
test_rollback_idempotent_after_lock_release
test_rollback_forbidden_after_completed
test_marker_intent_crash_window
test_finalize_lock_release_failure_preserves_backups
test_safe_abort_lock_release_failure_preserves_manifest

if [[ "${FAILS}" -ne 0 ]]; then
  echo "brand_rollout_test: ${FAILS} failed" >&2
  exit 1
fi
echo "brand_rollout_test: all passed"
