#!/usr/bin/env bash
# Создаёт explicit VFF-конфиг из legacy JSON без вывода содержимого.
set -euo pipefail

SOURCE="${1:-}"
OUTPUT="${2:-}"

if [[ -z "${SOURCE}" || -z "${OUTPUT}" ]]; then
  echo "usage: $0 <source-config.json> <output-config-vff.json>" >&2
  exit 1
fi

if [[ ! -f "${SOURCE}" ]]; then
  echo "render-vff-config: source not found: ${SOURCE}" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "render-vff-config: jq is required" >&2
  exit 1
fi

tmp="$(mktemp)"
cleanup() { rm -f "${tmp}"; }
trap cleanup EXIT

# Не печатаем JSON: только jq → файл. Обязательные legacy-поля проверяются внутри jq.
if ! jq -e '
  def req($path; $val):
    if ($val | type) != "string" or ($val | gsub("^\\s+|\\s+$";"")) == "" then
      error($path + " is required and must be a non-empty string")
    else
      ($val | gsub("^\\s+|\\s+$";""))
    end;
  . as $root
  | (req("web_sales.public_base_url"; ($root.web_sales.public_base_url // ""))) as $pbu
  | (req("services.category"; ($root.services.category // ""))) as $cat
  | (req("payments.profile"; ($root.payments.profile // ""))) as $prof
  | $root
  | .brand = {
      "id": "vff",
      "name": "VPN for Friends",
      "allowed_hosts": ["connect.vpn-for-friends.com"],
      "public_base_url": $pbu,
      "landing_url": "https://vpn-for-friends.com",
      "service_category": $cat,
      "web_user_login_prefix": "web_",
      "web_user_source": "vpn-for-friends.com",
      "payment_profile": $prof
    }
' "${SOURCE}" >"${tmp}"; then
  echo "render-vff-config: failed to build explicit VFF config from source" >&2
  exit 1
fi

chmod 0600 "${tmp}"
mkdir -p "$(dirname "${OUTPUT}")"
mv "${tmp}" "${OUTPUT}"
chmod 0600 "${OUTPUT}"
trap - EXIT

echo "render-vff-config: wrote ${OUTPUT}"
