APP_NAME := bot
SERVICE_NAME := bot.service

SERVER_USER := root
SERVER_HOST := fr-mrs-1
REMOTE_DIR := /opt/bot
REMOTE_BIN := $(REMOTE_DIR)/$(APP_NAME)

LOCAL_BIN := ./dist/$(APP_NAME)-linux-amd64

.PHONY: test build upload install smoke deploy status logs

test:
	go test ./...

build:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_BIN) ./cmd/bot

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