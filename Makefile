APP_NAME := bot

# --- Brand profiles ---
VFF_SERVER_HOST := fr-mrs-1
VFF_SERVICE_NAME := bot.service
VFF_REMOTE_DIR := /opt/bot
VFF_REMOTE_BINARY := $(VFF_REMOTE_DIR)/$(APP_NAME)
VFF_REMOTE_CONFIG_LEGACY := $(VFF_REMOTE_DIR)/config.json
VFF_REMOTE_CONFIG_EXPLICIT := $(VFF_REMOTE_DIR)/config-vff.json
VFF_DROPIN_FILE := /etc/systemd/system/$(VFF_SERVICE_NAME).d/10-vpnbot-config.conf
VFF_BRAND_ID := vff
VFF_PUBLIC_BASE_URL := https://connect.vpn-for-friends.com
VFF_EXPECT_CATEGORY := vpn-mz-test
VFF_EXPECT_PAYMENT_PROFILE := telegram_bot

FC_SERVER_HOST := fr-mrs-1
FC_SERVICE_NAME := bot-friends-connect.service
FC_REMOTE_DIR := /opt/bot-friends-connect
FC_REMOTE_BINARY := $(FC_REMOTE_DIR)/$(APP_NAME)
FC_REMOTE_CONFIG_LEGACY := $(FC_REMOTE_DIR)/config.json
FC_REMOTE_CONFIG_EXPLICIT := $(FC_REMOTE_DIR)/config-fc.json
FC_DROPIN_FILE := /etc/systemd/system/$(FC_SERVICE_NAME).d/10-vpnbot-config.conf
FC_BRAND_ID := fc
FC_PUBLIC_BASE_URL := https://connect-fc.vpn-for-friends.com
FC_EXPECT_CATEGORY := vpn-mz-fc
FC_EXPECT_PAYMENT_PROFILE := telegram_friends_connect_bot

# Legacy Makefile defaults = VFF (existing deploy/status/logs targets).
SERVER_USER := root
SERVER_HOST := $(VFF_SERVER_HOST)
SERVICE_NAME := $(VFF_SERVICE_NAME)
REMOTE_DIR := $(VFF_REMOTE_DIR)
REMOTE_BIN := $(VFF_REMOTE_BINARY)
REMOTE_CONFIG_LEGACY := $(VFF_REMOTE_CONFIG_LEGACY)
REMOTE_CONFIG_VFF := $(VFF_REMOTE_CONFIG_EXPLICIT)
REMOTE_EXPLICIT_CONFIG := $(VFF_REMOTE_CONFIG_EXPLICIT)

LOCAL_BIN := ./dist/$(APP_NAME)-linux-amd64
LOCAL_CONFIGCHECK := ./dist/configcheck-linux-amd64

define EXPORT_VFF
SERVER_USER=$(SERVER_USER) \
SERVER_HOST=$(VFF_SERVER_HOST) \
SERVICE_NAME=$(VFF_SERVICE_NAME) \
REMOTE_DIR=$(VFF_REMOTE_DIR) \
REMOTE_BINARY=$(VFF_REMOTE_BINARY) \
REMOTE_LEGACY_CONFIG=$(VFF_REMOTE_CONFIG_LEGACY) \
REMOTE_EXPLICIT_CONFIG=$(VFF_REMOTE_CONFIG_EXPLICIT) \
REMOTE_CONFIG_VFF=$(VFF_REMOTE_CONFIG_EXPLICIT) \
REMOTE_CONFIG_LEGACY=$(VFF_REMOTE_CONFIG_LEGACY) \
DROPIN_FILE=$(VFF_DROPIN_FILE) \
EXPECTED_BRAND_ID=$(VFF_BRAND_ID) \
BRAND_LABEL=VFF \
SMOKE_BASE_URL=$(VFF_PUBLIC_BASE_URL) \
EXPECT_PUBLIC_BASE_URL=$(VFF_PUBLIC_BASE_URL) \
EXPECT_SERVICE_CATEGORY=$(VFF_EXPECT_CATEGORY) \
EXPECT_PAYMENT_PROFILE=$(VFF_EXPECT_PAYMENT_PROFILE) \
BRAND_NAME="VPN for Friends" \
ALLOWED_HOST=connect.vpn-for-friends.com \
LANDING_URL=https://vpn-for-friends.com \
WEB_LOGIN_PREFIX=web_ \
WEB_USER_SOURCE=vpn-for-friends.com
endef

define EXPORT_FC
SERVER_USER=$(SERVER_USER) \
SERVER_HOST=$(FC_SERVER_HOST) \
SERVICE_NAME=$(FC_SERVICE_NAME) \
REMOTE_DIR=$(FC_REMOTE_DIR) \
REMOTE_BINARY=$(FC_REMOTE_BINARY) \
REMOTE_LEGACY_CONFIG=$(FC_REMOTE_CONFIG_LEGACY) \
REMOTE_EXPLICIT_CONFIG=$(FC_REMOTE_CONFIG_EXPLICIT) \
DROPIN_FILE=$(FC_DROPIN_FILE) \
EXPECTED_BRAND_ID=$(FC_BRAND_ID) \
BRAND_LABEL=FC \
SMOKE_BASE_URL=$(FC_PUBLIC_BASE_URL) \
EXPECT_PUBLIC_BASE_URL=$(FC_PUBLIC_BASE_URL) \
EXPECT_SERVICE_CATEGORY=$(FC_EXPECT_CATEGORY) \
EXPECT_PAYMENT_PROFILE=$(FC_EXPECT_PAYMENT_PROFILE) \
BRAND_NAME="Friends Connect" \
ALLOWED_HOST=connect-fc.vpn-for-friends.com \
LANDING_URL=https://friends-connect.club \
WEB_LOGIN_PREFIX=web_ \
WEB_USER_SOURCE=vpn-for-friends.com
endef

.PHONY: test build upload install smoke deploy status logs rollback \
	config-check build-configcheck \
	vff-config-render deploy-vff-config activate-vff-config rollback-vff-config smoke-vff \
	deploy-fc fc-config-render deploy-fc-config activate-fc-config rollback-fc-config \
	smoke-fc status-fc logs-fc

test:
	go test ./...
	bash scripts/test/vff_ops_test.sh
	bash scripts/test/brand_ops_test.sh

build:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_BIN) ./cmd/bot

build-configcheck:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_CONFIGCHECK) ./cmd/configcheck

upload: build
	scp $(LOCAL_BIN) $(SERVER_USER)@$(SERVER_HOST):$(REMOTE_BIN).new

install: upload
	ssh $(SERVER_USER)@$(SERVER_HOST) 'set -e; \
		cd $(REMOTE_DIR); \
		if [ -f $(REMOTE_BIN) ]; then cp $(REMOTE_BIN) $(REMOTE_BIN).bak.$$(date +%Y%m%d-%H%M%S); fi; \
		mv $(REMOTE_BIN).new $(REMOTE_BIN); \
		chmod +x $(REMOTE_BIN); \
		systemctl restart $(SERVICE_NAME); \
		systemctl --no-pager --full status $(SERVICE_NAME) | head -40'

smoke:
	curl -fsS https://connect.vpn-for-friends.com/premium-connect >/dev/null
	curl -fsS https://connect.vpn-for-friends.com/buy >/dev/null
	curl -fsS https://connect.vpn-for-friends.com/api/public/services >/dev/null

deploy: test install smoke
	@echo "Deploy OK"

status:
	ssh $(SERVER_USER)@$(SERVER_HOST) 'systemctl --no-pager --full status $(SERVICE_NAME)'

logs:
	ssh $(SERVER_USER)@$(SERVER_HOST) 'journalctl -u $(SERVICE_NAME) -n 100 --no-pager'

rollback:
	ssh $(SERVER_USER)@$(SERVER_HOST) 'set -e; \
		cd $(REMOTE_DIR); \
		LAST=$$(ls -1t $(APP_NAME).bak.* 2>/dev/null | head -1); \
		if [ -z "$$LAST" ]; then echo "No backup found"; exit 1; fi; \
		echo "Rollback to $$LAST"; \
		cp "$$LAST" "$(APP_NAME)"; \
		chmod +x "$(APP_NAME)"; \
		systemctl restart $(SERVICE_NAME); \
		systemctl --no-pager --full status $(SERVICE_NAME) | head -40'

# --- Explicit brand config ---

config-check:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required (e.g. make config-check CONFIG=/path/to/config.json)" >&2; exit 1; fi
	go run ./cmd/configcheck -config "$(CONFIG)"

vff-config-render:
	@if [ -z "$(SOURCE)" ] || [ -z "$(OUTPUT)" ]; then \
		echo "SOURCE and OUTPUT are required" >&2; exit 1; \
	fi
	@$(EXPORT_VFF) bash scripts/render-vff-config.sh "$(SOURCE)" "$(OUTPUT)"
	@$(MAKE) config-check CONFIG="$(OUTPUT)"

deploy-vff-config:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required" >&2; exit 1; fi
	@$(EXPORT_VFF) bash scripts/deploy-vff-config.sh "$(CONFIG)"

activate-vff-config:
	@$(EXPORT_VFF) bash scripts/activate-vff-config.sh

rollback-vff-config:
	@$(EXPORT_VFF) bash scripts/rollback-vff-config.sh

smoke-vff:
	@$(EXPORT_VFF) bash scripts/smoke-vff.sh

# --- Friends Connect ---

deploy-fc:
	@$(EXPORT_FC) bash scripts/deploy-brand-binary.sh

fc-config-render:
	@if [ -z "$(SOURCE)" ] || [ -z "$(OUTPUT)" ]; then \
		echo "SOURCE and OUTPUT are required" >&2; exit 1; \
	fi
	@$(EXPORT_FC) bash scripts/render-brand-config.sh \
		--source "$(SOURCE)" \
		--output "$(OUTPUT)" \
		--brand-id "$(FC_BRAND_ID)" \
		--brand-name "Friends Connect" \
		--allowed-host connect-fc.vpn-for-friends.com \
		--landing-url https://friends-connect.club \
		--web-login-prefix web_ \
		--web-user-source vpn-for-friends.com \
		--expect-public-base-url "$(FC_PUBLIC_BASE_URL)" \
		--expect-service-category "$(FC_EXPECT_CATEGORY)" \
		--expect-payment-profile "$(FC_EXPECT_PAYMENT_PROFILE)"
	@$(MAKE) config-check CONFIG="$(OUTPUT)"

deploy-fc-config:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required" >&2; exit 1; fi
	@$(EXPORT_FC) bash scripts/deploy-brand-config.sh "$(CONFIG)"

activate-fc-config:
	@$(EXPORT_FC) bash scripts/activate-brand-config.sh

rollback-fc-config:
	@$(EXPORT_FC) bash scripts/rollback-brand-config.sh

smoke-fc:
	@$(EXPORT_FC) bash scripts/smoke-brand.sh

status-fc:
	ssh $(SERVER_USER)@$(FC_SERVER_HOST) 'systemctl --no-pager --full status $(FC_SERVICE_NAME)'

logs-fc:
	ssh $(SERVER_USER)@$(FC_SERVER_HOST) 'journalctl -u $(FC_SERVICE_NAME) -n 100 --no-pager'
