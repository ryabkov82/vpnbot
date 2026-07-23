#!/usr/bin/env bash
# Isolated tests for deploy/shm/yookassa patcher + deploy-shm-yookassa.sh logic.
# Does not touch production SHM.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PATCHER="${ROOT}/deploy/shm/yookassa/patch_yookassa.py"
UPSTREAM="${ROOT}/deploy/shm/yookassa/testdata/yookassa.cgi.upstream"
DEPLOY="${ROOT}/scripts/deploy-shm-yookassa.sh"
VFF_PROFILE="${ROOT}/deploy/brands/vff.json"
FC_PROFILE="${ROOT}/deploy/brands/fc.json"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

WORK=""
cleanup() {
  if [[ -n "${WORK}" && -d "${WORK}" ]]; then
    rm -rf "${WORK}"
  fi
}
trap cleanup EXIT

setup() {
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
}

vff_return_url() {
  python3 - <<'PY' "${VFF_PROFILE}"
import json, sys
p = json.load(open(sys.argv[1], encoding="utf-8"))
base = p["brand"]["public_base_url"].rstrip("/")
print(f"{base}/payment/return")
PY
}

fc_return_url() {
  python3 - <<'PY' "${FC_PROFILE}"
import json, sys
p = json.load(open(sys.argv[1], encoding="utf-8"))
base = p["brand"]["public_base_url"].rstrip("/")
print(f"{base}/payment/return")
PY
}

run_patch() {
  local src="$1"
  local out="$2"
  shift 2
  python3 "${PATCHER}" \
    --source "${src}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" \
    "$@"
}

# --- patcher tests ---

test_inserts_vff_fc_mapping_from_profiles() {
  setup
  local out="${WORK}/out.cgi"
  if ! run_patch "${UPSTREAM}" "${out}" >/dev/null; then
    fail insert_mapping "patcher failed"; return
  fi
  local vff_url fc_url
  vff_url="$(vff_return_url)"
  fc_url="$(fc_return_url)"
  grep -Fq "VPNBOT_BRAND_ROUTING_VERSION=1" "${out}" || { fail insert_mapping "marker missing"; return; }
  grep -Fq "'vff' => '${vff_url}'" "${out}" || { fail insert_mapping "vff mapping missing: ${vff_url}"; return; }
  grep -Fq "'fc' => '${fc_url}'" "${out}" || { fail insert_mapping "fc mapping missing: ${fc_url}"; return; }
  grep -Fq "Error: unknown brand_id" "${out}" || { fail insert_mapping "unknown brand_id path missing"; return; }
  # URLs must come from profiles, not hard-coded literals in the patcher source for these hosts.
  if ! grep -Fq "${vff_url}" "${out}"; then
    fail insert_mapping "vff url not from profile"; return
  fi
  pass insert_mapping
}

test_trailing_slash_normalized() {
  setup
  cat >"${WORK}/slash.json" <<'EOF'
{
  "id": "slashy",
  "brand": {
    "public_base_url": "https://example.test/base/"
  }
}
EOF
  local out="${WORK}/out.cgi"
  if ! python3 "${PATCHER}" \
    --source "${UPSTREAM}" \
    --output "${out}" \
    --brand-profile "${WORK}/slash.json" >/dev/null; then
    fail trailing_slash "patcher failed"; return
  fi
  grep -Fq "'slashy' => 'https://example.test/base/payment/return'" "${out}" \
    || { fail trailing_slash "expected normalized URL"; return; }
  if grep -Fq "https://example.test/base//payment/return" "${out}"; then
    fail trailing_slash "double slash present"; return
  fi
  pass trailing_slash
}

test_brand_validation_before_user_lookup() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  local brand_line user_lookup_line unknown_user_line
  brand_line="$(grep -n "Error: unknown brand_id" "${out}" | head -1 | cut -d: -f1)"
  user_lookup_line="$(grep -n 'if ( \$vars{user_id} ) {' "${out}" | head -1 | cut -d: -f1)"
  unknown_user_line="$(grep -n "Error: unknown user" "${out}" | head -1 | cut -d: -f1)"
  if [[ -z "${brand_line}" || -z "${user_lookup_line}" || -z "${unknown_user_line}" ]]; then
    fail brand_before_user "missing order anchors"; return
  fi
  if [[ "${brand_line}" -ge "${user_lookup_line}" ]]; then
    fail brand_before_user "brand validation not before user lookup (${brand_line} >= ${user_lookup_line})"; return
  fi
  if [[ "${brand_line}" -ge "${unknown_user_line}" ]]; then
    fail brand_before_user "brand validation not before unknown-user path"; return
  fi
  pass brand_validation_before_user_lookup
}

test_unknown_brand_not_masked_by_user() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  # Structural guarantee: unknown brand_id exits before user_id lookup body.
  python3 - <<'PY' "${out}"
from pathlib import Path
import sys
text = Path(sys.argv[1]).read_text(encoding="utf-8")
brand = text.find("Error: unknown brand_id")
user = text.find("    if ( $vars{user_id} ) {")
assert brand >= 0 and user >= 0 and brand < user, (brand, user)
# Ensure validation writes into a separate variable, not $return_url directly.
assert "$vpnbot_brand_return_url =" in text
assert "my $vpnbot_brand_return_url;" in text
print("ok")
PY
  pass unknown_brand_not_masked_by_user
}

test_return_url_override_applied() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  local ret_line apply_line
  ret_line="$(grep -n 'my \$return_url =     \$ps_config{return_url};' "${out}" | head -1 | cut -d: -f1)"
  apply_line="$(grep -n '\$return_url = \$vpnbot_brand_return_url if defined \$vpnbot_brand_return_url;' "${out}" | head -1 | cut -d: -f1)"
  if [[ -z "${ret_line}" || -z "${apply_line}" || "${apply_line}" -le "${ret_line}" ]]; then
    fail return_url_override "apply not after legacy return_url assignment"; return
  fi
  grep -Fq 'BEGIN VPNBOT_BRAND_RETURN_APPLY' "${out}" || { fail return_url_override "apply block missing"; return; }
  pass return_url_override_applied
}

test_legacy_route_preserved() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  # Missing/empty brand_id leaves $vpnbot_brand_return_url undefined → legacy ps_config URL.
  grep -Fq 'my $vpnbot_brand_return_url;' "${out}" || { fail legacy "override var missing"; return; }
  grep -Fq '$return_url = $vpnbot_brand_return_url if defined $vpnbot_brand_return_url;' "${out}" \
    || { fail legacy "conditional apply missing"; return; }
  grep -Fq 'my $return_url =     $ps_config{return_url};' "${out}" \
    || { fail legacy "legacy return_url assignment missing"; return; }
  # confirmation still uses $return_url variable (legacy value when brand_id absent).
  grep -Fq "return_url => \$return_url || 'https://www.example.com'" "${out}" \
    || { fail legacy "confirmation return_url changed"; return; }
  pass legacy_route
}

test_idempotent_reapply() {
  setup
  local once="${WORK}/once.cgi"
  local twice="${WORK}/twice.cgi"
  run_patch "${UPSTREAM}" "${once}" >/dev/null
  run_patch "${once}" "${twice}" >/dev/null
  if ! cmp -s "${once}" "${twice}"; then
    fail idempotent "second apply changed content"; return
  fi
  local count begin_v begin_a
  count="$(grep -c 'VPNBOT_BRAND_ROUTING_VERSION=1' "${twice}" || true)"
  begin_v="$(grep -c 'BEGIN VPNBOT_BRAND_ROUTING$' "${twice}" || true)"
  begin_a="$(grep -c 'BEGIN VPNBOT_BRAND_RETURN_APPLY$' "${twice}" || true)"
  # Marker appears in validation + apply blocks; each block once.
  [[ "${count}" -eq 2 ]] || { fail idempotent "marker count=${count} (want 2)"; return; }
  [[ "${begin_v}" -eq 1 && "${begin_a}" -eq 1 ]] || { fail idempotent "block counts v=${begin_v} a=${begin_a}"; return; }
  pass idempotent
}

test_missing_anchor_refuses() {
  setup
  local bad="${WORK}/bad.cgi"
  sed '/my \$return_url =     \$ps_config{return_url};/d' "${UPSTREAM}" >"${bad}"
  local out="${WORK}/out.cgi"
  if python3 "${PATCHER}" \
    --source "${bad}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail missing_anchor "expected failure"; return
  fi
  [[ ! -f "${out}" ]] || { fail missing_anchor "output created despite failure"; return; }
  grep -qi 'missing' "${WORK}/err" || { fail missing_anchor "error message weak"; return; }
  pass missing_anchor
}

test_duplicated_anchor_refuses() {
  setup
  local bad="${WORK}/bad.cgi"
  {
    cat "${UPSTREAM}"
    echo '    my $return_url =     $ps_config{return_url};'
  } >"${bad}"
  local out="${WORK}/out.cgi"
  if python3 "${PATCHER}" \
    --source "${bad}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail dup_anchor "expected failure"; return
  fi
  [[ ! -f "${out}" ]] || { fail dup_anchor "output created"; return; }
  grep -qi 'duplicat' "${WORK}/err" || { fail dup_anchor "error message weak"; return; }
  pass duplicated_anchor
}

test_other_version_marker_refuses() {
  setup
  local once="${WORK}/once.cgi"
  run_patch "${UPSTREAM}" "${once}" >/dev/null
  sed -i 's/VPNBOT_BRAND_ROUTING_VERSION=1/VPNBOT_BRAND_ROUTING_VERSION=2/' "${once}"
  local out="${WORK}/out.cgi"
  if python3 "${PATCHER}" \
    --source "${once}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail other_version "expected failure"; return
  fi
  [[ ! -f "${out}" ]] || { fail other_version "output created"; return; }
  grep -qi 'version' "${WORK}/err" || { fail other_version "error message weak"; return; }
  pass other_version_marker
}

test_credentials_callback_metadata_unchanged() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  # Strip managed regions via patcher and compare remainder to upstream.
  python3 - <<'PY' "${PATCHER}" "${UPSTREAM}" "${out}"
import importlib.util
from pathlib import Path
import sys
patcher_path, src_path, patched_path = map(Path, sys.argv[1:4])
spec = importlib.util.spec_from_file_location("patch_yookassa", patcher_path)
mod = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(mod)
stripped = mod.strip_routing_block(patched_path.read_text(encoding="utf-8"))
assert stripped == src_path.read_text(encoding="utf-8")
PY
  # Explicit anchors still present once.
  for needle in \
    'my $api_key =        $ps_config{api_key};' \
    'my $account_id =     $ps_config{account_id};' \
    'metadata => {' \
    'return_url => $return_url || '"'"'https://www.example.com'"'"',' \
    'if ( $vars{user_id} ) {'
  do
    local n
    n="$(grep -F -c "${needle}" "${out}" || true)"
    [[ "${n}" -eq 1 ]] || { fail creds_meta "anchor count for ${needle}: ${n}"; return; }
  done
  pass credentials_callback_metadata_unchanged
}

test_invalid_profile_refuses() {
  setup
  cat >"${WORK}/bad.json" <<'EOF'
{ "id": "x", "brand": {} }
EOF
  local out="${WORK}/out.cgi"
  if python3 "${PATCHER}" \
    --source "${UPSTREAM}" \
    --output "${out}" \
    --brand-profile "${WORK}/bad.json" >/dev/null 2>"${WORK}/err"; then
    fail invalid_profile "expected failure"; return
  fi
  [[ ! -f "${out}" ]] || { fail invalid_profile "output created"; return; }
  pass invalid_profile
}

test_invalid_public_base_url_refuses() {
  setup
  cat >"${WORK}/badurl.json" <<'EOF'
{
  "id": "bad",
  "brand": { "public_base_url": "not-a-url" }
}
EOF
  local out="${WORK}/out.cgi"
  if python3 "${PATCHER}" \
    --source "${UPSTREAM}" \
    --output "${out}" \
    --brand-profile "${WORK}/badurl.json" >/dev/null 2>"${WORK}/err"; then
    fail invalid_url "expected failure"; return
  fi
  [[ ! -f "${out}" ]] || { fail invalid_url "output created"; return; }
  grep -qi 'public_base_url' "${WORK}/err" || { fail invalid_url "error weak"; return; }
  pass invalid_public_base_url
}

# --- deploy script local/mocked tests ---

write_fake_remote_host() {
  # Creates a fake PATH with ssh/scp that operate on ${WORK}/remote_fs
  local bin="${WORK}/bin"
  mkdir -p "${bin}" "${WORK}/remote_fs/opt/shm/pay_systems" "${WORK}/remote_fs/opt/bot"
  cp "${UPSTREAM}" "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  chmod 0755 "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  chown "$(id -u):$(id -g)" "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi" 2>/dev/null || true

  # Minimal brand runtime config for probe api.base_url resolution.
  cat >"${WORK}/remote_fs/opt/bot/config-vff.json" <<EOF
{
  "api": { "base_url": "http://127.0.0.1:${PROBE_PORT}" },
  "brand": {
    "id": "vff",
    "public_base_url": "https://connect.vpn-for-friends.com",
    "yookassa_pay_system": "yookassa"
  }
}
EOF

  cat >"${bin}/ssh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
# Drop ssh options/user@host; run remote command locally against fake FS.
while [[ \$# -gt 0 ]]; do
  case "\$1" in
    -o|-p) shift 2 || true ;;
    -*) shift ;;
    *@*) shift; break ;;
    *) break ;;
  esac
done
export FAKE_ROOT="${WORK}/remote_fs"
export PATH="${bin}:\${PATH}"
cmd="\$*"
# Map SHM host paths and container CGI paths into the fake filesystem.
cmd="\${cmd//\\/opt\\//${WORK}/remote_fs/opt/}"
cmd="\${cmd//\\/app\\/data\\/pay_systems\\//${WORK}/remote_fs/opt/shm/pay_systems/}"
bash -c "\$cmd"
EOF
  chmod 0755 "${bin}/ssh"

  cat >"${bin}/scp" <<EOF
#!/usr/bin/env bash
set -euo pipefail
# scp [-q] [-o ...] src dest
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
if [[ "\$src" == *:* ]]; then
  # download remote:local
  rpath="\${src#*:}"
  cp "\${fake}\${rpath}" "\${dest}"
elif [[ "\$dest" == *:* ]]; then
  rpath="\${dest#*:}"
  mkdir -p "\$(dirname "\${fake}\${rpath}")"
  cp "\${src}" "\${fake}\${rpath}"
else
  cp "\${src}" "\${dest}"
fi
EOF
  chmod 0755 "${bin}/scp"

  cat >"${bin}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
# docker compose exec -T core perl -c <path>
if [[ "${1:-}" == "compose" && "${2:-}" == "exec" ]]; then
  shift 2
  [[ "${1:-}" == "-T" ]] && shift
  service="${1:-}"; shift || true
  if [[ "${1:-}" == "perl" && "${2:-}" == "-c" ]]; then
    cpath="${3:-}"
    if [[ ! -f "${cpath}" ]]; then
      echo "docker-mock: missing ${cpath}" >&2
      exit 1
    fi
    # Lightweight stand-in for perl -c: file must be non-empty and look like CGI.
    if ! grep -q 'ps_config' "${cpath}"; then
      echo "docker-mock: perl -c failed (does not look like yookassa.cgi)" >&2
      exit 1
    fi
    echo "${cpath} syntax OK"
    exit 0
  fi
fi
echo "docker-mock: unsupported: $*" >&2
exit 99
EOF
  chmod 0755 "${bin}/docker"

  export PATH="${bin}:${PATH}"
  export FAKE_ROOT="${WORK}/remote_fs"
}

start_probe_server() {
  PROBE_PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
  cat >"${WORK}/probe_server.py" <<'PY'
#!/usr/bin/env python3
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse

PORT = int(os.environ["PORT"])

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        p = urlparse(self.path)
        if p.path != "/shm/pay_systems/yookassa.cgi":
            self.send_response(404); self.end_headers(); return
        qs = parse_qs(p.query)
        brand = qs.get("brand_id", [None])[0]
        if brand is None or brand == "":
            body = b"Error: unknown user\n"
        elif brand in ("vff", "fc"):
            body = b"Error: unknown user\n"
        else:
            body = b"Error: unknown brand_id\n"
        self.send_response(400)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)
    def log_message(self, *args):
        return

HTTPServer(("127.0.0.1", PORT), H).serve_forever()
PY
  PORT="${PROBE_PORT}" python3 "${WORK}/probe_server.py" &
  PROBE_PID=$!
  for _ in $(seq 1 50); do
    if curl -sS --max-time 1 "http://127.0.0.1:${PROBE_PORT}/shm/pay_systems/yookassa.cgi?action=create&user_id=-1&amount=1&ps=yookassa" >/dev/null 2>&1; then
      break
    fi
    sleep 0.05
  done
}

stop_probe_server() {
  if [[ -n "${PROBE_PID:-}" ]] && kill -0 "${PROBE_PID}" 2>/dev/null; then
    kill "${PROBE_PID}" 2>/dev/null || true
    wait "${PROBE_PID}" 2>/dev/null || true
  fi
  PROBE_PID=""
}

test_deploy_check_mocked() {
  setup
  start_probe_server
  write_fake_remote_host
  # Point brand_profile SSH fetch at fake host via SHM_* and by loading vff
  # which uses fr-mrs-1 — our ssh mock ignores host.
  export SHM_HOST=fake-shm
  export SHM_USER=root
  export SHM_DIR=/opt/shm
  export SHM_YK_PROBE_API_BASE="http://127.0.0.1:${PROBE_PORT}"
  if ! bash "${DEPLOY}" check >"${WORK}/out" 2>"${WORK}/err"; then
    stop_probe_server
    fail deploy_check "check failed: $(cat "${WORK}/err")"; return
  fi
  grep -q 'check OK' "${WORK}/out" || { stop_probe_server; fail deploy_check "no OK"; return; }
  # Must not dump api_key lines.
  if grep -qi 'api_key' "${WORK}/out" "${WORK}/err"; then
    stop_probe_server
    fail deploy_check "api_key leaked"; return
  fi
  stop_probe_server
  pass deploy_check_mocked
}

test_deploy_install_mocked() {
  setup
  start_probe_server
  write_fake_remote_host
  export SHM_HOST=fake-shm
  export SHM_USER=root
  export SHM_DIR=/opt/shm
  export SHM_YK_PROBE_API_BASE="http://127.0.0.1:${PROBE_PORT}"
  export SHM_YK_PROBE_PAY_SYSTEM=yookassa
  if ! bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_probe_server
    fail deploy_install "deploy failed: $(cat "${WORK}/err")"; return
  fi
  local installed="${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  grep -Fq 'VPNBOT_BRAND_ROUTING_VERSION=1' "${installed}" \
    || { stop_probe_server; fail deploy_install "marker missing on installed CGI"; return; }
  local bak
  bak="$(ls -1 "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi.bak."* 2>/dev/null | head -1 || true)"
  [[ -n "${bak}" ]] || { stop_probe_server; fail deploy_install "no backup"; return; }
  stop_probe_server
  pass deploy_install_mocked
}

test_deploy_rollback_mocked() {
  setup
  write_fake_remote_host
  export SHM_HOST=fake-shm
  export SHM_USER=root
  export SHM_DIR=/opt/shm
  local cgi="${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  local bak="${cgi}.bak.20260101T000000Z"
  echo 'BACKUP_CONTENT' >"${bak}"
  echo 'CURRENT' >"${cgi}"
  if ! BACKUP="/opt/shm/pay_systems/yookassa.cgi.bak.20260101T000000Z" \
    bash "${DEPLOY}" rollback >"${WORK}/out" 2>"${WORK}/err"; then
    fail deploy_rollback "rollback failed: $(cat "${WORK}/err")"; return
  fi
  grep -Fq 'BACKUP_CONTENT' "${cgi}" || { fail deploy_rollback "content not restored"; return; }
  pass deploy_rollback_mocked
}

test_probe_helpers() {
  setup
  start_probe_server
  # shellcheck source=../lib/shm_yookassa_probes.sh
  source "${ROOT}/scripts/lib/shm_yookassa_probes.sh"
  if ! shm_yookassa_run_brand_routing_probes "http://127.0.0.1:${PROBE_PORT}" "yookassa" >"${WORK}/out" 2>"${WORK}/err"; then
    stop_probe_server
    fail probes "probe suite failed: $(cat "${WORK}/err")"; return
  fi
  stop_probe_server
  pass probe_helpers
}

# --- run ---

test_inserts_vff_fc_mapping_from_profiles
test_trailing_slash_normalized
test_brand_validation_before_user_lookup
test_unknown_brand_not_masked_by_user
test_return_url_override_applied
test_legacy_route_preserved
test_idempotent_reapply
test_missing_anchor_refuses
test_duplicated_anchor_refuses
test_other_version_marker_refuses
test_credentials_callback_metadata_unchanged
test_invalid_profile_refuses
test_invalid_public_base_url_refuses
test_probe_helpers
test_deploy_check_mocked
test_deploy_install_mocked
test_deploy_rollback_mocked

if [[ "${FAILS}" -ne 0 ]]; then
  echo "FAILED ${FAILS} shm_yookassa_patcher tests" >&2
  exit 1
fi
echo "OK shm_yookassa_patcher_test"
