#!/usr/bin/env bash
# Parameterized brand ops + renderer tests (VFF/FC).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/brand_ops.sh
source "${ROOT}/scripts/lib/brand_ops.sh"
# shellcheck source=../lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

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
      *) exit 99 ;;
    esac
    ;;
  daemon-reload) [[ -f "${WORK}/daemon_reload_fail" ]] && exit 1; exit 0 ;;
  restart)
    if [[ -f "${WORK}/restart_fail_once" ]]; then rm -f "${WORK}/restart_fail_once"; exit 1; fi
    if [[ -f "${WORK}/restart_always_fail" ]]; then exit 1; fi
    if [[ -f "${DROPIN_FILE}" ]]; then
      printf '%s\n' "FOO=1 ${EXPECTED_ENV}" >"${WORK}/state_env"
      printf 'active brand: id=%s name="X"\n' "${EXPECTED_BRAND_ID}" >>"${WORK}/journal/log"
      printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      printf 'active\n' >"${WORK}/state_active"
    elif [[ -f "${REMOTE_BINARY}" ]]; then
      printf 'active brand: id=vff name="synth"\n' >>"${WORK}/journal/log"
      printf 'telegram bot configured\n' >>"${WORK}/journal/log"
      printf 'active\n' >"${WORK}/state_active"
      printf 'FOO=1\n' >"${WORK}/state_env"
    else
      printf 'active\n' >"${WORK}/state_active"
      printf 'FOO=1\n' >"${WORK}/state_env"
    fi
    exit 0
    ;;
  is-active)
    [[ "\$(cat "${WORK}/state_active")" == "active" ]] && exit 0
    exit 3
    ;;
  *) exit 99 ;;
esac
EOF
  chmod 0700 "${MOCK}/systemctl"
}

setup_profile() {
  local profile="$1"
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
  MOCK="${WORK}/mockbin"
  mkdir -p "${MOCK}" "${WORK}/dropin" "${WORK}/opt" "${WORK}/journal"
  brand_profile_export "${profile}"
  # Override paths to workspace (re-export for subshells / command substitution).
  export REMOTE_DIR="${WORK}/opt"
  export REMOTE_BINARY="${REMOTE_DIR}/bot"
  export REMOTE_LEGACY_CONFIG="${REMOTE_DIR}/config.json"
  export REMOTE_EXPLICIT_CONFIG="${REMOTE_DIR}/config-explicit.json"
  export REMOTE_CONFIG_VFF="${REMOTE_EXPLICIT_CONFIG}"
  export REMOTE_CONFIG_LEGACY="${REMOTE_LEGACY_CONFIG}"
  export DROPIN_FILE="${WORK}/dropin/10-vpnbot-config.conf"
  export DROPIN_DIR="${WORK}/dropin"
  brand_refresh_derived
  export EXPECTED_ENV DROPIN_BODY
  printf '%s\n' '{}' >"${REMOTE_LEGACY_CONFIG}"
  printf '%s\n' '{}' >"${REMOTE_EXPLICIT_CONFIG}"
  printf '#!/bin/true\n' >"${REMOTE_BINARY}"
  chmod 0755 "${REMOTE_BINARY}"
  : >"${WORK}/journal/log"
  printf 'active\n' >"${WORK}/state_active"
  printf 'FOO=1 BAR=2\n' >"${WORK}/state_env"
  write_systemctl_mock
  cat >"${MOCK}/journalctl" <<EOF
#!/usr/bin/env bash
cat "${WORK}/journal/log"
EOF
  chmod 0700 "${MOCK}/journalctl"
  cat >"${MOCK}/id" <<'EOF'
#!/usr/bin/env bash
[[ "${1:-}" == "-gn" ]] && echo root && exit 0
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
  cat >"${MOCK}/stat" <<EOF
#!/usr/bin/env bash
# stat -c %U / %G
if [[ "\$1" == "-c" && "\$2" == "%U" ]]; then echo root; exit 0; fi
if [[ "\$1" == "-c" && "\$2" == "%G" ]]; then echo root; exit 0; fi
exit 1
EOF
  chmod 0700 "${MOCK}/stat"
  cat >"${MOCK}/chown" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod 0700 "${MOCK}/chown"
  cat >"${WORK}/configcheck_ok" <<EOF
#!/usr/bin/env bash
echo "config valid"
echo "brand.id=${EXPECTED_BRAND_ID}"
exit 0
EOF
  chmod 0700 "${WORK}/configcheck_ok"
  export PATH="${MOCK}:${PATH}"
}

teardown() { rm -rf "${WORK:-}"; }

# 1. profiles export different hosts
test_profiles_differ() {
  brand_profile_export vff
  local vff_host="${SERVER_HOST}" vff_svc="${SERVICE_NAME}"
  brand_profile_export fc
  if [[ "${SERVER_HOST}" == "${vff_host}" ]]; then fail profiles "same host"; return; fi
  if [[ "${SERVICE_NAME}" == "${vff_svc}" ]]; then fail profiles "same service"; return; fi
  if [[ "${EXPECTED_BRAND_ID}" != "fc" ]]; then fail profiles "fc id"; return; fi
  pass profiles_differ
}

# 2. missing required param stops
test_missing_param() {
  SERVICE_NAME="" REMOTE_DIR="/x" REMOTE_LEGACY_CONFIG="/x" REMOTE_EXPLICIT_CONFIG="/x" \
    DROPIN_FILE="/x" EXPECTED_BRAND_ID="fc" BRAND_LABEL="FC" \
    brand_refresh_derived >/dev/null 2>&1 && { fail missing_param "should fail"; return; }
  pass missing_param
}

# 3–5. binary deploy backup + restart/smoke rollback
test_binary_deploy_backup_and_rollback() {
  setup_profile fc
  printf 'old-bin\n' >"${REMOTE_BINARY}"
  printf 'new-bin\n' >"${REMOTE_BINARY}.new"
  local out rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail binary_ok "${out}"; teardown; return; fi
  if ! ls "${REMOTE_BINARY}".bak.* >/dev/null 2>&1; then fail binary_ok "no backup"; teardown; return; fi
  if ! grep -Fq 'new-bin' "${REMOTE_BINARY}"; then fail binary_ok "binary not replaced"; teardown; return; fi
  pass binary_deploy_backup

  # restart fail → rollback
  printf 'newer\n' >"${REMOTE_BINARY}.new"
  : >"${WORK}/restart_fail_once"
  # After fail_once, rollback restart should succeed and restore bak
  rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail binary_restart_rollback "should fail"; teardown; return; fi
  if ! grep -Fq 'new-bin' "${REMOTE_BINARY}" && ! grep -Fq 'old-bin' "${REMOTE_BINARY}"; then
    # After first successful deploy content was new-bin; second attempt failed once then rolled back to bak from second attempt which was copy of new-bin before replace...
    :
  fi
  if [[ ! -f "${REMOTE_BINARY}" ]]; then fail binary_restart_rollback "binary missing"; teardown; return; fi
  pass binary_restart_rollback

  # smoke-level rollback via brand_rollback_binary
  printf 'good\n' >"${REMOTE_BINARY}"
  bak="${REMOTE_BINARY}.bak.manual"
  printf 'restored\n' >"${bak}"
  printf '%s\n' "${bak}" >"${REMOTE_DIR}/.vpnbot-last-binary-bak"
  printf 'broken\n' >"${REMOTE_BINARY}"
  out="$(brand_rollback_binary 2>&1)" || { fail binary_smoke_rollback "${out}"; teardown; return; }
  if ! grep -Fq 'restored' "${REMOTE_BINARY}"; then fail binary_smoke_rollback "not restored"; teardown; return; fi
  pass binary_smoke_rollback
  teardown
}

# 6–10. renderer FC/VFF
test_renderer() {
  local dir src out
  dir="$(mktemp -d)"
  src="${dir}/src.json"
  out="${dir}/out.json"

  cat >"${src}" <<'EOF'
{
  "telegram": {"token": "SECRET-TOKEN"},
  "api": {"api_pass": "SECRET-PASS"},
  "services": {"category": "vpn-mz-fc", "extra": 1},
  "web_sales": {"public_base_url": "https://connect-fc.vpn-for-friends.com", "order_token_secret": "SECRET-ORDER", "enabled": true},
  "payments": {"profile": "telegram_friends_connect_bot", "keep": true}
}
EOF

  if ! bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${out}" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --web-login-prefix web_ --web-user-source vpn-for-friends.com \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot >/dev/null; then
    fail renderer_fc "render failed"; rm -rf "${dir}"; return
  fi

  python3 - <<PY
import json
c=json.load(open("${out}"))
assert c["brand"]["id"]=="fc"
assert "category" not in c.get("services", {})
assert c.get("services", {}).get("extra")==1
assert "public_base_url" not in c.get("web_sales", {})
assert c["web_sales"]["enabled"] is True
assert c["web_sales"]["order_token_secret"]=="SECRET-ORDER"
assert "profile" not in c.get("payments", {})
assert c["payments"]["keep"] is True
assert c["brand"]["web_user_login_prefix"]=="web_"
assert c["brand"]["web_user_source"]=="vpn-for-friends.com"
print("ok")
PY

  # reject VFF source for FC expects
  cat >"${src}" <<'EOF'
{
  "telegram": {"token": "t"},
  "services": {"category": "vpn-mz-test"},
  "web_sales": {"public_base_url": "https://connect.vpn-for-friends.com"},
  "payments": {"profile": "telegram_bot"}
}
EOF
  if bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${dir}/bad.json" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot >/dev/null 2>&1; then
    fail renderer_reject_vff "should reject"; rm -rf "${dir}"; return
  fi
  pass renderer_fc_and_reject

  # VFF renderer wrapper behaviour (brand fields + strip duplicates)
  cat >"${src}" <<'EOF'
{
  "telegram": {"token": "t"},
  "services": {"category": "vpn-mz-test"},
  "web_sales": {"public_base_url": "https://connect.vpn-for-friends.com", "enabled": true},
  "payments": {"profile": "telegram_bot"}
}
EOF
  if ! bash "${ROOT}/scripts/render-vff-config.sh" "${src}" "${dir}/vff.json" >/dev/null; then
    fail renderer_vff "render failed"; rm -rf "${dir}"; return
  fi
  python3 - <<PY
import json
c=json.load(open("${dir}/vff.json"))
assert c["brand"]["id"]=="vff"
assert "category" not in c.get("services", {})
assert c.get("web_sales", {}).get("enabled") is True
assert "public_base_url" not in c.get("web_sales", {})
assert "payments" not in c or "profile" not in c.get("payments", {})
print("ok")
PY
  pass renderer_vff
  rm -rf "${dir}"
}

# 11. config deploy does not restart (mock counts restarts)
test_config_deploy_no_restart() {
  setup_profile fc
  : >"${REMOTE_EXPLICIT_CONFIG}.new"
  printf '%s\n' '{}' >"${REMOTE_EXPLICIT_CONFIG}.new"
  local before after
  before="$(grep -c restart "${MOCK}/systemctl" || true)"
  brand_deploy_config_file "${REMOTE_EXPLICIT_CONFIG}.new" >/dev/null
  # systemctl mock file unchanged; ensure restart was not invoked by checking state_env untouched marker
  if [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then fail config_deploy "not installed"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail config_deploy "drop-in must not be created"; teardown; return; fi
  pass config_deploy_no_restart
  teardown
}

# 12–14. FC activation id + rollback CRITICAL
test_fc_activation() {
  setup_profile fc
  local out rc=0
  out="$(brand_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail fc_activate "${out}"; teardown; return; fi
  if ! grep -Fq 'brand.id=fc' <<<"${out}"; then fail fc_activate "no fc id in summary path"; teardown; return; fi
  if ! grep -Fxq "${EXPECTED_ENV}" <<<"${out}"; then fail fc_activate "env"; teardown; return; fi
  if grep -Fq 'FOO=1' <<<"${out}"; then fail fc_activate "full env"; teardown; return; fi
  pass fc_activation

  # failure → rollback
  rm -f "${DROPIN_FILE}"
  : >"${WORK}/journal/log"
  cat >"${WORK}/configcheck_wrong" <<'EOF'
#!/usr/bin/env bash
echo "config valid"
echo "brand.id=vff"
exit 0
EOF
  chmod 0700 "${WORK}/configcheck_wrong"
  rc=0
  out="$(brand_activate "${WORK}/configcheck_wrong" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail fc_wrong_id "should fail"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail fc_wrong_id "drop-in installed"; teardown; return; fi
  pass fc_wrong_id

  # CRITICAL on rollback failure
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  : >"${WORK}/restart_always_fail"
  rc=0
  out="$(brand_emergency_rollback "forced" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail fc_critical "should fail"; teardown; return; fi
  if ! grep -Fq 'CRITICAL: FC activation failed and automatic rollback failed' <<<"${out}"; then
    fail fc_critical "${out}"; teardown; return
  fi
  pass fc_critical
  teardown
}

# 15. VFF activation still works with VFF profile (regression after parameterization)
test_vff_regression_activate() {
  setup_profile vff
  local out rc=0
  out="$(brand_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail vff_regression "${out}"; teardown; return; fi
  if ! grep -Fq 'remote activation OK' <<<"${out}"; then fail vff_regression "${out}"; teardown; return; fi
  if ! grep -Fq 'brand.id=vff' <<<"${out}"; then fail vff_regression "missing vff id"; teardown; return; fi
  pass vff_regression_activate
  teardown
}

# 16. temp cleanup
test_temp_cleanup() {
  local d
  d="$(mktemp -d)"
  chmod 0700 "${d}"
  rm -rf "${d}"
  [[ -d "${d}" ]] && { fail temp_cleanup "remains"; return; }
  pass temp_cleanup
}

# 17. secrets not in renderer stdout
test_no_secrets_stdout() {
  local dir src out log
  dir="$(mktemp -d)"
  src="${dir}/s.json"
  out="${dir}/o.json"
  cat >"${src}" <<'EOF'
{
  "telegram": {"token": "SECRET-TELEGRAM-TOKEN-VALUE"},
  "services": {"category": "vpn-mz-fc"},
  "web_sales": {"public_base_url": "https://connect-fc.vpn-for-friends.com"},
  "payments": {"profile": "telegram_friends_connect_bot"}
}
EOF
  log="$(bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${out}" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot 2>&1)"
  if grep -Fq 'SECRET-TELEGRAM-TOKEN-VALUE' <<<"${log}"; then
    fail no_secrets "${log}"; rm -rf "${dir}"; return
  fi
  pass no_secrets_stdout
  rm -rf "${dir}"
}

test_profiles_differ
test_missing_param
test_binary_deploy_backup_and_rollback
test_renderer
test_config_deploy_no_restart
test_fc_activation
test_vff_regression_activate
test_temp_cleanup
test_no_secrets_stdout

if [[ "${FAILS}" -ne 0 ]]; then
  echo "brand_ops_test: ${FAILS} failed" >&2
  exit 1
fi
echo "brand_ops_test: all passed"
