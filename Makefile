.PHONY: build test test-e2e clean help

BINARY_NAME=agent-framework
BUILD_DIR=bin

help: ## Display available Makefile commands
	@echo "Go Agent-Framework Build System"
	@echo "================================"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Compile the framework binary
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/main.go

test: ## Run unit and integration package tests
	go test ./...

test-e2e: ## Run end-to-end test suite
	go test -v ./tests/e2e/...

clean: ## Clean built binaries and temporary test artifacts
	rm -rf $(BUILD_DIR) main *.test *.db *.db-wal *.db-shm logs/ uploads/ emails/ scratch/
	rm -f tests/e2e/testdata/agent_framework.db* tests/e2e/output/*
