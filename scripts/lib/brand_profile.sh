#!/usr/bin/env bash
# Generic brand profile loader. Reads declarative JSON from deploy/brands/<id>.json,
# validates it strictly, and exports the environment contract consumed by brand_ops.sh.
# No secrets, no eval, no shell generated from JSON.
# shellcheck shell=bash

# Directory holding profile JSON files. Overridable for tests only.
brand_profile_dir() {
  if [[ -n "${BRAND_PROFILES_DIR:-}" ]]; then
    printf '%s' "${BRAND_PROFILES_DIR%/}"
    return 0
  fi
  local d
  d="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  printf '%s' "${d}/deploy/brands"
}

# Strict JSON validation program. Returns the profile id on success, else errors.
# Argument: --arg expected <filename-stem>
_brand_profile_jq_validate='
def trim: gsub("^[ \t\r\n]+|[ \t\r\n]+$";"");
def ctl: test("[\u0000-\u001f]");
def s($v;$n):
  if ($v|type) != "string" then error($n + ": must be a string")
  elif ($v|trim) == "" then error($n + ": must be a non-empty string")
  elif ($v|ctl) then error($n + ": must not contain control characters")
  else $v end;
def abspath($v;$n):
  (s($v;$n)) as $x
  | if ($x|startswith("/")|not) then error($n + ": must be an absolute path")
    elif ($x|test("(^|/)\\.\\.(/|$)")) then error($n + ": must not contain ..")
    else $x end;
def within($child;$dir;$n):
  if ($child|startswith($dir + "/")|not) then error($n + ": must be inside " + $dir)
  else $child end;

if type != "object" then error("profile: must be a JSON object") else . end
| . as $p
| s($p.id; "id") as $id
| (if $id != $expected then error("id (" + $id + ") must equal profile filename (" + $expected + ")") else $id end) as $id
| s($p.label; "label")
| s($p.name; "name")
| (s($p.server.user; "server.user")
   | if (test("^[a-z_][a-z0-9_-]*$")|not) then error("server.user: must match ^[a-z_][a-z0-9_-]*$") else . end)
| (s($p.server.host; "server.host")
   | if (test("^[A-Za-z0-9][A-Za-z0-9._-]*$")|not) then error("server.host: invalid host/alias") else . end)
| (s($p.runtime.service; "runtime.service")
   | if (test("^[A-Za-z0-9][A-Za-z0-9_.@-]*\\.service$")|not) then error("runtime.service: must be a safe systemd unit ending in .service") else . end) as $svc
| abspath($p.runtime.directory; "runtime.directory") as $dir
| within(abspath($p.runtime.binary; "runtime.binary"); $dir; "runtime.binary")
| within(abspath($p.runtime.legacy_config; "runtime.legacy_config"); $dir; "runtime.legacy_config") as $legacy
| within(abspath($p.runtime.explicit_config; "runtime.explicit_config"); $dir; "runtime.explicit_config") as $explicit
| (if $legacy == $explicit then error("runtime.legacy_config and runtime.explicit_config must differ") else . end)
| abspath($p.runtime.dropin; "runtime.dropin") as $dropin
| (if ($dropin|startswith("/etc/systemd/system/" + $svc + ".d/")|not)
     then error("runtime.dropin: must be under /etc/systemd/system/" + $svc + ".d/") else . end)
| (s($p.brand.allowed_host; "brand.allowed_host")
   | if (test("^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$")|not)
       then error("brand.allowed_host: must be a bare DNS hostname (no scheme/port/path)") else . end)
| s($p.brand.public_base_url; "brand.public_base_url")
| s($p.brand.landing_url; "brand.landing_url")
| s($p.brand.service_category; "brand.service_category")
| s($p.brand.payment_profile; "brand.payment_profile")
| (s($p.brand.yookassa_pay_system; "brand.yookassa_pay_system")
   | if (test("^[a-z0-9][a-z0-9_-]*$")|not) then error("brand.yookassa_pay_system: must match ^[a-z0-9][a-z0-9_-]*$") else . end)
| s($p.brand.web_login_prefix; "brand.web_login_prefix")
| s($p.brand.web_user_source; "brand.web_user_source")
| ([$p | paths | .[] | select(type=="string")]
   | map(select(test("token|password|passwd|secret|api[_-]?key|apikey|private[_-]?key";"i")))
   | if length > 0 then error("secret-like key(s) present: " + (join(","))) else . end)
| $id
'

# brand_profile_validate_https_url <field> <url>
# Strict absolute https URL: DNS host, optional port 1..65535, optional path.
# Rejects userinfo, query, fragment, whitespace/control chars, non-https schemes.
brand_profile_validate_https_url() {
  local field="${1:?field required}" url="${2:?url required}"
  local rest hostport host port

  if [[ -z "${url}" ]]; then
    echo "brand_profile: ${field}: must be a non-empty https URL" >&2
    return 1
  fi
  # Reject whitespace and ASCII control characters.
  if [[ "${url}" == *$'\n'* || "${url}" == *$'\r'* || "${url}" == *$'\t'* || "${url}" == *' '* ]]; then
    echo "brand_profile: ${field}: must not contain whitespace" >&2
    return 1
  fi
  if [[ "${url}" =~ [[:cntrl:]] ]]; then
    echo "brand_profile: ${field}: must not contain control characters" >&2
    return 1
  fi
  if [[ "${url}" != https://* ]]; then
    echo "brand_profile: ${field}: must be an absolute https:// URL" >&2
    return 1
  fi
  if [[ "${url}" == *'?'* ]]; then
    echo "brand_profile: ${field}: query strings are not allowed" >&2
    return 1
  fi
  if [[ "${url}" == *'#'* ]]; then
    echo "brand_profile: ${field}: fragments are not allowed" >&2
    return 1
  fi

  rest="${url#https://}"
  if [[ -z "${rest}" ]]; then
    echo "brand_profile: ${field}: missing host" >&2
    return 1
  fi
  # userinfo is forbidden (anything before @).
  if [[ "${rest}" == *@* ]]; then
    echo "brand_profile: ${field}: userinfo is not allowed" >&2
    return 1
  fi

  if [[ "${rest}" == */* ]]; then
    hostport="${rest%%/*}"
  else
    hostport="${rest}"
  fi
  if [[ -z "${hostport}" ]]; then
    echo "brand_profile: ${field}: missing host" >&2
    return 1
  fi

  if [[ "${hostport}" == *:* ]]; then
    host="${hostport%%:*}"
    port="${hostport#*:}"
    if [[ -z "${host}" || -z "${port}" ]]; then
      echo "brand_profile: ${field}: invalid host:port" >&2
      return 1
    fi
    if [[ ! "${port}" =~ ^[0-9]+$ ]]; then
      echo "brand_profile: ${field}: port must be an integer" >&2
      return 1
    fi
    if (( 10#${port} < 1 || 10#${port} > 65535 )); then
      echo "brand_profile: ${field}: port must be in 1..65535" >&2
      return 1
    fi
  else
    host="${hostport}"
  fi

  # DNS hostname (same contract as brand.allowed_host).
  if [[ ! "${host}" =~ ^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$ ]]; then
    echo "brand_profile: ${field}: invalid DNS hostname" >&2
    return 1
  fi
  return 0
}

# brand_profile_validate_file <id> <file>: strict validation of the JSON file.
brand_profile_validate_file() {
  local id="${1:?brand id required}"
  local file="${2:?profile file required}"
  if ! command -v jq >/dev/null 2>&1; then
    echo "brand_profile: jq is required" >&2
    return 1
  fi
  if [[ ! -f "${file}" ]]; then
    echo "brand_profile: profile not found: ${file}" >&2
    return 1
  fi
  if ! jq -e . "${file}" >/dev/null 2>&1; then
    echo "brand_profile: malformed JSON: ${file}" >&2
    return 1
  fi
  if ! jq -er --arg expected "${id}" "${_brand_profile_jq_validate}" "${file}" >/dev/null; then
    return 1
  fi
  return 0
}

# brand_profile_validate_loaded: verify the exported environment contract is complete.
brand_profile_validate_loaded() {
  local v missing=()
  for v in SERVER_USER SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_BINARY \
    REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE EXPECTED_BRAND_ID \
    BRAND_LABEL SMOKE_BASE_URL EXPECT_PUBLIC_BASE_URL EXPECT_SERVICE_CATEGORY \
    EXPECT_PAYMENT_PROFILE EXPECT_YOOKASSA_PAY_SYSTEM BRAND_NAME ALLOWED_HOST LANDING_URL WEB_LOGIN_PREFIX \
    WEB_USER_SOURCE REMOTE_CONFIG_VFF REMOTE_CONFIG_LEGACY; do
    if [[ -z "${!v:-}" ]]; then
      missing+=("${v}")
    fi
  done
  if ((${#missing[@]} > 0)); then
    echo "brand_profile: incomplete environment after load: ${missing[*]}" >&2
    return 1
  fi
  if [[ "${REMOTE_CONFIG_VFF}" != "${REMOTE_EXPLICIT_CONFIG}" ]]; then
    echo "brand_profile: REMOTE_CONFIG_VFF alias mismatch" >&2
    return 1
  fi
  if [[ "${REMOTE_CONFIG_LEGACY}" != "${REMOTE_LEGACY_CONFIG}" ]]; then
    echo "brand_profile: REMOTE_CONFIG_LEGACY alias mismatch" >&2
    return 1
  fi
  return 0
}

# brand_profile_load <brand-id>: validate and export the profile environment contract.
brand_profile_load() {
  local id="${1:-}"
  if [[ -z "${id}" ]]; then
    echo "brand_profile_load: brand id required" >&2
    return 1
  fi
  # Reject anything that could enable path traversal or shell tricks.
  if [[ ! "${id}" =~ ^[a-z0-9][a-z0-9_-]*$ ]]; then
    echo "brand_profile_load: invalid brand id '${id}' (want ^[a-z0-9][a-z0-9_-]*\$)" >&2
    return 1
  fi

  local dir file
  dir="$(brand_profile_dir)"
  file="${dir}/${id}.json"

  if [[ ! -f "${file}" ]]; then
    echo "brand_profile_load: profile not found for '${id}': ${file}" >&2
    return 1
  fi

  if ! brand_profile_validate_file "${id}" "${file}"; then
    echo "brand_profile_load: profile '${id}' failed validation" >&2
    return 1
  fi

  # Read validated values as a newline-delimited stream (no eval; values are
  # already validated to be free of control characters incl. newlines).
  local vals=()
  if ! mapfile -t vals < <(jq -er '
    .id, .label, .name,
    .server.user, .server.host,
    .runtime.service, .runtime.directory, .runtime.binary,
    .runtime.legacy_config, .runtime.explicit_config, .runtime.dropin,
    .brand.allowed_host, .brand.public_base_url, .brand.landing_url,
    .brand.service_category, .brand.payment_profile, .brand.yookassa_pay_system,
    .brand.web_login_prefix, .brand.web_user_source
  ' "${file}"); then
    echo "brand_profile_load: failed to read profile '${id}'" >&2
    return 1
  fi
  if [[ "${#vals[@]}" -ne 19 ]]; then
    echo "brand_profile_load: unexpected field count for '${id}'" >&2
    return 1
  fi

  export EXPECTED_BRAND_ID="${vals[0]}"
  export BRAND_LABEL="${vals[1]}"
  export BRAND_NAME="${vals[2]}"
  export SERVER_USER="${vals[3]}"
  export SERVER_HOST="${vals[4]}"
  export SERVICE_NAME="${vals[5]}"
  export REMOTE_DIR="${vals[6]}"
  export REMOTE_BINARY="${vals[7]}"
  export REMOTE_LEGACY_CONFIG="${vals[8]}"
  export REMOTE_EXPLICIT_CONFIG="${vals[9]}"
  export DROPIN_FILE="${vals[10]}"
  export ALLOWED_HOST="${vals[11]}"
  export EXPECT_PUBLIC_BASE_URL="${vals[12]}"
  export SMOKE_BASE_URL="${vals[12]}"
  export LANDING_URL="${vals[13]}"
  export EXPECT_SERVICE_CATEGORY="${vals[14]}"
  export EXPECT_PAYMENT_PROFILE="${vals[15]}"
  export EXPECT_YOOKASSA_PAY_SYSTEM="${vals[16]}"
  export WEB_LOGIN_PREFIX="${vals[17]}"
  export WEB_USER_SOURCE="${vals[18]}"

  # Compatibility aliases for older VFF scripts/tests.
  export REMOTE_CONFIG_VFF="${REMOTE_EXPLICIT_CONFIG}"
  export REMOTE_CONFIG_LEGACY="${REMOTE_LEGACY_CONFIG}"

  if ! brand_profile_validate_https_url "brand.public_base_url" "${EXPECT_PUBLIC_BASE_URL}"; then
    return 1
  fi
  if ! brand_profile_validate_https_url "brand.landing_url" "${LANDING_URL}"; then
    return 1
  fi

  brand_profile_validate_loaded || return 1
  return 0
}

# Backward-compatible alias. Delegates to the generic loader.
brand_profile_export() {
  brand_profile_load "$@"
}

# brand_profile_summary <brand-id>: print only a safe, non-secret summary.
brand_profile_summary() {
  local id="${1:-}"
  if ! brand_profile_load "${id}"; then
    return 1
  fi
  printf 'brand.id=%s\n' "${EXPECTED_BRAND_ID}"
  printf 'brand.label=%s\n' "${BRAND_LABEL}"
  printf 'server.host=%s\n' "${SERVER_HOST}"
  printf 'runtime.service=%s\n' "${SERVICE_NAME}"
  printf 'runtime.directory=%s\n' "${REMOTE_DIR}"
  printf 'runtime.binary=%s\n' "${REMOTE_BINARY}"
  printf 'runtime.legacy_config=%s\n' "${REMOTE_LEGACY_CONFIG}"
  printf 'runtime.explicit_config=%s\n' "${REMOTE_EXPLICIT_CONFIG}"
  printf 'runtime.dropin=%s\n' "${DROPIN_FILE}"
  printf 'brand.public_base_url=%s\n' "${EXPECT_PUBLIC_BASE_URL}"
  printf 'brand.service_category=%s\n' "${EXPECT_SERVICE_CATEGORY}"
  printf 'brand.payment_profile=%s\n' "${EXPECT_PAYMENT_PROFILE}"
  printf 'brand.yookassa_pay_system=%s\n' "${EXPECT_YOOKASSA_PAY_SYSTEM}"
}
