#!/usr/bin/env bash
# Isolated tests for scripts/lib/shm_yookassa_probes.sh (mock HTTP server).
# Does not touch production SHM.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/shm_yookassa_probes.sh
source "${ROOT}/scripts/lib/shm_yookassa_probes.sh"

VFF_PROFILE="${ROOT}/deploy/brands/vff.json"
FC_PROFILE="${ROOT}/deploy/brands/fc.json"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

WORK=""
MOCK_PID=""
MOCK_PORT=""
MODE="ok"
cleanup() {
  if [[ -n "${MOCK_PID}" ]] && kill -0 "${MOCK_PID}" 2>/dev/null; then
    kill "${MOCK_PID}" 2>/dev/null || true
    wait "${MOCK_PID}" 2>/dev/null || true
  fi
  MOCK_PID=""
  if [[ -n "${WORK}" && -d "${WORK}" ]]; then
    rm -rf "${WORK}"
  fi
}
trap cleanup EXIT

vff_url() {
  shm_yookassa_return_url_from_profile "${VFF_PROFILE}"
}
fc_url() {
  shm_yookassa_return_url_from_profile "${FC_PROFILE}"
}

start_mock() {
  MODE="${1:-ok}"
  WORK="$(mktemp -d)"
  chmod 0700 "${WORK}"
  local vff fc
  vff="$(vff_url)"
  fc="$(fc_url)"
  MOCK_PORT="$(python3 - <<'PY'
import socket
s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()
PY
)"
  cat >"${WORK}/server.py" <<PY
#!/usr/bin/env python3
import json, os, time
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse

MODE = os.environ.get("SMOKE_YK_MODE", "ok")
PORT = int(os.environ["PORT"])
VFF = ${vff@Q}
FC = ${fc@Q}
ROUTES = {"vff": VFF, "fc": FC}

class H(BaseHTTPRequestHandler):
    def do_GET(self):
        p = urlparse(self.path)
        if p.path == "/ready":
            self.send_response(200); self.end_headers(); self.wfile.write(b"ok"); return
        if p.path != "/shm/pay_systems/yookassa.cgi":
            self.send_response(404); self.end_headers(); return
        qs = parse_qs(p.query)
        action = qs.get("action", [""])[0]
        brand = qs.get("brand_id", [None])[0]

        if MODE == "hang":
            time.sleep(30)

        if action == "vpnbot_route_check":
            if MODE == "route_wrong_url":
                body = json.dumps({
                    "status": 200, "brand_id": brand or "vff",
                    "return_url": "https://evil.example/payment/return",
                }).encode()
                self.send_response(200)
            elif MODE == "route_incomplete_json":
                body = b'{"status":200,"brand_id":"vff"}'
                self.send_response(200)
            elif MODE == "route_400_on_success":
                body = b"Error: unknown brand_id\\n"
                self.send_response(400)
            elif MODE == "route_accept_invalid":
                body = json.dumps({
                    "status": 200, "brand_id": "not-a-brand",
                    "return_url": VFF,
                }).encode()
                self.send_response(200)
            elif MODE == "ok":
                if brand in ROUTES:
                    body = json.dumps({
                        "status": 200,
                        "brand_id": brand,
                        "return_url": ROUTES[brand],
                    }).encode()
                    self.send_response(200)
                else:
                    body = b"Error: unknown brand_id\\n"
                    self.send_response(400)
            else:
                body = b"Error: unknown brand_id\\n"
                self.send_response(400)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers(); self.wfile.write(body); return

        # create probes
        if brand is None or brand == "":
            body = b"Error: unknown user\\n"
        elif brand in ("vff", "fc"):
            body = b"Error: unknown user\\n"
        else:
            body = b"Error: unknown brand_id\\n"
        self.send_response(400)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers(); self.wfile.write(body)

    def log_message(self, *a):
        return

HTTPServer(("127.0.0.1", PORT), H).serve_forever()
PY
  SMOKE_YK_MODE="${MODE}" PORT="${MOCK_PORT}" python3 "${WORK}/server.py" &
  MOCK_PID=$!
  for _ in $(seq 1 50); do
    curl -sS --max-time 1 "http://127.0.0.1:${MOCK_PORT}/ready" >/dev/null 2>&1 && return 0
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
  rm -rf "${WORK}"
  WORK=""
}

base() { echo "http://127.0.0.1:${MOCK_PORT}"; }

test_route_check_vff_ok() {
  start_mock ok
  if ! shm_yookassa_probe_route_check "$(base)" yookassa vff ok "$(vff_url)"; then
    stop_mock; fail route_vff "failed"; return
  fi
  stop_mock; pass route_check_vff_ok
}

test_route_check_fc_ok() {
  start_mock ok
  if ! shm_yookassa_probe_route_check "$(base)" yookassa fc ok "$(fc_url)"; then
    stop_mock; fail route_fc "failed"; return
  fi
  stop_mock; pass route_check_fc_ok
}

test_route_check_wrong_url_fails() {
  start_mock route_wrong_url
  if shm_yookassa_probe_route_check "$(base)" yookassa vff ok "$(vff_url)" >/dev/null 2>&1; then
    stop_mock; fail wrong_url "accepted wrong URL"; return
  fi
  stop_mock; pass route_check_wrong_url_fails
}

test_route_check_incomplete_json_fails() {
  start_mock route_incomplete_json
  if shm_yookassa_probe_route_check "$(base)" yookassa vff ok "$(vff_url)" >/dev/null 2>&1; then
    stop_mock; fail incomplete "accepted incomplete JSON"; return
  fi
  stop_mock; pass route_check_incomplete_json_fails
}

test_route_check_http400_on_success_fails() {
  start_mock route_400_on_success
  if shm_yookassa_probe_route_check "$(base)" yookassa vff ok "$(vff_url)" >/dev/null 2>&1; then
    stop_mock; fail http400 "accepted 400 as success"; return
  fi
  stop_mock; pass route_check_http400_on_success_fails
}

test_route_check_invalid_accepted_fails() {
  start_mock route_accept_invalid
  # Probe expects reject for invalid brand; server wrongly returns 200.
  if shm_yookassa_probe_route_check "$(base)" yookassa not-a-brand reject >/dev/null 2>&1; then
    stop_mock; fail invalid_accepted "reject mode passed on 200"; return
  fi
  stop_mock; pass route_check_invalid_accepted_fails
}

test_route_check_timeout_fails() {
  start_mock hang
  SMOKE_YOOKASSA_CGI_MAX_TIME=1 SHM_YK_CGI_MAX_TIME=1 \
    shm_yookassa_probe_route_check "$(base)" yookassa vff ok "$(vff_url)" >/dev/null 2>&1 \
    && { stop_mock; fail timeout "hang accepted"; return; }
  stop_mock; pass route_check_timeout_fails
}

test_create_probes_still_work() {
  start_mock ok
  if ! shm_yookassa_run_create_probes "$(base)" yookassa >/dev/null 2>&1; then
    stop_mock; fail create_probes "create suite failed"; return
  fi
  stop_mock; pass create_probes_still_work
}

test_full_suite_ok() {
  start_mock ok
  if ! shm_yookassa_run_brand_routing_probes "$(base)" yookassa >/dev/null 2>&1; then
    stop_mock; fail full_suite "failed"; return
  fi
  stop_mock; pass full_suite_ok
}

test_expected_urls_from_profiles() {
  local v ff
  v="$(vff_url)"; ff="$(fc_url)"
  [[ "${v}" == https://connect.vpn-for-friends.com/payment/return ]] || {
    fail urls_from_profiles "vff=${v}"; return
  }
  [[ "${ff}" == https://connect.friends-connect.club/payment/return ]] || {
    fail urls_from_profiles "fc=${ff}"; return
  }
  pass expected_urls_from_profiles
}

test_route_check_vff_ok
test_route_check_fc_ok
test_route_check_wrong_url_fails
test_route_check_incomplete_json_fails
test_route_check_http400_on_success_fails
test_route_check_invalid_accepted_fails
test_route_check_timeout_fails
test_create_probes_still_work
test_full_suite_ok
test_expected_urls_from_profiles

if [[ "${FAILS}" -ne 0 ]]; then
  echo "FAILED ${FAILS} shm_yookassa_probes tests" >&2
  exit 1
fi
echo "OK shm_yookassa_probes_test"
