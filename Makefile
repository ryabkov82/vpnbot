APP_NAME := bot

LOCAL_BIN := ./dist/$(APP_NAME)-linux-amd64
LOCAL_CONFIGCHECK := ./dist/configcheck-linux-amd64

# The Makefile is brand-agnostic: it knows no hosts, paths, categories, URLs or
# payment profiles. Every operational target loads a declarative profile from
# deploy/brands/<BRAND>.json via scripts/lib/brand_profile.sh.

.PHONY: test build build-configcheck config-check \
	brand-profile brand-deploy brand-config-render brand-config-deploy \
	brand-config-activate brand-config-rollback brand-smoke brand-status \
	brand-logs brand-rollback \
	deploy status logs rollback smoke \
	vff-config-render deploy-vff-config activate-vff-config rollback-vff-config smoke-vff \
	deploy-fc fc-config-render deploy-fc-config activate-fc-config rollback-fc-config \
	smoke-fc status-fc logs-fc

# --- Build / test utilities (brand-agnostic) ---

test:
	go test ./...
	bash scripts/test/vff_ops_test.sh
	bash scripts/test/brand_ops_test.sh
	bash scripts/test/brand_profiles_test.sh

build:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_BIN) ./cmd/bot

build-configcheck:
	mkdir -p dist
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(LOCAL_CONFIGCHECK) ./cmd/configcheck

config-check:
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required (e.g. make config-check CONFIG=/path/to/config.json)" >&2; exit 1; fi
	go run ./cmd/configcheck -config "$(CONFIG)"

# --- Generic brand targets (BRAND=<id> required) ---

brand-profile:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-profile BRAND=fc)" >&2; exit 1; fi
	@bash scripts/brand-profile.sh "$(BRAND)"

brand-deploy:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-deploy BRAND=fc)" >&2; exit 1; fi
	bash scripts/deploy-brand-binary.sh "$(BRAND)"

brand-config-render:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-config-render BRAND=fc SOURCE=... OUTPUT=...)" >&2; exit 1; fi
	@if [ -z "$(SOURCE)" ] || [ -z "$(OUTPUT)" ]; then echo "SOURCE and OUTPUT are required" >&2; exit 1; fi
	bash scripts/render-brand-config.sh "$(BRAND)" --source "$(SOURCE)" --output "$(OUTPUT)"
	@$(MAKE) config-check CONFIG="$(OUTPUT)"

brand-config-deploy:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-config-deploy BRAND=fc CONFIG=...)" >&2; exit 1; fi
	@if [ -z "$(CONFIG)" ]; then echo "CONFIG is required" >&2; exit 1; fi
	bash scripts/deploy-brand-config.sh "$(BRAND)" "$(CONFIG)"

brand-config-activate:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-config-activate BRAND=fc)" >&2; exit 1; fi
	bash scripts/activate-brand-config.sh "$(BRAND)"

brand-config-rollback:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-config-rollback BRAND=fc)" >&2; exit 1; fi
	bash scripts/rollback-brand-config.sh "$(BRAND)"

brand-smoke:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-smoke BRAND=fc)" >&2; exit 1; fi
	bash scripts/smoke-brand.sh "$(BRAND)"

brand-status:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-status BRAND=fc)" >&2; exit 1; fi
	bash scripts/status-brand.sh "$(BRAND)"

brand-logs:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-logs BRAND=fc)" >&2; exit 1; fi
	bash scripts/logs-brand.sh "$(BRAND)"

brand-rollback:
	@if [ -z "$(BRAND)" ]; then echo "BRAND is required (e.g. make brand-rollback BRAND=fc)" >&2; exit 1; fi
	bash scripts/rollback-brand-binary.sh "$(BRAND)"

# --- Backward-compatible aliases (delegate only; no profile values here) ---

# Legacy general targets historically meant VFF.
deploy:
	@$(MAKE) brand-deploy BRAND=vff
status:
	@$(MAKE) brand-status BRAND=vff
logs:
	@$(MAKE) brand-logs BRAND=vff
rollback:
	@$(MAKE) brand-rollback BRAND=vff
smoke:
	@$(MAKE) brand-smoke BRAND=vff

# VFF config aliases.
vff-config-render:
	@$(MAKE) brand-config-render BRAND=vff SOURCE="$(SOURCE)" OUTPUT="$(OUTPUT)"
deploy-vff-config:
	@$(MAKE) brand-config-deploy BRAND=vff CONFIG="$(CONFIG)"
activate-vff-config:
	@$(MAKE) brand-config-activate BRAND=vff
rollback-vff-config:
	@$(MAKE) brand-config-rollback BRAND=vff
smoke-vff:
	@$(MAKE) brand-smoke BRAND=vff

# Friends Connect aliases.
deploy-fc:
	@$(MAKE) brand-deploy BRAND=fc
fc-config-render:
	@$(MAKE) brand-config-render BRAND=fc SOURCE="$(SOURCE)" OUTPUT="$(OUTPUT)"
deploy-fc-config:
	@$(MAKE) brand-config-deploy BRAND=fc CONFIG="$(CONFIG)"
activate-fc-config:
	@$(MAKE) brand-config-activate BRAND=fc
rollback-fc-config:
	@$(MAKE) brand-config-rollback BRAND=fc
smoke-fc:
	@$(MAKE) brand-smoke BRAND=fc
status-fc:
	@$(MAKE) brand-status BRAND=fc
logs-fc:
	@$(MAKE) brand-logs BRAND=fc
