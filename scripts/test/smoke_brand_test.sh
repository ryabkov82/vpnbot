#!/usr/bin/env bash
# Focused tests for Happ redirect checks in scripts/smoke-brand.sh (mock curl, no network).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

WORK=""
cleanup() {
  if [[ -n "${WORK}" && -d "${WORK}" ]]; then
    rm -rf "${WORK}"
  fi
  WORK=""
}
trap cleanup EXIT

write_mock_curl() {
  cat >"${WORK}/bin/curl" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
MODE="${SMOKE_CURL_MODE:-ok}"
outfile="/dev/null"
write_fmt=""
url=""
args=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    -o)
      outfile="$2"
      shift 2
      ;;
    -w)
      write_fmt="$2"
      shift 2
      ;;
    --max-time|--data-urlencode)
      shift 2
      ;;
    -sS|-s|-S|--get)
      shift
      ;;
    *)
      args+=("$1")
      shift
      ;;
  esac
done
for a in "${args[@]+"${args[@]}"}"; do
  if [[ "$a" == http://* || "$a" == https://* ]]; then
    url="$a"
  fi
done
if [[ -z "${url}" && ${#args[@]} -gt 0 ]]; then
  url="${args[${#args[@]}-1]}"
fi

emit() {
  local code="$1"
  local body="$2"
  if [[ "${outfile}" != "/dev/null" && -n "${outfile}" ]]; then
    printf '%s' "${body}" >"${outfile}"
  fi
  if [[ "${write_fmt}" == *http_code* ]]; then
    printf '%s' "${code}"
  fi
}

if [[ "${MODE}" == "transport" ]]; then
  exit 7
fi

if [[ "${url}" == *"/shm/pay_systems/yookassa.cgi"* ]]; then
  emit 400 'Error: unknown user'
  exit 0
fi

if [[ "${url}" == *"/redirect.html"* ]]; then
  if [[ "${url}" == *"url=https"* ]]; then
    case "${MODE}" in
      invalid_200)
        emit 200 'ok'
        ;;
      invalid_400_bad_body)
        emit 400 'something else'
        ;;
      *)
        emit 400 'Invalid Happ URL'
        ;;
    esac
    exit 0
  fi
  case "${MODE}" in
    shm_admin)
      emit 200 '<!doctype html><html><head><title>SHM Admin</title></head><body>admin</body></html>'
      ;;
    http_502)
      emit 502 'bad gateway'
      ;;
    missing_marker)
      emit 200 '<!doctype html><html><head><title>Other</title></head><body>ok</body></html>'
      ;;
    *)
      emit 200 '<!doctype html><html><head><title>Открытие Happ</title></head><body><script>URLSearchParams; var x="happ://";</script></body></html>'
      ;;
  esac
  exit 0
fi

emit 200 'ok'
exit 0
EOF
  chmod 0700 "${WORK}/bin/curl"
}

write_probe_config() {
  cat >"${WORK}/probe-config.json" <<'EOF'
{
  "api": {"base_url": "https://shm-api.test.example", "api_login": "x", "api_pass": "y"},
  "brand": {
    "id": "vff",
    "name": "VPN for Friends",
    "public_base_url": "https://connect.vpn-for-friends.com",
    "yookassa_pay_system": "yookassa"
  }
}
EOF
}

run_smoke() {
  local mode="$1"
  SMOKE_CURL_MODE="${mode}" PATH="${WORK}/bin:${PATH}" \
    bash "${ROOT}/scripts/smoke-brand.sh" vff --config "${WORK}/probe-config.json"
}

setup() {
  cleanup
  WORK="$(mktemp -d)"
  mkdir -p "${WORK}/bin"
  write_probe_config
  write_mock_curl
}

expect_pass() {
  local name="$1"
  local mode="$2"
  shift 2
  local needle
  setup
  local out rc=0
  out="$(run_smoke "${mode}" 2>&1)" || rc=$?
  if [[ "${rc}" -ne 0 ]]; then
    fail "${name}" "rc=${rc} ${out}"
    return
  fi
  for needle in "$@"; do
    if ! grep -Fq -- "${needle}" <<<"${out}"; then
      fail "${name}" "missing ${needle}: ${out}"
      return
    fi
  done
  pass "${name}"
}

expect_fail() {
  local name="$1"
  local mode="$2"
  local needle="$3"
  setup
  local out rc=0
  out="$(run_smoke "${mode}" 2>&1)" || rc=$?
  if [[ "${rc}" -eq 0 ]]; then
    fail "${name}" "expected failure: ${out}"
    return
  fi
  if [[ -n "${needle}" ]] && ! grep -Fq -- "${needle}" <<<"${out}"; then
    fail "${name}" "missing ${needle}: ${out}"
    return
  fi
  pass "${name}"
}

expect_pass valid_happ_ok ok \
  '-> 200 (Happ redirect OK)' \
  '-> 400 (invalid scheme rejected)' \
  'smoke-VFF: OK'

expect_fail shm_admin_page shm_admin 'unexpected SHM Admin page from Happ redirect route'
expect_fail valid_http_502 http_502 'Happ redirect route returned HTTP 502'
expect_fail missing_happ_marker missing_marker 'Happ redirect marker missing'
expect_fail invalid_scheme_http_200 invalid_200 'Happ redirect invalid scheme returned HTTP 200'
expect_fail invalid_scheme_bad_body invalid_400_bad_body "Happ redirect invalid scheme body missing 'Invalid Happ URL'"

if [[ "${FAILS}" -ne 0 ]]; then
  echo "smoke_brand_test: ${FAILS} failed" >&2
  exit 1
fi
echo "smoke_brand_test: all passed"
