#!/usr/bin/env bash
# Isolated tests for deploy/shm/yookassa patcher + deploy-shm-yookassa.sh logic.
# Does not touch production SHM.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
PATCHER="${ROOT}/deploy/shm/yookassa/patch_yookassa.py"
UPSTREAM="${ROOT}/deploy/shm/yookassa/testdata/yookassa.cgi.upstream"
V1_FIXTURE="${ROOT}/deploy/shm/yookassa/testdata/yookassa.cgi.v1"
DEPLOY="${ROOT}/scripts/deploy-shm-yookassa.sh"
VFF_PROFILE="${ROOT}/deploy/brands/vff.json"
FC_PROFILE="${ROOT}/deploy/brands/fc.json"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

WORK=""
PROBE_PID=""
cleanup() {
  if [[ -n "${PROBE_PID}" ]] && kill -0 "${PROBE_PID}" 2>/dev/null; then
    kill "${PROBE_PID}" 2>/dev/null || true
    wait "${PROBE_PID}" 2>/dev/null || true
  fi
  PROBE_PID=""
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
print(p["brand"]["public_base_url"].rstrip("/") + "/payment/return")
PY
}

fc_return_url() {
  python3 - <<'PY' "${FC_PROFILE}"
import json, sys
p = json.load(open(sys.argv[1], encoding="utf-8"))
print(p["brand"]["public_base_url"].rstrip("/") + "/payment/return")
PY
}

run_patch() {
  local src="$1"
  local out="$2"
  python3 "${PATCHER}" \
    --source "${src}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}"
}

# --- patcher ---

test_upstream_to_v2() {
  setup
  local out="${WORK}/out.cgi"
  if ! run_patch "${UPSTREAM}" "${out}" >/dev/null; then
    fail upstream_v2 "patcher failed"; return
  fi
  grep -Fq "VPNBOT_BRAND_ROUTING_VERSION=2" "${out}" || { fail upstream_v2 "VERSION=2 missing"; return; }
  grep -Fq "vpnbot_route_check" "${out}" || { fail upstream_v2 "route_check missing"; return; }
  grep -Fq "BEGIN VPNBOT_BRAND_SHARED" "${out}" || { fail upstream_v2 "SHARED missing"; return; }
  grep -Fq "BEGIN VPNBOT_BRAND_CREATE_VALIDATE" "${out}" || { fail upstream_v2 "CREATE_VALIDATE missing"; return; }
  local n
  n="$(grep -c 'my %vpnbot_brand_return_urls' "${out}" || true)"
  [[ "${n}" -eq 1 ]] || { fail upstream_v2 "allowlist count=${n}"; return; }
  pass upstream_to_v2
}

test_v1_upgrades_to_v2() {
  setup
  [[ -f "${V1_FIXTURE}" ]] || { fail v1_upgrade "missing v1 fixture"; return; }
  local out="${WORK}/out.cgi"
  local status
  status="$(run_patch "${V1_FIXTURE}" "${out}" 2>&1)" || { fail v1_upgrade "patch failed: ${status}"; return; }
  grep -Fq 'upgraded: VERSION=1' <<<"${status}" || { fail v1_upgrade "status: ${status}"; return; }
  grep -Fq "VPNBOT_BRAND_ROUTING_VERSION=2" "${out}" || { fail v1_upgrade "not v2"; return; }
  grep -Fq "vpnbot_route_check" "${out}" || { fail v1_upgrade "no route_check"; return; }
  ! grep -Fq "VPNBOT_BRAND_ROUTING_VERSION=1" "${out}" || { fail v1_upgrade "v1 marker remains"; return; }
  ! grep -Fq "BEGIN VPNBOT_BRAND_ROUTING$" "${out}" || { fail v1_upgrade "v1 ROUTING remains"; return; }
  pass v1_upgrades_to_v2
}

test_v2_idempotent() {
  setup
  local once="${WORK}/once.cgi" twice="${WORK}/twice.cgi"
  run_patch "${UPSTREAM}" "${once}" >/dev/null
  run_patch "${once}" "${twice}" >/dev/null
  if ! cmp -s "${once}" "${twice}"; then
    fail v2_idempotent "byte content changed"; return
  fi
  local count
  count="$(grep -c 'VPNBOT_BRAND_ROUTING_VERSION=2' "${twice}" || true)"
  [[ "${count}" -eq 3 ]] || { fail v2_idempotent "marker count=${count} want 3"; return; }
  pass v2_idempotent
}

test_unknown_version_refuses() {
  setup
  local once="${WORK}/once.cgi" out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${once}" >/dev/null
  for ver in 3 99; do
    sed "s/VPNBOT_BRAND_ROUTING_VERSION=2/VPNBOT_BRAND_ROUTING_VERSION=${ver}/g" "${once}" >"${WORK}/bad${ver}.cgi"
    if python3 "${PATCHER}" \
      --source "${WORK}/bad${ver}.cgi" \
      --output "${out}" \
      --brand-profile "${VFF_PROFILE}" \
      --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
      fail unknown_ver "VERSION=${ver} accepted"; return
    fi
    grep -qi 'version' "${WORK}/err" || { fail unknown_ver "weak error for ${ver}"; return; }
    rm -f "${out}"
  done
  pass unknown_version_refuses
}

test_damaged_v1_refuses() {
  setup
  local bad="${WORK}/bad.cgi" out="${WORK}/out.cgi"
  # Remove APPLY block from complete V1.
  sed '/BEGIN VPNBOT_BRAND_RETURN_APPLY/,/END VPNBOT_BRAND_RETURN_APPLY/d' "${V1_FIXTURE}" >"${bad}"
  if python3 "${PATCHER}" \
    --source "${bad}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail damaged_v1 "accepted damaged V1"; return
  fi
  grep -qiE 'missing|incomplete|not unique' "${WORK}/err" || { fail damaged_v1 "weak err"; return; }
  [[ ! -f "${out}" ]] || { fail damaged_v1 "output created"; return; }
  pass damaged_v1_refuses
}

test_duplicated_markers_refuse() {
  setup
  local once="${WORK}/once.cgi" bad="${WORK}/bad.cgi" out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${once}" >/dev/null
  {
    cat "${once}"
    grep -A20 'BEGIN VPNBOT_BRAND_SHARED' "${once}" | head -5
    echo '    # BEGIN VPNBOT_BRAND_SHARED'
    echo '    # END VPNBOT_BRAND_SHARED'
  } >"${bad}"
  if python3 "${PATCHER}" \
    --source "${bad}" \
    --output "${out}" \
    --brand-profile "${VFF_PROFILE}" \
    --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail dup_markers "accepted duplicates"; return
  fi
  pass duplicated_markers_refuse
}

test_single_shared_mapping() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  local vff_url fc_url n
  vff_url="$(vff_return_url)"
  fc_url="$(fc_return_url)"
  n="$(grep -c 'my %vpnbot_brand_return_urls' "${out}" || true)"
  [[ "${n}" -eq 1 ]] || { fail shared_map "allowlist count=${n}"; return; }
  grep -Fq "'vff' => '${vff_url}'" "${out}" || { fail shared_map "vff missing"; return; }
  grep -Fq "'fc' => '${fc_url}'" "${out}" || { fail shared_map "fc missing"; return; }
  # Mapping appears only in SHARED; create validate references the hash, does not redefine it.
  python3 - <<'PY' "${out}"
from pathlib import Path
import sys
text = Path(sys.argv[1]).read_text()
shared = text.split("BEGIN VPNBOT_BRAND_SHARED", 1)[1].split("END VPNBOT_BRAND_SHARED", 1)[0]
create = text.split("BEGIN VPNBOT_BRAND_CREATE_VALIDATE", 1)[1].split("END VPNBOT_BRAND_CREATE_VALIDATE", 1)[0]
assert "my %vpnbot_brand_return_urls" in shared
assert "my %vpnbot_brand_return_urls" not in create
assert "vpnbot_brand_return_urls{" in create
assert "vpnbot_route_check" in shared
print("ok")
PY
  pass single_shared_mapping
}

test_route_check_before_side_effects() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  python3 - <<'PY' "${out}"
from pathlib import Path
import sys
text = Path(sys.argv[1]).read_text()
route = text.find("vpnbot_route_check")
create = text.find("if ( $vars{action} eq 'create' || $vars{action} eq 'payment' ) {")
user = text.find("    if ( $vars{user_id} ) {")
api = text.find("api.yookassa.ru")
assert 0 <= route < create < user < api, (route, create, user, api)
shared = text[text.find("BEGIN VPNBOT_BRAND_SHARED"):text.find("END VPNBOT_BRAND_SHARED")]
assert "if ( $vars{user_id} )" not in shared
assert "api.yookassa.ru" not in shared
assert "unknown user" not in shared
print("ok")
PY
  pass route_check_before_side_effects
}

test_route_check_urls_from_profiles() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  local vff_url fc_url
  vff_url="$(vff_return_url)"
  fc_url="$(fc_return_url)"
  grep -Fq "return_url => \$vpnbot_brand_return_urls{\$vpnbot_brand_id}" "${out}" \
    || { fail route_urls "route_check does not return allowlist URL"; return; }
  grep -Fq "'vff' => '${vff_url}'" "${out}" || { fail route_urls "vff"; return; }
  grep -Fq "'fc' => '${fc_url}'" "${out}" || { fail route_urls "fc"; return; }
  pass route_check_urls_from_profiles
}

test_route_check_fail_closed_empty_brand() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  grep -Fq "unless ( defined \$vpnbot_brand_id && length(\$vpnbot_brand_id)" "${out}" \
    || { fail route_fail_closed "empty brand guard missing"; return; }
  # Count unknown brand_id exits: shared route_check + create validate.
  local n
  n="$(grep -c "Error: unknown brand_id" "${out}" || true)"
  [[ "${n}" -ge 2 ]] || { fail route_fail_closed "unknown brand_id paths=${n}"; return; }
  pass route_check_fail_closed_empty_brand
}

test_create_legacy_and_order() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  local brand_line user_line
  brand_line="$(grep -n "BEGIN VPNBOT_BRAND_CREATE_VALIDATE" "${out}" | head -1 | cut -d: -f1)"
  user_line="$(grep -n 'if ( \$vars{user_id} ) {' "${out}" | head -1 | cut -d: -f1)"
  [[ "${brand_line}" -lt "${user_line}" ]] || { fail create_legacy "validate after user"; return; }
  grep -Fq 'my $return_url =     $ps_config{return_url};' "${out}" || { fail create_legacy "legacy missing"; return; }
  grep -Fq '$return_url = $vpnbot_brand_return_url if defined $vpnbot_brand_return_url;' "${out}" \
    || { fail create_legacy "apply missing"; return; }
  pass create_legacy_and_order
}

test_credentials_unchanged() {
  setup
  local out="${WORK}/out.cgi"
  run_patch "${UPSTREAM}" "${out}" >/dev/null
  python3 - <<'PY' "${PATCHER}" "${UPSTREAM}" "${out}"
import importlib.util
from pathlib import Path
import sys
patcher_path, src_path, patched_path = map(Path, sys.argv[1:4])
spec = importlib.util.spec_from_file_location("patch_yookassa", patcher_path)
mod = importlib.util.module_from_spec(spec)
spec.loader.exec_module(mod)
stripped = mod.strip_routing_block(patched_path.read_text(encoding="utf-8"))
assert stripped == src_path.read_text(encoding="utf-8")
PY
  for needle in \
    'my $api_key =        $ps_config{api_key};' \
    'my $account_id =     $ps_config{account_id};' \
    'metadata => {' \
    'return_url => $return_url || '"'"'https://www.example.com'"'"',' \
    'HTTP::Request->new( POST => "https://api.yookassa.ru/v3/payments")'
  do
    local n
    n="$(grep -F -c "${needle}" "${out}" || true)"
    [[ "${n}" -eq 1 ]] || { fail creds "count ${needle}: ${n}"; return; }
  done
  pass credentials_unchanged
}

test_trailing_slash_normalized() {
  setup
  cat >"${WORK}/slash.json" <<'EOF'
{ "id": "slashy", "brand": { "public_base_url": "https://example.test/base/" } }
EOF
  local out="${WORK}/out.cgi"
  python3 "${PATCHER}" --source "${UPSTREAM}" --output "${out}" --brand-profile "${WORK}/slash.json" >/dev/null
  grep -Fq "'slashy' => 'https://example.test/base/payment/return'" "${out}" || { fail slash "bad url"; return; }
  pass trailing_slash_normalized
}

test_missing_upstream_anchor_refuses() {
  setup
  local bad="${WORK}/bad.cgi" out="${WORK}/out.cgi"
  sed '/my \$return_url =     \$ps_config{return_url};/d' "${UPSTREAM}" >"${bad}"
  if python3 "${PATCHER}" --source "${bad}" --output "${out}" \
    --brand-profile "${VFF_PROFILE}" --brand-profile "${FC_PROFILE}" >/dev/null 2>"${WORK}/err"; then
    fail missing_anchor "accepted"; return
  fi
  pass missing_upstream_anchor_refuses
}

# --- deploy mocked ---

write_fake_remote_host() {
  local bin="${WORK}/bin"
  mkdir -p "${bin}" "${WORK}/remote_fs/opt/shm/pay_systems" "${WORK}/remote_fs/opt/bot"
  local src_cgi="${1:-${UPSTREAM}}"
  cp "${src_cgi}" "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  chmod 0755 "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"

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
cmd="\${cmd//\\/opt\\//${WORK}/remote_fs/opt/}"
cmd="\${cmd//\\/app\\/data\\/pay_systems\\//${WORK}/remote_fs/opt/shm/pay_systems/}"
bash -c "\$cmd"
EOF
  chmod 0755 "${bin}/ssh"

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
src="\${args[0]}"; dest="\${args[1]}"; fake="${WORK}/remote_fs"
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

  cat >"${bin}/docker" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
if [[ "${1:-}" == "compose" && "${2:-}" == "exec" ]]; then
  shift 2
  [[ "${1:-}" == "-T" ]] && shift
  shift || true
  if [[ "${1:-}" == "perl" && "${2:-}" == "-c" ]]; then
    cpath="${3:-}"
    [[ -f "${cpath}" ]] || { echo "missing ${cpath}" >&2; exit 1; }
    grep -q 'ps_config' "${cpath}" || exit 1
    echo "${cpath} syntax OK"; exit 0
  fi
fi
exit 99
EOF
  chmod 0755 "${bin}/docker"
  export PATH="${bin}:${PATH}"
}

start_probe_server() {
  local vff_url fc_url
  vff_url="$(vff_return_url)"
  fc_url="$(fc_return_url)"
  PROBE_PORT="$(python3 - <<'PY'
import socket
s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()
PY
)"
  cat >"${WORK}/probe_server.py" <<PY
#!/usr/bin/env python3
import json, os
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse

PORT = int(os.environ["PORT"])
VFF = ${vff_url@Q}
FC = ${fc_url@Q}
ROUTES = {"vff": VFF, "fc": FC}

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        p = urlparse(self.path)
        if p.path != "/shm/pay_systems/yookassa.cgi":
            self.send_response(404); self.end_headers(); return
        qs = parse_qs(p.query)
        action = qs.get("action", [""])[0]
        brand = qs.get("brand_id", [None])[0]
        if action == "vpnbot_route_check":
            if brand in ROUTES:
                body = json.dumps({
                    "status": 200,
                    "brand_id": brand,
                    "return_url": ROUTES[brand],
                }).encode()
                self.send_response(200)
            else:
                body = b"Error: unknown brand_id\n"
                self.send_response(400)
            self.send_header("Content-Type", "application/json" if brand in ROUTES else "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return
        # create
        if brand is None or brand == "":
            body = b"Error: unknown user\n"
        elif brand in ("vff", "fc"):
            body = b"Error: unknown user\n"
        else:
            body = b"Error: unknown brand_id\n"
        self.send_response(400)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers(); self.wfile.write(body)
    def log_message(self, *a):
        return

HTTPServer(("127.0.0.1", PORT), H).serve_forever()
PY
  PORT="${PROBE_PORT}" python3 "${WORK}/probe_server.py" &
  PROBE_PID=$!
  for _ in $(seq 1 50); do
    curl -sS --max-time 1 \
      "http://127.0.0.1:${PROBE_PORT}/shm/pay_systems/yookassa.cgi?action=vpnbot_route_check&brand_id=vff&ps=yookassa" \
      >/dev/null 2>&1 && break
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

test_deploy_check_reports_v1_upgrade() {
  setup
  start_probe_server
  write_fake_remote_host "${V1_FIXTURE}"
  export SHM_HOST=fake-shm SHM_USER=root SHM_DIR=/opt/shm
  export SHM_YK_PROBE_API_BASE="http://127.0.0.1:${PROBE_PORT}"
  if ! bash "${DEPLOY}" check >"${WORK}/out" 2>"${WORK}/err"; then
    stop_probe_server
    fail deploy_v1_check "failed: $(cat "${WORK}/err")"; return
  fi
  grep -q 'VERSION=1 → VERSION=2' "${WORK}/out" || {
    stop_probe_server
    fail deploy_v1_check "upgrade note missing: $(cat "${WORK}/out")"; return
  }
  stop_probe_server
  pass deploy_check_reports_v1_upgrade
}

test_deploy_install_v2() {
  setup
  start_probe_server
  write_fake_remote_host "${UPSTREAM}"
  export SHM_HOST=fake-shm SHM_USER=root SHM_DIR=/opt/shm
  export SHM_YK_PROBE_API_BASE="http://127.0.0.1:${PROBE_PORT}"
  export SHM_YK_PROBE_PAY_SYSTEM=yookassa
  if ! bash "${DEPLOY}" deploy >"${WORK}/out" 2>"${WORK}/err"; then
    stop_probe_server
    fail deploy_install "failed: $(cat "${WORK}/err")"; return
  fi
  local installed="${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  grep -Fq 'VPNBOT_BRAND_ROUTING_VERSION=2' "${installed}" || {
    stop_probe_server; fail deploy_install "VERSION=2 missing"; return
  }
  grep -Fq 'vpnbot_route_check' "${installed}" || {
    stop_probe_server; fail deploy_install "route_check missing"; return
  }
  ls "${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi.bak."* >/dev/null 2>&1 \
    || { stop_probe_server; fail deploy_install "no backup"; return; }
  stop_probe_server
  pass deploy_install_v2
}

test_deploy_rollback_mocked() {
  setup
  write_fake_remote_host
  export SHM_HOST=fake-shm SHM_USER=root SHM_DIR=/opt/shm
  local cgi="${WORK}/remote_fs/opt/shm/pay_systems/yookassa.cgi"
  local bak="${cgi}.bak.20260101T000000Z"
  echo 'BACKUP_CONTENT' >"${bak}"
  echo 'CURRENT' >"${cgi}"
  if ! BACKUP="/opt/shm/pay_systems/yookassa.cgi.bak.20260101T000000Z" \
    bash "${DEPLOY}" rollback >"${WORK}/out" 2>"${WORK}/err"; then
    fail deploy_rollback "failed: $(cat "${WORK}/err")"; return
  fi
  grep -Fq 'BACKUP_CONTENT' "${cgi}" || { fail deploy_rollback "not restored"; return; }
  pass deploy_rollback_mocked
}

# --- run ---

test_upstream_to_v2
test_v1_upgrades_to_v2
test_v2_idempotent
test_unknown_version_refuses
test_damaged_v1_refuses
test_duplicated_markers_refuse
test_single_shared_mapping
test_route_check_before_side_effects
test_route_check_urls_from_profiles
test_route_check_fail_closed_empty_brand
test_create_legacy_and_order
test_credentials_unchanged
test_trailing_slash_normalized
test_missing_upstream_anchor_refuses
test_deploy_check_reports_v1_upgrade
test_deploy_install_v2
test_deploy_rollback_mocked

if [[ "${FAILS}" -ne 0 ]]; then
  echo "FAILED ${FAILS} shm_yookassa_patcher tests" >&2
  exit 1
fi
echo "OK shm_yookassa_patcher_test"
