# ==============================================================================
# Variables & Environment
# ==============================================================================
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

APP_NAME := leakcheck-demo
BIN_DIR  := bin

# ==============================================================================
# Phony declarations
# ==============================================================================
.PHONY: help build run check test lint lint-fix tidy clean
.PHONY: docker-build up down logs

# ==============================================================================
# Default
# ==============================================================================
default: help

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-28s\033[0m %s\n", $$1, $$2}'

# ==============================================================================
# Build & Run
# ==============================================================================
build: ## Build the binary locally
	go build -o $(BIN_DIR)/$(APP_NAME) ./main.go

run: ## Run the demo server locally (ARGS=--simulate-outage to force fail-open)
	go run ./main.go serve $(ARGS)

check: ## Check a password: make check PASSWORD=hunter2
	@echo -n '$(PASSWORD)' | go run ./main.go check

# ==============================================================================
# Test & Lint
# ==============================================================================
test: ## Run tests with race detector
	go test -v -race ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

lint-fix: ## Run golangci-lint with auto-fix
	golangci-lint run --fix ./...

tidy: ## Tidy and verify module dependencies
	go mod tidy
	go mod verify

# ==============================================================================
# Docker
# ==============================================================================
docker-build: ## Build the Docker image
	docker build -t $(APP_NAME):latest .

up: .env ## Start the stack with docker-compose (always rebuilds)
	docker compose up --build -d

down: ## Stop the docker-compose stack
	docker compose down

logs: ## Tail compose stack logs
	docker compose logs -f

# Fail fast with a helpful message when .env is missing.
.env:
	@echo "error: .env not found. Run: cp .env.skel .env && edit LEAKCHECK_API_KEY" && exit 1

# ==============================================================================
# Clean
# ==============================================================================
clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)/
	go clean -testcache
