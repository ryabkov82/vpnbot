#!/usr/bin/env bash
# Isolated tests for tg_payments_webapp patcher + deploy script (mock HTTP).
# Does not touch production SHM.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PATCHER="${ROOT}/deploy/shm/templates/tg_payments_webapp/patch_template.py"
UPSTREAM="${ROOT}/deploy/shm/templates/tg_payments_webapp/testdata/tg_payments_webapp.upstream.html"
LOGIC="${ROOT}/deploy/shm/templates/tg_payments_webapp/testdata/payment_url_logic.mjs"
DEPLOY="${ROOT}/scripts/deploy-shm-tg-payments-template.sh"
CONFIG_LIB="${ROOT}/scripts/lib/shm_tg_payments_config.sh"
MARKER="VPNBOT_TG_PAYMENT_ROUTING_VERSION=1"

# shellcheck source=../lib/shm_tg_payments_config.sh
source "${CONFIG_LIB}"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

WORK=""
MOCK_PID=""
ORIG_PATH="${PATH}"
cleanup() {
  if [[ -n "${MOCK_PID}" ]] && kill -0 "${MOCK_PID}" 2>/dev/null; then
    kill "${MOCK_PID}" 2>/dev/null || true
    wait "${MOCK_PID}" 2>/dev/null || true
  fi
  MOCK_PID=""
  [[ -n "${WORK}" && -d "${WORK}" ]] && rm -rf "${WORK}"
  PATH="${ORIG_PATH}"
  unset CONFIG SCP_FAIL LOCAL_TMP CONFIG_USER CONFIG_HOST CONFIG_REMOTE_PATH || true
}
trap cleanup EXIT
setup() {
  PATH="${ORIG_PATH}"
  unset CONFIG SCP_FAIL LOCAL_TMP || true
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
}

run_patch() {
  python3 "${PATCHER}" --source "$1" --output "$2"
}

# --- patcher ---

test_upstream_to_v1() {
  setup
  local out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${out}" >/dev/null || { fail upstream_v1 "patch failed"; return; }
  grep -Fq "${MARKER}" "${out}" || { fail upstream_v1 "marker"; return; }
  grep -Fq "urlParams.get('brand_id')" "${out}" || { fail upstream_v1 "brand_id"; return; }
  grep -Fq "urlParams.get('yookassa_ps')" "${out}" || { fail upstream_v1 "yookassa_ps"; return; }
  grep -Fq "actualPs === ShmPayApp.yookassaPaySystem" "${out}" || { fail upstream_v1 "ps match"; return; }
  pass upstream_to_v1
}

test_v1_idempotent() {
  setup
  local a="${WORK}/a.html" b="${WORK}/b.html"
  run_patch "${UPSTREAM}" "${a}" >/dev/null
  run_patch "${a}" "${b}" >/dev/null
  cmp -s "${a}" "${b}" || { fail idempotent "changed"; return; }
  pass v1_idempotent
}

test_unknown_version_refuses() {
  setup
  local a="${WORK}/a.html" out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${a}" >/dev/null
  sed 's/VPNBOT_TG_PAYMENT_ROUTING_VERSION=1/VPNBOT_TG_PAYMENT_ROUTING_VERSION=9/g' "${a}" >"${WORK}/bad.html"
  if python3 "${PATCHER}" --source "${WORK}/bad.html" --output "${out}" >/dev/null 2>"${WORK}/err"; then
    fail unknown_ver "accepted"; return
  fi
  grep -qi version "${WORK}/err" || { fail unknown_ver "weak"; return; }
  pass unknown_version_refuses
}

test_missing_anchor_refuses() {
  setup
  local bad="${WORK}/bad.html" out="${WORK}/out.html"
  sed '/let ack_email = urlParams.get('\''ack_email'\'');/d' "${UPSTREAM}" >"${bad}"
  if python3 "${PATCHER}" --source "${bad}" --output "${out}" >/dev/null 2>"${WORK}/err"; then
    fail missing_anchor "accepted"; return
  fi
  pass missing_anchor_refuses
}

test_duplicated_anchor_refuses() {
  setup
  local bad="${WORK}/bad.html" out="${WORK}/out.html"
  {
    cat "${UPSTREAM}"
    echo '        makePayment(shm_url, internal) {'
  } >"${bad}"
  if python3 "${PATCHER}" --source "${bad}" --output "${out}" >/dev/null 2>"${WORK}/err"; then
    fail dup_anchor "accepted"; return
  fi
  pass duplicated_anchor_refuses
}

test_damaged_managed_refuses() {
  setup
  local a="${WORK}/a.html" out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${a}" >/dev/null
  sed '/BEGIN VPNBOT_TG_PAYMENT_INIT/,/END VPNBOT_TG_PAYMENT_INIT/d' "${a}" >"${WORK}/bad.html"
  if python3 "${PATCHER}" --source "${WORK}/bad.html" --output "${out}" >/dev/null 2>"${WORK}/err"; then
    fail damaged "accepted"; return
  fi
  pass damaged_managed_refuses
}

test_unrelated_html_css_unchanged() {
  setup
  local out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  python3 - <<'PY' "${PATCHER}" "${UPSTREAM}" "${out}"
import importlib.util
from pathlib import Path
import sys
spec = importlib.util.spec_from_file_location("p", sys.argv[1])
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
src = Path(sys.argv[2]).read_text(encoding="utf-8")
patched = Path(sys.argv[3]).read_text(encoding="utf-8")
base = mod.strip_managed_blocks(patched)
base = mod._restore_legacy_opens(base)
assert base == src
# CSS snippet unchanged
assert "input[type=\"email\"]::-webkit-input-placeholder" in patched
assert "<!DOCTYPE html>" in patched
print("ok")
PY
  pass unrelated_html_css_unchanged
}

test_protected_urls_and_no_creds() {
  setup
  local out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  grep -Fq "{{ config.api.url }}/shm/v1/telegram/webapp/auth" "${out}" || { fail urls "auth"; return; }
  grep -Fq "{{ config.api.url }}/shm/v1/user/pay/paysystems" "${out}" || { fail urls "paysystems"; return; }
  ! grep -qiE 'api_pass|sk_live_|account_id' "${out}" || { fail urls "creds"; return; }
  ! grep -Fq "shm_url + amount + '&email='" "${out}" || { fail urls "legacy email"; return; }
  pass protected_urls_and_no_creds
}

test_js_logic_harness() {
  node "${LOGIC}" || { fail js_logic "node harness failed"; return; }
  pass js_logic_harness
}

test_template_contains_amount_then_params() {
  setup
  local out="${WORK}/out.html"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  python3 - <<'PY' "${out}"
from pathlib import Path
import sys
text = Path(sys.argv[1]).read_text()
i_raw = text.find("var rawPaymentURL = shm_url + amount;")
i_brand = text.find("paymentURL.searchParams.set('brand_id'")
i_email = text.find("paymentURL.searchParams.set('email'")
assert 0 <= i_raw < i_brand < i_email
print("ok")
PY
  pass amount_then_params
}

# --- deploy mock ---

write_config() {
  cat >"${WORK}/config.json" <<EOF
{
  "api": {
    "base_url": "http://127.0.0.1:${MOCK_PORT}",
    "api_login": "testlogin",
    "api_pass": "test-secret-pass"
  }
}
EOF
}

start_mock() {
  local mode="${1:-ok}"
  MOCK_PORT="$(python3 - <<'PY'
import socket
s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()
PY
)"
  cp "${UPSTREAM}" "${WORK}/store.html"
  cat >"${WORK}/server.py" <<'PY'
#!/usr/bin/env python3
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

MODE = os.environ["MODE"]
STORE = Path(os.environ["STORE"])
PORT = int(os.environ["PORT"])
STATE = Path(os.environ["STATE"])
STATE.write_text("0")

class H(BaseHTTPRequestHandler):
    def _auth_ok(self):
        return self.headers.get("Authorization", "").startswith("Basic ")

    def do_GET(self):
        if self.path.startswith("/shm/v1/admin/template/tg_payments_webapp"):
            if not self._auth_ok():
                self.send_response(401); self.end_headers(); return
            body = STORE.read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return
        if self.path.startswith("/shm/v1/public/tg_payments_webapp"):
            if MODE == "public_fail":
                self.send_response(500); self.end_headers(); return
            body = STORE.read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return
        self.send_response(404); self.end_headers()

    def do_POST(self):
        if not self.path.startswith("/shm/v1/admin/template/tg_payments_webapp"):
            self.send_response(404); self.end_headers(); return
        if not self._auth_ok():
            self.send_response(401); self.end_headers(); return
        n = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(n)
        count = int(STATE.read_text() or "0") + 1
        STATE.write_text(str(count))
        if MODE == "post_fail":
            self.send_response(500); self.end_headers(); return
        if MODE == "verify_mismatch" and count == 1:
            # First POST (candidate): corrupt so verify GET mismatches.
            STORE.write_bytes(body + b"\n<!--corrupt-->\n")
            self.send_response(200); self.end_headers(); return
        STORE.write_bytes(body)
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.end_headers()

    def log_message(self, *a):
        return

HTTPServer(("127.0.0.1", PORT), H).serve_forever()
PY
  : >"${WORK}/state.txt"
  MODE="${mode}" STORE="${WORK}/store.html" STATE="${WORK}/state.txt" PORT="${MOCK_PORT}" \
    python3 "${WORK}/server.py" &
  MOCK_PID=$!
  for _ in $(seq 1 50); do
    curl -sS --max-time 1 -u testlogin:test-secret-pass \
      "http://127.0.0.1:${MOCK_PORT}/shm/v1/admin/template/tg_payments_webapp" >/dev/null 2>&1 && return 0
    sleep 0.05
  done
  return 1
}

stop_mock() {
  if [[ -n "${MOCK_PID}" ]] && kill -0 "${MOCK_PID}" 2>/dev/null; then
    kill "${MOCK_PID}" 2>/dev/null || true
    wait "${MOCK_PID}" 2>/dev/null || true
  fi
  MOCK_PID=""
}

fake_backup_host() {
  local bin="${WORK}/bin"
  mkdir -p "${bin}" "${WORK}/remote_fs/opt/shm/template-backups"
  cat >"${bin}/ssh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
while [[ \$# -gt 0 ]]; do
  case "\$1" in -o|-p) shift 2||true ;; -*) shift ;; *@*) shift; break ;; *) break ;; esac
done
cmd="\$*"
cmd="\${cmd//\\/opt\\//${WORK}/remote_fs/opt/}"
bash -c "\$cmd"
EOF
  chmod 0755 "${bin}/ssh"
  # Shared scp mock: runtime config fetch + backup store/restore.
  cat >"${bin}/scp" <<EOF
#!/usr/bin/env bash
set -euo pipefail
args=()
while [[ \$# -gt 0 ]]; do
  case "\$1" in
    -o) shift 2 || true ;;
    -q) shift ;;
    *) args+=("\$1"); shift ;;
  esac
done
src="\${args[0]}"
dest="\${args[1]}"
fake="${WORK}/remote_fs"
if [[ "\${SCP_FAIL:-0}" == "1" ]]; then
  echo "scp: mocked failure" >&2
  exit 1
fi
if [[ "\$src" == *config-vff.json ]]; then
  if [[ ! -f "${WORK}/remote_config.json" ]]; then
    echo "scp-mock: missing ${WORK}/remote_config.json" >&2
    exit 1
  fi
  cp "${WORK}/remote_config.json" "\$dest"
  printf '%s\n' "\$dest" >"${WORK}/scp_dest_seen"
  exit 0
fi
if [[ "\$src" == *:* ]]; then
  cp "\${fake}\${src#*:}" "\${dest}"
elif [[ "\$dest" == *:* ]]; then
  mkdir -p "\$(dirname "\${fake}\${dest#*:}")"
  cp "\${src}" "\${fake}\${dest#*:}"
else
  cp "\${src}" "\${dest}"
fi
EOF
  chmod 0755 "${bin}/scp"
  export PATH="${bin}:${PATH}"
  export SHM_BACKUP_HOST=fake-shm
  export SHM_BACKUP_USER=root
  export SHM_BACKUP_DIR=/opt/shm/template-backups
}

test_deploy_check_mocked() {
  setup
  start_mock ok
  write_config
  fake_backup_host
  if ! CONFIG="${WORK}/config.json" bash "${DEPLOY}" check >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail deploy_check "$(cat "${WORK}/err")"; return
  fi
  if grep -qiE 'test-secret-pass|api_pass' "${WORK}/out" "${WORK}/err"; then
    stop_mock; fail deploy_check "secret leaked"; return
  fi
  grep -q 'check OK' "${WORK}/err" || { stop_mock; fail deploy_check "no OK in stderr"; return; }
  # check must not POST
  [[ "$(cat "${WORK}/state.txt")" == "0" ]] || {
    stop_mock; fail deploy_check "unexpected POST during check"; return
  }
  stop_mock
  pass deploy_check_mocked
}

test_deploy_post_and_verify() {
  setup
  start_mock ok
  write_config
  fake_backup_host
  if ! CONFIG="${WORK}/config.json" bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail deploy_ok "$(cat "${WORK}/err")"; return
  fi
  grep -Fq "${MARKER}" "${WORK}/store.html" || { stop_mock; fail deploy_ok "store not patched"; return; }
  ls "${WORK}/remote_fs/opt/shm/template-backups/"tg_payments_webapp.*.html >/dev/null \
    || { stop_mock; fail deploy_ok "no backup"; return; }
  if grep -qiE 'test-secret-pass' "${WORK}/out" "${WORK}/err"; then
    stop_mock; fail deploy_ok "secret leaked"; return
  fi
  stop_mock
  pass deploy_post_and_verify
}

test_deploy_post_fail_no_change() {
  setup
  start_mock post_fail
  write_config
  fake_backup_host
  cp "${UPSTREAM}" "${WORK}/before.html"
  if CONFIG="${WORK}/config.json" bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail post_fail "deploy succeeded"; return
  fi
  cmp -s "${WORK}/before.html" "${WORK}/store.html" || { stop_mock; fail post_fail "store changed"; return; }
  stop_mock
  pass deploy_post_fail_no_change
}

test_deploy_verify_mismatch_rollback() {
  setup
  start_mock verify_mismatch
  write_config
  fake_backup_host
  # After mismatch, cleanup/deploy should POST backup restoring upstream-ish content.
  if CONFIG="${WORK}/config.json" bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail verify_mismatch "deploy succeeded"; return
  fi
  # Store should not remain as successful candidate with marker only from corrupt path —
  # rollback restores backup (upstream without marker).
  if grep -Fq "${MARKER}" "${WORK}/store.html" && ! grep -Fq 'corrupt' "${WORK}/store.html"; then
    # acceptable if rolled back to upstream (no marker)
    :
  fi
  ! grep -Fq '<!--corrupt-->' "${WORK}/store.html" || {
    # rollback should have overwritten corrupt store
    stop_mock; fail verify_mismatch "corrupt remains"; return
  }
  stop_mock
  pass deploy_verify_mismatch_rollback
}

test_deploy_public_fail_rollback() {
  setup
  # public_fail: POST succeeds, verify GET admin matches, public fails → rollback
  start_mock public_fail
  write_config
  fake_backup_host
  if CONFIG="${WORK}/config.json" bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail public_fail "deploy succeeded"; return
  fi
  # After rollback, store should equal upstream (no marker)
  ! grep -Fq "${MARKER}" "${WORK}/store.html" || {
    stop_mock; fail public_fail "marker remains after rollback"; return
  }
  stop_mock
  pass deploy_public_fail_rollback
}

# --- resolve_config regression ---

write_remote_config_fixture() {
  cat >"${WORK}/remote_config.json" <<'EOF'
{
  "api": {
    "base_url": "http://127.0.0.1:9",
    "api_login": "cfglogin",
    "api_pass": "cfg-secret-pass"
  }
}
EOF
}

test_resolve_config_path_only_stdout() {
  setup
  write_remote_config_fixture
  fake_backup_host
  LOCAL_TMP="${WORK}/tmp"
  mkdir -p "${LOCAL_TMP}"
  chmod 0700 "${LOCAL_TMP}"
  unset CONFIG || true
  unset SCP_FAIL || true
  export CONFIG_USER=root CONFIG_HOST=fr-mrs-1 CONFIG_REMOTE_PATH=/opt/bot/config-vff.json

  local path
  path="$(shm_tg_payments_resolve_config 2>"${WORK}/err")" || {
    fail resolve_stdout "resolve failed: $(cat "${WORK}/err")"; return
  }
  [[ -f "${path}" ]] || { fail resolve_stdout "path missing: ${path}"; return; }
  [[ "${path}" == "${LOCAL_TMP}/runtime-config.json" ]] || {
    fail resolve_stdout "unexpected path ${path}"; return
  }
  local lines
  lines="$(shm_tg_payments_resolve_config 2>/dev/null | wc -l)"
  [[ "${lines}" -eq 1 ]] || { fail resolve_stdout "stdout lines=${lines}"; return; }
  grep -q 'fetching runtime config' "${WORK}/err" || {
    fail resolve_stdout "log missing on stderr"; return
  }
  ! grep -q 'fetching runtime config' <<<"${path}" || {
    fail resolve_stdout "log leaked into path"; return
  }
  jq -e '.api.base_url' "${path}" >/dev/null || {
    fail resolve_stdout "jq cannot read config"; return
  }
  [[ -f "${path}" ]] || { fail resolve_stdout "config vanished"; return; }
  if grep -qiE 'cfg-secret-pass|api_pass' "${WORK}/err" <<<"${path}"; then
    fail resolve_stdout "secret leaked"; return
  fi
  pass resolve_config_path_only_stdout
}

test_resolve_config_local_CONFIG() {
  setup
  write_remote_config_fixture
  fake_backup_host
  LOCAL_TMP="${WORK}/tmp"; mkdir -p "${LOCAL_TMP}"
  export CONFIG="${WORK}/remote_config.json"
  unset SCP_FAIL || true
  local path
  path="$(shm_tg_payments_resolve_config 2>"${WORK}/err")" || {
    fail resolve_local "failed"; return
  }
  [[ "${path}" == "${WORK}/remote_config.json" ]] || {
    fail resolve_local "path=${path}"; return
  }
  [[ ! -f "${WORK}/scp_dest_seen" ]] || {
    fail resolve_local "scp was called despite CONFIG"; return
  }
  pass resolve_config_local_CONFIG
}

test_resolve_config_scp_fail_before_api() {
  setup
  write_remote_config_fixture
  fake_backup_host
  LOCAL_TMP="${WORK}/tmp"; mkdir -p "${LOCAL_TMP}"
  unset CONFIG || true
  export SCP_FAIL=1
  if shm_tg_payments_resolve_config >/dev/null 2>"${WORK}/err"; then
    fail resolve_scp_fail "expected failure"; return
  fi
  grep -qi 'failed to download' "${WORK}/err" || {
    fail resolve_scp_fail "weak error"; return
  }
  pass resolve_config_scp_fail_before_api
}

test_check_remote_config_no_post_no_secret() {
  setup
  start_mock ok
  cat >"${WORK}/remote_config.json" <<EOF
{
  "api": {
    "base_url": "http://127.0.0.1:${MOCK_PORT}",
    "api_login": "testlogin",
    "api_pass": "test-secret-pass"
  }
}
EOF
  fake_backup_host
  unset CONFIG || true
  unset SCP_FAIL || true
  export TG_TEMPLATE_CONFIG_HOST=fr-mrs-1
  export TG_TEMPLATE_CONFIG_USER=root
  export TG_TEMPLATE_CONFIG_REMOTE=/opt/bot/config-vff.json
  if ! bash "${DEPLOY}" check >"${WORK}/out" 2>"${WORK}/err"; then
    stop_mock; fail check_remote "$(cat "${WORK}/err")"; return
  fi
  grep -q 'fetching runtime config' "${WORK}/err" || {
    stop_mock; fail check_remote "fetch log missing"; return
  }
  ! grep -q 'fetching runtime config' "${WORK}/out" || {
    stop_mock; fail check_remote "fetch log on stdout"; return
  }
  [[ "$(cat "${WORK}/state.txt")" == "0" ]] || {
    stop_mock; fail check_remote "POST during check"; return
  }
  if grep -qiE 'test-secret-pass|api_pass' "${WORK}/out" "${WORK}/err"; then
    stop_mock; fail check_remote "secret leaked"; return
  fi
  stop_mock
  pass check_remote_config_no_post_no_secret
}

# --- run ---

test_upstream_to_v1
test_v1_idempotent
test_unknown_version_refuses
test_missing_anchor_refuses
test_duplicated_anchor_refuses
test_damaged_managed_refuses
test_unrelated_html_css_unchanged
test_protected_urls_and_no_creds
test_js_logic_harness
test_template_contains_amount_then_params
test_resolve_config_path_only_stdout
test_resolve_config_local_CONFIG
test_resolve_config_scp_fail_before_api
test_check_remote_config_no_post_no_secret
test_deploy_check_mocked
test_deploy_post_and_verify
test_deploy_post_fail_no_change
test_deploy_verify_mismatch_rollback
test_deploy_public_fail_rollback

if [[ "${FAILS}" -ne 0 ]]; then
  echo "FAILED ${FAILS} shm_tg_payments_template tests" >&2
  exit 1
fi
echo "OK shm_tg_payments_template_test"
