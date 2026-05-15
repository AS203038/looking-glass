SHELL          := /usr/bin/env bash
.SHELLFLAGS    := -eu -o pipefail -c

GO             ?= go
NPM            ?= npm
PNPM           ?= pnpm
BUF            ?= buf
DOCKER         ?= docker

ROOT_DIR       := $(abspath $(CURDIR))
WEBUI_DIR      := $(ROOT_DIR)/webui
PROTO_DIR      := $(ROOT_DIR)/protobuf
SERVER_DIR     := $(ROOT_DIR)/cmd/server
CLI_DIR        := $(ROOT_DIR)/cmd/cli
DIST_DIR       := $(SERVER_DIR)/dist

SERVER_BIN     := $(ROOT_DIR)/looking-glass
CLI_BIN        := $(ROOT_DIR)/lg-cli

VERSION        ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "untracked")

GO_LDFLAGS_SERVER := -X github.com/AS203038/looking-glass/pkg/utils.release=$(VERSION)
GO_LDFLAGS_CLI    := -X main.Version=$(VERSION)
GOFLAGS           ?=

IMAGE_NAME     ?= looking-glass
IMAGE_TAG      ?= $(VERSION)

CYAN  := \033[36m
BOLD  := \033[1m
RESET := \033[0m

.DEFAULT_GOAL := help
.PHONY: help
help: ## Show this help message
	@printf "$(BOLD)Looking Glass - Makefile$(RESET)\n\n"
	@printf "Usage:\n  make $(CYAN)<target>$(RESET)\n\n"
	@printf "Targets:\n"
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z0-9_.-]+:.*## / { \
		printf "  $(CYAN)%-22s$(RESET) %s\n", $$1, $$2 \
	}' $(MAKEFILE_LIST) | sort
	@printf "\nUseful variables:\n"
	@printf "  $(CYAN)VERSION$(RESET)     Version string baked into binaries (default: $(VERSION))\n"
	@printf "  $(CYAN)IMAGE_NAME$(RESET)  Docker image name (default: $(IMAGE_NAME))\n"
	@printf "  $(CYAN)IMAGE_TAG$(RESET)   Docker image tag (default: $(IMAGE_TAG))\n"

.PHONY: version
version: ## Print computed version
	@echo "$(VERSION)"

.PHONY: install
install: install-go install-proto install-webui ## Install all dependencies

.PHONY: install-go
install-go: ## Download Go module dependencies
	@printf "$(BOLD)>> Installing Go dependencies$(RESET)\n"
	$(GO) mod download

.PHONY: install-webui
install-webui: ## Install webui npm/pnpm dependencies
	@printf "$(BOLD)>> Installing webui dependencies$(RESET)\n"
	cd $(WEBUI_DIR) && $(PNPM) install --frozen-lockfile

.PHONY: install-proto
install-proto: ## Install protobuf workspace dependencies
	@printf "$(BOLD)>> Installing protobuf dependencies$(RESET)\n"
	cd $(PROTO_DIR) && $(NPM) install

.PHONY: install-tools
install-tools: ## Install developer CLI tools (buf)
	@printf "$(BOLD)>> Installing developer tools$(RESET)\n"
	$(GO) install github.com/bufbuild/buf/cmd/buf@latest

.PHONY: generate
generate: proto ## Run all code generation

.PHONY: proto
proto: ## Generate Go and TypeScript code from .proto files
	@printf "$(BOLD)>> Generating protobuf code$(RESET)\n"
	cd $(PROTO_DIR) && $(BUF) generate

.PHONY: proto-lint
proto-lint: ## Lint .proto files
	cd $(PROTO_DIR) && $(BUF) lint

.PHONY: proto-format
proto-format: ## Format .proto files in place
	cd $(PROTO_DIR) && $(BUF) format -w

.PHONY: dev-webui
dev-webui: ## Run the SvelteKit dev server (hot reload)
	@printf "$(BOLD)>> Starting webui dev server$(RESET)\n"
	cd $(WEBUI_DIR) && $(PNPM) run dev

.PHONY: build-webui
build-webui: ## Build the webui (output goes to cmd/server/dist)
	@printf "$(BOLD)>> Building webui$(RESET)\n"
	cd $(WEBUI_DIR) && $(PNPM) run build

.PHONY: check-webui
check-webui: ## Run svelte-check / typescript type checks
	cd $(WEBUI_DIR) && $(PNPM) run check

.PHONY: lint-webui
lint-webui: ## Run prettier + eslint checks
	cd $(WEBUI_DIR) && $(PNPM) run lint

.PHONY: format-webui
format-webui: ## Format webui sources with prettier
	cd $(WEBUI_DIR) && $(PNPM) run format

.PHONY: preview-webui
preview-webui: ## Preview the production webui build
	cd $(WEBUI_DIR) && $(PNPM) run preview

.PHONY: build-server
build-server: $(DIST_DIR) ## Build the server binary (requires webui build)
	@printf "$(BOLD)>> Building server binary$(RESET)\n"
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) \
		-ldflags="$(GO_LDFLAGS_SERVER)" \
		-o $(SERVER_BIN) ./cmd/server

$(DIST_DIR):
	@mkdir -p $(DIST_DIR)
	@touch $(DIST_DIR)/.gitkeep

.PHONY: dev-server
dev-server: $(DIST_DIR) ## Run the server from sources (without embedded UI)
	@printf "$(BOLD)>> Running server$(RESET)\n"
	$(GO) run ./cmd/server

.PHONY: run
run: build ## Build everything and run the resulting binary
	$(SERVER_BIN)

.PHONY: build-cli
build-cli: ## Build the lg-cli binary
	@printf "$(BOLD)>> Building lg-cli$(RESET)\n"
	CGO_ENABLED=0 $(GO) build $(GOFLAGS) \
		-ldflags="$(GO_LDFLAGS_CLI)" \
		-o $(CLI_BIN) ./cmd/cli

.PHONY: dev-cli
dev-cli: ## Run the CLI from sources (pass args via ARGS="...")
	$(GO) run ./cmd/cli $(ARGS)

.PHONY: build
build: build-webui build-server build-cli ## Build webui + server + cli (production artifact)

.PHONY: all
all: install generate build ## Install deps, generate code, and build everything

.PHONY: test
test: test-go ## Run all tests

.PHONY: test-go
test-go: $(DIST_DIR) ## Run Go tests
	@printf "$(BOLD)>> Running Go tests$(RESET)\n"
	$(GO) test ./...

.PHONY: test-cover
test-cover: $(DIST_DIR) ## Run Go tests with coverage
	$(GO) test -cover ./...

.PHONY: lint
lint: lint-go lint-webui proto-lint ## Run all linters

.PHONY: lint-go
lint-go: $(DIST_DIR) ## Run go vet
	$(GO) vet ./...

.PHONY: fmt
fmt: fmt-go format-webui proto-format ## Format all sources

.PHONY: fmt-go
fmt-go: ## Run gofmt on Go sources
	gofmt -s -w cmd pkg

.PHONY: tidy
tidy: ## Run go mod tidy
	$(GO) mod tidy

.PHONY: check
check: lint test ## Run all checks (lint + tests)

.PHONY: docker-build
docker-build: ## Build the Docker image
	@printf "$(BOLD)>> Building Docker image $(IMAGE_NAME):$(IMAGE_TAG)$(RESET)\n"
	$(DOCKER) build \
		--build-arg VERSION=$(VERSION) \
		-t $(IMAGE_NAME):$(IMAGE_TAG) \
		-t $(IMAGE_NAME):latest \
		.

.PHONY: docker-run
docker-run: ## Run the Docker image (requires ./config.yaml)
	$(DOCKER) run --rm -it \
		-p 8080:8080 \
		-v $(ROOT_DIR)/config.yaml:/config.yaml:ro \
		$(IMAGE_NAME):$(IMAGE_TAG)

.PHONY: clean
clean: ## Remove build artifacts (binaries, webui dist, node_modules caches)
	@printf "$(BOLD)>> Cleaning build artifacts$(RESET)\n"
	rm -f $(SERVER_BIN) $(CLI_BIN)
	rm -rf $(DIST_DIR)
	rm -rf $(WEBUI_DIR)/.svelte-kit $(WEBUI_DIR)/build

.PHONY: distclean
distclean: clean ## Remove build artifacts AND installed dependencies
	@printf "$(BOLD)>> Removing installed dependencies$(RESET)\n"
	rm -rf $(WEBUI_DIR)/node_modules
	rm -rf $(PROTO_DIR)/node_modules
