#!/usr/bin/env bash
# Isolated tests for scripts/lib/smoke_yookassa_cgi.sh (mock HTTP server).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/smoke_yookassa_cgi.sh
source "${ROOT}/scripts/lib/smoke_yookassa_cgi.sh"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

MOCK_DIR=""
MOCK_PID=""
MOCK_PORT=""

cleanup() {
  if [[ -n "${MOCK_PID}" ]] && kill -0 "${MOCK_PID}" 2>/dev/null; then
    kill "${MOCK_PID}" 2>/dev/null || true
    wait "${MOCK_PID}" 2>/dev/null || true
  fi
  MOCK_PID=""
  if [[ -n "${MOCK_DIR}" && -d "${MOCK_DIR}" ]]; then
    rm -rf "${MOCK_DIR}"
  fi
  MOCK_DIR=""
}
trap cleanup EXIT

write_mock_server() {
  cat >"${MOCK_DIR}/server.py" <<'PY'
#!/usr/bin/env python3
import json
import os
from http.server import BaseHTTPRequestHandler, HTTPServer
from urllib.parse import parse_qs, urlparse

MODE = os.environ.get("SMOKE_YK_MODE", "ok400")
PORT = int(os.environ["PORT"])


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        parsed = urlparse(self.path)
        if parsed.path == "/ready":
            self.send_response(200)
            self.send_header("Content-Length", "2")
            self.end_headers()
            self.wfile.write(b"ok")
            return
        if parsed.path != "/shm/pay_systems/yookassa.cgi":
            self.send_response(404)
            self.end_headers()
            self.wfile.write(b"not found")
            return
        qs = parse_qs(parsed.query)
        with open(os.environ["OBSERVED_QUERY"], "w", encoding="utf-8") as f:
            json.dump(
                {
                    "host": self.headers.get("Host", ""),
                    "path": parsed.path,
                    "query": {k: v[0] if len(v) == 1 else v for k, v in qs.items()},
                },
                f,
            )

        if MODE == "hang":
            import time
            time.sleep(30)

        if MODE == "ok400":
            status, body = 400, b"Error: unknown user\n"
        elif MODE == "ok400json":
            status, body = 400, b'{"error":"unknown user"}\n'
        elif MODE == "502":
            status, body = 502, b"Empty response from script\n"
        elif MODE == "200":
            status, body = 200, b"payment_created"
        elif MODE == "400other":
            status, body = 400, b"Error: invalid amount\n"
        elif MODE == "empty400":
            status, body = 400, b""
        elif MODE == "hang":
            status, body = 400, b"Error: unknown user\n"
        else:
            status, body = 500, b"unexpected mode"

        self.send_response(status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if body:
            self.wfile.write(body)

    def log_message(self, *_args):
        return


HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
PY
  chmod 0700 "${MOCK_DIR}/server.py"
}

start_mock() {
  local mode="$1"
  cleanup
  MOCK_DIR="$(mktemp -d)"
  write_mock_server
  MOCK_PORT="$(python3 - <<'PY'
import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()
PY
)"
  local observed="${MOCK_DIR}/observed.json"
  : >"${observed}"
  PORT="${MOCK_PORT}" SMOKE_YK_MODE="${mode}" OBSERVED_QUERY="${observed}" \
    python3 "${MOCK_DIR}/server.py" &
  MOCK_PID=$!
  local i ready=0
  for i in $(seq 1 50); do
    if curl -sS --max-time 1 "http://127.0.0.1:${MOCK_PORT}/ready" -o /dev/null 2>/dev/null; then
      ready=1
      break
    fi
    if ! kill -0 "${MOCK_PID}" 2>/dev/null; then
      fail mock_start "server died"
      return 1
    fi
    sleep 0.05
  done
  if [[ "${ready}" -ne 1 ]]; then
    fail mock_start "server not ready on port ${MOCK_PORT}"
    return 1
  fi
}

write_probe_config() {
  local api_base="$1"
  local ps="${2:-yookassa}"
  local public_base="${3:-https://connect.brand.example}"
  local path="${MOCK_DIR}/probe-config.json"
  cat >"${path}" <<EOF
{
  "api": {"base_url": "${api_base}", "api_login": "secret-login", "api_pass": "secret-pass"},
  "brand": {
    "id": "vff",
    "name": "VPN for Friends",
    "public_base_url": "${public_base}",
    "yookassa_pay_system": "${ps}"
  }
}
EOF
  printf '%s\n' "${path}"
}

assert_query() {
  local want_ps="${1:-yookassa_probe}"
  python3 - <<PY
import json
obs = json.load(open("${MOCK_DIR}/observed.json"))
q = obs["query"]
assert q.get("action") == "create", obs
assert q.get("user_id") == "-1", obs
assert q.get("amount") == "1", obs
assert q.get("ps") == "${want_ps}", obs
assert obs.get("path") == "/shm/pay_systems/yookassa.cgi", obs
print("ok")
PY
}

run_check() {
  SMOKE_YOOKASSA_LABEL="test" SMOKE_YOOKASSA_CGI_MAX_TIME="${1:-5}" \
    smoke_yookassa_cgi_check "http://127.0.0.1:${MOCK_PORT}" "yookassa_probe"
}

test_ok_400_unknown_user() {
  start_mock ok400 || return
  local out rc=0
  out="$(run_check 5 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail ok400 "rc=${rc} ${out}"; return; fi
  if ! grep -Fq 'controlled unknown-user rejection' <<<"${out}"; then
    fail ok400 "msg: ${out}"; return
  fi
  assert_query yookassa_probe >/dev/null || { fail ok400 "query"; return; }
  pass ok400_unknown_user
}

test_ok_400_json() {
  start_mock ok400json || return
  local out rc=0
  out="$(run_check 5 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail ok400json "rc=${rc} ${out}"; return; fi
  pass ok400_json_unknown_user
}

test_fail_502_empty_response() {
  start_mock 502 || return
  local out rc=0
  out="$(run_check 5 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail fail502 "should fail"; return; fi
  if ! grep -Eiq 'unexpected HTTP 502|Empty response' <<<"${out}"; then
    fail fail502 "msg: ${out}"; return
  fi
  pass fail_502_empty_response
}

test_fail_http_200() {
  start_mock 200 || return
  local out rc=0
  out="$(run_check 5 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail fail200 "should fail"; return; fi
  if ! grep -Fq 'unexpected success HTTP 200' <<<"${out}"; then
    fail fail200 "msg: ${out}"; return
  fi
  pass fail_http_200
}

test_fail_400_other_message() {
  start_mock 400other || return
  local out rc=0
  out="$(run_check 5 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail fail400other "should fail"; return; fi
  if ! grep -Fq "without expected 'unknown user'" <<<"${out}"; then
    fail fail400other "msg: ${out}"; return
  fi
  pass fail_400_other_message
}

test_fail_timeout_or_connection() {
  cleanup
  local out rc=0
  out="$(SMOKE_YOOKASSA_LABEL=test SMOKE_YOOKASSA_CGI_MAX_TIME=2 \
    smoke_yookassa_cgi_check "http://127.0.0.1:1" "yookassa_probe" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail conn "should fail"; return; fi
  if ! grep -Fq 'transport/timeout error' <<<"${out}"; then
    fail conn "msg: ${out}"; return
  fi

  start_mock hang || return
  out=""; rc=0
  out="$(run_check 1 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail timeout "should fail"; return; fi
  if ! grep -Fq 'transport/timeout error' <<<"${out}"; then
    fail timeout "msg: ${out}"; return
  fi
  pass fail_timeout_or_connection
}

test_fail_empty_pay_system() {
  local out rc=0
  out="$(SMOKE_YOOKASSA_LABEL=test smoke_yookassa_cgi_check "http://127.0.0.1:9" "" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail empty_ps "should fail"; return; fi
  if ! grep -Fq 'invalid or empty yookassa_pay_system' <<<"${out}"; then
    fail empty_ps "msg: ${out}"; return
  fi
  pass fail_empty_pay_system
}

test_cgi_uses_api_base_not_public_url() {
  start_mock ok400 || return
  local cfg out rc=0
  cfg="$(write_probe_config "http://127.0.0.1:${MOCK_PORT}" "yookassa" "https://connect.brand.example")"
  out="$(SMOKE_YOOKASSA_LABEL=test SMOKE_YOOKASSA_CGI_MAX_TIME=5 \
    smoke_yookassa_cgi_check_from_config "${cfg}" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail api_base "rc=${rc} ${out}"; return; fi
  if ! grep -Fq 'using api.base_url from config' <<<"${out}"; then
    fail api_base "missing api.base_url note: ${out}"; return
  fi
  if grep -Fq 'connect.brand.example' <<<"${out}"; then
    fail api_base "must not mention public brand URL: ${out}"; return
  fi
  python3 - <<PY
import json
obs = json.load(open("${MOCK_DIR}/observed.json"))
assert obs["host"].startswith("127.0.0.1:"), obs
assert "connect.brand.example" not in obs["host"], obs
assert obs["query"].get("ps") == "yookassa", obs
print("ok")
PY
  pass cgi_uses_api_base_not_public_url
}

test_read_config_extracts_api_and_ps() {
  MOCK_DIR="$(mktemp -d)"
  local cfg pair api_base ps
  cfg="$(write_probe_config "https://shm.example.com" "yookassa" "https://connect.brand.example")"
  pair="$(SMOKE_YOOKASSA_LABEL=test smoke_yookassa_cgi_read_config "${cfg}")" || {
    fail read_cfg "read failed"; return
  }
  api_base="${pair%%$'\t'*}"
  ps="${pair#*$'\t'}"
  if [[ "${api_base}" != "https://shm.example.com" ]]; then
    fail read_cfg "api_base=${api_base}"; return
  fi
  if [[ "${ps}" != "yookassa" ]]; then
    fail read_cfg "ps=${ps}"; return
  fi
  if grep -Eiq 'secret-login|secret-pass' <<<"${pair}"; then
    fail read_cfg "leaked secrets"; return
  fi
  pass read_config_extracts_api_and_ps
}

test_read_config_rejects_equal_public_url() {
  MOCK_DIR="$(mktemp -d)"
  local cfg out rc=0
  cfg="$(write_probe_config "https://connect.brand.example" "yookassa" "https://connect.brand.example")"
  out="$(SMOKE_YOOKASSA_LABEL=test smoke_yookassa_cgi_read_config "${cfg}" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail equal_url "should fail"; return; fi
  if ! grep -Fq 'must not equal brand.public_base_url' <<<"${out}"; then
    fail equal_url "msg: ${out}"; return
  fi
  pass read_config_rejects_equal_public_url
}

test_read_config_requires_api_base() {
  MOCK_DIR="$(mktemp -d)"
  local cfg out rc=0
  cfg="${MOCK_DIR}/no-api.json"
  cat >"${cfg}" <<'EOF'
{"brand":{"public_base_url":"https://connect.brand.example","yookassa_pay_system":"yookassa"}}
EOF
  out="$(SMOKE_YOOKASSA_LABEL=test smoke_yookassa_cgi_read_config "${cfg}" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail no_api "should fail"; return; fi
  if ! grep -Fq 'api.base_url' <<<"${out}"; then
    fail no_api "msg: ${out}"; return
  fi
  pass read_config_requires_api_base
}

test_ok_400_unknown_user
test_ok_400_json
test_fail_502_empty_response
test_fail_http_200
test_fail_400_other_message
test_fail_timeout_or_connection
test_fail_empty_pay_system
test_cgi_uses_api_base_not_public_url
test_read_config_extracts_api_and_ps
test_read_config_rejects_equal_public_url
test_read_config_requires_api_base

if [[ "${FAILS}" -ne 0 ]]; then
  echo "smoke_yookassa_cgi_test: ${FAILS} failed" >&2
  exit 1
fi
echo "smoke_yookassa_cgi_test: all passed"
