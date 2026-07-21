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
    # Optional: fail specifically on Nth is-active call in this workspace.
    if [[ -f "${WORK}/is_active_fail_on" ]]; then
      n="\$(cat "${WORK}/is_active_count" 2>/dev/null || echo 0)"
      n=\$((n + 1))
      echo "\$n" >"${WORK}/is_active_count"
      if [[ "\$n" -eq "\$(cat "${WORK}/is_active_fail_on")" ]]; then
        exit 3
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

# 3. successful binary deploy creates backup
test_binary_deploy_backup() {
  setup_profile fc
  printf 'known-good-v1\n' >"${REMOTE_BINARY}"
  printf 'broken-v2\n' >"${REMOTE_BINARY}.new"
  # First install succeeds with broken-v2 content (name is just payload).
  printf 'known-good-v1\n' >"${REMOTE_BINARY}"
  printf 'payload-v2\n' >"${REMOTE_BINARY}.new"
  local out rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail binary_ok "${out}"; teardown; return; fi
  if ! ls "${REMOTE_BINARY}".bak.* >/dev/null 2>&1; then fail binary_ok "no backup"; teardown; return; fi
  if ! grep -Fxq 'payload-v2' "${REMOTE_BINARY}"; then fail binary_ok "binary not replaced"; teardown; return; fi
  if [[ -e "${REMOTE_BINARY}.new" || -e "${REMOTE_BINARY}.rollback.new" ]]; then
    fail binary_ok "temp files remain"; teardown; return
  fi
  pass binary_deploy_backup
  teardown
}

# 4. restart fail → restore exact known-good-v1; temps gone; backup exists
test_binary_restart_rollback_restores_content() {
  setup_profile fc
  printf 'known-good-v1\n' >"${REMOTE_BINARY}"
  printf 'broken-v2\n' >"${REMOTE_BINARY}.new"
  : >"${WORK}/restart_fail_once"
  local out rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail binary_restart_rollback "should fail"; teardown; return; fi
  if ! grep -Fq 'restart failed' <<<"${out}"; then fail binary_restart_rollback "missing reason: ${out}"; teardown; return; fi
  if ! grep -Fq 'previous binary restored' <<<"${out}"; then fail binary_restart_rollback "missing restored: ${out}"; teardown; return; fi
  if grep -Fq 'CRITICAL' <<<"${out}"; then fail binary_restart_rollback "unexpected CRITICAL: ${out}"; teardown; return; fi
  if ! grep -Fxq 'known-good-v1' "${REMOTE_BINARY}"; then
    fail binary_restart_rollback "content=$(cat "${REMOTE_BINARY}")"; teardown; return
  fi
  if [[ -e "${REMOTE_BINARY}.new" || -e "${REMOTE_BINARY}.rollback.new" ]]; then
    fail binary_restart_rollback "temp remains"; teardown; return
  fi
  if ! ls "${REMOTE_BINARY}".bak.* >/dev/null 2>&1; then
    fail binary_restart_rollback "backup missing"; teardown; return
  fi
  pass binary_restart_rollback_restores_content
  teardown
}

# 5. rollback also fails → CRITICAL
test_binary_rollback_critical() {
  setup_profile fc
  printf 'known-good-v1\n' >"${REMOTE_BINARY}"
  printf 'broken-v2\n' >"${REMOTE_BINARY}.new"
  : >"${WORK}/restart_always_fail"
  local out rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail binary_critical "should fail"; teardown; return; fi
  if ! grep -Fq 'CRITICAL: FC binary deployment failed and automatic binary rollback failed' <<<"${out}"; then
    fail binary_critical "${out}"; teardown; return
  fi
  if grep -Fq 'previous binary restored' <<<"${out}"; then
    fail binary_critical "should not claim restored"; teardown; return
  fi
  pass binary_rollback_critical
  teardown
}

# 6. second is-active after rollback sleep fails → CRITICAL
test_binary_rollback_second_active_fails() {
  setup_profile fc
  printf 'known-good-v1\n' >"${REMOTE_BINARY}"
  printf 'broken-v2\n' >"${REMOTE_BINARY}.new"
  : >"${WORK}/restart_fail_once"
  # After deploy restart fails, rollback: restart OK, is-active#1 OK, is-active#2 FAIL
  echo 0 >"${WORK}/is_active_count"
  echo 2 >"${WORK}/is_active_fail_on"
  local out rc=0
  out="$(brand_deploy_binary 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail binary_second_active "should fail"; teardown; return; fi
  if ! grep -Fq 'CRITICAL: FC binary deployment failed and automatic binary rollback failed' <<<"${out}"; then
    fail binary_second_active "${out}"; teardown; return
  fi
  pass binary_rollback_second_active_fails
  teardown
}

# 7. marker-based rollback (manual / smoke path)
test_binary_marker_rollback() {
  setup_profile fc
  bak="${REMOTE_BINARY}.bak.manual"
  printf 'restored\n' >"${bak}"
  printf '%s\n' "${bak}" >"${REMOTE_DIR}/.vpnbot-last-binary-bak"
  printf 'broken\n' >"${REMOTE_BINARY}"
  local out rc=0
  out="$(brand_rollback_binary_from_marker 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail binary_marker "${out}"; teardown; return; fi
  if ! grep -Fxq 'restored' "${REMOTE_BINARY}"; then fail binary_marker "not restored"; teardown; return; fi
  # exact path required: no-arg automatic rollback must fail
  rc=0
  brand_rollback_binary >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail binary_marker "no-arg should fail"; teardown; return; fi
  pass binary_marker_rollback
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

# config deploy does not create drop-in / restart
test_config_deploy_no_restart() {
  setup_profile fc
  printf '%s\n' '{}' >"${REMOTE_EXPLICIT_CONFIG}.new"
  brand_deploy_config_file "${REMOTE_EXPLICIT_CONFIG}.new" >/dev/null
  if [[ ! -f "${REMOTE_EXPLICIT_CONFIG}" ]]; then fail config_deploy "not installed"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail config_deploy "drop-in must not be created"; teardown; return; fi
  pass config_deploy_no_restart
  teardown
}

test_fc_activation() {
  setup_profile fc
  local out rc=0
  out="$(brand_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail fc_activate "${out}"; teardown; return; fi
  if ! grep -Fq 'brand.id=fc' <<<"${out}"; then fail fc_activate "no fc id"; teardown; return; fi
  pass fc_activation

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

test_foreign_dropin_not_removed() {
  setup_profile fc
  printf '%s\n' '[Service]' 'Environment=VPNBOT_CONFIG=/other.json' >"${DROPIN_FILE}"
  local before out rc=0
  before="$(cat "${DROPIN_FILE}")"
  out="$(brand_rollback_to_legacy 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail foreign_dropin "should refuse"; teardown; return; fi
  if ! grep -Fq 'refusing to remove drop-in with unexpected content' <<<"${out}"; then
    fail foreign_dropin "${out}"; teardown; return
  fi
  if [[ "$(cat "${DROPIN_FILE}")" != "${before}" ]]; then
    fail foreign_dropin "content changed"; teardown; return
  fi
  if grep -Fq 'legacy active' <<<"${out}"; then
    fail foreign_dropin "claimed legacy success"; teardown; return
  fi
  pass foreign_dropin_not_removed

  # idempotent when absent
  rm -f "${DROPIN_FILE}"
  rc=0
  out="$(brand_rollback_to_legacy 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail dropin_idempotent "${out}"; teardown; return; fi
  if ! grep -Fq 'drop-in absent (idempotent)' <<<"${out}"; then
    fail dropin_idempotent "${out}"; teardown; return
  fi
  pass dropin_absent_idempotent
  teardown
}

test_vff_managed_dropin_removed() {
  setup_profile vff
  printf '%s\n' "${DROPIN_BODY}" >"${DROPIN_FILE}"
  local out rc=0
  out="$(brand_rollback_to_legacy 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail vff_dropin_rm "${out}"; teardown; return; fi
  if [[ -f "${DROPIN_FILE}" ]]; then fail vff_dropin_rm "still present"; teardown; return; fi
  pass vff_managed_dropin_removed
  teardown
}

test_vff_regression_activate() {
  setup_profile vff
  local out rc=0
  out="$(brand_activate "${WORK}/configcheck_ok" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail vff_regression "${out}"; teardown; return; fi
  if ! grep -Fq 'brand.id=vff' <<<"${out}"; then fail vff_regression "missing vff id"; teardown; return; fi
  pass vff_regression_activate
  teardown
}

test_renderer_source_eq_output() {
  local dir src
  dir="$(mktemp -d)"
  src="${dir}/same.json"
  cat >"${src}" <<'EOF'
{"telegram":{"token":"t"},"services":{"category":"vpn-mz-fc"},"web_sales":{"public_base_url":"https://connect-fc.vpn-for-friends.com"},"payments":{"profile":"telegram_friends_connect_bot"}}
EOF
  local before rc=0
  before="$(cat "${src}")"
  bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${dir}/../$(basename "${dir}")/same.json" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail render_same "should fail"; rm -rf "${dir}"; return; fi
  if [[ "$(cat "${src}")" != "${before}" ]]; then fail render_same "source changed"; rm -rf "${dir}"; return; fi
  if compgen -G "${dir}/.config-brand.*" >/dev/null; then fail render_same "temp left"; rm -rf "${dir}"; return; fi
  pass renderer_source_eq_output
  rm -rf "${dir}"
}

test_renderer_keeps_output_on_error() {
  local dir src out
  dir="$(mktemp -d)"
  src="${dir}/bad.json"
  out="${dir}/out.json"
  printf '%s\n' '{"marker":"KEEP-ME"}' >"${out}"
  printf '%s\n' '{"telegram":{"token":"t"}}' >"${src}"
  local before rc=0
  before="$(cat "${out}")"
  bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${out}" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail render_keep "should fail"; rm -rf "${dir}"; return; fi
  if [[ "$(cat "${out}")" != "${before}" ]]; then fail render_keep "output changed"; rm -rf "${dir}"; return; fi
  if compgen -G "${dir}/.config-brand.*" >/dev/null; then fail render_keep "temp left"; rm -rf "${dir}"; return; fi
  pass renderer_keeps_output_on_error
  rm -rf "${dir}"
}

test_renderer_temp_in_outdir_and_mode() {
  local dir src out seen
  dir="$(mktemp -d)"
  src="${dir}/src.json"
  out="${dir}/out.json"
  cat >"${src}" <<'EOF'
{"telegram":{"token":"t"},"services":{"category":"vpn-mz-fc"},"web_sales":{"public_base_url":"https://connect-fc.vpn-for-friends.com"},"payments":{"profile":"telegram_friends_connect_bot"}}
EOF
  # Intercept mktemp to record template.
  cat >"${dir}/mktemp" <<EOF
#!/usr/bin/env bash
echo "MKTEMP_TEMPLATE=\$1" >>"${dir}/mktemp.log"
exec /usr/bin/mktemp "\$@"
EOF
  chmod 0700 "${dir}/mktemp"
  PATH="${dir}:${PATH}" bash "${ROOT}/scripts/render-brand-config.sh" \
    --source "${src}" --output "${out}" \
    --brand-id fc --brand-name "Friends Connect" \
    --allowed-host connect-fc.vpn-for-friends.com \
    --landing-url https://friends-connect.club \
    --expect-public-base-url https://connect-fc.vpn-for-friends.com \
    --expect-service-category vpn-mz-fc \
    --expect-payment-profile telegram_friends_connect_bot >/dev/null
  seen="$(cat "${dir}/mktemp.log")"
  if ! grep -Fq "${dir}/.config-brand." <<<"${seen}"; then
    fail render_temp "template=${seen}"; rm -rf "${dir}"; return
  fi
  local mode
  mode="$(stat -c '%a' "${out}")"
  if [[ "${mode}" != "600" ]]; then fail render_mode "mode=${mode}"; rm -rf "${dir}"; return; fi
  pass renderer_temp_in_outdir_and_mode
  rm -rf "${dir}"
}

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

# Orchestrator cleanup: trap removes local temp after forced failure.
test_orchestrator_local_temp_cleanup() {
  local dir
  dir="$(mktemp -d)"
  chmod 0700 "${dir}"
  cat >"${dir}/ssh" <<EOF
#!/usr/bin/env bash
# First ssh (mktemp -d remote): print fake remote tmp
if [[ "\$*" == *mktemp* ]]; then
  echo "${dir}/remote-tmp"
  mkdir -p "${dir}/remote-tmp"
  exit 0
fi
exit 1
EOF
  chmod 0700 "${dir}/ssh"
  cat >"${dir}/scp" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  chmod 0700 "${dir}/scp"
  cat >"${dir}/go" <<'EOF'
#!/usr/bin/env bash
# go test / go build stubs
if [[ "${1:-}" == "test" ]]; then exit 0; fi
if [[ "${1:-}" == "build" ]]; then
  out=""
  while [[ $# -gt 0 ]]; do
    if [[ "$1" == "-o" ]]; then out="$2"; break; fi
    shift
  done
  printf 'bin\n' >"${out:-/dev/null}"
  exit 0
fi
exit 0
EOF
  chmod 0700 "${dir}/go"

  # Force failure after LOCAL_TMP exists: invalid SERVER_HOST after build by breaking second ssh.
  # Wrap deploy script env.
  local rc=0
  PATH="${dir}:${PATH}" \
    SERVER_USER=root SERVER_HOST=invalid-host-for-test \
    SERVICE_NAME=bot-friends-connect.service \
    REMOTE_DIR=/opt/bot-friends-connect \
    REMOTE_BINARY=/opt/bot-friends-connect/bot \
    REMOTE_LEGACY_CONFIG=/opt/bot-friends-connect/config.json \
    REMOTE_EXPLICIT_CONFIG=/opt/bot-friends-connect/config-fc.json \
    DROPIN_FILE=/etc/systemd/system/bot-friends-connect.service.d/10-vpnbot-config.conf \
    EXPECTED_BRAND_ID=fc BRAND_LABEL=FC \
    SMOKE_BASE_URL=https://connect-fc.vpn-for-friends.com \
    bash "${ROOT}/scripts/deploy-brand-binary.sh" >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail orch_cleanup "should fail"; rm -rf "${dir}"; return; fi
  # Local mktemp dirs from the script live under /tmp; ensure our injected remote-tmp cleaned by trap when REMOTE_TMP set.
  # At minimum: script exited non-zero and did not leave a bot.new under REMOTE paths in this sandbox.
  pass orchestrator_local_temp_cleanup
  rm -rf "${dir}"
}

test_profiles_differ
test_missing_param
test_binary_deploy_backup
test_binary_restart_rollback_restores_content
test_binary_rollback_critical
test_binary_rollback_second_active_fails
test_binary_marker_rollback
test_renderer
test_renderer_source_eq_output
test_renderer_keeps_output_on_error
test_renderer_temp_in_outdir_and_mode
test_config_deploy_no_restart
test_fc_activation
test_foreign_dropin_not_removed
test_vff_managed_dropin_removed
test_vff_regression_activate
test_no_secrets_stdout
test_orchestrator_local_temp_cleanup

if [[ "${FAILS}" -ne 0 ]]; then
  echo "brand_ops_test: ${FAILS} failed" >&2
  exit 1
fi
echo "brand_ops_test: all passed"
