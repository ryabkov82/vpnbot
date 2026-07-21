#!/usr/bin/env bash
# Build explicit brand config from legacy JSON using a brand profile.
# Never prints JSON. No set -x. No secrets.
# Usage: bash scripts/render-brand-config.sh <brand-id> --source FILE --output FILE
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"

BRAND_ID_ARG=""
SOURCE=""
OUTPUT=""

usage() {
  cat >&2 <<'EOF'
usage: bash scripts/render-brand-config.sh <brand-id> --source FILE --output FILE
       (brand-id may also be provided as --brand ID or via BRAND=ID)
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --brand) BRAND_ID_ARG="${2:-}"; shift 2 ;;
    --source) SOURCE="${2:-}"; shift 2 ;;
    --output) OUTPUT="${2:-}"; shift 2 ;;
    -h | --help) usage; exit 0 ;;
    -*) echo "render-brand-config: unknown arg: $1" >&2; usage; exit 1 ;;
    *)
      if [[ -z "${BRAND_ID_ARG}" ]]; then BRAND_ID_ARG="$1"; else
        echo "render-brand-config: unexpected arg: $1" >&2; usage; exit 1
      fi
      shift
      ;;
  esac
done

BRAND_ID_ARG="${BRAND_ID_ARG:-${BRAND:-}}"
if [[ -z "${BRAND_ID_ARG}" || -z "${SOURCE}" || -z "${OUTPUT}" ]]; then
  echo "render-brand-config: brand-id, --source and --output are required" >&2
  usage
  exit 1
fi

brand_profile_load "${BRAND_ID_ARG}" || exit 1

# Values come exclusively from the loaded profile (never from the Makefile).
BRAND_ID="${EXPECTED_BRAND_ID}"

if [[ ! -f "${SOURCE}" ]]; then
  echo "render-brand-config: source not found: ${SOURCE}" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "render-brand-config: jq is required" >&2
  exit 1
fi

canonical_existing() {
  local p="$1"
  if command -v realpath >/dev/null 2>&1; then
    realpath -m "$p" 2>/dev/null || realpath "$p"
  else
    readlink -f "$p"
  fi
}

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
