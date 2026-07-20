#!/usr/bin/env bash
# Public smoke checks for VFF (no secrets, no mutations).
set -euo pipefail

URLS=(
  "https://connect.vpn-for-friends.com/api/public/services"
  "https://connect.vpn-for-friends.com/account"
  "https://connect.vpn-for-friends.com/buy"
  "https://connect.vpn-for-friends.com/premium-connect"
)

for u in "${URLS[@]}"; do
  code=""
  if ! code="$(curl -sS -o /dev/null -w '%{http_code}' "$u")"; then
    echo "smoke-vff: transport error for $u" >&2
    exit 1
  fi
  echo "$u -> $code"
  case "$code" in
    5*)
      echo "smoke-vff: unexpected 5xx from $u" >&2
      exit 1
      ;;
    '' | 000)
      echo "smoke-vff: empty/zero status for $u" >&2
      exit 1
      ;;
  esac
done

echo "smoke-vff: OK"
