APP_NAME := bot
SERVICE_NAME := bot.service

SERVER_USER := root
SERVER_HOST := fr-mrs-1
REMOTE_DIR := /opt/bot
REMOTE_BIN := $(REMOTE_DIR)/$(APP_NAME)
REMOTE_CONFIG_LEGACY := $(REMOTE_DIR)/config.json
REMOTE_CONFIG_VFF := $(REMOTE_DIR)/config-vff.json

LOCAL_BIN := ./dist/$(APP_NAME)-linux-amd64
LOCAL_CONFIGCHECK := ./dist/configcheck-linux-amd64

export SERVER_USER SERVER_HOST REMOTE_DIR REMOTE_CONFIG_VFF REMOTE_CONFIG_LEGACY SERVICE_NAME

.PHONY: test build upload install smoke deploy status logs rollback \
	config-check build-configcheck vff-config-render deploy-vff-config \
	activate-vff-config rollback-vff-config smoke-vff

test:
	go test ./...
	bash scripts/test/vff_ops_test.sh

build:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_BIN) ./cmd/bot

# Optional local artifact; activate builds into a private temp dir instead.
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

# --- Explicit VFF config (no secrets printed) ---

config-check:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required (e.g. make config-check CONFIG=/path/to/config.json)" >&2; exit 1; fi
	go run ./cmd/configcheck -config "$(CONFIG)"

vff-config-render:
	@if [ -z "$(SOURCE)" ] || [ -z "$(OUTPUT)" ]; then \
		echo "SOURCE and OUTPUT are required (e.g. make vff-config-render SOURCE=/secure/config.json OUTPUT=/secure/config-vff.json)" >&2; \
		exit 1; \
	fi
	@bash scripts/render-vff-config.sh "$(SOURCE)" "$(OUTPUT)"
	@$(MAKE) config-check CONFIG="$(OUTPUT)"

deploy-vff-config:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required" >&2; exit 1; fi
	@bash scripts/deploy-vff-config.sh "$(CONFIG)"

activate-vff-config:
	@bash scripts/activate-vff-config.sh

rollback-vff-config:
	@bash scripts/rollback-vff-config.sh

smoke-vff:
	@bash scripts/smoke-vff.sh
