#!/usr/bin/env bash
# VFF wrapper around render-brand-config.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="${1:-}"
OUTPUT="${2:-}"

if [[ -z "${SOURCE}" || -z "${OUTPUT}" ]]; then
  echo "usage: bash $0 <source-config.json> <output-config-vff.json>" >&2
  exit 1
fi

# shellcheck source=lib/brand_profile.sh
source "${ROOT}/scripts/lib/brand_profile.sh"
brand_profile_export vff

bash "${ROOT}/scripts/render-brand-config.sh" \
  --source "${SOURCE}" \
  --output "${OUTPUT}" \
  --brand-id "${EXPECTED_BRAND_ID}" \
  --brand-name "${BRAND_NAME}" \
  --allowed-host "${ALLOWED_HOST}" \
  --landing-url "${LANDING_URL}" \
  --web-login-prefix "${WEB_LOGIN_PREFIX}" \
  --web-user-source "${WEB_USER_SOURCE}" \
  --expect-public-base-url "${EXPECT_PUBLIC_BASE_URL}" \
  --expect-service-category "${EXPECT_SERVICE_CATEGORY}" \
  --expect-payment-profile "${EXPECT_PAYMENT_PROFILE}"
