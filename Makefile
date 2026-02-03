include .env
# =========================
# Project configuration
# =========================
APP_NAME := skuld-cli
BIN_DIR  := /tmp/skuld-cli
BIN      := $(BIN_DIR)/$(APP_NAME)

# =========================
# Go configuration
# =========================
GO      := go
GOOS   := linux
GOARCH := amd64
CGO_ENABLED := 0
GOFLAGS := -trimpath -ldflags="-s -w"

# =========================
# Deploy configuration
# =========================
DEPLOY_BINARY := $(DEPLOY_PATH)/$(APP_NAME)

# =========================
# Helper
# =========================
ARGS := $(filter-out $@,$(MAKECMDGOALS))
RUN_ARGS := $(wordlist 2,$(words $(MAKECMDGOALS)),$(MAKECMDGOALS))

.PHONY: all build run dev clean deploy help

# =========================
# Targets
# =========================

all: build

%:
	@:

build: clean
	@echo "==> Building $(APP_NAME)"
	@mkdir -p $(BIN_DIR)
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) $(GO) build $(GOFLAGS) -o $(BIN) .

run:
	@echo "==> Running $(APP_NAME) $(ARGS)"
	$(GO) run . $(ARGS)

build-run:
	@$(MAKE) build
	@$(BIN) $(RUN_ARGS)

deploy: build
	@echo "==> Deploying $(APP_NAME) to $(DEPLOY_USER)@$(DEPLOY_HOST)"
	@set -euo pipefail; \
	printf "→ Upload binary\n"; \
	rsync -avz $(BIN) $(DEPLOY_USER)@$(DEPLOY_HOST):/tmp/$(APP_NAME); \
	printf "→ Install binary\n"; \
	ssh -t $(DEPLOY_USER)@$(DEPLOY_HOST) 'set -euo pipefail; sudo install -m 0755 /tmp/$(APP_NAME) $(DEPLOY_BINARY); printf "✓ Install successful\n";'; \
	printf "✓ Deploy finished\n"

clean:
	@echo "==> Cleaning build artifacts"
	rm -rf $(BIN_DIR)

help:
	@echo ""
	@echo "Available targets:"
	@echo "  make build            Build the CLI"
	@echo "  make run <args>       Run Cobra CLI with commands/flags"
	@echo "  make deploy           Deploy binary to remote server"
	@echo "  make clean            Remove build artifacts"
	@echo ""
