#!/usr/bin/env bash
# Build explicit brand config from legacy JSON. Never prints JSON. No set -x.
set -euo pipefail

SOURCE=""
OUTPUT=""
BRAND_ID=""
BRAND_NAME=""
ALLOWED_HOST=""
LANDING_URL=""
WEB_LOGIN_PREFIX="web_"
WEB_USER_SOURCE="vpn-for-friends.com"
EXPECT_PUBLIC_BASE_URL=""
EXPECT_SERVICE_CATEGORY=""
EXPECT_PAYMENT_PROFILE=""

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/render-brand-config.sh \
  --source FILE --output FILE \
  --brand-id ID --brand-name NAME \
  --allowed-host HOST --landing-url URL \
  [--web-login-prefix web_] [--web-user-source vpn-for-friends.com] \
  [--expect-public-base-url URL] \
  [--expect-service-category CAT] \
  [--expect-payment-profile PROFILE]
EOF
}

canonical_existing() {
  # Absolute canonical path for an existing file or directory.
  local p="$1"
  if command -v realpath >/dev/null 2>&1; then
    realpath -m "$p" 2>/dev/null || realpath "$p"
  else
    readlink -f "$p"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source) SOURCE="${2:-}"; shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    --brand-id) BRAND_ID="${2:-}"; shift 2 ;;
    --brand-name) BRAND_NAME="${2:-}"; shift 2 ;;
    --allowed-host) ALLOWED_HOST="${2:-}"; shift 2 ;;
    --landing-url) LANDING_URL="${2:-}"; shift 2 ;;
    --web-login-prefix) WEB_LOGIN_PREFIX="${2:-}"; shift 2 ;;
    --web-user-source) WEB_USER_SOURCE="${2:-}"; shift 2 ;;
    --expect-public-base-url) EXPECT_PUBLIC_BASE_URL="${2:-}"; shift 2 ;;
    --expect-service-category) EXPECT_SERVICE_CATEGORY="${2:-}"; shift 2 ;;
    --expect-payment-profile) EXPECT_PAYMENT_PROFILE="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    *) echo "render-brand-config: unknown arg: $1" >&2; usage; exit 1 ;;
  esac
done

for req in SOURCE OUTPUT BRAND_ID BRAND_NAME ALLOWED_HOST LANDING_URL WEB_LOGIN_PREFIX WEB_USER_SOURCE; do
  if [[ -z "${!req}" ]]; then
    echo "render-brand-config: missing required argument" >&2
    usage
    exit 1
  fi
done

if [[ ! -f "${SOURCE}" ]]; then
  echo "render-brand-config: source not found: ${SOURCE}" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "render-brand-config: jq is required" >&2
  exit 1
fi

source_abs="$(canonical_existing "${SOURCE}")"
output_dir="$(dirname -- "${OUTPUT}")"
mkdir -p "${output_dir}"
output_dir_abs="$(canonical_existing "${output_dir}")"
output_base="$(basename -- "${OUTPUT}")"
output_abs="${output_dir_abs}/${output_base}"

if [[ "${source_abs}" == "${output_abs}" ]]; then
  echo "render-brand-config: SOURCE and OUTPUT must not be the same path" >&2
  exit 1
fi

tmp="$(mktemp "${output_dir_abs}/.config-brand.XXXXXX")"
chmod 0600 "${tmp}"

cleanup() {
  rm -f "${tmp}"
}
trap cleanup EXIT

# jq args via --arg; no secrets printed. On failure previous OUTPUT is untouched.
if ! jq -e \
  --arg brand_id "${BRAND_ID}" \
  --arg brand_name "${BRAND_NAME}" \
  --arg allowed_host "${ALLOWED_HOST}" \
  --arg landing_url "${LANDING_URL}" \
  --arg web_prefix "${WEB_LOGIN_PREFIX}" \
  --arg web_source "${WEB_USER_SOURCE}" \
  --arg expect_pbu "${EXPECT_PUBLIC_BASE_URL}" \
  --arg expect_cat "${EXPECT_SERVICE_CATEGORY}" \
  --arg expect_prof "${EXPECT_PAYMENT_PROFILE}" \
  '
  def req($path; $val):
    if ($val | type) != "string" or ($val | gsub("^\\s+|\\s+$";"")) == "" then
      error($path + " is required and must be a non-empty string")
    else
      ($val | gsub("^\\s+|\\s+$";""))
    end;
  def assert_eq($path; $got; $want):
    if ($want | length) == 0 then
      $got
    elif $got != $want then
      error($path + " must be " + $want + " (got " + $got + ")")
    else
      $got
    end;
  . as $root
  | (req("web_sales.public_base_url"; ($root.web_sales.public_base_url // ""))) as $pbu0
  | (req("services.category"; ($root.services.category // ""))) as $cat0
  | (req("payments.profile"; ($root.payments.profile // ""))) as $prof0
  | (assert_eq("web_sales.public_base_url"; $pbu0; $expect_pbu)) as $pbu
  | (assert_eq("services.category"; $cat0; $expect_cat)) as $cat
  | (assert_eq("payments.profile"; $prof0; $expect_prof)) as $prof
  | $root
  | .brand = {
      "id": $brand_id,
      "name": $brand_name,
      "allowed_hosts": [$allowed_host],
      "public_base_url": $pbu,
      "landing_url": $landing_url,
      "service_category": $cat,
      "web_user_login_prefix": $web_prefix,
      "web_user_source": $web_source,
      "payment_profile": $prof
    }
  | del(.services.category)
  | if ((.services // {}) | type) == "object" and ((.services | keys | length) == 0) then del(.services) else . end
  | del(.web_sales.public_base_url)
  | if ((.web_sales // {}) | type) == "object" and ((.web_sales | keys | length) == 0) then del(.web_sales) else . end
  | del(.payments.profile)
  | if ((.payments // {}) | type) == "object" and ((.payments | keys | length) == 0) then del(.payments) else . end
  ' "${SOURCE}" >"${tmp}"; then
  echo "render-brand-config: failed to build explicit config from source" >&2
  exit 1
fi

chmod 0600 "${tmp}"
mv "${tmp}" "${output_abs}"
tmp=""
trap - EXIT
chmod 0600 "${output_abs}"

echo "render-brand-config: wrote ${output_abs} (brand.id=${BRAND_ID})"
