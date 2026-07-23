#!/usr/bin/env bash
# Validation matrix for declarative brand profiles + generic Make wiring.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

PROFILES_DIR="${ROOT}/deploy/brands"

FAILS=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s: %s\n' "$1" "$2" >&2; FAILS=$((FAILS + 1)); }

# Canonical valid profile used as a base for negative mutations (unique from vff/fc).
base_json() {
  cat <<'EOF'
{
  "id": "demo",
  "label": "DEMO",
  "name": "Demo Brand",
  "server": { "user": "root", "host": "demo-host" },
  "runtime": {
    "service": "bot-demo.service",
    "directory": "/opt/bot-demo",
    "binary": "/opt/bot-demo/bot",
    "legacy_config": "/opt/bot-demo/config.json",
    "explicit_config": "/opt/bot-demo/config-demo.json",
    "dropin": "/etc/systemd/system/bot-demo.service.d/10-vpnbot-config.conf"
  },
  "brand": {
    "allowed_host": "demo.example.com",
    "public_base_url": "https://demo.example.com",
    "landing_url": "https://demo-landing.example.com",
    "service_category": "vpn-demo",
    "payment_profile": "telegram_demo_bot",
    "yookassa_pay_system": "yookassa_demo",
    "web_login_prefix": "web_",
    "web_user_source": "vpn-for-friends.com"
  }
}
EOF
}

# ---------------------------------------------------------------------------
# Per-profile checks for the real production profiles.
# ---------------------------------------------------------------------------
test_each_profile() {
  local f id
  for f in "${PROFILES_DIR}"/*.json; do
    id="$(basename "${f}" .json)"

    # filename matches .id
    local jid
    jid="$(jq -r '.id' "${f}")"
    if [[ "${jid}" != "${id}" ]]; then fail "profile:${id}" "filename != .id (${jid})"; continue; fi

    # loader succeeds and exports the full contract
    if ! ( brand_profile_load "${id}" ) >/dev/null 2>&1; then
      fail "profile:${id}" "loader failed"; continue
    fi
    brand_profile_load "${id}" >/dev/null 2>&1

    local v ok=1
    for v in SERVER_USER SERVER_HOST SERVICE_NAME REMOTE_DIR REMOTE_BINARY \
      REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE EXPECTED_BRAND_ID \
      BRAND_LABEL SMOKE_BASE_URL EXPECT_PUBLIC_BASE_URL EXPECT_SERVICE_CATEGORY \
      EXPECT_PAYMENT_PROFILE EXPECT_YOOKASSA_PAY_SYSTEM BRAND_NAME ALLOWED_HOST LANDING_URL WEB_LOGIN_PREFIX \
      WEB_USER_SOURCE REMOTE_CONFIG_VFF REMOTE_CONFIG_LEGACY; do
      if [[ -z "${!v:-}" ]]; then fail "profile:${id}" "unset ${v}"; ok=0; fi
    done
    [[ "${ok}" -eq 1 ]] || continue

    # absolute paths
    for v in REMOTE_DIR REMOTE_BINARY REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG DROPIN_FILE; do
      if [[ "${!v}" != /* ]]; then fail "profile:${id}" "${v} not absolute"; ok=0; fi
    done
    [[ "${ok}" -eq 1 ]] || continue

    # legacy != explicit; configs/binary inside directory
    if [[ "${REMOTE_LEGACY_CONFIG}" == "${REMOTE_EXPLICIT_CONFIG}" ]]; then
      fail "profile:${id}" "legacy == explicit"; continue
    fi
    for v in REMOTE_BINARY REMOTE_LEGACY_CONFIG REMOTE_EXPLICIT_CONFIG; do
      if [[ "${!v}" != "${REMOTE_DIR}/"* ]]; then fail "profile:${id}" "${v} not inside REMOTE_DIR"; ok=0; fi
    done
    [[ "${ok}" -eq 1 ]] || continue

    # drop-in matches service unit
    if [[ "${DROPIN_FILE}" != "/etc/systemd/system/${SERVICE_NAME}.d/"* ]]; then
      fail "profile:${id}" "dropin not under service .d"; continue
    fi

    # public URL + allowed host validity
    if [[ "${EXPECT_PUBLIC_BASE_URL}" != https://* ]]; then fail "profile:${id}" "public url not https"; continue; fi
    if [[ "${ALLOWED_HOST}" == *"://"* || "${ALLOWED_HOST}" == *:* || "${ALLOWED_HOST}" == */* ]]; then
      fail "profile:${id}" "allowed_host not bare"; continue
    fi

    # SMOKE_BASE_URL sourced from profile public base url
    if [[ "${SMOKE_BASE_URL}" != "${EXPECT_PUBLIC_BASE_URL}" ]]; then
      fail "profile:${id}" "smoke != public_base_url"; continue
    fi

    # summary has no secret-like content
    local summary
    summary="$(brand_profile_summary "${id}" 2>/dev/null)"
    if grep -Eiq 'token|password|passwd|secret|api[_-]?key|apikey|private[_-]?key' <<<"${summary}"; then
      fail "profile:${id}" "summary contains secret-like text"; continue
    fi

    # repository file mode: not group/other writable
    local mode
    mode="$(stat -c '%a' "${f}")"
    if (( (0${mode} & 022) != 0 )); then fail "profile:${id}" "insecure file mode ${mode}"; continue; fi

    # no secret-like keys in JSON
    if jq -e '[paths | .[] | select(type=="string")]
              | map(select(test("token|password|passwd|secret|api[_-]?key|apikey|private[_-]?key";"i")))
              | length > 0' "${f}" >/dev/null; then
      fail "profile:${id}" "secret-like key in JSON"; continue
    fi

    pass "profile:${id}"
  done
}

# ---------------------------------------------------------------------------
# Global uniqueness / allowed-duplication across all production profiles.
# ---------------------------------------------------------------------------
test_global_uniqueness() {
  local field
  for field in '.id' '.runtime.service' '.runtime.directory' '.runtime.binary' \
    '.runtime.explicit_config' '.runtime.dropin' '.brand.public_base_url' \
    '.brand.service_category' '.brand.payment_profile' '.brand.yookassa_pay_system'; do
    local total uniq
    total="$(jq -r "${field}" "${PROFILES_DIR}"/*.json | wc -l)"
    uniq="$(jq -r "${field}" "${PROFILES_DIR}"/*.json | sort -u | wc -l)"
    if [[ "${total}" -ne "${uniq}" ]]; then
      fail global_uniqueness "duplicate values for ${field}"; return
    fi
  done
  pass global_uniqueness
}

# ---------------------------------------------------------------------------
# Third brand with ZERO code changes (fixture dir override).
# ---------------------------------------------------------------------------
test_third_brand_no_code_change() {
  local d; d="$(mktemp -d)"
  base_json >"${d}/demo.json"
  chmod 0644 "${d}/demo.json"

  local rc=0
  ( BRAND_PROFILES_DIR="${d}" brand_profile_load demo ) >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -ne 0 ]]; then fail third_brand "loader failed for demo"; rm -rf "${d}"; return; fi

  # exported values are correct
  (
    BRAND_PROFILES_DIR="${d}" brand_profile_load demo >/dev/null 2>&1
    [[ "${EXPECTED_BRAND_ID}" == "demo" ]] || exit 1
    [[ "${SERVICE_NAME}" == "bot-demo.service" ]] || exit 1
    [[ "${SERVER_HOST}" == "demo-host" ]] || exit 1
    [[ "${SMOKE_BASE_URL}" == "https://demo.example.com" ]] || exit 1
  ) || { fail third_brand "exports wrong for demo"; rm -rf "${d}"; return; }

  # generic script dry path works for a brand that does not exist in code
  local out
  out="$(BRAND_PROFILES_DIR="${d}" bash "${ROOT}/scripts/brand-profile.sh" demo 2>&1)" || {
    fail third_brand "brand-profile.sh failed: ${out}"; rm -rf "${d}"; return
  }
  if ! grep -Fxq 'brand.id=demo' <<<"${out}"; then fail third_brand "summary missing demo: ${out}"; rm -rf "${d}"; return; fi

  # neither Makefile nor loader hardcodes 'demo'
  if grep -Fq 'demo' "${ROOT}/Makefile"; then fail third_brand "Makefile hardcodes demo"; rm -rf "${d}"; return; fi
  if grep -Fq 'demo' "${ROOT}/scripts/lib/brand_profile.sh"; then fail third_brand "loader hardcodes demo"; rm -rf "${d}"; return; fi

  rm -rf "${d}"
  pass third_brand_no_code_change
}

# ---------------------------------------------------------------------------
# Negative validation matrix. Each must fail the loader (before any ssh/scp/etc).
# ---------------------------------------------------------------------------
neg_file() {
  # neg_file <name> <target-id> <mutation-jq-or-RAW:...>
  local name="$1" target="$2" mutate="$3"
  local d; d="$(mktemp -d)"
  if [[ "${mutate}" == RAW:* ]]; then
    printf '%s' "${mutate#RAW:}" >"${d}/${target}.json"
  elif [[ -n "${mutate}" ]]; then
    base_json | jq "${mutate}" >"${d}/${target}.json"
  else
    base_json >"${d}/${target}.json"
  fi
  chmod 0644 "${d}/${target}.json" 2>/dev/null || true
  local rc=0
  ( BRAND_PROFILES_DIR="${d}" brand_profile_load "${target}" ) >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail "neg:${name}" "loader should have failed"; else pass "neg:${name}"; fi
  rm -rf "${d}"
}

neg_direct() {
  # neg_direct <name> <brand-id-arg>
  local name="$1" id="$2"
  local rc=0
  ( brand_profile_load "${id}" ) >/dev/null 2>&1 || rc=$?
  if [[ "${rc}" -eq 0 ]]; then fail "neg:${name}" "loader should have failed"; else pass "neg:${name}"; fi
}

test_negative_matrix() {
  neg_direct unknown_brand nonexistent-brand
  neg_direct path_traversal '../fc'
  neg_direct uppercase_id 'FC'

  neg_file missing_field demo 'del(.brand.allowed_host)'
  neg_file wrong_type demo '.brand.public_base_url = 123'
  neg_file empty_string demo '.name = ""'
  neg_file id_mismatch demo '.id = "other"'
  neg_file relative_runtime_path demo '.runtime.directory = "opt/bot-demo"'
  neg_file runtime_path_dotdot demo '.runtime.binary = "/opt/bot-demo/../bot"'
  neg_file legacy_eq_explicit demo '.runtime.explicit_config = .runtime.legacy_config'
  neg_file binary_outside_dir demo '.runtime.binary = "/opt/other/bot"'
  neg_file dropin_other_service demo '.runtime.dropin = "/etc/systemd/system/bot-other.service.d/10-vpnbot-config.conf"'
  neg_file invalid_server_host demo '.server.host = "bad host"'
  neg_file allowed_host_scheme demo '.brand.allowed_host = "https://demo.example.com"'
  neg_file allowed_host_port demo '.brand.allowed_host = "demo.example.com:8090"'
  neg_file invalid_url demo '.brand.public_base_url = "ftp://demo.example.com"'
  neg_file https_without_host demo '.brand.public_base_url = "https://"'
  neg_file url_userinfo demo '.brand.public_base_url = "https://user@demo.example.com"'
  neg_file url_whitespace demo '.brand.public_base_url = "https://demo.example.com/path with space"'
  neg_file url_fragment demo '.brand.landing_url = "https://demo.example.com/#x"'
  neg_file url_port_zero demo '.brand.public_base_url = "https://demo.example.com:0"'
  neg_file url_port_too_high demo '.brand.public_base_url = "https://demo.example.com:65536"'
  neg_file malformed_hostname_url demo '.brand.public_base_url = "https://bad_host"'
  neg_file server_user_leading_dash demo '.server.user = "-root"'
  neg_file server_user_at demo '.server.user = "root@host"'
  neg_file service_leading_dash demo '.runtime.service = "-bot.service"'
  neg_file service_whitespace demo '.runtime.service = "bot friends.service"'
  neg_file token_key demo '.brand.token = "x"'
  neg_file password_key demo '.server.password = "x"'
  neg_file malformed_json demo 'RAW:{ this is not json'
}

# ---------------------------------------------------------------------------
# Make dry-run matrix (no execution beyond make -n / recursive make).
# ---------------------------------------------------------------------------
mkn() { make -n -C "${ROOT}" "$@" 2>&1; }

test_make_dryrun_matrix() {
  local out

  for b in vff fc; do
    out="$(mkn brand-deploy BRAND="${b}")" || { fail "mkn:brand-deploy:${b}" "make -n failed"; return; }
    grep -Fq "deploy-brand-binary.sh" <<<"${out}" || { fail "mkn:brand-deploy:${b}" "script missing"; return; }
    grep -Fq "${b}" <<<"${out}" || { fail "mkn:brand-deploy:${b}" "brand id missing"; return; }

    out="$(mkn brand-config-activate BRAND="${b}")" || { fail "mkn:activate:${b}" "make -n failed"; return; }
    grep -Fq "activate-brand-config.sh" <<<"${out}" || { fail "mkn:activate:${b}" "script missing"; return; }
    grep -Fq "${b}" <<<"${out}" || { fail "mkn:activate:${b}" "brand id missing"; return; }

    out="$(mkn brand-status BRAND="${b}")" || { fail "mkn:status:${b}" "make -n failed"; return; }
    grep -Fq "status-brand.sh" <<<"${out}" || { fail "mkn:status:${b}" "script missing"; return; }
    grep -Fq "${b}" <<<"${out}" || { fail "mkn:status:${b}" "brand id missing"; return; }
  done

  # aliases delegate to the generic engine with the right brand
  out="$(mkn deploy-fc)" || { fail "mkn:deploy-fc" "make -n failed"; return; }
  grep -Fq "deploy-brand-binary.sh" <<<"${out}" || { fail "mkn:deploy-fc" "no generic script"; return; }
  grep -Fq "fc" <<<"${out}" || { fail "mkn:deploy-fc" "no fc"; return; }

  out="$(mkn activate-fc-config)" || { fail "mkn:activate-fc-config" "make -n failed"; return; }
  grep -Fq "activate-brand-config.sh" <<<"${out}" || { fail "mkn:activate-fc-config" "no generic script"; return; }

  out="$(mkn deploy)" || { fail "mkn:deploy" "make -n failed"; return; }
  grep -Fq "deploy-brand-binary.sh" <<<"${out}" || { fail "mkn:deploy" "no generic script"; return; }
  grep -Fq "vff" <<<"${out}" || { fail "mkn:deploy" "no vff"; return; }

  out="$(mkn activate-vff-config)" || { fail "mkn:activate-vff-config" "make -n failed"; return; }
  grep -Fq "activate-brand-config.sh" <<<"${out}" || { fail "mkn:activate-vff-config" "no generic script"; return; }
  grep -Fq "vff" <<<"${out}" || { fail "mkn:activate-vff-config" "no vff"; return; }

  out="$(mkn brand-rollout BRAND=fc CONFIG=/secure/config-fc.json)" || { fail "mkn:brand-rollout:fc" "make -n failed"; return; }
  grep -Fq "rollout-brand.sh" <<<"${out}" || { fail "mkn:brand-rollout:fc" "script missing"; return; }
  grep -Fq "fc" <<<"${out}" || { fail "mkn:brand-rollout:fc" "brand missing"; return; }
  grep -Fq "/secure/config-fc.json" <<<"${out}" || { fail "mkn:brand-rollout:fc" "CONFIG missing"; return; }

  out="$(mkn brand-rollout BRAND=vff CONFIG=/secure/config-vff.json)" || { fail "mkn:brand-rollout:vff" "make -n failed"; return; }
  grep -Fq "rollout-brand.sh" <<<"${out}" || { fail "mkn:brand-rollout:vff" "script missing"; return; }
  grep -Fq "vff" <<<"${out}" || { fail "mkn:brand-rollout:vff" "brand missing"; return; }

  out="$(mkn rollout-fc CONFIG=/secure/config-fc.json)" || { fail "mkn:rollout-fc" "make -n failed"; return; }
  grep -Fq "rollout-brand.sh" <<<"${out}" || { fail "mkn:rollout-fc" "no generic script"; return; }
  grep -Fq "fc" <<<"${out}" || { fail "mkn:rollout-fc" "no fc"; return; }

  out="$(mkn rollout-vff CONFIG=/secure/config-vff.json)" || { fail "mkn:rollout-vff" "make -n failed"; return; }
  grep -Fq "rollout-brand.sh" <<<"${out}" || { fail "mkn:rollout-vff" "no generic script"; return; }
  grep -Fq "vff" <<<"${out}" || { fail "mkn:rollout-vff" "no vff"; return; }

  pass make_dryrun_matrix
}

test_makefile_has_no_brand_params() {
  local mk="${ROOT}/Makefile"
  if grep -Eq 'EXPORT_VFF|EXPORT_FC|VFF_SERVER_HOST|FC_SERVER_HOST' "${mk}"; then
    fail makefile_clean "legacy brand params still present"; return
  fi
  # No hardcoded production values in the Makefile.
  if grep -Eq 'fr-mrs-1|/opt/bot|telegram_bot|telegram_friends_connect_bot|vpn-for-friends\.com|friends-connect\.club|vpn-mz-' "${mk}"; then
    fail makefile_clean "hardcoded profile value in Makefile"; return
  fi
  pass makefile_has_no_brand_params
}

test_each_profile
test_global_uniqueness
test_third_brand_no_code_change
test_negative_matrix
test_make_dryrun_matrix
test_makefile_has_no_brand_params

if [[ "${FAILS}" -ne 0 ]]; then
  echo "brand_profiles_test: ${FAILS} failed" >&2
  exit 1
fi
echo "brand_profiles_test: all passed"
