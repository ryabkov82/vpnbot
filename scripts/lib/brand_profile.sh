#!/usr/bin/env bash
# Export brand profile env from named profile (vff|fc). No secrets.
# shellcheck shell=bash

brand_profile_export() {
  local profile="${1:-}"
  # Profile identity fields are always forced so switching vff↔fc is reliable.
  case "${profile}" in
    vff | VFF)
      export SERVER_USER="${SERVER_USER:-root}"
      export SERVER_HOST=fr-mrs-1
      export SERVICE_NAME=bot.service
      export REMOTE_DIR=/opt/bot
      export REMOTE_BINARY=/opt/bot/bot
      export REMOTE_LEGACY_CONFIG=/opt/bot/config.json
      export REMOTE_EXPLICIT_CONFIG=/opt/bot/config-vff.json
      export DROPIN_FILE=/etc/systemd/system/bot.service.d/10-vpnbot-config.conf
      export EXPECTED_BRAND_ID=vff
      export BRAND_LABEL=VFF
      export SMOKE_BASE_URL=https://connect.vpn-for-friends.com
      export EXPECT_PUBLIC_BASE_URL=https://connect.vpn-for-friends.com
      export EXPECT_SERVICE_CATEGORY=vpn-mz-test
      export EXPECT_PAYMENT_PROFILE=telegram_bot
      export BRAND_NAME="VPN for Friends"
      export ALLOWED_HOST=connect.vpn-for-friends.com
      export LANDING_URL=https://vpn-for-friends.com
      export WEB_LOGIN_PREFIX=web_
      export WEB_USER_SOURCE=vpn-for-friends.com
      ;;
    fc | FC)
      export SERVER_USER="${SERVER_USER:-root}"
      export SERVER_HOST=fra-01
      export SERVICE_NAME=bot-friends-connect.service
      export REMOTE_DIR=/opt/bot-friends-connect
      export REMOTE_BINARY=/opt/bot-friends-connect/bot
      export REMOTE_LEGACY_CONFIG=/opt/bot-friends-connect/config.json
      export REMOTE_EXPLICIT_CONFIG=/opt/bot-friends-connect/config-fc.json
      export DROPIN_FILE=/etc/systemd/system/bot-friends-connect.service.d/10-vpnbot-config.conf
      export EXPECTED_BRAND_ID=fc
      export BRAND_LABEL=FC
      export SMOKE_BASE_URL=https://connect-fc.vpn-for-friends.com
      export EXPECT_PUBLIC_BASE_URL=https://connect-fc.vpn-for-friends.com
      export EXPECT_SERVICE_CATEGORY=vpn-mz-fc
      export EXPECT_PAYMENT_PROFILE=telegram_friends_connect_bot
      export BRAND_NAME="Friends Connect"
      export ALLOWED_HOST=connect-fc.vpn-for-friends.com
      export LANDING_URL=https://friends-connect.club
      export WEB_LOGIN_PREFIX=web_
      export WEB_USER_SOURCE=vpn-for-friends.com
      ;;
    *)
      echo "brand_profile_export: unknown profile ${profile:-<empty>} (want vff|fc)" >&2
      return 1
      ;;
  esac

  # Compatibility aliases for older VFF scripts.
  export REMOTE_CONFIG_VFF="${REMOTE_EXPLICIT_CONFIG}"
  export REMOTE_CONFIG_LEGACY="${REMOTE_LEGACY_CONFIG}"
}
